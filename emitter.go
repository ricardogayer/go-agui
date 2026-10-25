package agui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"

	wire "github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// ErrClientGone indicates that writing to the socket failed: the client
// disconnected. It is a sticky error — after it the Emitter becomes a no-op.
// Test for it with errors.Is.
//
// PT: ErrClientGone indica que a escrita no socket falhou: o cliente desconectou.
// É um erro pegajoso — depois dele o Emitter vira no-op. Teste com errors.Is.
var ErrClientGone = errors.New("agui: client disconnected")

// ErrEventDropped indicates that an event was refused because it does not
// conform to AG-UI 1.0 — either to the official JSON Schema
// (*wire.ValidationError) or to the event sequence rules (*wire.ProtocolError)
// — and therefore was not written. Unlike [ErrClientGone], it does NOT
// interrupt the stream: the event is dropped, the run goes on, and a terminal
// event still reaches the client.
//
// Why the distinction matters: a conformant consumer treats a malformed event
// or a sequence violation as fatal and tears the whole run down. Refusing the
// invalid event before it is written and keeping the stream alive is the only
// way for the client to receive a terminal event explaining what happened,
// instead of a connection that dies silently.
//
// PT: ErrEventDropped indica que um evento foi recusado por não estar conforme
// o AG-UI 1.0 — seja ao JSON Schema oficial (*wire.ValidationError), seja às
// regras de sequência de eventos (*wire.ProtocolError) — e por isso não foi
// escrito. Diferente de [ErrClientGone], NÃO interrompe o stream: o evento
// cai, o run segue, e um evento terminal ainda alcança o cliente.
//
// Por que a distinção importa: um consumidor conforme trata um evento
// malformado ou uma violação de sequência como fatal e derruba o run inteiro.
// Recusar o evento inválido antes de escrevê-lo e manter o stream vivo é a
// única forma de o cliente receber um evento terminal explicando o que houve,
// em vez de uma conexão que morre em branco.
var ErrEventDropped = errors.New("agui: event dropped: not conformant to AG-UI 1.0")

// DroppedFunc is notified when an event is dropped with [ErrEventDropped].
//
// PT: DroppedFunc é notificada quando um evento é descartado por [ErrEventDropped].
type DroppedFunc func(ctx context.Context, ev Event, err error)

// Emitter writes AG-UI 1.0 events to a stream — SSE or, when the client
// negotiated it, the HTTP + Protobuf binding.
//
// Every event goes through a [stream.SSE] before reaching the socket: it is
// validated against the official AG-UI 1.0 JSON Schema and checked against
// the sequence rules of the specification (run lifecycle, open/content/close
// discipline, pending tool calls, ...). A non-conformant event is never
// written: it is reported through [WithEmitterDropped] and the stream goes on.
//
// It stores the first transport error and becomes a no-op after it, so that
// the agent loop stays lean and only checks [Emitter.Err] at convenient
// points. On the first write failure it invokes cancel (when non-nil), which
// cancels the run context and promptly aborts an in-flight model stream,
// instead of continuing to generate — and pay — against a client that has
// already left.
//
// It is safe for concurrent use. This is not gratuitous care: tools reach the
// Emitter through the context (see [RunContext]) and may emit from another
// goroutine, and two simultaneous writes to the same io.Writer would interleave
// frames.
//
// PT: Emitter escreve eventos AG-UI 1.0 num stream — SSE ou, quando o cliente
// o negociou, o binding HTTP + Protobuf.
//
// Todo evento passa por um [stream.SSE] antes de chegar ao socket: é validado
// contra o JSON Schema oficial do AG-UI 1.0 e verificado contra as regras de
// sequência da especificação (ciclo de vida do run, disciplina
// abrir/conteúdo/fechar, tool calls pendentes, ...). Um evento não conforme
// nunca é escrito: é informado via [WithEmitterDropped] e o stream segue.
//
// Guarda o primeiro erro de transporte e vira no-op depois dele, para que o
// laço do agente permaneça enxuto e só consulte [Emitter.Err] em pontos
// convenientes. Na primeira falha de escrita invoca cancel (quando não-nil), o
// que cancela o contexto do run e aborta prontamente um stream do modelo em
// voo, em vez de continuar gerando — e pagando — contra um cliente que já foi
// embora.
//
// É seguro para uso concorrente. Isso não é zelo gratuito: as tools alcançam o
// Emitter pelo contexto (ver [RunContext]) e podem emitir de outra goroutine,
// e duas escritas simultâneas no mesmo io.Writer entrelaçariam quadros.
type Emitter struct {
	ctx         context.Context
	w           io.Writer
	stream      *stream.SSE
	threadID    string
	runID       string
	parentRunID *string
	cancel      context.CancelFunc
	onDrop      DroppedFunc

	mu         sync.Mutex
	err        error // first transport error; after it, everything is a no-op (PT: primeiro erro de transporte; depois dele, tudo é no-op)
	terminated bool  // RUN_FINISHED or RUN_ERROR already went out (PT: já saiu RUN_FINISHED ou RUN_ERROR)
	usage      []TokenUsage
	calls      []string        // tool calls started in this run, in call order (PT: tool calls iniciadas neste run, em ordem)
	started    map[string]bool // set view of calls (PT: conjunto de calls)
	answered   map[string]bool // tool calls answered with TOOL_CALL_RESULT (PT: tool calls respondidas com TOOL_CALL_RESULT)
}

// EmitterOption configures an Emitter.
//
// PT: EmitterOption configura um Emitter.
type EmitterOption func(*Emitter)

// WithEmitterCancel ties the Emitter to the run's cancellation: the first
// write failure calls cancel exactly once.
//
// PT: WithEmitterCancel liga o Emitter ao cancelamento do run: a primeira falha de
// escrita chama cancel uma única vez.
func WithEmitterCancel(cancel context.CancelFunc) EmitterOption {
	return func(e *Emitter) { e.cancel = cancel }
}

// WithEmitterIDs pins the threadId and runId announced in the events. Empty
// values are replaced by generated ids.
//
// PT: WithEmitterIDs fixa o threadId e o runId anunciados nos eventos. Vazios são
// substituídos por ids gerados.
func WithEmitterIDs(threadID, runID string) EmitterOption {
	return func(e *Emitter) {
		e.threadID, e.runID = threadID, runID
	}
}

// WithEmitterParentRunID sets the parentRunId announced on RUN_STARTED (the
// run this one was branched from). Empty omits it.
//
// PT: WithEmitterParentRunID define o parentRunId anunciado no RUN_STARTED (o
// run do qual este foi ramificado). Vazio o omite.
func WithEmitterParentRunID(parentRunID string) EmitterOption {
	return func(e *Emitter) {
		if parentRunID != "" {
			e.parentRunID = &parentRunID
		}
	}
}

// WithEmitterDropped registers the dropped-event callback.
//
// PT: WithEmitterDropped registra o callback de evento descartado.
func WithEmitterDropped(f DroppedFunc) EmitterOption {
	return func(e *Emitter) { e.onDrop = f }
}

// WithEmitterContext sets the context passed to the dropped-event callback.
//
// PT: WithEmitterContext define o contexto passado ao callback de evento
// descartado.
func WithEmitterContext(ctx context.Context) EmitterOption {
	return func(e *Emitter) {
		if ctx != nil {
			e.ctx = ctx
		}
	}
}

// WithEmitterStream writes through an already-built [stream.SSE] — for
// instance one returned by [stream.Negotiate], which speaks Protobuf when the
// client asked for it. The [Handler] uses this; outside of it, it is rarely
// needed: without it the Emitter writes SSE frames to the io.Writer given to
// [NewEmitter].
//
// PT: WithEmitterStream escreve através de um [stream.SSE] já construído — por
// exemplo um devolvido por [stream.Negotiate], que fala Protobuf quando o
// cliente o pediu. O [Handler] usa isto; fora dele raramente é necessário: sem
// isto o Emitter escreve quadros SSE no io.Writer dado a [NewEmitter].
func WithEmitterStream(s *stream.SSE) EmitterOption {
	return func(e *Emitter) {
		if s != nil {
			e.stream = s
		}
	}
}

// NewEmitter creates an Emitter that writes SSE frames to w.
//
// It is usable with no options at all — which matters for tests, where a
// bytes.Buffer is enough:
//
//	var buf bytes.Buffer
//	e := agui.NewEmitter(&buf)
//
// PT: NewEmitter cria um Emitter que escreve quadros SSE em w.
//
// Sem opções ele já é utilizável — o que importa para testes, onde um
// bytes.Buffer basta, como no exemplo acima.
func NewEmitter(w io.Writer, opts ...EmitterOption) *Emitter {
	e := &Emitter{
		ctx:      context.Background(),
		w:        w,
		started:  map[string]bool{},
		answered: map[string]bool{},
	}
	for _, o := range opts {
		o(e)
	}
	if e.threadID == "" {
		e.threadID = uuid.NewString()
	}
	if e.runID == "" {
		e.runID = uuid.NewString()
	}
	if e.stream == nil {
		e.stream = stream.NewWriter(w, stream.ForThread(e.threadID))
	}
	return e
}

// Err returns the first transport error, wrapping [ErrClientGone].
//
// PT: Err devolve o primeiro erro de transporte, embrulhando [ErrClientGone].
func (e *Emitter) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// Terminated reports whether the run has already emitted a terminal event
// (RUN_FINISHED or RUN_ERROR), or can no longer emit one because the client
// is gone.
//
// The protocol requires exactly one terminal event per run. The handler checks
// this so it does not emit a RUN_ERROR on top of a RUN_FINISHED the Runner has
// already sent — two terminals confuse the client as much as none.
//
// PT: Terminated informa se o run já emitiu um evento terminal (RUN_FINISHED ou
// RUN_ERROR), ou não pode mais emiti-lo porque o cliente foi embora.
//
// O protocolo exige exatamente um evento terminal por run. O handler consulta
// isto para não emitir um RUN_ERROR em cima de um RUN_FINISHED que o Runner já
// mandou — dois terminais confundem o cliente tanto quanto nenhum.
func (e *Emitter) Terminated() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.terminated
}

// ThreadID and RunID expose the identifiers of the current run.
//
// PT: ThreadID e RunID expõem os identificadores do run corrente.
func (e *Emitter) ThreadID() string { return e.threadID }
func (e *Emitter) RunID() string    { return e.runID }

// Protobuf reports whether the stream speaks the HTTP + Protobuf binding
// instead of SSE.
//
// PT: Protobuf informa se o stream fala o binding HTTP + Protobuf em vez de SSE.
func (e *Emitter) Protobuf() bool { return e.stream.Protobuf() }

// PendingToolCalls returns the tool calls this run started and has not
// answered with TOOL_CALL_RESULT, in call order — exactly what AG-UI 1.0
// requires in the pendingToolCallIds of the success outcome.
//
// PT: PendingToolCalls devolve as tool calls que este run iniciou e não
// respondeu com TOOL_CALL_RESULT, em ordem de chamada — exatamente o que o
// AG-UI 1.0 exige no pendingToolCallIds do outcome de sucesso.
func (e *Emitter) PendingToolCalls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, id := range e.calls {
		if !e.answered[id] {
			out = append(out, id)
		}
	}
	return out
}

// startedToolCall reports whether TOOL_CALL_START went out for id in this run.
//
// PT: startedToolCall informa se o TOOL_CALL_START de id saiu neste run.
func (e *Emitter) startedToolCall(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started[id]
}

// SetUsage sets the token usage reported on the terminal event (RUN_FINISHED
// or RUN_ERROR). The [Agent] fills it with what the model reported.
//
// PT: SetUsage define o uso de tokens informado no evento terminal
// (RUN_FINISHED ou RUN_ERROR). O [Agent] o preenche com o que o modelo
// informou.
func (e *Emitter) SetUsage(usage ...TokenUsage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.usage = usage
}

// Send emits any AG-UI 1.0 event — the escape hatch for what the typed
// helpers do not cover. A terminal event (RUN_FINISHED, RUN_ERROR) goes
// through the same at-most-once guard as [Emitter.RunFinished].
//
// The returned error is nil when the event was written, wraps
// [ErrEventDropped] when it was refused as non-conformant (the stream goes
// on), or wraps [ErrClientGone] when the client is gone.
//
// PT: Send emite qualquer evento AG-UI 1.0 — a válvula de escape para o que os
// helpers tipados não cobrem. Um evento terminal (RUN_FINISHED, RUN_ERROR)
// passa pela mesma guarda de no-máximo-uma-vez de [Emitter.RunFinished].
//
// O erro devolvido é nil quando o evento foi escrito, embrulha
// [ErrEventDropped] quando foi recusado por não conformidade (o stream segue),
// ou embrulha [ErrClientGone] quando o cliente foi embora.
func (e *Emitter) Send(ev Event) error {
	switch t := ev.(type) {
	case *wire.RunFinishedEvent, *wire.RunErrorEvent:
		return e.writeTerminal(t)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.writeLocked(ev)
}

func (e *Emitter) write(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.writeLocked(ev)
}

func (e *Emitter) writeLocked(ev Event) error {
	if e.err != nil {
		return e.err
	}
	err := e.stream.Send(ev)
	if err == nil {
		e.track(ev)
		return nil
	}
	if errors.Is(err, stream.ErrStreamFailed) {
		e.err = fmt.Errorf("%w: %w", ErrClientGone, err)
		e.terminated = true // nothing more can reach the client (PT: nada mais alcança o cliente)
		if e.cancel != nil {
			e.cancel()
		}
		return e.err
	}
	err = fmt.Errorf("%w: %w", ErrEventDropped, err)
	if e.onDrop != nil {
		e.onDrop(e.ctx, ev, err)
	}
	return err
}

// track records the tool calls the run started and answered.
//
// PT: track registra as tool calls que o run iniciou e respondeu.
func (e *Emitter) track(ev Event) {
	switch t := ev.(type) {
	case *wire.ToolCallStartEvent:
		if !e.started[t.ToolCallID] {
			e.started[t.ToolCallID] = true
			e.calls = append(e.calls, t.ToolCallID)
		}
	case *wire.ToolCallResultEvent:
		e.answered[t.ToolCallID] = true
	case *wire.RunStartedEvent:
		e.calls, e.started, e.answered = nil, map[string]bool{}, map[string]bool{}
	}
}

// writeTerminal emits a terminal event, at most once per run.
//
// A RUN_FINISHED that the protocol refuses (say, a text message still open)
// would leave the client without a terminal event; it is replaced by a
// RUN_ERROR, which is always accepted while the run is active.
//
// PT: writeTerminal emite um evento terminal, no máximo uma vez por run.
//
// Um RUN_FINISHED recusado pelo protocolo (digamos, uma mensagem de texto
// ainda aberta) deixaria o cliente sem evento terminal; ele é substituído por
// um RUN_ERROR, que é sempre aceito enquanto o run está ativo.
func (e *Emitter) writeTerminal(ev Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminated {
		return nil
	}
	switch t := ev.(type) {
	case *wire.RunFinishedEvent:
		if t.Usage == nil {
			t.Usage = e.usage
		}
	case *wire.RunErrorEvent:
		if t.Usage == nil {
			t.Usage = e.usage
		}
	}
	err := e.writeLocked(ev)
	if err == nil || e.err != nil {
		e.terminated = true
		return err
	}
	if _, finished := ev.(*wire.RunFinishedEvent); finished {
		fallback := wire.NewRunErrorEvent("the agent could not finish the run in conformance with AG-UI " + ProtocolVersion)
		fallback.Code = Ptr("PROTOCOL_VIOLATION")
		fallback.Usage = e.usage
		if e.writeLocked(fallback) == nil {
			e.terminated = true
		}
	}
	return err
}

// --- run lifecycle / ciclo de vida do run ---

// RunStarted emits RUN_STARTED, declaring protocolVersion "1.0". The
// [Handler] already emits it; a Runner must not repeat it.
//
// PT: RunStarted emite RUN_STARTED, declarando protocolVersion "1.0". O
// [Handler] já o emite; um Runner não deve repeti-lo.
func (e *Emitter) RunStarted() {
	ev := wire.NewRunStartedEvent(e.threadID, e.runID)
	ev.ParentRunID = e.parentRunID
	e.write(ev)
}

// RunFinished emits RUN_FINISHED, ending the run successfully.
//
// PT: RunFinished emite RUN_FINISHED, encerrando o run com sucesso.
func (e *Emitter) RunFinished() {
	_ = e.writeTerminal(wire.NewRunFinishedEvent(e.threadID, e.runID))
}

// RunFinishedWithResult ends the run successfully with a result value.
//
// PT: RunFinishedWithResult encerra o run com sucesso e um valor de resultado.
func (e *Emitter) RunFinishedWithResult(result any) {
	ev := wire.NewRunFinishedEvent(e.threadID, e.runID)
	ev.Result = result
	_ = e.writeTerminal(ev)
}

// RunFinishedPending ends the run successfully leaving frontend tool calls
// for the client to execute: the success outcome with pendingToolCallIds. The
// ids must be exactly the calls the run started and did not answer, in call
// order — [Emitter.PendingToolCalls] returns that list.
//
// PT: RunFinishedPending encerra o run com sucesso deixando tool calls de
// frontend para o cliente executar: o outcome de sucesso com
// pendingToolCallIds. Os ids devem ser exatamente as chamadas que o run
// iniciou e não respondeu, em ordem — [Emitter.PendingToolCalls] devolve essa
// lista.
func (e *Emitter) RunFinishedPending(toolCallIDs ...string) {
	ev := wire.NewRunFinishedEvent(e.threadID, e.runID)
	ev.Outcome = &wire.SuccessOutcome{PendingToolCallIDs: toolCallIDs}
	_ = e.writeTerminal(ev)
}

// RunFinishedWithInterrupts ends the run asking the client for a decision:
// the interrupt outcome.
//
// The client answers in the next run through RunAgentInput.resume; see
// [Interrupt] and [RunContext.ResumeFor].
//
// PT: RunFinishedWithInterrupts encerra o run pedindo uma decisão ao cliente:
// o outcome de interrupt.
//
// O cliente responde no run seguinte através de RunAgentInput.resume; ver
// [Interrupt] e [RunContext.ResumeFor].
func (e *Emitter) RunFinishedWithInterrupts(its []InterruptPoint) {
	ev := wire.NewRunFinishedEvent(e.threadID, e.runID)
	ev.Outcome = &wire.InterruptOutcome{Interrupts: its}
	_ = e.writeTerminal(ev)
}

// RunCancelled ends a run that was stopped on purpose: the cancelled outcome.
//
// PT: RunCancelled encerra um run que foi parado de propósito: o outcome
// cancelled.
func (e *Emitter) RunCancelled() {
	ev := wire.NewRunFinishedEvent(e.threadID, e.runID)
	ev.Outcome = &wire.CancelledOutcome{}
	_ = e.writeTerminal(ev)
}

// RunError emits RUN_ERROR, ending the run with a failure.
//
// PT: RunError emite RUN_ERROR, encerrando o run com falha.
func (e *Emitter) RunError(msg string) {
	_ = e.writeTerminal(wire.NewRunErrorEvent(msg))
}

// RunErrorCode emits RUN_ERROR with a machine-readable code.
//
// PT: RunErrorCode emite RUN_ERROR com um código legível por máquina.
func (e *Emitter) RunErrorCode(msg, code string) {
	ev := wire.NewRunErrorEvent(msg)
	if code != "" {
		ev.Code = &code
	}
	_ = e.writeTerminal(ev)
}

// --- steps ---

// StepStarted and StepFinished emit STEP_STARTED/STEP_FINISHED. They are UI
// labels; the loop uses them around the model call and the tool execution (see
// [WithStepNames]).
//
// PT: StepStarted e StepFinished emitem STEP_STARTED/STEP_FINISHED. São rótulos de
// UI; o laço os usa em volta da chamada ao modelo e da execução das tools (ver
// [WithStepNames]).
func (e *Emitter) StepStarted(name string)  { e.write(wire.NewStepStartedEvent(name)) }
func (e *Emitter) StepFinished(name string) { e.write(wire.NewStepFinishedEvent(name)) }

// --- text messages / mensagens de texto ---

// TextStart opens an assistant text block with the given messageId. Every
// opened block needs its matching [Emitter.TextEnd].
//
// PT: TextStart abre um bloco de texto do assistente com o messageId dado. Todo
// bloco aberto precisa do [Emitter.TextEnd] correspondente.
func (e *Emitter) TextStart(id string) {
	ev := wire.NewTextMessageStartEvent(id)
	ev.Role = Ptr(wire.TextMessageRoleAssistant)
	e.write(ev)
}

// TextContent emits a text delta for block id. An empty delta is ignored: it
// carries nothing.
//
// PT: TextContent emite um delta de texto do bloco id. Um delta vazio é
// ignorado: não carrega nada.
func (e *Emitter) TextContent(id, delta string) {
	if delta == "" {
		return
	}
	e.write(wire.NewTextMessageContentEvent(id, delta))
}

// TextEnd closes text block id.
//
// PT: TextEnd fecha o bloco de texto id.
func (e *Emitter) TextEnd(id string) { e.write(wire.NewTextMessageEndEvent(id)) }

// --- tool calls ---

// ToolStart opens a tool call. parentMessageID links the call to the assistant
// message that produced it — this is what lets the client render the call
// inside the right message. Empty omits the field.
//
// PT: ToolStart abre uma tool call. parentMessageID liga a chamada à mensagem do
// assistente que a produziu — é o que permite ao cliente renderizar a chamada
// dentro da mensagem certa. Vazio omite o campo.
func (e *Emitter) ToolStart(toolCallID, name, parentMessageID string) {
	ev := wire.NewToolCallStartEvent(toolCallID, name)
	if parentMessageID != "" {
		ev.ParentMessageID = &parentMessageID
	}
	e.write(ev)
}

// ToolArgs emits a fragment of the tool call arguments. An empty fragment is
// ignored.
//
// PT: ToolArgs emite um fragmento dos argumentos da tool call. Um fragmento
// vazio é ignorado.
func (e *Emitter) ToolArgs(toolCallID, delta string) {
	if delta == "" {
		return
	}
	e.write(wire.NewToolCallArgsEvent(toolCallID, delta))
}

// ToolEnd closes the tool call opened by [Emitter.ToolStart].
//
// PT: ToolEnd fecha a tool call aberta por [Emitter.ToolStart].
func (e *Emitter) ToolEnd(toolCallID string) { e.write(wire.NewToolCallEndEvent(toolCallID)) }

// ToolResult emits TOOL_CALL_RESULT — the answer to a tool call executed on
// the server.
//
// PT: ToolResult emite TOOL_CALL_RESULT — a resposta a uma tool call
// executada no servidor.
func (e *Emitter) ToolResult(messageID, toolCallID, content string) {
	e.write(wire.NewToolCallResultEvent(messageID, toolCallID, content))
}

// ToolResultParts emits TOOL_CALL_RESULT with multimodal content.
//
// PT: ToolResultParts emite TOOL_CALL_RESULT com conteúdo multimodal.
func (e *Emitter) ToolResultParts(messageID, toolCallID string, parts ...ContentPart) {
	ev := wire.NewToolCallResultEvent(messageID, toolCallID, "")
	ev.Content = wire.PartsContent(parts...)
	e.write(ev)
}

// --- reasoning / raciocínio ---

// ReasoningStart and ReasoningEnd open and close a reasoning span;
// ReasoningMessageStart, ReasoningMessageContent and ReasoningMessageEnd
// stream a visible reasoning message inside it.
//
// PT: ReasoningStart e ReasoningEnd abrem e fecham um span de raciocínio;
// ReasoningMessageStart, ReasoningMessageContent e ReasoningMessageEnd
// transmitem uma mensagem de raciocínio visível dentro dele.
func (e *Emitter) ReasoningStart(spanID string) { e.write(wire.NewReasoningStartEvent(spanID)) }
func (e *Emitter) ReasoningEnd(spanID string)   { e.write(wire.NewReasoningEndEvent(spanID)) }
func (e *Emitter) ReasoningMessageStart(id string) {
	e.write(wire.NewReasoningMessageStartEvent(id))
}
func (e *Emitter) ReasoningMessageContent(id, delta string) {
	if delta == "" {
		return
	}
	e.write(wire.NewReasoningMessageContentEvent(id, delta))
}
func (e *Emitter) ReasoningMessageEnd(id string) { e.write(wire.NewReasoningMessageEndEvent(id)) }

// --- state and messages / estado e mensagens ---

// StateSnapshot emits STATE_SNAPSHOT. A nil snapshot becomes an empty object.
//
// PT: StateSnapshot emite STATE_SNAPSHOT. Um snapshot nil vira um objeto vazio.
func (e *Emitter) StateSnapshot(snapshot any) {
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	e.write(wire.NewStateSnapshotEvent(snapshot))
}

// StateDelta emits STATE_DELTA with JSON Patch operations. An empty list is
// ignored.
//
// PT: StateDelta emite STATE_DELTA com operações JSON Patch. Uma lista vazia é
// ignorada.
func (e *Emitter) StateDelta(ops []PatchOp) {
	if len(ops) == 0 {
		return
	}
	e.write(wire.NewStateDeltaEvent(ops...))
}

// MessagesSnapshot emits MESSAGES_SNAPSHOT — the reconciliation event the
// Python adapter emits at the end of every run
// (get_state_and_messages_snapshots). Without it the frontend's message list
// is left unreconciled after tool calls.
//
// PT: MessagesSnapshot emite MESSAGES_SNAPSHOT — o evento de reconciliação que o
// adaptador Python emite ao fim de cada run (get_state_and_messages_snapshots).
// Sem ele a lista de mensagens do frontend fica sem reconciliação após tool
// calls.
func (e *Emitter) MessagesSnapshot(msgs []Message) {
	e.write(wire.NewMessagesSnapshotEvent(msgs...))
}

// --- activity / atividade ---

// ActivitySnapshot creates or replaces an activity message: structured
// progress the client renders (a plan, a search, a checklist).
//
// PT: ActivitySnapshot cria ou substitui uma mensagem de atividade: progresso
// estruturado que o cliente renderiza (um plano, uma busca, um checklist).
func (e *Emitter) ActivitySnapshot(messageID, activityType string, content map[string]any) {
	if content == nil {
		content = map[string]any{}
	}
	e.write(wire.NewActivitySnapshotEvent(messageID, activityType, content))
}

// ActivityDelta amends an activity message created by
// [Emitter.ActivitySnapshot] with JSON Patch operations.
//
// PT: ActivityDelta altera uma mensagem de atividade criada por
// [Emitter.ActivitySnapshot] com operações JSON Patch.
func (e *Emitter) ActivityDelta(messageID, activityType string, ops []PatchOp) {
	if len(ops) == 0 {
		return
	}
	e.write(wire.NewActivityDeltaEvent(messageID, activityType, ops...))
}

// --- passthrough ---

// Custom emits a CUSTOM event. The client ignores names it does not know, so
// this is a safe channel for data outside the protocol.
//
// PT: Custom emite um CUSTOM. O cliente ignora nomes que não conhece, então isto é
// um canal seguro para dados fora do protocolo.
func (e *Emitter) Custom(name string, value any) {
	e.write(wire.NewCustomEvent(name, value))
}

// Raw emits a RAW event carrying an event of an external system verbatim;
// source names that system (empty omits it).
//
// PT: Raw emite um evento RAW carregando um evento de um sistema externo como
// está; source nomeia esse sistema (vazio o omite).
func (e *Emitter) Raw(event any, source string) {
	ev := wire.NewRawEvent(event)
	if source != "" {
		ev.Source = &source
	}
	e.write(ev)
}

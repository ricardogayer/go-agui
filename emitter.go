package agui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

// ErrClientGone indicates that writing to the socket failed: the client
// disconnected. It is a sticky error — after it the Emitter becomes a no-op.
// Test for it with errors.Is.
//
// PT: ErrClientGone indica que a escrita no socket falhou: o cliente desconectou.
// É um erro pegajoso — depois dele o Emitter vira no-op. Teste com errors.Is.
var ErrClientGone = errors.New("agui: client disconnected")

// ErrEventDropped indicates that an event was rejected during encoding or SDK
// validation and therefore was not written. Unlike [ErrClientGone], it does
// NOT interrupt the stream: the event is dropped, the run goes on, and a
// terminal event still reaches the client.
//
// Why the distinction matters: in @ag-ui/client the parser runs
// `EventSchemas.parse(json)` and calls `eventSubject.error(err)` in the catch —
// a single event outside the schema tears down the whole Observable. Discarding
// the invalid event and keeping the stream alive is the only way for the client
// to receive a RUN_ERROR explaining what happened, instead of a connection that
// dies silently.
//
// PT: ErrEventDropped indica que um evento foi recusado na codificação ou na
// validação do SDK e por isso não foi escrito. Diferente de [ErrClientGone],
// NÃO interrompe o stream: o evento cai, o run segue, e um evento terminal
// ainda alcança o cliente.
//
// Por que a distinção importa: em @ag-ui/client o parser faz
// `EventSchemas.parse(json)` e chama `eventSubject.error(err)` no catch — um
// único evento fora do schema derruba o Observable inteiro. Descartar o evento
// inválido e manter o stream vivo é a única forma de o cliente receber um
// RUN_ERROR explicando o que houve, em vez de uma conexão que morre em branco.
var ErrEventDropped = errors.New("agui: event dropped during encoding")

// DroppedFunc is notified when an event is dropped with [ErrEventDropped].
//
// PT: DroppedFunc é notificada quando um evento é descartado por [ErrEventDropped].
type DroppedFunc func(ctx context.Context, ev Event, err error)

// Emitter serializes AG-UI events into an SSE stream.
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
// SSE frames.
//
// PT: Emitter serializa eventos AG-UI num fluxo SSE.
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
// e duas escritas simultâneas no mesmo io.Writer entrelaçariam quadros SSE.
type Emitter struct {
	ctx      context.Context
	w        io.Writer
	sse      *sse.SSEWriter
	threadID string
	runID    string
	cancel   context.CancelFunc
	onDrop   DroppedFunc

	mu         sync.Mutex
	err        error // first transport error; after it, everything is a no-op (PT: primeiro erro de transporte; depois dele, tudo é no-op)
	terminated bool  // RUN_FINISHED or RUN_ERROR already went out (PT: já saiu RUN_FINISHED ou RUN_ERROR)
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
// values are replaced by generated ids — the SDK rejects events with those
// fields empty.
//
// PT: WithEmitterIDs fixa o threadId e o runId anunciados nos eventos. Vazios são
// substituídos por ids gerados — o SDK recusa eventos com esses campos vazios.
func WithEmitterIDs(threadID, runID string) EmitterOption {
	return func(e *Emitter) {
		e.threadID, e.runID = threadID, runID
	}
}

// WithEmitterDropped registers the dropped-event callback.
//
// PT: WithEmitterDropped registra o callback de evento descartado.
func WithEmitterDropped(f DroppedFunc) EmitterOption {
	return func(e *Emitter) { e.onDrop = f }
}

// WithEmitterContext sets the context used for writes.
//
// PT: WithEmitterContext define o contexto usado nas escritas.
func WithEmitterContext(ctx context.Context) EmitterOption {
	return func(e *Emitter) {
		if ctx != nil {
			e.ctx = ctx
		}
	}
}

// WithEmitterSSEWriter reuses an already-built *sse.SSEWriter. The handler
// uses this to share a writer across requests; outside of it, it is rarely
// needed.
//
// PT: WithEmitterSSEWriter reaproveita um *sse.SSEWriter já construído. O handler
// usa isto para compartilhar um writer entre requisições; fora dele raramente é
// necessário.
func WithEmitterSSEWriter(sw *sse.SSEWriter) EmitterOption {
	return func(e *Emitter) {
		if sw != nil {
			e.sse = sw
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
	e := &Emitter{ctx: context.Background(), w: w, sse: sse.NewSSEWriter()}
	for _, o := range opts {
		o(e)
	}
	if e.threadID == "" {
		e.threadID = events.GenerateThreadID()
	}
	if e.runID == "" {
		e.runID = events.GenerateRunID()
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
// (RUN_FINISHED or RUN_ERROR).
//
// The protocol requires exactly one terminal event per run. The handler checks
// this so it does not emit a RUN_ERROR on top of a RUN_FINISHED the Runner has
// already sent — two terminals confuse the client as much as none.
//
// PT: Terminated informa se o run já emitiu um evento terminal (RUN_FINISHED ou
// RUN_ERROR).
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

func (e *Emitter) write(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.writeLocked(ev)
}

func (e *Emitter) writeLocked(ev Event) {
	if e.err != nil {
		return
	}
	err := e.sse.WriteEvent(e.ctx, e.w, ev)
	if err == nil {
		return
	}
	if isTransportError(err) {
		e.err = fmt.Errorf("%w: %w", ErrClientGone, err)
		if e.cancel != nil {
			e.cancel()
		}
		return
	}
	if e.onDrop != nil {
		e.onDrop(e.ctx, ev, fmt.Errorf("%w: %w", ErrEventDropped, err))
	}
}

// writeTerminal emits a terminal event, at most once per run.
//
// PT: writeTerminal emite um evento terminal, no máximo uma vez por run.
func (e *Emitter) writeTerminal(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminated {
		return
	}
	e.terminated = true
	e.writeLocked(ev)
}

// isTransportError reports whether the WriteEvent error came from the
// write/flush on the socket (the client went away) rather than from encoding
// the event (a content bug).
//
// The SDK exports neither typed errors nor sentinels, so this matches the
// prefixes of the wrappers in pkg/encoding/sse/writer.go. It is a coupling to
// another library's strings, which is why it is isolated here and covered by
// the canary test TestIsTransportErrorRecognizesRealWriteFailure, which forces
// a real write failure: if the SDK changes those messages, the test breaks in
// CI instead of the symptom showing up in production as a run that dies
// silently.
//
// PT: isTransportError informa se o erro de WriteEvent veio da escrita/flush no
// socket (o cliente foi embora) e não da codificação do evento (bug de
// conteúdo).
//
// O SDK não exporta erros tipados nem sentinelas, então isto casa com os
// prefixos dos wrappers em pkg/encoding/sse/writer.go. É um acoplamento a
// strings de outra biblioteca, e por isso está isolado aqui e coberto pelo
// teste-canário TestIsTransportErrorRecognizesRealWriteFailure, que força uma
// falha de escrita de verdade: se o SDK mudar essas mensagens, o teste quebra
// em CI em vez de o sintoma aparecer em produção como um run que morre calado.
func isTransportError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SSE write failed") || strings.Contains(msg, "SSE flush failed")
}

// --- run lifecycle / ciclo de vida do run ---

// RunStarted emits RUN_STARTED. The [Handler] already emits it; a Runner must
// not repeat it.
//
// PT: RunStarted emite RUN_STARTED. O [Handler] já o emite; um Runner não deve
// repeti-lo.
func (e *Emitter) RunStarted() { e.write(events.NewRunStartedEvent(e.threadID, e.runID)) }

// RunFinished emits RUN_FINISHED, ending the run successfully.
//
// PT: RunFinished emite RUN_FINISHED, encerrando o run com sucesso.
func (e *Emitter) RunFinished() {
	e.writeTerminal(events.NewRunFinishedEvent(e.threadID, e.runID))
}

// RunFinishedWithInterrupts ends the run asking the client for a decision.
//
// The client answers in the next run through RunAgentInput.resume; see
// [Interrupt] and [RunContext.ResumeFor].
//
// PT: RunFinishedWithInterrupts encerra o run pedindo uma decisão ao cliente.
//
// O cliente responde no run seguinte através de RunAgentInput.resume; ver
// [Interrupt] e [RunContext.ResumeFor].
func (e *Emitter) RunFinishedWithInterrupts(its []InterruptPoint) {
	e.writeTerminal(events.NewRunFinishedEventWithOptions(
		e.threadID, e.runID, events.WithInterruptOutcome(its)))
}

// RunError emits RUN_ERROR, ending the run with a failure.
//
// PT: RunError emite RUN_ERROR, encerrando o run com falha.
func (e *Emitter) RunError(msg string) {
	e.writeTerminal(events.NewRunErrorEvent(msg, events.WithRunID(e.runID)))
}

// --- steps ---

// StepStarted and StepFinished emit STEP_STARTED/STEP_FINISHED. They are UI
// labels; the loop uses them around the model call and the tool execution (see
// [WithStepNames]).
//
// PT: StepStarted e StepFinished emitem STEP_STARTED/STEP_FINISHED. São rótulos de
// UI; o laço os usa em volta da chamada ao modelo e da execução das tools (ver
// [WithStepNames]).
func (e *Emitter) StepStarted(name string)  { e.write(events.NewStepStartedEvent(name)) }
func (e *Emitter) StepFinished(name string) { e.write(events.NewStepFinishedEvent(name)) }

// --- text messages / mensagens de texto ---

// TextStart opens an assistant text block with the given messageId. Every
// opened block needs its matching [Emitter.TextEnd], and two blocks must not
// overlap.
//
// PT: TextStart abre um bloco de texto do assistente com o messageId dado. Todo
// bloco aberto precisa do [Emitter.TextEnd] correspondente, e dois blocos não
// podem se sobrepor.
func (e *Emitter) TextStart(id string) {
	e.write(events.NewTextMessageStartEvent(id, events.WithRole("assistant")))
}

// TextContent emits a text delta for block id. An empty delta is ignored — the
// SDK would reject it.
//
// PT: TextContent emite um delta de texto do bloco id. Um delta vazio é ignorado —
// o SDK o recusaria.
func (e *Emitter) TextContent(id, delta string) {
	if delta == "" {
		return
	}
	e.write(events.NewTextMessageContentEvent(id, delta))
}

// TextEnd closes text block id.
//
// PT: TextEnd fecha o bloco de texto id.
func (e *Emitter) TextEnd(id string) { e.write(events.NewTextMessageEndEvent(id)) }

// --- tool calls ---

// ToolStart opens a tool call. parentMessageID links the call to the assistant
// message that produced it — this is what lets the client render the call
// inside the right message. Empty omits the field.
//
// PT: ToolStart abre uma tool call. parentMessageID liga a chamada à mensagem do
// assistente que a produziu — é o que permite ao cliente renderizar a chamada
// dentro da mensagem certa. Vazio omite o campo.
func (e *Emitter) ToolStart(toolCallID, name, parentMessageID string) {
	if parentMessageID == "" {
		e.write(events.NewToolCallStartEvent(toolCallID, name))
		return
	}
	e.write(events.NewToolCallStartEvent(toolCallID, name,
		events.WithParentMessageID(parentMessageID)))
}

// ToolArgs emits a fragment of the tool call arguments. An empty fragment is
// ignored — the SDK would reject it.
//
// PT: ToolArgs emite um fragmento dos argumentos da tool call. Um fragmento vazio é
// ignorado — o SDK o recusaria.
func (e *Emitter) ToolArgs(toolCallID, delta string) {
	if delta == "" {
		return
	}
	e.write(events.NewToolCallArgsEvent(toolCallID, delta))
}

// ToolEnd closes the tool call opened by [Emitter.ToolStart].
//
// PT: ToolEnd fecha a tool call aberta por [Emitter.ToolStart].
func (e *Emitter) ToolEnd(toolCallID string) { e.write(events.NewToolCallEndEvent(toolCallID)) }

// ToolResult emits TOOL_CALL_RESULT — the result the client renders for a tool
// executed on the server. Empty content becomes "(empty)", because the SDK
// rejects the empty field.
//
// PT: ToolResult emite TOOL_CALL_RESULT — o resultado que o cliente renderiza para
// uma tool executada no servidor. Um conteúdo vazio vira "(empty)", porque o
// SDK recusa o campo vazio.
func (e *Emitter) ToolResult(messageID, toolCallID, content string) {
	if content == "" {
		content = "(empty)"
	}
	e.write(events.NewToolCallResultEvent(messageID, toolCallID, content))
}

// --- state and messages / estado e mensagens ---

// StateSnapshot emits STATE_SNAPSHOT. A nil snapshot becomes an empty object:
// `null` is not a valid state on the client side.
//
// PT: StateSnapshot emite STATE_SNAPSHOT. Um snapshot nil vira um objeto vazio:
// `null` não é um estado válido do lado do cliente.
func (e *Emitter) StateSnapshot(snapshot any) {
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	e.write(events.NewStateSnapshotEvent(snapshot))
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
	e.write(events.NewStateDeltaEvent(ops))
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
func (e *Emitter) MessagesSnapshot(msgs []types.Message) {
	e.write(events.NewMessagesSnapshotEvent(msgs))
}

// --- custom ---

// Custom emits a CUSTOM event. The client ignores names it does not know, so
// this is a safe channel for data outside the protocol.
//
// PT: Custom emite um CUSTOM. O cliente ignora nomes que não conhece, então isto é
// um canal seguro para dados fora do protocolo.
func (e *Emitter) Custom(name string, value any) {
	e.write(events.NewCustomEvent(name, events.WithValue(value)))
}

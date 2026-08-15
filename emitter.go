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

// ErrClientGone indica que a escrita no socket falhou: o cliente desconectou.
// É um erro pegajoso — depois dele o Emitter vira no-op. Teste com errors.Is.
var ErrClientGone = errors.New("agui: cliente desconectou")

// ErrEventDropped indica que um evento foi recusado na codificação ou na
// validação do SDK e por isso não foi escrito. Diferente de [ErrClientGone],
// NÃO interrompe o stream: o evento cai, o run segue, e um evento terminal
// ainda alcança o cliente.
//
// Por que a distinção importa: em @ag-ui/client o parser faz
// `EventSchemas.parse(json)` e chama `eventSubject.error(err)` no catch — um
// único evento fora do schema derruba o Observable inteiro. Descartar o evento
// inválido e manter o stream vivo é a única forma de o cliente receber um
// RUN_ERROR explicando o que houve, em vez de uma conexão que morre em branco.
var ErrEventDropped = errors.New("agui: evento descartado na codificação")

// DroppedFunc é notificada quando um evento é descartado por [ErrEventDropped].
type DroppedFunc func(ctx context.Context, ev Event, err error)

// Emitter serializa eventos AG-UI num fluxo SSE.
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
	err        error // primeiro erro de transporte; depois dele, tudo é no-op
	terminated bool  // já saiu RUN_FINISHED ou RUN_ERROR
}

// EmitterOption configura um Emitter.
type EmitterOption func(*Emitter)

// WithEmitterCancel liga o Emitter ao cancelamento do run: a primeira falha de
// escrita chama cancel uma única vez.
func WithEmitterCancel(cancel context.CancelFunc) EmitterOption {
	return func(e *Emitter) { e.cancel = cancel }
}

// WithEmitterIDs fixa o threadId e o runId anunciados nos eventos. Vazios são
// substituídos por ids gerados — o SDK recusa eventos com esses campos vazios.
func WithEmitterIDs(threadID, runID string) EmitterOption {
	return func(e *Emitter) {
		e.threadID, e.runID = threadID, runID
	}
}

// WithEmitterDropped registra o callback de evento descartado.
func WithEmitterDropped(f DroppedFunc) EmitterOption {
	return func(e *Emitter) { e.onDrop = f }
}

// WithEmitterContext define o contexto usado nas escritas.
func WithEmitterContext(ctx context.Context) EmitterOption {
	return func(e *Emitter) {
		if ctx != nil {
			e.ctx = ctx
		}
	}
}

// WithEmitterSSEWriter reaproveita um *sse.SSEWriter já construído. O handler
// usa isto para compartilhar um writer entre requisições; fora dele raramente é
// necessário.
func WithEmitterSSEWriter(sw *sse.SSEWriter) EmitterOption {
	return func(e *Emitter) {
		if sw != nil {
			e.sse = sw
		}
	}
}

// NewEmitter cria um Emitter que escreve quadros SSE em w.
//
// Sem opções ele já é utilizável — o que importa para testes, onde um
// bytes.Buffer basta:
//
//	var buf bytes.Buffer
//	e := agui.NewEmitter(&buf)
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

// Err devolve o primeiro erro de transporte, embrulhando [ErrClientGone].
func (e *Emitter) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// Terminated informa se o run já emitiu um evento terminal (RUN_FINISHED ou
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

// ThreadID e RunID expõem os identificadores do run corrente.
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

// writeTerminal emite um evento terminal, no máximo uma vez por run.
func (e *Emitter) writeTerminal(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminated {
		return
	}
	e.terminated = true
	e.writeLocked(ev)
}

// isTransportError informa se o erro de WriteEvent veio da escrita/flush no
// socket (o cliente foi embora) e não da codificação do evento (bug de
// conteúdo).
//
// O SDK não exporta erros tipados nem sentinelas, então isto casa com os
// prefixos dos wrappers em pkg/encoding/sse/writer.go. É um acoplamento a
// strings de outra biblioteca, e por isso está isolado aqui e coberto pelo
// teste-canário TestIsTransportErrorReconheceFalhaDeEscritaReal, que força uma
// falha de escrita de verdade: se o SDK mudar essas mensagens, o teste quebra
// em CI em vez de o sintoma aparecer em produção como um run que morre calado.
func isTransportError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SSE write failed") || strings.Contains(msg, "SSE flush failed")
}

// --- ciclo de vida do run ---

// RunStarted emite RUN_STARTED. O [Handler] já o emite; um Runner não deve
// repeti-lo.
func (e *Emitter) RunStarted() { e.write(events.NewRunStartedEvent(e.threadID, e.runID)) }

// RunFinished emite RUN_FINISHED, encerrando o run com sucesso.
func (e *Emitter) RunFinished() {
	e.writeTerminal(events.NewRunFinishedEvent(e.threadID, e.runID))
}

// RunFinishedWithInterrupts encerra o run pedindo uma decisão ao cliente.
//
// O cliente responde no run seguinte através de RunAgentInput.resume; ver
// [Interrupt] e [RunContext.ResumeFor].
func (e *Emitter) RunFinishedWithInterrupts(its []InterruptPoint) {
	e.writeTerminal(events.NewRunFinishedEventWithOptions(
		e.threadID, e.runID, events.WithInterruptOutcome(its)))
}

// RunError emite RUN_ERROR, encerrando o run com falha.
func (e *Emitter) RunError(msg string) {
	e.writeTerminal(events.NewRunErrorEvent(msg, events.WithRunID(e.runID)))
}

// --- steps ---

// StepStarted e StepFinished emitem STEP_STARTED/STEP_FINISHED. São rótulos de
// UI; o laço os usa em volta da chamada ao modelo e da execução das tools (ver
// [WithStepNames]).
func (e *Emitter) StepStarted(name string)  { e.write(events.NewStepStartedEvent(name)) }
func (e *Emitter) StepFinished(name string) { e.write(events.NewStepFinishedEvent(name)) }

// --- mensagens de texto ---

// TextStart abre um bloco de texto do assistente com o messageId dado. Todo
// bloco aberto precisa do [Emitter.TextEnd] correspondente, e dois blocos não
// podem se sobrepor.
func (e *Emitter) TextStart(id string) {
	e.write(events.NewTextMessageStartEvent(id, events.WithRole("assistant")))
}

// TextContent emite um delta de texto do bloco id. Um delta vazio é ignorado —
// o SDK o recusaria.
func (e *Emitter) TextContent(id, delta string) {
	if delta == "" {
		return
	}
	e.write(events.NewTextMessageContentEvent(id, delta))
}

// TextEnd fecha o bloco de texto id.
func (e *Emitter) TextEnd(id string) { e.write(events.NewTextMessageEndEvent(id)) }

// --- tool calls ---

// ToolStart abre uma tool call. parentMessageID liga a chamada à mensagem do
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

// ToolArgs emite um fragmento dos argumentos da tool call. Um fragmento vazio é
// ignorado — o SDK o recusaria.
func (e *Emitter) ToolArgs(toolCallID, delta string) {
	if delta == "" {
		return
	}
	e.write(events.NewToolCallArgsEvent(toolCallID, delta))
}

// ToolEnd fecha a tool call aberta por [Emitter.ToolStart].
func (e *Emitter) ToolEnd(toolCallID string) { e.write(events.NewToolCallEndEvent(toolCallID)) }

// ToolResult emite TOOL_CALL_RESULT — o resultado que o cliente renderiza para
// uma tool executada no servidor. Um conteúdo vazio vira "(empty)", porque o
// SDK recusa o campo vazio.
func (e *Emitter) ToolResult(messageID, toolCallID, content string) {
	if content == "" {
		content = "(empty)"
	}
	e.write(events.NewToolCallResultEvent(messageID, toolCallID, content))
}

// --- estado e mensagens ---

// StateSnapshot emite STATE_SNAPSHOT. Um snapshot nil vira um objeto vazio:
// `null` não é um estado válido do lado do cliente.
func (e *Emitter) StateSnapshot(snapshot any) {
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	e.write(events.NewStateSnapshotEvent(snapshot))
}

// StateDelta emite STATE_DELTA com operações JSON Patch. Uma lista vazia é
// ignorada.
func (e *Emitter) StateDelta(ops []PatchOp) {
	if len(ops) == 0 {
		return
	}
	e.write(events.NewStateDeltaEvent(ops))
}

// MessagesSnapshot emite MESSAGES_SNAPSHOT — o evento de reconciliação que o
// adaptador Python emite ao fim de cada run (get_state_and_messages_snapshots).
// Sem ele a lista de mensagens do frontend fica sem reconciliação após tool
// calls.
func (e *Emitter) MessagesSnapshot(msgs []types.Message) {
	e.write(events.NewMessagesSnapshotEvent(msgs))
}

// --- custom ---

// Custom emite um CUSTOM. O cliente ignora nomes que não conhece, então isto é
// um canal seguro para dados fora do protocolo.
func (e *Emitter) Custom(name string, value any) {
	e.write(events.NewCustomEvent(name, events.WithValue(value)))
}

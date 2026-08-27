package agui

import (
	"context"
	"log/slog"
)

// RunContext is everything a tool needs to know about the run it is being
// executed in.
//
// It exists because a tool's function receives only `(ctx, input)` — that is
// how Eino invokes it — and therefore the context is the ONLY channel through
// which per-request data can arrive. Without it, a tool would have no way to
// emit an event, touch the shared state or know which call it is serving.
//
// A RunContext is created by the loop and injected into ctx before each
// invocation. Outside a run it does not exist: see [FromContext].
//
// PT: RunContext é tudo o que uma tool precisa saber sobre o run em que está sendo
// executada.
//
// Ele existe porque a função de uma tool recebe apenas `(ctx, entrada)` — é
// assim que o Eino a invoca — e portanto o contexto é o ÚNICO canal por onde
// dados por requisição podem chegar. Sem isto, uma tool não teria como emitir
// um evento, mexer no estado compartilhado ou saber qual chamada está
// atendendo.
//
// Um RunContext é criado pelo laço e injetado no ctx antes de cada invocação.
// Fora de um run ele não existe: ver [FromContext].
type RunContext struct {
	emitter    *Emitter
	state      *State
	input      *RunAgentInput
	logger     *slog.Logger
	toolCallID string
	toolName   string
	resume     map[string]any // interruptID/toolCallID → payload
}

type runContextKey struct{}

// FromContext retrieves the [RunContext] of the current run. ok is false
// outside a run — in a direct call to the tool's function in a test, for
// example.
//
// A tool that wants to be testable outside a run should handle ok=false
// instead of using [MustFromContext].
//
// PT: FromContext recupera o [RunContext] do run corrente. ok é falso fora de um
// run — numa chamada direta à função da tool num teste, por exemplo.
//
// Uma tool que queira ser testável fora de um run deve tratar ok=false em vez
// de usar [MustFromContext].
func FromContext(ctx context.Context) (*RunContext, bool) {
	rc, ok := ctx.Value(runContextKey{}).(*RunContext)
	return rc, ok
}

// MustFromContext is [FromContext] with a panic when there is no run.
//
// Use it when the tool only makes sense inside a run; the panic message says
// exactly what happened, which is better than a nil dereference three frames
// below.
//
// PT: MustFromContext é [FromContext] com pânico quando não há run.
//
// Use quando a tool só faz sentido dentro de um run; a mensagem do pânico diz
// exatamente o que houve, o que é melhor do que um nil dereference três frames
// abaixo.
func MustFromContext(ctx context.Context) *RunContext {
	rc, ok := FromContext(ctx)
	if !ok {
		panic("agui: no RunContext in this context — this function can only be " +
			"called from inside a tool executed by an agui.Agent")
	}
	return rc
}

func (r *RunContext) attach(ctx context.Context) context.Context {
	return context.WithValue(ctx, runContextKey{}, r)
}

// forToolCall derives a RunContext for a specific call. The shared fields
// (emitter, state, input) are the same pointers; only the call identity
// changes.
//
// PT: forToolCall deriva um RunContext para uma chamada específica. Os campos
// compartilhados (emitter, estado, input) são os mesmos ponteiros; só a
// identidade da chamada muda.
func (r *RunContext) forToolCall(id, name string) *RunContext {
	c := *r
	c.toolCallID = id
	c.toolName = name
	return &c
}

// Emitter returns the run's event emitter. Emitting from here is safe under
// concurrency; what you must NOT do is emit terminal events
// (RunFinished/RunError) from inside a tool — the loop is what ends the run.
//
// PT: Emitter devolve o emissor de eventos do run. Emitir daqui é seguro em
// concorrência; o que NÃO se deve fazer é emitir eventos terminais
// (RunFinished/RunError) de dentro de uma tool — quem encerra o run é o laço.
func (r *RunContext) Emitter() *Emitter { return r.emitter }

// State returns the run's shared state. Writing to it applies and emits the
// corresponding STATE_DELTA — see [State].
//
// PT: State devolve o estado compartilhado do run. Escrever nele aplica e emite o
// STATE_DELTA correspondente — ver [State].
func (r *RunContext) State() *State { return r.state }

// Input returns the raw RunAgentInput of the request, for whoever needs
// forwardedProps, the threadId or the application context.
//
// PT: Input devolve o RunAgentInput cru da requisição, para quem precisa de
// forwardedProps, do threadId ou do contexto da aplicação.
func (r *RunContext) Input() *RunAgentInput { return r.input }

// Logger returns a logger already annotated with thread and run.
//
// PT: Logger devolve um logger já anotado com thread e run.
func (r *RunContext) Logger() *slog.Logger { return r.logger }

// ThreadID and RunID identify the conversation and the current execution.
//
// PT: ThreadID e RunID identificam a conversa e a execução corrente.
func (r *RunContext) ThreadID() string { return r.emitter.ThreadID() }
func (r *RunContext) RunID() string    { return r.emitter.RunID() }

// ToolCallID is the id of the call this tool is serving. Empty outside the
// execution of a tool.
//
// It is the key for [RunContext.ResumeFor] and for the Interrupt: it is how
// the client correlates the question and the answer.
//
// PT: ToolCallID é o id da chamada que esta tool está atendendo. Vazio fora da
// execução de uma tool.
//
// É a chave para [RunContext.ResumeFor] e para o Interrupt: é por ele que o
// cliente correlaciona a pergunta e a resposta.
func (r *RunContext) ToolCallID() string { return r.toolCallID }

// ToolName is the name of the tool being executed. Empty outside a tool.
//
// PT: ToolName é o nome da tool em execução. Vazio fora de uma tool.
func (r *RunContext) ToolName() string { return r.toolName }

// Resume returns the resume entries received in RunAgentInput.resume.
//
// PT: Resume devolve as entradas de retomada recebidas em RunAgentInput.resume.
func (r *RunContext) Resume() []ResumeEntry {
	if r.input == nil {
		return nil
	}
	return r.input.Resume
}

// ResumeFor returns the payload the client sent for an interrupt identified by
// interruptID, and whether it exists.
//
// The usage pattern in a tool that asks for human confirmation is this:
//
//	func approve(ctx context.Context, in Purchase) (string, error) {
//		rc := agui.MustFromContext(ctx)
//		if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
//			return process(in, payload) // second pass: the human answered
//		}
//		return "", agui.Interrupt(agui.InterruptRequest{
//			Reason:  "needs_approval",
//			Message: "Confirm the purchase?",
//		})
//	}
//
// Only entries with status "resolved" are considered: a "cancelled" decision
// is not a resume value, and returning it as if it were would make the tool
// execute precisely what the user refused. Use [RunContext.Resume] to inspect
// the cancellations.
//
// PT: ResumeFor devolve o payload que o cliente enviou para um interrupt
// identificado por interruptID, e se ele existe.
//
// O padrão de uso numa tool que pede confirmação humana é o do exemplo acima.
//
// Só entradas com status "resolved" são consideradas: uma decisão "cancelled"
// não é um valor de retomada, e devolvê-la como se fosse faria a tool executar
// justamente o que o usuário recusou. Use [RunContext.Resume] para inspecionar
// os cancelamentos.
func (r *RunContext) ResumeFor(interruptID string) (any, bool) {
	if interruptID == "" {
		return nil, false
	}
	if r.resume == nil {
		r.resume = indexResume(r.Resume())
	}
	v, ok := r.resume[interruptID]
	return v, ok
}

// indexResume builds the interruptID → payload index of the resolved resumes.
//
// PT: indexResume monta o índice interruptID → payload das retomadas resolvidas.
func indexResume(entries []ResumeEntry) map[string]any {
	m := make(map[string]any, len(entries))
	for _, e := range entries {
		if e.InterruptID == "" || e.Status != ResumeStatusResolved {
			continue
		}
		// A nil payload is still an answer: the human approved without sending
		// data. The map needs to record the KEY, and the nil value is legitimate.
		//
		// PT: Um payload nulo ainda é uma resposta: o humano aprovou sem enviar
		// dados. O mapa precisa registrar a CHAVE, e o valor nil é legítimo.
		m[e.InterruptID] = e.Payload
	}
	return m
}

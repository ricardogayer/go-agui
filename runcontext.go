package agui

import (
	"context"
	"log/slog"
)

// RunContext é tudo o que uma tool precisa saber sobre o run em que está sendo
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

// FromContext recupera o [RunContext] do run corrente. ok é falso fora de um
// run — numa chamada direta à função da tool num teste, por exemplo.
//
// Uma tool que queira ser testável fora de um run deve tratar ok=false em vez
// de usar [MustFromContext].
func FromContext(ctx context.Context) (*RunContext, bool) {
	rc, ok := ctx.Value(runContextKey{}).(*RunContext)
	return rc, ok
}

// MustFromContext é [FromContext] com pânico quando não há run.
//
// Use quando a tool só faz sentido dentro de um run; a mensagem do pânico diz
// exatamente o que houve, o que é melhor do que um nil dereference três frames
// abaixo.
func MustFromContext(ctx context.Context) *RunContext {
	rc, ok := FromContext(ctx)
	if !ok {
		panic("agui: não há RunContext neste contexto — esta função só pode ser " +
			"chamada de dentro de uma tool executada por um agui.Agent")
	}
	return rc
}

func (r *RunContext) attach(ctx context.Context) context.Context {
	return context.WithValue(ctx, runContextKey{}, r)
}

// forToolCall deriva um RunContext para uma chamada específica. Os campos
// compartilhados (emitter, estado, input) são os mesmos ponteiros; só a
// identidade da chamada muda.
func (r *RunContext) forToolCall(id, name string) *RunContext {
	c := *r
	c.toolCallID = id
	c.toolName = name
	return &c
}

// Emitter devolve o emissor de eventos do run. Emitir daqui é seguro em
// concorrência; o que NÃO se deve fazer é emitir eventos terminais
// (RunFinished/RunError) de dentro de uma tool — quem encerra o run é o laço.
func (r *RunContext) Emitter() *Emitter { return r.emitter }

// State devolve o estado compartilhado do run. Escrever nele aplica e emite o
// STATE_DELTA correspondente — ver [State].
func (r *RunContext) State() *State { return r.state }

// Input devolve o RunAgentInput cru da requisição, para quem precisa de
// forwardedProps, do threadId ou do contexto da aplicação.
func (r *RunContext) Input() *RunAgentInput { return r.input }

// Logger devolve um logger já anotado com thread e run.
func (r *RunContext) Logger() *slog.Logger { return r.logger }

// ThreadID e RunID identificam a conversa e a execução corrente.
func (r *RunContext) ThreadID() string { return r.emitter.ThreadID() }
func (r *RunContext) RunID() string    { return r.emitter.RunID() }

// ToolCallID é o id da chamada que esta tool está atendendo. Vazio fora da
// execução de uma tool.
//
// É a chave para [RunContext.ResumeFor] e para o Interrupt: é por ele que o
// cliente correlaciona a pergunta e a resposta.
func (r *RunContext) ToolCallID() string { return r.toolCallID }

// ToolName é o nome da tool em execução. Vazio fora de uma tool.
func (r *RunContext) ToolName() string { return r.toolName }

// Resume devolve as entradas de retomada recebidas em RunAgentInput.resume.
func (r *RunContext) Resume() []ResumeEntry {
	if r.input == nil {
		return nil
	}
	return r.input.Resume
}

// ResumeFor devolve o payload que o cliente enviou para um interrupt
// identificado por interruptID, e se ele existe.
//
// O padrão de uso numa tool que pede confirmação humana é este:
//
//	func aprovar(ctx context.Context, in Compra) (string, error) {
//		rc := agui.MustFromContext(ctx)
//		if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
//			return processar(in, payload) // segunda passagem: o humano respondeu
//		}
//		return "", agui.Interrupt(agui.InterruptRequest{
//			Reason:  "needs_approval",
//			Message: "Confirmar a compra?",
//		})
//	}
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

// indexResume monta o índice interruptID → payload das retomadas resolvidas.
func indexResume(entries []ResumeEntry) map[string]any {
	m := make(map[string]any, len(entries))
	for _, e := range entries {
		if e.InterruptID == "" || e.Status != ResumeStatusResolved {
			continue
		}
		// Um payload nulo ainda é uma resposta: o humano aprovou sem enviar
		// dados. O mapa precisa registrar a CHAVE, e o valor nil é legítimo.
		m[e.InterruptID] = e.Payload
	}
	return m
}

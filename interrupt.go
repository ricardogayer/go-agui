package agui

import (
	"errors"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// ResumeStatus constants, re-exported from the SDK.
//
// PT: Constantes de ResumeStatus, reexportadas do SDK.
const (
	ResumeStatusResolved  = types.ResumeStatusResolved
	ResumeStatusCancelled = types.ResumeStatusCancelled
)

// InterruptRequest describes the pause a tool asks for: what needs to be
// decided and how the client should present the decision.
//
// PT: InterruptRequest descreve a pausa que uma tool pede: o que precisa ser
// decidido e como o cliente deve apresentar a decisão.
type InterruptRequest struct {
	// Reason is the machine-readable code of the pause ("needs_approval").
	// The client usually uses it to choose which component to render.
	//
	// PT: Reason é o código da pausa, legível por máquina ("needs_approval").
	// O cliente costuma usá-lo para escolher qual componente renderizar.
	Reason string

	// Message is the text presented to the human.
	//
	// PT: Message é o texto apresentado ao humano.
	Message string

	// ResponseSchema describes, in JSON Schema, the expected format of the
	// response. Optional: a simple approval does not need a schema.
	//
	// PT: ResponseSchema descreve, em JSON Schema, o formato esperado da resposta.
	// Opcional: uma aprovação simples não precisa de schema.
	ResponseSchema map[string]any

	// Metadata travels with the pause without interpretation by the library.
	//
	// PT: Metadata acompanha a pausa sem interpretação da biblioteca.
	Metadata map[string]any
}

// InterruptError is the error a tool returns to pause the run.
//
// Use [Interrupt] to build it. Detect it with errors.As when you need to
// distinguish the pause from a real failure.
//
// PT: InterruptError é o erro que uma tool devolve para pausar o run.
//
// Use [Interrupt] para construí-lo. Detecte com errors.As quando precisar
// distinguir a pausa de uma falha de verdade.
type InterruptError struct {
	Request InterruptRequest
}

func (e *InterruptError) Error() string {
	if e.Request.Message != "" {
		return "agui: run paused awaiting decision: " + e.Request.Message
	}
	return "agui: run paused awaiting decision"
}

// Interrupt returns the error that pauses the run for a human decision.
//
// The tool returns it like any other error; the loop recognizes the type, ends
// the run with RUN_FINISHED carrying the interrupt outcome, and leaves the
// pending call unanswered in the MESSAGES_SNAPSHOT (nothing is stored on the
// server). The client answers in RunAgentInput.resume, and on the next pass
// the tool retrieves the answer via [RunContext.ResumeFor]:
//
//	func approvePurchase(ctx context.Context, in Purchase) (string, error) {
//		rc := agui.MustFromContext(ctx)
//		if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
//			return process(in, payload)
//		}
//		return "", agui.Interrupt(agui.InterruptRequest{
//			Reason:  "needs_approval",
//			Message: "Confirm the purchase?",
//		})
//	}
//
// # Why a signal of our own and not Eino's tool.Interrupt
//
// Eino has `tool.Interrupt`, and its SIGNALING half would work here. The
// RESUME half does not: `tool.GetInterruptState` and `tool.GetResumeContext`
// read a NON-EXPORTED context key (eino/internal/core), populated only by the
// graph runner of the `compose` package. Since this loop invokes the tools
// directly — which is what allows handling tools the server does not execute
// —, there is no way to fill that key, and Eino also does not export a
// predicate to detect the interrupt error.
//
// The transport of the pause, therefore, is AG-UI's own: the protocol already
// has Interrupt and ResumeEntry designed exactly for this.
//
// PT: Interrupt devolve o erro que pausa o run para uma decisão humana.
//
// A tool o devolve como qualquer outro erro; o laço reconhece o tipo, encerra o
// run com RUN_FINISHED carregando o outcome de interrupt, e deixa a chamada
// pendente sem resposta no MESSAGES_SNAPSHOT (nada é guardado no servidor). O
// cliente responde em RunAgentInput.resume, e na próxima passagem
// a tool recupera a resposta por [RunContext.ResumeFor], como no exemplo acima.
//
// # Por que um sinal próprio e não o tool.Interrupt do Eino
//
// O Eino tem `tool.Interrupt`, e a metade de SINALIZAÇÃO dele funcionaria aqui.
// A metade de RETOMADA não: `tool.GetInterruptState` e `tool.GetResumeContext`
// leem uma chave de contexto NÃO EXPORTADA (eino/internal/core), populada
// apenas pelo runner de grafo do pacote `compose`. Como este laço invoca as
// tools diretamente — que é o que permite tratar tools que o servidor não
// executa —, não há como preencher essa chave, e o Eino também não exporta
// predicado para detectar o erro de interrupt.
//
// O transporte da pausa, portanto, é o do próprio AG-UI: o protocolo já tem
// Interrupt e ResumeEntry desenhados exatamente para isto.
func Interrupt(req InterruptRequest) error {
	return &InterruptError{Request: req}
}

// asInterrupt extracts the InterruptError from an error chain.
//
// PT: asInterrupt extrai o InterruptError de uma cadeia de erros.
func asInterrupt(err error) (*InterruptError, bool) {
	var ie *InterruptError
	if errors.As(err, &ie) {
		return ie, true
	}
	return nil, false
}

// toAGUI converts the pause into the protocol type. The interrupt id is the
// tool call id: it is what the client already has in hand to correlate the
// answer, and what [RunContext.ResumeFor] expects to receive back.
//
// PT: toAGUI converte a pausa no tipo do protocolo. O id do interrupt é o id da
// tool call: é o que o cliente já tem em mãos para correlacionar a resposta, e
// o que [RunContext.ResumeFor] espera receber de volta.
func (e *InterruptError) toAGUI(toolCallID string) InterruptPoint {
	return types.Interrupt{
		ID:             toolCallID,
		ToolCallID:     toolCallID,
		Reason:         e.Request.Reason,
		Message:        e.Request.Message,
		ResponseSchema: e.Request.ResponseSchema,
		Metadata:       e.Request.Metadata,
	}
}

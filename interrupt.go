package agui

import (
	"errors"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// Constantes de ResumeStatus, reexportadas do SDK.
const (
	ResumeStatusResolved  = types.ResumeStatusResolved
	ResumeStatusCancelled = types.ResumeStatusCancelled
)

// InterruptRequest descreve a pausa que uma tool pede: o que precisa ser
// decidido e como o cliente deve apresentar a decisão.
type InterruptRequest struct {
	// Reason é o código da pausa, legível por máquina ("needs_approval").
	// O cliente costuma usá-lo para escolher qual componente renderizar.
	Reason string

	// Message é o texto apresentado ao humano.
	Message string

	// ResponseSchema descreve, em JSON Schema, o formato esperado da resposta.
	// Opcional: uma aprovação simples não precisa de schema.
	ResponseSchema map[string]any

	// Metadata acompanha a pausa sem interpretação da biblioteca.
	Metadata map[string]any
}

// InterruptError é o erro que uma tool devolve para pausar o run.
//
// Use [Interrupt] para construí-lo. Detecte com errors.As quando precisar
// distinguir a pausa de uma falha de verdade.
type InterruptError struct {
	Request InterruptRequest
}

func (e *InterruptError) Error() string {
	if e.Request.Message != "" {
		return "agui: run pausado aguardando decisão: " + e.Request.Message
	}
	return "agui: run pausado aguardando decisão"
}

// Interrupt devolve o erro que pausa o run para uma decisão humana.
//
// A tool o devolve como qualquer outro erro; o laço reconhece o tipo, encerra o
// run com RUN_FINISHED carregando o outcome de interrupt, e guarda as chamadas
// pendentes. O cliente responde em RunAgentInput.resume, e na próxima passagem
// a tool recupera a resposta por [RunContext.ResumeFor]:
//
//	func aprovarCompra(ctx context.Context, in Compra) (string, error) {
//		rc := agui.MustFromContext(ctx)
//		if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
//			return processar(in, payload)
//		}
//		return "", agui.Interrupt(agui.InterruptRequest{
//			Reason:  "needs_approval",
//			Message: "Confirmar a compra?",
//		})
//	}
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

// asInterrupt extrai o InterruptError de uma cadeia de erros.
func asInterrupt(err error) (*InterruptError, bool) {
	var ie *InterruptError
	if errors.As(err, &ie) {
		return ie, true
	}
	return nil, false
}

// toAGUI converte a pausa no tipo do protocolo. O id do interrupt é o id da
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

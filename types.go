package agui

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/cloudwego/eino/components/tool"
)

// Os aliases abaixo reexportam os tipos do SDK AG-UI que aparecem na superfície
// desta biblioteca.
//
// O motivo é concreto: o SDK não tem tag semver — as 151 tags do repositório do
// protocolo são `release/YYYY-MM-DD` — e por isso é consumido por
// pseudo-version. Sem estes aliases, quem usa a biblioteca precisaria declarar
// `github.com/ag-ui-protocol/ag-ui/sdks/community/go` no próprio go.mod só para
// escrever a assinatura de um Runner, herdando um problema de versionamento que
// é nosso, não dele. Com eles, o import block de uma aplicação é `agui` + Eino.
//
// São aliases (`=`) e não tipos novos: um *types.RunAgentInput decodificado por
// qualquer outro código continua sendo aceito aqui, e vice-versa.
type (
	// RunAgentInput é o corpo do POST: a conversa inteira, as tools de
	// frontend, o estado e o contexto da aplicação.
	RunAgentInput = types.RunAgentInput

	// Message é uma mensagem da conversa no formato do fio.
	Message = types.Message

	// Role é o papel de uma Message ("user", "assistant", "tool", ...).
	Role = types.Role

	// ClientTool é uma tool declarada pelo frontend em RunAgentInput.tools.
	// Chama-se ClientTool, e não Tool, para não colidir com [Tool] — que é a
	// tool do lado do servidor.
	ClientTool = types.Tool

	// Context é uma entrada de RunAgentInput.context: o contexto da aplicação
	// que o runtime injeta (na lição 4, o catálogo de componentes A2UI).
	Context = types.Context

	// InterruptPoint é a representação NO FIO de uma pausa que aguarda decisão
	// humana — o que viaja no outcome do RUN_FINISHED.
	//
	// Não confundir com a função [Interrupt], que é o que uma tool chama para
	// pedir a pausa, nem com [InterruptRequest], que é o pedido em si. A
	// biblioteca converte um no outro; só quem monta o evento terminal à mão
	// precisa deste tipo.
	InterruptPoint = types.Interrupt

	// ResumeEntry é a resposta do cliente a um Interrupt, recebida em
	// RunAgentInput.resume no run seguinte.
	ResumeEntry = types.ResumeEntry

	// ResumeStatus diz se o humano resolveu ou cancelou um Interrupt.
	ResumeStatus = types.ResumeStatus

	// ToolCall é uma chamada de tool no formato do fio.
	ToolCall = types.ToolCall

	// FunctionCall é o par nome/argumentos de uma [ToolCall].
	FunctionCall = types.FunctionCall

	// PatchOp é uma operação JSON Patch (RFC 6902) de um STATE_DELTA.
	PatchOp = events.JSONPatchOperation

	// Event é um evento AG-UI já validável e serializável.
	Event = events.Event

	// EventType é o discriminador de um Event ("RUN_STARTED", ...). Prefira as
	// constantes a strings soltas: um typo em string compila e passa no teste.
	EventType = events.EventType

	// Tool é uma tool do servidor. É o [tool.BaseTool] do Eino, de propósito:
	// qualquer tool do ecossistema eino-ext (MCP, busca, ...) entra sem
	// adaptador. Ver a documentação do pacote para a regra que decide se uma
	// Tool é executada aqui ou devolvida ao cliente.
	Tool = tool.BaseTool
)

// Constantes de Role, reexportadas do SDK.
const (
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleSystem    = types.RoleSystem
	RoleDeveloper = types.RoleDeveloper
	RoleTool      = types.RoleTool
)

// Constantes de EventType, reexportadas para que asserções de teste e switches
// não dependam de strings literais.
const (
	EventRunStarted       = events.EventTypeRunStarted
	EventRunFinished      = events.EventTypeRunFinished
	EventRunError         = events.EventTypeRunError
	EventStepStarted      = events.EventTypeStepStarted
	EventStepFinished     = events.EventTypeStepFinished
	EventTextMessageStart = events.EventTypeTextMessageStart
	EventTextMessageChunk = events.EventTypeTextMessageContent
	EventTextMessageEnd   = events.EventTypeTextMessageEnd
	EventToolCallStart    = events.EventTypeToolCallStart
	EventToolCallArgs     = events.EventTypeToolCallArgs
	EventToolCallEnd      = events.EventTypeToolCallEnd
	EventToolCallResult   = events.EventTypeToolCallResult
	EventStateSnapshot    = events.EventTypeStateSnapshot
	EventStateDelta       = events.EventTypeStateDelta
	EventMessagesSnapshot = events.EventTypeMessagesSnapshot
	EventCustom           = events.EventTypeCustom
)

// Must desembrulha um par (valor, erro) e entra em pânico se houver erro.
//
// Existe para o `main()`, onde um erro de construção não tem para onde ir e um
// `if err != nil { log.Fatal(err) }` a cada linha só adiciona ruído:
//
//	ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt(prompt)))
//
// É um Must genérico em vez de um MustXxx por construtor. Não use fora da
// inicialização.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

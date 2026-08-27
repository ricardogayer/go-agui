package agui

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/cloudwego/eino/components/tool"
)

// The aliases below re-export the AG-UI SDK types that appear on the surface
// of this library.
//
// The reason is concrete: the SDK has no semver tag — the 151 tags of the
// protocol repository are `release/YYYY-MM-DD` — and is therefore consumed by
// pseudo-version. Without these aliases, users of the library would have to
// declare `github.com/ag-ui-protocol/ag-ui/sdks/community/go` in their own
// go.mod just to write the signature of a Runner, inheriting a versioning
// problem that is ours, not theirs. With them, an application's import block
// is `agui` + Eino.
//
// They are aliases (`=`) and not new types: a *types.RunAgentInput decoded by
// any other code is still accepted here, and vice versa.
//
// PT: Os aliases abaixo reexportam os tipos do SDK AG-UI que aparecem na
// superfície desta biblioteca.
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
	// RunAgentInput is the body of the POST: the whole conversation, the
	// frontend tools, the state and the application context.
	//
	// PT: RunAgentInput é o corpo do POST: a conversa inteira, as tools de
	// frontend, o estado e o contexto da aplicação.
	RunAgentInput = types.RunAgentInput

	// Message is a conversation message in wire format.
	//
	// PT: Message é uma mensagem da conversa no formato do fio.
	Message = types.Message

	// Role is the role of a Message ("user", "assistant", "tool", ...).
	//
	// PT: Role é o papel de uma Message ("user", "assistant", "tool", ...).
	Role = types.Role

	// ClientTool is a tool declared by the frontend in RunAgentInput.tools.
	// It is named ClientTool, not Tool, so as not to collide with [Tool] — which
	// is the server-side tool.
	//
	// PT: ClientTool é uma tool declarada pelo frontend em RunAgentInput.tools.
	// Chama-se ClientTool, e não Tool, para não colidir com [Tool] — que é a
	// tool do lado do servidor.
	ClientTool = types.Tool

	// Context is an entry of RunAgentInput.context: the application context
	// injected by the runtime (in lesson 4, the A2UI component catalog).
	//
	// PT: Context é uma entrada de RunAgentInput.context: o contexto da
	// aplicação que o runtime injeta (na lição 4, o catálogo de componentes
	// A2UI).
	Context = types.Context

	// InterruptPoint is the ON-THE-WIRE representation of a pause awaiting a
	// human decision — what travels in the outcome of RUN_FINISHED.
	//
	// Not to be confused with the [Interrupt] function, which is what a tool
	// calls to request the pause, nor with [InterruptRequest], which is the
	// request itself. The library converts one into the other; only whoever
	// builds the terminal event by hand needs this type.
	//
	// PT: InterruptPoint é a representação NO FIO de uma pausa que aguarda
	// decisão humana — o que viaja no outcome do RUN_FINISHED.
	//
	// Não confundir com a função [Interrupt], que é o que uma tool chama para
	// pedir a pausa, nem com [InterruptRequest], que é o pedido em si. A
	// biblioteca converte um no outro; só quem monta o evento terminal à mão
	// precisa deste tipo.
	InterruptPoint = types.Interrupt

	// ResumeEntry is the client's answer to an Interrupt, received in
	// RunAgentInput.resume on the next run.
	//
	// PT: ResumeEntry é a resposta do cliente a um Interrupt, recebida em
	// RunAgentInput.resume no run seguinte.
	ResumeEntry = types.ResumeEntry

	// ResumeStatus says whether the human resolved or cancelled an Interrupt.
	//
	// PT: ResumeStatus diz se o humano resolveu ou cancelou um Interrupt.
	ResumeStatus = types.ResumeStatus

	// ToolCall is a tool call in wire format.
	//
	// PT: ToolCall é uma chamada de tool no formato do fio.
	ToolCall = types.ToolCall

	// FunctionCall is the name/arguments pair of a [ToolCall].
	//
	// PT: FunctionCall é o par nome/argumentos de uma [ToolCall].
	FunctionCall = types.FunctionCall

	// PatchOp is a JSON Patch (RFC 6902) operation of a STATE_DELTA.
	//
	// PT: PatchOp é uma operação JSON Patch (RFC 6902) de um STATE_DELTA.
	PatchOp = events.JSONPatchOperation

	// Event is an AG-UI event that is already validatable and serializable.
	//
	// PT: Event é um evento AG-UI já validável e serializável.
	Event = events.Event

	// EventType is the discriminator of an Event ("RUN_STARTED", ...). Prefer
	// the constants to loose strings: a typo in a string compiles and passes
	// the test.
	//
	// PT: EventType é o discriminador de um Event ("RUN_STARTED", ...). Prefira
	// as constantes a strings soltas: um typo em string compila e passa no
	// teste.
	EventType = events.EventType

	// Tool is a server tool. It is Eino's [tool.BaseTool], on purpose: any tool
	// from the eino-ext ecosystem (MCP, search, ...) comes in without an
	// adapter. See the package documentation for the rule that decides whether
	// a Tool is executed here or handed back to the client.
	//
	// PT: Tool é uma tool do servidor. É o [tool.BaseTool] do Eino, de
	// propósito: qualquer tool do ecossistema eino-ext (MCP, busca, ...) entra
	// sem adaptador. Ver a documentação do pacote para a regra que decide se
	// uma Tool é executada aqui ou devolvida ao cliente.
	Tool = tool.BaseTool
)

// Role constants, re-exported from the SDK.
//
// PT: Constantes de Role, reexportadas do SDK.
const (
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleSystem    = types.RoleSystem
	RoleDeveloper = types.RoleDeveloper
	RoleTool      = types.RoleTool
)

// EventType constants, re-exported so that test assertions and switches do not
// depend on string literals.
//
// PT: Constantes de EventType, reexportadas para que asserções de teste e
// switches não dependam de strings literais.
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

// Must unwraps a (value, error) pair and panics if there is an error.
//
// It exists for `main()`, where a construction error has nowhere to go and an
// `if err != nil { log.Fatal(err) }` on every line only adds noise:
//
//	ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt(prompt)))
//
// It is a generic Must instead of a MustXxx per constructor. Do not use it
// outside initialization.
//
// PT: Must desembrulha um par (valor, erro) e entra em pânico se houver erro.
//
// Existe para o `main()`, onde um erro de construção não tem para onde ir e um
// `if err != nil { log.Fatal(err) }` a cada linha só adiciona ruído (ver o
// exemplo acima).
//
// É um Must genérico em vez de um MustXxx por construtor. Não use fora da
// inicialização.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

package agui

import (
	"github.com/cloudwego/eino/components/tool"

	wire "github.com/ricardogayer/go-agui/pkg/agui"
)

// The aliases below re-export the AG-UI 1.0 data model (package
// [github.com/ricardogayer/go-agui/pkg/agui], imported here as `wire`) on the
// surface of this library.
//
// With them, an application's import block is `agui` + Eino: writing the
// signature of a Runner, building a message or answering an interrupt needs no
// second import. They are aliases (`=`) and not new types: a
// *wire.RunAgentInput decoded by any other code is still accepted here, and
// vice versa.
//
// PT: Os aliases abaixo reexportam o modelo de dados do AG-UI 1.0 (pacote
// [github.com/ricardogayer/go-agui/pkg/agui], importado aqui como `wire`) na
// superfície desta biblioteca.
//
// Com eles, o import block de uma aplicação é `agui` + Eino: escrever a
// assinatura de um Runner, montar uma mensagem ou responder a um interrupt não
// exige um segundo import. São aliases (`=`) e não tipos novos: um
// *wire.RunAgentInput decodificado por qualquer outro código continua sendo
// aceito aqui, e vice-versa.
type (
	// RunAgentInput is the body of the POST: the whole conversation, the
	// frontend tools, the state and the application context.
	//
	// PT: RunAgentInput é o corpo do POST: a conversa inteira, as tools de
	// frontend, o estado e o contexto da aplicação.
	RunAgentInput = wire.RunAgentInput

	// Message is a conversation message in wire format: one of
	// *UserMessage, *AssistantMessage, *SystemMessage, *DeveloperMessage,
	// *ToolMessage, *ActivityMessage or *ReasoningMessage. Use a type switch
	// to read it.
	//
	// PT: Message é uma mensagem da conversa no formato do fio: um de
	// *UserMessage, *AssistantMessage, *SystemMessage, *DeveloperMessage,
	// *ToolMessage, *ActivityMessage ou *ReasoningMessage. Use um type switch
	// para lê-la.
	Message = wire.Message

	// Messages is a list of [Message] that (un)marshals by role.
	//
	// PT: Messages é uma lista de [Message] que (de)serializa pelo role.
	Messages = wire.Messages

	// The concrete message types.
	//
	// PT: Os tipos concretos de mensagem.
	UserMessage      = wire.UserMessage
	AssistantMessage = wire.AssistantMessage
	SystemMessage    = wire.SystemMessage
	DeveloperMessage = wire.DeveloperMessage
	ToolMessage      = wire.ToolMessage
	ActivityMessage  = wire.ActivityMessage
	ReasoningMessage = wire.ReasoningMessage

	// Content is the content of a user or tool message: a string or a list of
	// multimodal parts. Build it with [TextContent] or [PartsContent].
	//
	// PT: Content é o conteúdo de uma mensagem de usuário ou de tool: uma
	// string ou uma lista de partes multimodais. Monte com [TextContent] ou
	// [PartsContent].
	Content = wire.Content

	// ContentPart is one multimodal part (text, image, audio, video,
	// document).
	//
	// PT: ContentPart é uma parte multimodal (texto, imagem, áudio, vídeo,
	// documento).
	ContentPart = wire.ContentPart

	// Role is the role of a Message ("user", "assistant", "tool", ...).
	//
	// PT: Role é o papel de uma Message ("user", "assistant", "tool", ...).
	Role = wire.Role

	// ClientTool is a tool declared by the frontend in RunAgentInput.tools.
	// It is named ClientTool, not Tool, so as not to collide with [Tool] — which
	// is the server-side tool.
	//
	// PT: ClientTool é uma tool declarada pelo frontend em RunAgentInput.tools.
	// Chama-se ClientTool, e não Tool, para não colidir com [Tool] — que é a
	// tool do lado do servidor.
	ClientTool = wire.Tool

	// Context is an entry of RunAgentInput.context: the application context
	// injected by the runtime (e.g. the A2UI component catalog injected by
	// CopilotKit's A2UI middleware).
	//
	// PT: Context é uma entrada de RunAgentInput.context: o contexto da
	// aplicação que o runtime injeta (ex.: o catálogo de componentes A2UI
	// injetado pelo middleware A2UI do CopilotKit).
	Context = wire.Context

	// InterruptPoint is the ON-THE-WIRE representation of a pause awaiting a
	// human decision — what travels in the interrupt outcome of RUN_FINISHED.
	//
	// Not to be confused with the [Interrupt] function, which is what a tool
	// calls to request the pause, nor with [InterruptRequest], which is the
	// request itself. The library converts one into the other; only whoever
	// builds the terminal event by hand needs this type.
	//
	// PT: InterruptPoint é a representação NO FIO de uma pausa que aguarda
	// decisão humana — o que viaja no outcome de interrupt do RUN_FINISHED.
	//
	// Não confundir com a função [Interrupt], que é o que uma tool chama para
	// pedir a pausa, nem com [InterruptRequest], que é o pedido em si. A
	// biblioteca converte um no outro; só quem monta o evento terminal à mão
	// precisa deste tipo.
	InterruptPoint = wire.Interrupt

	// ResumeEntry is the client's answer to an Interrupt, received in
	// RunAgentInput.resume on the next run.
	//
	// PT: ResumeEntry é a resposta do cliente a um Interrupt, recebida em
	// RunAgentInput.resume no run seguinte.
	ResumeEntry = wire.ResumeEntry

	// ResumeStatus says whether the human resolved or cancelled an Interrupt.
	//
	// PT: ResumeStatus diz se o humano resolveu ou cancelou um Interrupt.
	ResumeStatus = wire.ResumeStatus

	// ToolCall is a tool call in wire format.
	//
	// PT: ToolCall é uma chamada de tool no formato do fio.
	ToolCall = wire.ToolCall

	// FunctionCall is the name/arguments pair of a [ToolCall].
	//
	// PT: FunctionCall é o par nome/argumentos de uma [ToolCall].
	FunctionCall = wire.FunctionCall

	// TokenUsage is the token accounting reported on the terminal event.
	//
	// PT: TokenUsage é a contabilidade de tokens informada no evento terminal.
	TokenUsage = wire.TokenUsage

	// AgentCapabilities is the AG-UI 1.0 capability declaration of an agent;
	// serve it with [Capabilities].
	//
	// PT: AgentCapabilities é a declaração de capacidades AG-UI 1.0 de um
	// agente; sirva-a com [Capabilities].
	AgentCapabilities = wire.AgentCapabilities

	// PatchOp is a JSON Patch (RFC 6902) operation of a STATE_DELTA. Build it
	// with wire.Add, wire.Replace, ... or as a literal.
	//
	// PT: PatchOp é uma operação JSON Patch (RFC 6902) de um STATE_DELTA.
	// Monte com wire.Add, wire.Replace, ... ou como literal.
	PatchOp = wire.PatchOperation

	// Event is an AG-UI 1.0 event: one of the 31 concrete event types of
	// package wire.
	//
	// PT: Event é um evento AG-UI 1.0: um dos 31 tipos concretos de evento do
	// pacote wire.
	Event = wire.Event

	// EventType is the discriminator of an Event ("RUN_STARTED", ...). Prefer
	// the constants to loose strings: a typo in a string compiles and passes
	// the test.
	//
	// PT: EventType é o discriminador de um Event ("RUN_STARTED", ...). Prefira
	// as constantes a strings soltas: um typo em string compila e passa no
	// teste.
	EventType = wire.EventType

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

// ProtocolVersion is the AG-UI protocol version this library speaks; it is
// declared on every RUN_STARTED.
//
// PT: ProtocolVersion é a versão do protocolo AG-UI que esta biblioteca fala;
// é declarada em todo RUN_STARTED.
const ProtocolVersion = wire.ProtocolVersion

// Role constants, re-exported from the data model.
//
// PT: Constantes de Role, reexportadas do modelo de dados.
const (
	RoleUser      = wire.RoleUser
	RoleAssistant = wire.RoleAssistant
	RoleSystem    = wire.RoleSystem
	RoleDeveloper = wire.RoleDeveloper
	RoleTool      = wire.RoleTool
	RoleActivity  = wire.RoleActivity
	RoleReasoning = wire.RoleReasoning
)

// EventType constants, re-exported so that test assertions and switches do not
// depend on string literals. All 31 events of AG-UI 1.0 are covered.
//
// PT: Constantes de EventType, reexportadas para que asserções de teste e
// switches não dependam de strings literais. Os 31 eventos do AG-UI 1.0
// estão cobertos.
const (
	EventRunStarted              = wire.EventTypeRunStarted
	EventRunFinished             = wire.EventTypeRunFinished
	EventRunError                = wire.EventTypeRunError
	EventStepStarted             = wire.EventTypeStepStarted
	EventStepFinished            = wire.EventTypeStepFinished
	EventTextMessageStart        = wire.EventTypeTextMessageStart
	EventTextMessageContent      = wire.EventTypeTextMessageContent
	EventTextMessageEnd          = wire.EventTypeTextMessageEnd
	EventTextMessageChunk        = wire.EventTypeTextMessageChunk
	EventToolCallStart           = wire.EventTypeToolCallStart
	EventToolCallArgs            = wire.EventTypeToolCallArgs
	EventToolCallEnd             = wire.EventTypeToolCallEnd
	EventToolCallChunk           = wire.EventTypeToolCallChunk
	EventToolCallResult          = wire.EventTypeToolCallResult
	EventReasoningStart          = wire.EventTypeReasoningStart
	EventReasoningMessageStart   = wire.EventTypeReasoningMessageStart
	EventReasoningMessageContent = wire.EventTypeReasoningMessageContent
	EventReasoningMessageEnd     = wire.EventTypeReasoningMessageEnd
	EventReasoningMessageChunk   = wire.EventTypeReasoningMessageChunk
	EventReasoningEnd            = wire.EventTypeReasoningEnd
	EventReasoningEncryptedValue = wire.EventTypeReasoningEncryptedValue
	EventStateSnapshot           = wire.EventTypeStateSnapshot
	EventStateDelta              = wire.EventTypeStateDelta
	EventMessagesSnapshot        = wire.EventTypeMessagesSnapshot
	EventActivitySnapshot        = wire.EventTypeActivitySnapshot
	EventActivityDelta           = wire.EventTypeActivityDelta
	EventSubagentStarted         = wire.EventTypeSubagentStarted
	EventSubagentFinished        = wire.EventTypeSubagentFinished
	EventSubagentError           = wire.EventTypeSubagentError
	EventRaw                     = wire.EventTypeRaw
	EventCustom                  = wire.EventTypeCustom
)

// TextContent returns string [Content].
//
// PT: TextContent devolve um [Content] de string.
func TextContent(s string) Content { return wire.TextContent(s) }

// PartsContent returns multimodal [Content] made of parts.
//
// PT: PartsContent devolve um [Content] multimodal feito de partes.
func PartsContent(parts ...ContentPart) Content { return wire.PartsContent(parts...) }

// Ptr returns a pointer to v. Optional fields of the wire types are pointers,
// because on the wire an absent field differs from an empty one.
//
// PT: Ptr devolve um ponteiro para v. Campos opcionais dos tipos do fio são
// ponteiros, porque no fio um campo ausente difere de um vazio.
func Ptr[T any](v T) *T { return &v }

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

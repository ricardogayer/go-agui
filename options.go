package agui

import (
	"context"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/schema"
)

// DefaultMaxIterations is the default cap on model↔tools loop rounds per run.
//
// Eight comfortably covers the real case (calling a data tool, then a
// presentation one, then answering) and limits the damage of a pathological
// loop, in which the model insists on a tool that always fails — each round is
// a paid call to the provider.
//
// PT: DefaultMaxIterations é o teto padrão de voltas do laço modelo↔tools por
// run.
//
// Oito cobre com folga o caso real (chamar uma tool de dados, depois uma de
// apresentação, depois responder) e limita o prejuízo de um laço patológico, em
// que o modelo insiste numa tool que sempre falha — cada volta é uma chamada
// paga ao provedor.
const DefaultMaxIterations = 8

// MaxIterationsCeiling is the absolute ceiling accepted by [WithMaxIterations].
//
// PT: MaxIterationsCeiling é o teto absoluto aceito por [WithMaxIterations].
const MaxIterationsCeiling = 64

// ContextFormatter turns the request's `context` into a system block.
// The default is [AppContextBlock]; see [WithContextFormatter].
//
// PT: ContextFormatter transforma o `context` da requisição num bloco de
// sistema. O padrão é [AppContextBlock]; ver [WithContextFormatter].
type ContextFormatter func([]Context) string

// MessagePreprocessor receives the messages already prepared for the model and
// returns the ones that will actually be sent. It runs on every loop round.
//
// PT: MessagePreprocessor recebe as mensagens já prontas para o modelo e
// devolve as que serão de fato enviadas. Roda a cada volta do laço.
type MessagePreprocessor func(ctx context.Context, msgs []*schema.Message) []*schema.Message

// StateFactory builds the shared state of a run.
//
// The default seeds the state with `RunAgentInput.state` and returns it intact,
// which is the correct behaviour for an agent without state of its own: the
// client's `agent.state` must not regress just because it passed through here.
//
// PT: StateFactory constrói o estado compartilhado de um run.
//
// O padrão semeia o estado com `RunAgentInput.state` e o devolve intacto, que é
// o comportamento correto para um agente sem estado próprio: o `agent.state` do
// cliente não pode regredir só porque passou por aqui.
type StateFactory func(ctx context.Context, in *RunAgentInput) (*State, error)

// TurnInfo describes a model turn that has already completed.
//
// PT: TurnInfo descreve um turno do modelo já concluído.
type TurnInfo struct {
	Iteration int
	Duration  time.Duration
	ToolCalls int
	// Usage carries the turn's tokens when the provider reports them. It is the
	// only way to know how much a run cost.
	//
	// PT: Usage traz os tokens do turno quando o provedor os informa. É a única
	// forma de saber quanto um run custou.
	Usage *schema.TokenUsage
}

// ToolRunInfo describes the execution of a server tool that has already
// completed. Not to be confused with Eino's schema.ToolInfo, which is the
// schema declaration.
//
// PT: ToolRunInfo descreve a execução de uma tool de servidor já concluída.
// Não confundir com schema.ToolInfo do Eino, que é a declaração do schema.
type ToolRunInfo struct {
	Name        string
	ToolCallID  string
	Args        string
	Result      string
	Duration    time.Duration
	Err         error
	Interrupted bool
}

// RunInfo summarizes a finished run.
//
// PT: RunInfo resume um run terminado.
type RunInfo struct {
	Iterations int
	Duration   time.Duration
	// Outcome is "success", "interrupt" or "error".
	//
	// PT: Outcome é "success", "interrupt" ou "error".
	Outcome string
	Err     error
}

// Hooks are the run's observation points. All fields are optional.
//
// They exist because a logger does not answer the questions one asks in
// production: how much it cost, which tools ran, how long each turn took.
//
// PT: Hooks são os pontos de observação do run. Todos os campos são opcionais.
//
// Existem porque um logger não responde as perguntas que se faz em produção:
// quanto custou, quais tools rodaram, quanto tempo cada turno levou.
type Hooks struct {
	OnRunStart   func(ctx context.Context, in *RunAgentInput)
	OnRunEnd     func(ctx context.Context, info RunInfo)
	OnModelTurn  func(ctx context.Context, info TurnInfo)
	OnToolResult func(ctx context.Context, info ToolRunInfo)
}

type options struct {
	systemPrompt  string
	tools         []Tool
	maxIterations int
	logger        *slog.Logger
	ctxFormatter  ContextFormatter
	preprocessor  MessagePreprocessor
	stateFactory  StateFactory
	hooks         Hooks
	stepModel     string
	stepTools     string
}

// Option configures an [Agent].
//
// PT: Option configura um [Agent].
type Option func(*options)

// WithSystemPrompt sets the agent's system prompt.
//
// It is only injected if the conversation does not already carry a system
// message from the client — whoever sends an explicit system message is in
// charge of it.
//
// PT: WithSystemPrompt define o prompt de sistema do agente.
//
// Ele só é injetado se a conversa ainda não trouxer uma mensagem de sistema do
// cliente — quem manda um system explícito manda nele.
func WithSystemPrompt(prompt string) Option {
	return func(o *options) { o.systemPrompt = prompt }
}

// WithTools registers the agent's tools.
//
// It accepts any Eino [tool.BaseTool], which includes the tools of the eino-ext
// ecosystem (MCP, search) without an adapter. Whatever implements
// `InvokableRun` is executed here; whatever implements only `Info` is announced
// to the model and hands control back to the client. See [NewTool],
// [NewRawTool] and [NewClientTool].
//
// Successive calls accumulate.
//
// PT: WithTools registra as tools do agente.
//
// Aceita qualquer [tool.BaseTool] do Eino, o que inclui as tools do ecossistema
// eino-ext (MCP, busca) sem adaptador. Quem implementa `InvokableRun` é
// executado aqui; quem implementa só `Info` é anunciado ao modelo e devolve o
// controle ao cliente. Ver [NewTool], [NewRawTool] e [NewClientTool].
//
// Chamadas sucessivas acumulam.
func WithTools(tools ...Tool) Option {
	return func(o *options) { o.tools = append(o.tools, tools...) }
}

// WithMaxIterations limits the model↔tools loop rounds per run.
// Values outside [1, [MaxIterationsCeiling]] make [New] return an error.
//
// PT: WithMaxIterations limita as voltas do laço modelo↔tools por run.
// Valores fora de [1, [MaxIterationsCeiling]] fazem [New] devolver erro.
func WithMaxIterations(n int) Option {
	return func(o *options) { o.maxIterations = n }
}

// WithLogger sets the agent's logger. The default is slog.Default().
//
// PT: WithLogger define o logger do agente. O padrão é slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithContextFormatter replaces the formatting of the request's `context`.
//
// The default, [AppContextBlock], reproduces byte for byte the block that the
// Python CopilotKitMiddleware produced. Changing this changes what the model
// reads — read the documentation of [AppContextBlock] first, because that is
// the channel through which things arrive without which the model hallucinates
// (the A2UI component catalog, for example).
//
// PT: WithContextFormatter troca a formatação do `context` da requisição.
//
// O padrão, [AppContextBlock], reproduz byte a byte o bloco que o
// CopilotKitMiddleware do Python produzia. Trocar isto muda o que o modelo lê —
// leia a documentação de [AppContextBlock] antes, porque é por esse canal que
// chegam coisas sem as quais o modelo alucina (o catálogo de componentes A2UI,
// por exemplo).
func WithContextFormatter(f ContextFormatter) Option {
	return func(o *options) {
		if f != nil {
			o.ctxFormatter = f
		}
	}
}

// NoContext turns off forwarding of the request's `context` to the model.
//
// PT: NoContext desliga o repasse do `context` da requisição ao modelo.
func NoContext() Option {
	return func(o *options) { o.ctxFormatter = func([]Context) string { return "" } }
}

// WithMessagePreprocessor installs a final transformation on the messages
// before each call to the model — truncating history, injecting few-shots,
// redacting sensitive data.
//
// PT: WithMessagePreprocessor instala uma última transformação nas mensagens
// antes de cada chamada ao modelo — truncar histórico, injetar few-shots,
// redigir dados sensíveis.
func WithMessagePreprocessor(f MessagePreprocessor) Option {
	return func(o *options) { o.preprocessor = f }
}

// WithState replaces the construction of the run's shared state.
//
// Use it when the agent has state of its own to seed or validate:
//
//	agui.WithState(func(ctx context.Context, in *agui.RunAgentInput) (*agui.State, error) {
//		s := agui.NewState(map[string]any{"todos": []any{}})
//		return s, s.Apply(seedFromClient(in.State))
//	})
//
// PT: WithState troca a construção do estado compartilhado do run.
//
// Use quando o agente tiver estado próprio a semear ou validar (ver o exemplo
// acima).
func WithState(f StateFactory) Option {
	return func(o *options) {
		if f != nil {
			o.stateFactory = f
		}
	}
}

// WithHooks installs the run's observers. See [Hooks].
//
// PT: WithHooks instala os observadores do run. Ver [Hooks].
func WithHooks(h Hooks) Option {
	return func(o *options) { o.hooks = h }
}

// WithStepNames replaces the names of the STEP_STARTED/STEP_FINISHED emitted
// around the model call and the tool execution. They are UI labels; the
// defaults are "model" and "tools".
//
// PT: WithStepNames troca os nomes dos STEP_STARTED/STEP_FINISHED emitidos em
// volta da chamada ao modelo e da execução das tools. São rótulos de UI; os
// padrões são "model" e "tools".
func WithStepNames(model, tools string) Option {
	return func(o *options) {
		if model != "" {
			o.stepModel = model
		}
		if tools != "" {
			o.stepTools = tools
		}
	}
}

func defaultOptions() *options {
	return &options{
		maxIterations: DefaultMaxIterations,
		logger:        slog.Default(),
		ctxFormatter:  AppContextBlock,
		stepModel:     "model",
		stepTools:     "tools",
		stateFactory: func(_ context.Context, in *RunAgentInput) (*State, error) {
			return stateFromAny(in.State), nil
		},
	}
}

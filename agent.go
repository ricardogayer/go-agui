package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Agent é um agente AG-UI: o laço modelo↔tools, o streaming dos eventos e as
// regras que decidem quem executa cada tool.
//
// É seguro para uso concorrente e não é mutado por requisição: as tools de
// frontend são ligadas ao modelo com WithTools, que devolve uma instância nova.
type Agent struct {
	base model.ToolCallingChatModel
	opts *options

	// invokable são as tools que o servidor executa, por nome.
	invokable map[string]tool.InvokableTool
	// infos são os schemas anunciados ao modelo, na ordem de registro.
	infos []*schema.ToolInfo
}

// New constrói um agente sobre um chat model do Eino.
//
// Recebe ctx porque os schemas das tools são resolvidos aqui — e `Info` pode
// fazer I/O de rede, como faz uma tool MCP. Resolver na construção, e não a
// cada requisição, é o que permite falhar na subida do processo em vez de no
// meio de uma conversa.
//
// Erros de construção de tools são agregados: uma struct tag inválida em duas
// tools diferentes aparece nas duas, de uma vez, com o nome de cada uma.
func New(ctx context.Context, m model.ToolCallingChatModel, opts ...Option) (*Agent, error) {
	if m == nil {
		return nil, errors.New("agui: o chat model não pode ser nil")
	}

	cfg := defaultOptions()
	for _, o := range opts {
		o(cfg)
	}
	if cfg.maxIterations < 1 || cfg.maxIterations > MaxIterationsCeiling {
		return nil, fmt.Errorf(
			"agui: WithMaxIterations(%d) fora da faixa aceita [1, %d]",
			cfg.maxIterations, MaxIterationsCeiling)
	}

	a := &Agent{
		base:      m,
		opts:      cfg,
		invokable: make(map[string]tool.InvokableTool, len(cfg.tools)),
		infos:     make([]*schema.ToolInfo, 0, len(cfg.tools)),
	}

	var errs []error
	seen := make(map[string]bool, len(cfg.tools))
	for _, t := range cfg.tools {
		info, err := t.Info(ctx)
		if err != nil {
			// brokenTool já embrulha a mensagem com o nome da tool.
			errs = append(errs, err)
			continue
		}
		if info == nil || info.Name == "" {
			errs = append(errs, errors.New("uma tool devolveu um ToolInfo sem nome"))
			continue
		}
		if seen[info.Name] {
			errs = append(errs, fmt.Errorf("tool %q registrada duas vezes", info.Name))
			continue
		}
		seen[info.Name] = true
		a.infos = append(a.infos, info)
		if inv, ok := t.(tool.InvokableTool); ok {
			a.invokable[info.Name] = inv
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("agui: %w", errors.Join(errs...))
	}
	return a, nil
}

// Run executa um run AG-UI completo, do primeiro turno do modelo ao evento
// terminal.
//
// RUN_STARTED já foi emitido por [Handler]; não o repita. O erro devolvido é
// para o transporte registrar — o evento terminal correspondente já foi emitido
// aqui.
func (a *Agent) Run(ctx context.Context, emit *Emitter, in *RunAgentInput) error {
	start := time.Now()
	logger := a.opts.logger.With("thread", emit.ThreadID(), "run", emit.RunID())

	if h := a.opts.hooks.OnRunStart; h != nil {
		h(ctx, in)
	}

	iterations := 0
	finish := func(outcome string, err error) error {
		if h := a.opts.hooks.OnRunEnd; h != nil {
			h(ctx, RunInfo{
				Iterations: iterations,
				Duration:   time.Since(start),
				Outcome:    outcome,
				Err:        err,
			})
		}
		return err
	}

	// 1. As tools de backend são fixas; as de frontend entram só como schema,
	//    ligadas por requisição. As duas listas vão juntas para o modelo, que
	//    não distingue entre elas — quem separa é o laço, mais abaixo.
	clientNames := ClientToolNames(in.Tools)
	cm, err := a.bindTools(in, logger)
	if err != nil {
		emit.RunError("failed to bind tools to the model")
		return finish("error", err)
	}

	// 2. Estado do run. O padrão devolve o que veio, para que o `agent.state`
	//    do cliente não regrida ao passar por aqui.
	state, err := a.opts.stateFactory(ctx, in)
	if err != nil {
		emit.RunError("failed to initialise the agent state")
		return finish("error", fmt.Errorf("construindo o estado do run: %w", err))
	}
	state.bind(emit)

	conv := FromRunInput(in.Messages)

	rc := &RunContext{
		emitter: emit,
		state:   state,
		input:   in,
		logger:  logger,
	}
	ctx = rc.attach(ctx)

	// 3. O contexto da requisição vira um bloco de sistema, como fazia o
	//    CopilotKitMiddleware do Python. É por aqui que middlewares do runtime
	//    entregam informação que o modelo não teria de outra forma.
	appContext := a.opts.ctxFormatter(in.Context)

	// 4. Retomada: se o cliente respondeu a um interrupt, as tools pausadas no
	//    run anterior rodam ANTES do modelo — o histórico que ele reenviou tem
	//    a tool call sem resposta, e é essa lacuna que fechamos aqui.
	if pausadas := pendingResumeCalls(conv, in.Resume); len(pausadas) > 0 {
		emit.StepStarted(a.opts.stepTools)
		its := a.settleToolCalls(ctx, rc, emit, conv, pausadas, clientNames, logger)
		emit.StepFinished(a.opts.stepTools)
		if len(its) > 0 {
			// Interrompeu de novo (outra tool do mesmo turno, ou a mesma
			// pedindo mais um dado).
			return finish("interrupt", a.finishInterrupt(emit, conv, state, its))
		}
	}

	for iterations = 1; iterations <= a.opts.maxIterations; iterations++ {
		// errors.Join cobre os dois jeitos de o cliente sumir: o contexto
		// cancelado (desconexão detectada pelo servidor HTTP) e a escrita que
		// falhou (detectada pelo Emitter). Um deles pode estar nil.
		if err := errors.Join(ctx.Err(), emit.Err()); err != nil {
			return finish("error", err) // cliente desconectou
		}

		turnStart := time.Now()
		emit.StepStarted(a.opts.stepModel)
		msgs := conv.ModelInput(a.opts.systemPrompt, appContext)
		if a.opts.preprocessor != nil {
			msgs = a.opts.preprocessor(ctx, msgs)
		}
		turn, err := streamTurn(ctx, emit, cm, msgs)
		emit.StepFinished(a.opts.stepModel)
		if err != nil {
			// Contexto cancelado ou emitter já bloqueado significam cliente ido
			// ou processo desligando — normal num servidor SSE. Não vale ERROR,
			// e o RUN_ERROR não chegaria a lugar nenhum de qualquer forma.
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) || emit.Err() != nil {
				logger.Info("run abortado (cliente saiu ou servidor desligando)")
				return finish("error", err)
			}
			logger.Error("turno do modelo falhou", "error", err)
			emit.RunError("the agent failed to generate a response")
			return finish("error", err)
		}

		assistant := turn.message
		conv.AppendWithID(assistant, turn.msgID)
		if h := a.opts.hooks.OnModelTurn; h != nil {
			h(ctx, TurnInfo{
				Iteration: iterations,
				Duration:  time.Since(turnStart),
				ToolCalls: len(assistant.ToolCalls),
				Usage:     turn.usage,
			})
		}

		if len(assistant.ToolCalls) == 0 {
			return finish("success", a.finishSuccess(emit, conv, state)) // resposta final
		}

		actionable := validateToolCalls(logger, assistant, conv)
		if len(actionable) == 0 {
			continue // só chamadas malformadas neste turno; o modelo se corrige
		}

		// 5. O ponto crítico: separar o que o servidor pode executar. O que é do
		//    cliente devolve o controle; o resto é uma tool de backend ou um
		//    nome alucinado pelo modelo.
		serverCalls, clientCalls := ClassifyToolCalls(actionable, clientNames)

		if len(clientCalls) > 0 {
			// Devolve o controle: o cliente executa e reenvia o histórico
			// completo num run novo.
			//
			// As chamadas de servidor deste turno não serão executadas, então
			// respondemos cada uma com um resultado sintético; senão o
			// MESSAGES_SNAPSHOT carregaria uma tool call sem resposta, e
			// validadores de sequência rejeitam isso.
			for _, sc := range serverCalls {
				const result = `{"error":"not executed: a client tool in this turn took priority"}`
				conv.Append(schema.ToolMessage(result, sc.ID))
			}
			return finish("success", a.finishSuccess(emit, conv, state))
		}

		emit.StepStarted(a.opts.stepTools)
		its := a.settleToolCalls(ctx, rc, emit, conv, serverCalls, clientNames, logger)
		emit.StepFinished(a.opts.stepTools)
		if len(its) > 0 {
			return finish("interrupt", a.finishInterrupt(emit, conv, state, its))
		}
	}

	iterations = a.opts.maxIterations
	logger.Warn("agente não convergiu dentro do orçamento de iterações",
		"maxIterations", a.opts.maxIterations)
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	err = fmt.Errorf("o agente não convergiu em %d iterações", a.opts.maxIterations)
	emit.RunError(fmt.Sprintf("agent did not converge within %d iterations", a.opts.maxIterations))
	return finish("error", err)
}

// bindTools liga ao modelo as tools de backend mais as de frontend da
// requisição.
func (a *Agent) bindTools(in *RunAgentInput, logger *slog.Logger) (model.ToolCallingChatModel, error) {
	infos := a.infos
	if len(in.Tools) > 0 {
		clientInfos := clientToolInfos(in.Tools, logger)
		// Concatena numa fatia nova: `append` direto em a.infos poderia
		// escrever na capacidade sobrando do slice compartilhado, e uma
		// requisição vazaria as tools de frontend na outra.
		infos = make([]*schema.ToolInfo, 0, len(a.infos)+len(clientInfos))
		infos = append(infos, a.infos...)
		infos = append(infos, clientInfos...)
	}
	if len(infos) == 0 {
		return a.base, nil
	}
	return a.base.WithTools(infos)
}

// settleToolCalls executa as chamadas de servidor de um turno, emitindo o
// resultado de cada uma. Devolve os interrupts pedidos pelas tools; lista vazia
// significa que o laço pode seguir.
func (a *Agent) settleToolCalls(
	ctx context.Context,
	rc *RunContext,
	emit *Emitter,
	conv *Conversation,
	calls []schema.ToolCall,
	clientNames map[string]bool,
	logger *slog.Logger,
) []InterruptPoint {
	var its []InterruptPoint
	for _, sc := range calls {
		// Uma tool que já foi cancelada pelo humano não roda: responde-se a
		// chamada com a recusa, e o modelo segue a partir dela.
		if cancelada(rc.Resume(), sc.ID) {
			const result = `{"status":"cancelled","note":"the user declined this action"}`
			emit.ToolResult(events.GenerateMessageID(), sc.ID, result)
			conv.Append(schema.ToolMessage(result, sc.ID))
			continue
		}

		started := time.Now()
		result, ie, toolErr := a.runServerTool(ctx, rc, sc, clientNames, logger)

		if h := a.opts.hooks.OnToolResult; h != nil {
			h(ctx, ToolRunInfo{
				Name:        sc.Function.Name,
				ToolCallID:  sc.ID,
				Args:        sc.Function.Arguments,
				Result:      result,
				Duration:    time.Since(started),
				Err:         toolErr,
				Interrupted: ie != nil,
			})
		}

		if ie != nil {
			// A chamada fica SEM resposta de propósito: é ela que o cliente
			// precisa ver pendente para renderizar a decisão, e é ela que a
			// retomada vai fechar.
			its = append(its, ie.toAGUI(sc.ID))
			continue
		}

		// O TOOL_CALL_* já saiu no streaming do turno; aqui vai só o resultado,
		// que é o que o cliente renderiza como "backend tool".
		emit.ToolResult(events.GenerateMessageID(), sc.ID, result)
		conv.Append(schema.ToolMessage(result, sc.ID))
	}
	return its
}

// runServerTool executa uma tool de backend e devolve o conteúdo do
// TOOL_CALL_RESULT, ou o interrupt que ela pediu. O terceiro retorno é o erro
// da execução, quando houve — ele já está representado no resultado JSON e
// existe apenas para o hook OnToolResult observar a falha original.
//
// Fora do interrupt, nada aqui aborta o run. Uma tool desconhecida, uma que
// falha ou um argumento inválido viram um resultado JSON de erro: o modelo lê a
// resposta no turno seguinte e se corrige, que é como o LangChain trata a
// exceção de uma tool. Derrubar o run deixaria o usuário sem resposta por causa
// de um nome errado.
func (a *Agent) runServerTool(
	ctx context.Context,
	rc *RunContext,
	tc schema.ToolCall,
	clientNames map[string]bool,
	logger *slog.Logger,
) (string, *InterruptError, error) {
	name := tc.Function.Name
	t, ok := a.invokable[name]
	if !ok {
		logger.Warn("o modelo chamou uma tool inexistente", "tool", name)
		return fmt.Sprintf(
			`{"error":"unknown tool %q: callable tools are %s"}`,
			name, jsonList(a.toolNames(clientNames))), nil, nil
	}

	// O RunContext desta chamada: é assim que a tool alcança o emitter, o
	// estado e — sobretudo — o id da chamada, que é a chave da retomada.
	callCtx := rc.forToolCall(tc.ID, name).attach(ctx)

	out, err := t.InvokableRun(callCtx, tc.Function.Arguments)
	if err != nil {
		if ie, ok := asInterrupt(err); ok {
			logger.Info("tool pediu uma decisão humana", "tool", name, "toolCallId", tc.ID)
			return "", ie, nil
		}
		logger.Error("tool de backend falhou", "tool", name, "error", err)
		msg, _ := json.Marshal(err.Error())
		return fmt.Sprintf(`{"error":"tool %q failed","detail":%s}`, name, msg), nil, err
	}
	if out == "" {
		// O emitter substituiria por "(empty)"; um JSON vazio é mais legível
		// para o modelo do que um marcador.
		return "[]", nil, nil
	}
	return out, nil, nil
}

// toolNames lista tudo que o modelo poderia ter chamado neste run — as tools de
// backend mais as de frontend enviadas na requisição.
func (a *Agent) toolNames(clientNames map[string]bool) []string {
	names := make([]string, 0, len(a.infos)+len(clientNames))
	for _, info := range a.infos {
		names = append(names, info.Name)
	}
	for name := range clientNames {
		names = append(names, name)
	}
	sort.Strings(names) // a iteração de um map é aleatória; a mensagem seria instável
	return names
}

func jsonList(names []string) string {
	b, err := json.Marshal(names)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// finishSuccess emite a cauda do run na mesma ordem do adaptador Python:
// STATE_SNAPSHOT, depois MESSAGES_SNAPSHOT, depois RUN_FINISHED
// (get_state_and_messages_snapshots, em ag_ui_langgraph/agent.py).
func (a *Agent) finishSuccess(emit *Emitter, conv *Conversation, state *State) error {
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	emit.RunFinished()
	return nil
}

// finishInterrupt encerra o run pedindo uma decisão. O MESSAGES_SNAPSHOT sai
// com a tool call pendente e SEM resposta — é ela que o cliente renderiza como
// a pergunta, e é ela que a retomada fecha.
func (a *Agent) finishInterrupt(emit *Emitter, conv *Conversation, state *State, its []InterruptPoint) error {
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	emit.RunFinishedWithInterrupts(its)
	return nil
}

// pendingResumeCalls devolve as tool calls que ficaram sem resposta no run
// anterior e para as quais o cliente mandou uma decisão agora.
//
// Não há store no servidor, e isso é deliberado: o AG-UI já reenvia o histórico
// inteiro a cada requisição, então a chamada pendente CHEGA na conversa — basta
// encontrá-la. Um store em memória não acrescentaria nada e passaria a estar
// errado assim que houvesse mais de uma réplica.
func pendingResumeCalls(conv *Conversation, resume []ResumeEntry) []schema.ToolCall {
	if len(resume) == 0 {
		return nil
	}
	decided := make(map[string]bool, len(resume))
	for _, e := range resume {
		if e.InterruptID != "" {
			decided[e.InterruptID] = true
		}
	}

	msgs := conv.Messages()
	answered := make(map[string]bool, len(msgs))
	for _, m := range msgs {
		if m.Role == schema.Tool && m.ToolCallID != "" {
			answered[m.ToolCallID] = true
		}
	}

	var out []schema.ToolCall
	for _, m := range msgs {
		if m.Role != schema.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID != "" && decided[tc.ID] && !answered[tc.ID] {
				out = append(out, tc)
			}
		}
	}
	return out
}

// cancelada informa se o humano recusou a chamada.
func cancelada(resume []ResumeEntry, toolCallID string) bool {
	for _, e := range resume {
		if e.InterruptID == toolCallID && e.Status == ResumeStatusCancelled {
			return true
		}
	}
	return false
}

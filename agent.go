package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// Agent is an AG-UI agent: the model↔tools loop, the event streaming and the
// rules that decide who executes each tool.
//
// It is safe for concurrent use and is not mutated per request: the frontend
// tools are bound to the model with WithTools, which returns a new instance.
//
// PT: Agent é um agente AG-UI: o laço modelo↔tools, o streaming dos eventos e
// as regras que decidem quem executa cada tool.
//
// É seguro para uso concorrente e não é mutado por requisição: as tools de
// frontend são ligadas ao modelo com WithTools, que devolve uma instância nova.
type Agent struct {
	base model.ToolCallingChatModel
	opts *options

	// invokable are the tools the server executes, by name.
	//
	// PT: invokable são as tools que o servidor executa, por nome.
	invokable map[string]tool.InvokableTool
	// infos are the schemas announced to the model, in registration order.
	//
	// PT: infos são os schemas anunciados ao modelo, na ordem de registro.
	infos []*schema.ToolInfo
	// clientOnly are the tools registered with [NewClientTool]: announced by
	// the server, executed by the client.
	//
	// PT: clientOnly são as tools registradas com [NewClientTool]: anunciadas
	// pelo servidor, executadas pelo cliente.
	clientOnly map[string]bool
}

// New builds an agent on top of an Eino chat model.
//
// It takes ctx because the tool schemas are resolved here — and `Info` may do
// network I/O, as an MCP tool does. Resolving at construction, and not on every
// request, is what allows failing at process start-up instead of in the middle
// of a conversation.
//
// Tool construction errors are aggregated: an invalid struct tag in two
// different tools shows up in both, at once, with the name of each one.
//
// PT: New constrói um agente sobre um chat model do Eino.
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
		return nil, errors.New("agui: the chat model must not be nil")
	}

	cfg := defaultOptions()
	for _, o := range opts {
		o(cfg)
	}
	if cfg.maxIterations < 1 || cfg.maxIterations > MaxIterationsCeiling {
		return nil, fmt.Errorf(
			"agui: WithMaxIterations(%d) outside the accepted range [1, %d]",
			cfg.maxIterations, MaxIterationsCeiling)
	}

	a := &Agent{
		base:       m,
		opts:       cfg,
		invokable:  make(map[string]tool.InvokableTool, len(cfg.tools)),
		infos:      make([]*schema.ToolInfo, 0, len(cfg.tools)),
		clientOnly: make(map[string]bool),
	}

	var errs []error
	seen := make(map[string]bool, len(cfg.tools))
	for _, t := range cfg.tools {
		info, err := t.Info(ctx)
		if err != nil {
			// brokenTool already wraps the message with the tool name.
			//
			// PT: brokenTool já embrulha a mensagem com o nome da tool.
			errs = append(errs, err)
			continue
		}
		if info == nil || info.Name == "" {
			errs = append(errs, errors.New("a tool returned a ToolInfo without a name"))
			continue
		}
		if seen[info.Name] {
			errs = append(errs, fmt.Errorf("tool %q registered twice", info.Name))
			continue
		}
		seen[info.Name] = true
		a.infos = append(a.infos, info)
		if inv, ok := t.(tool.InvokableTool); ok {
			a.invokable[info.Name] = inv
		} else {
			a.clientOnly[info.Name] = true
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("agui: %w", errors.Join(errs...))
	}
	return a, nil
}

// Run executes a complete AG-UI run, from the model's first turn to the
// terminal event.
//
// RUN_STARTED has already been emitted by [Handler]; do not repeat it. The
// returned error is for the transport to log — the corresponding terminal
// event has already been emitted here, except when the run was aborted by a
// cancelled context or a dead client, in which case [Handler] emits RUN_ERROR
// if the stream is still open.
//
// PT: Run executa um run AG-UI completo, do primeiro turno do modelo ao evento
// terminal.
//
// RUN_STARTED já foi emitido por [Handler]; não o repita. O erro devolvido é
// para o transporte registrar — o evento terminal correspondente já foi emitido
// aqui, exceto quando o run foi abortado por contexto cancelado ou cliente
// desconectado, caso em que [Handler] emite RUN_ERROR se o stream ainda estiver
// aberto.
func (a *Agent) Run(ctx context.Context, emit *Emitter, in *RunAgentInput) error {
	start := time.Now()
	logger := a.opts.logger.With("thread", emit.ThreadID(), "run", emit.RunID())

	if h := a.opts.hooks.OnRunStart; h != nil {
		h(ctx, in)
	}

	iterations := 0
	var usage *TokenUsage
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

	// 1. The backend tools are fixed; the frontend ones come in only as schema,
	//    bound per request. Both lists go together to the model, which does not
	//    distinguish between them — the loop, further below, is what separates
	//    them.
	//
	// PT: 1. As tools de backend são fixas; as de frontend entram só como
	//    schema, ligadas por requisição. As duas listas vão juntas para o
	//    modelo, que não distingue entre elas — quem separa é o laço, mais
	//    abaixo.
	clientNames := ClientToolNames(in.Tools)
	for name := range a.clientOnly {
		clientNames[name] = true
	}
	cm, err := a.bindTools(in, logger)
	if err != nil {
		emit.RunError("failed to bind tools to the model")
		return finish("error", err)
	}

	// 2. Run state. The default returns what came in, so that the client's
	//    `agent.state` does not regress by passing through here.
	//
	// PT: 2. Estado do run. O padrão devolve o que veio, para que o
	//    `agent.state` do cliente não regrida ao passar por aqui.
	state, err := a.opts.stateFactory(ctx, in)
	if err != nil {
		emit.RunError("failed to initialise the agent state")
		return finish("error", fmt.Errorf("building the run state: %w", err))
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

	// 3. The request context becomes a system block, as the Python
	//    CopilotKitMiddleware did. This is how runtime middlewares deliver
	//    information the model would not have otherwise.
	//
	// PT: 3. O contexto da requisição vira um bloco de sistema, como fazia o
	//    CopilotKitMiddleware do Python. É por aqui que middlewares do runtime
	//    entregam informação que o modelo não teria de outra forma.
	appContext := a.opts.ctxFormatter(in.Context)

	// 4. Resume: if the client answered an interrupt, the tools paused in the
	//    previous run execute BEFORE the model — the history it resent has the
	//    tool call without an answer, and that is the gap we close here.
	//
	// PT: 4. Retomada: se o cliente respondeu a um interrupt, as tools paused
	//    no run anterior rodam ANTES do modelo — o histórico que ele reenviou
	//    tem a tool call sem resposta, e é essa lacuna que fechamos aqui.
	paused, uncovered := a.pendingResumeCalls(conv, in.Resume, logger)
	if len(paused) > 0 || len(uncovered) > 0 {
		var its []InterruptPoint
		if len(paused) > 0 {
			emit.StepStarted(a.opts.stepTools)
			its = a.settleToolCalls(ctx, rc, emit, conv, paused, clientNames, logger)
			emit.StepFinished(a.opts.stepTools)
		}
		// AG-UI 1.0: a call paused in the previous run that the resume does
		// not cover stays open — omission is not abandonment, and the action
		// must not run on the strength of an absent entry. The run ends with
		// the interrupt outcome again instead of continuing past it.
		//
		// PT: AG-UI 1.0: uma chamada pausada no run anterior que o resume não
		// cobre continua aberta — omissão não é abandono, e a ação não pode
		// rodar com base numa entrada ausente. O run termina de novo com o
		// outcome de interrupt em vez de passar por cima dela.
		for _, tc := range uncovered {
			its = append(its, stillOpenInterrupt(tc))
		}
		if len(its) > 0 {
			// Interrupted again (another tool from the same turn, the same
			// one asking for one more piece of data, or an uncovered call).
			//
			// PT: Interrompeu de novo (outra tool do mesmo turno, a mesma
			// pedindo mais um dado, ou uma chamada não coberta).
			return finish("interrupt", a.finishInterrupt(emit, conv, state, its))
		}
	}

	callIDs := usedToolCallIDs(conv)
	for iterations = 1; iterations <= a.opts.maxIterations; iterations++ {
		// errors.Join covers the two ways the client can vanish: the cancelled
		// context (disconnection detected by the HTTP server) and the failed
		// write (detected by the Emitter). Either one may be nil.
		//
		// PT: errors.Join cobre os dois jeitos de o cliente sumir: o contexto
		// cancelado (desconexão detectada pelo servidor HTTP) e a escrita que
		// falhou (detectada pelo Emitter). Um deles pode estar nil.
		if err := errors.Join(ctx.Err(), emit.Err()); err != nil {
			return finish("error", err) // client disconnected (PT: cliente desconectou)
		}

		turnStart := time.Now()
		emit.StepStarted(a.opts.stepModel)
		msgs := conv.ModelInput(a.opts.systemPrompt, appContext)
		if a.opts.preprocessor != nil {
			msgs = a.opts.preprocessor(ctx, msgs)
		}
		turn, err := streamTurn(ctx, emit, cm, msgs, callIDs)
		emit.StepFinished(a.opts.stepModel)
		if err != nil {
			// A cancelled context or an already-blocked emitter means the
			// client is gone or the process is shutting down — normal for an
			// SSE server. It is not worth ERROR, and the RUN_ERROR would not
			// reach anywhere anyway.
			//
			// PT: Contexto cancelado ou emitter já bloqueado significam cliente
			// ido ou processo desligando — normal num servidor SSE. Não vale
			// ERROR, e o RUN_ERROR não chegaria a lugar nenhum de qualquer
			// forma.
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) || emit.Err() != nil {
				logger.Info("run aborted (client left or server shutting down)")
				return finish("error", err)
			}
			logger.Error("model turn failed", "error", err)
			emit.RunError("the agent failed to generate a response")
			return finish("error", err)
		}

		assistant := turn.message
		conv.AppendWithID(assistant, turn.msgID)
		if turn.usage != nil {
			usage = addUsage(usage, turn.usage)
			emit.SetUsage(*usage)
		}
		if h := a.opts.hooks.OnModelTurn; h != nil {
			h(ctx, TurnInfo{
				Iteration: iterations,
				Duration:  time.Since(turnStart),
				ToolCalls: len(assistant.ToolCalls),
				Usage:     turn.usage,
			})
		}

		if len(assistant.ToolCalls) == 0 {
			return finish("success", a.finishSuccess(emit, conv, state)) // final answer (PT: resposta final)
		}

		actionable := validateToolCalls(logger, emit, assistant, conv)
		if len(actionable) == 0 {
			continue // only malformed calls this turn; the model corrects itself (PT: só chamadas malformadas neste turno; o modelo se corrige)
		}

		// 5. The critical point: separating what the server can execute. What
		//    belongs to the client hands control back; the rest is a backend
		//    tool or a name hallucinated by the model.
		//
		// PT: 5. O ponto crítico: separar o que o servidor pode executar. O que
		//    é do cliente devolve o controle; o resto é uma tool de backend ou
		//    um nome alucinado pelo modelo.
		serverCalls, clientCalls := ClassifyToolCalls(actionable, clientNames)

		if len(clientCalls) > 0 {
			// Hand control back: the client executes and resends the complete
			// history in a new run.
			//
			// The server calls of this turn will not be executed, so we answer
			// each one with a synthetic result; otherwise the MESSAGES_SNAPSHOT
			// would carry a tool call without an answer, and the run's
			// pendingToolCallIds — which AG-UI 1.0 requires to name exactly
			// the calls left unanswered — would include them.
			//
			// PT: Devolve o controle: o cliente executa e reenvia o histórico
			// completo num run novo.
			//
			// As chamadas de servidor deste turno não serão executadas, então
			// respondemos cada uma com um resultado sintético; senão o
			// MESSAGES_SNAPSHOT carregaria uma tool call sem resposta, e o
			// pendingToolCallIds do run — que o AG-UI 1.0 exige que nomeie
			// exatamente as chamadas sem resposta — as incluiria.
			for _, sc := range serverCalls {
				const result = `{"error":"not executed: a client tool in this turn took priority"}`
				emit.ToolResult(uuid.NewString(), sc.ID, result)
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
	logger.Warn("agent did not converge within the iteration budget",
		"maxIterations", a.opts.maxIterations)
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	err = fmt.Errorf("the agent did not converge in %d iterations", a.opts.maxIterations)
	emit.RunError(fmt.Sprintf("agent did not converge within %d iterations", a.opts.maxIterations))
	return finish("error", err)
}

// bindTools binds to the model the backend tools plus the request's frontend
// tools.
//
// PT: bindTools liga ao modelo as tools de backend mais as de frontend da
// requisição.
func (a *Agent) bindTools(in *RunAgentInput, logger *slog.Logger) (model.ToolCallingChatModel, error) {
	infos := a.infos
	if len(in.Tools) > 0 {
		clientInfos := clientToolInfos(in.Tools, logger)
		// Concatenate into a new slice: a direct `append` on a.infos could
		// write into the spare capacity of the shared slice, and one request
		// would leak its frontend tools into another.
		//
		// PT: Concatena numa fatia nova: `append` direto em a.infos poderia
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

// settleToolCalls executes a turn's server calls, emitting the result of each
// one. It returns the interrupts requested by the tools; an empty list means
// the loop may continue.
//
// PT: settleToolCalls executa as chamadas de servidor de um turno, emitindo o
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
		// A tool that the human has already cancelled does not run: the call
		// is answered with the refusal, and the model continues from it.
		//
		// PT: Uma tool que já foi cancelada pelo humano não roda: responde-se a
		// chamada com a recusa, e o modelo segue a partir dela.
		if isCancelled(rc.Resume(), sc.ID) {
			const result = `{"status":"cancelled","note":"the user declined this action"}`
			emit.ToolResult(uuid.NewString(), sc.ID, result)
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
			// The call is left WITHOUT an answer on purpose: it is what the
			// client needs to see pending in order to render the decision, and
			// it is what the resume will close.
			//
			// PT: A chamada fica SEM resposta de propósito: é ela que o cliente
			// precisa ver pendente para renderizar a decisão, e é ela que a
			// retomada vai fechar.
			its = append(its, ie.toAGUI(sc.ID))
			continue
		}

		// The TOOL_CALL_* already went out in the turn's streaming; here goes
		// only the result, which is what the client renders as "backend tool".
		//
		// PT: O TOOL_CALL_* já saiu no streaming do turno; aqui vai só o
		// resultado, que é o que o cliente renderiza como "backend tool".
		emit.ToolResult(uuid.NewString(), sc.ID, result)
		conv.Append(schema.ToolMessage(result, sc.ID))
	}
	return its
}

// runServerTool executes a backend tool and returns the content of the
// TOOL_CALL_RESULT, or the interrupt it requested. The third return value is
// the execution error, when there was one — it is already represented in the
// JSON result and exists only so the OnToolResult hook can observe the
// original failure.
//
// Apart from the interrupt, nothing here aborts the run. An unknown tool, one
// that fails or an invalid argument become a JSON error result: the model reads
// the answer on the next turn and corrects itself, which is how LangChain
// handles a tool's exception. Taking the run down would leave the user without
// an answer because of a wrong name.
//
// PT: runServerTool executa uma tool de backend e devolve o conteúdo do
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
		logger.Warn("the model called a nonexistent tool", "tool", name)
		msg, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(
			"unknown tool %q: callable tools are %s",
			name, jsonList(a.toolNames(clientNames)))})
		return string(msg), nil, nil
	}

	// The RunContext of this call: this is how the tool reaches the emitter,
	// the state and — above all — the call id, which is the key of the resume.
	//
	// PT: O RunContext desta chamada: é assim que a tool alcança o emitter, o
	// estado e — sobretudo — o id da chamada, que é a chave da retomada.
	callCtx := rc.forToolCall(tc.ID, name).attach(ctx)

	out, err := t.InvokableRun(callCtx, tc.Function.Arguments)
	if err != nil {
		if ie, ok := asInterrupt(err); ok {
			logger.Info("tool requested a human decision", "tool", name, "toolCallId", tc.ID)
			return "", ie, nil
		}
		logger.Error("backend tool failed", "tool", name, "error", err)
		msg, _ := json.Marshal(err.Error())
		return fmt.Sprintf(`{"error":"tool %q failed","detail":%s}`, name, msg), nil, err
	}
	if out == "" {
		// The emitter would replace it with "(empty)"; an empty JSON is more
		// readable for the model than a marker.
		//
		// PT: O emitter substituiria por "(empty)"; um JSON vazio é mais
		// legível para o modelo do que um marcador.
		return "[]", nil, nil
	}
	return out, nil, nil
}

// toolNames lists everything the model could have called in this run — the
// backend tools plus the frontend ones sent in the request.
//
// PT: toolNames lista tudo que o modelo poderia ter chamado neste run — as
// tools de backend mais as de frontend enviadas na requisição.
func (a *Agent) toolNames(clientNames map[string]bool) []string {
	names := make([]string, 0, len(a.infos)+len(clientNames))
	for _, info := range a.infos {
		names = append(names, info.Name)
	}
	for name := range clientNames {
		if !a.clientOnly[name] { // already listed via infos (PT: já listada via infos)
			names = append(names, name)
		}
	}
	sort.Strings(names) // map iteration is random; the message would be unstable (PT: a iteração de um map é aleatória; a mensagem seria instável)
	return names
}

func jsonList(names []string) string {
	b, err := json.Marshal(names)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// finishSuccess emits the tail of the run in the same order as the Python
// adapter: STATE_SNAPSHOT, then MESSAGES_SNAPSHOT, then RUN_FINISHED
// (get_state_and_messages_snapshots, in ag_ui_langgraph/agent.py).
//
// When the run hands frontend tool calls to the client, RUN_FINISHED carries
// the success outcome with pendingToolCallIds: AG-UI 1.0 says a consumer must
// not read the absence of that list as "nothing pending".
//
// PT: finishSuccess emite a cauda do run na mesma ordem do adaptador Python:
// STATE_SNAPSHOT, depois MESSAGES_SNAPSHOT, depois RUN_FINISHED
// (get_state_and_messages_snapshots, em ag_ui_langgraph/agent.py).
//
// Quando o run entrega tool calls de frontend ao cliente, o RUN_FINISHED leva
// o outcome de sucesso com pendingToolCallIds: o AG-UI 1.0 diz que um
// consumidor não deve ler a ausência dessa lista como "nada pendente".
func (a *Agent) finishSuccess(emit *Emitter, conv *Conversation, state *State) error {
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	if pending := emit.PendingToolCalls(); len(pending) > 0 {
		emit.RunFinishedPending(pending...)
		return nil
	}
	emit.RunFinished()
	return nil
}

// finishInterrupt ends the run asking for a decision. The MESSAGES_SNAPSHOT
// goes out with the pending tool call and WITHOUT an answer — it is what the
// client renders as the question, and it is what the resume closes.
//
// PT: finishInterrupt encerra o run pedindo uma decisão. O MESSAGES_SNAPSHOT
// sai com a tool call pendente e SEM resposta — é ela que o cliente renderiza
// como a pergunta, e é ela que a retomada fecha.
func (a *Agent) finishInterrupt(emit *Emitter, conv *Conversation, state *State, its []InterruptPoint) error {
	emit.StateSnapshot(state.Snapshot())
	emit.MessagesSnapshot(conv.Snapshot())
	emit.RunFinishedWithInterrupts(its)
	return nil
}

// pendingResumeCalls returns the tool calls that were left without an answer
// in the previous run and for which the client has now sent a decision
// (paused), and the unanswered calls to this agent's server tools that the
// resume does not cover (uncovered). Resume entries that name no open call
// are logged and ignored, as AG-UI 1.0 asks of a producer.
//
// There is no store on the server, and that is deliberate: AG-UI already
// resends the whole history on every request, so the pending call ARRIVES in
// the conversation — it only has to be found. An in-memory store would add
// nothing and would become wrong as soon as there was more than one replica.
//
// PT: pendingResumeCalls devolve as tool calls que ficaram sem resposta no run
// anterior e para as quais o cliente mandou uma decisão agora (paused), e as
// chamadas sem resposta às tools de servidor deste agente que o resume não
// cobre (uncovered). Entradas do resume que não nomeiam chamada aberta são
// registradas em log e ignoradas, como o AG-UI 1.0 pede a um produtor.
//
// Não há store no servidor, e isso é deliberado: o AG-UI já reenvia o histórico
// inteiro a cada requisição, então a chamada pendente CHEGA na conversa — basta
// encontrá-la. Um store em memória não acrescentaria nada e passaria a estar
// errado assim que houvesse mais de uma réplica.
func (a *Agent) pendingResumeCalls(conv *Conversation, resume []ResumeEntry, logger *slog.Logger) (paused, uncovered []schema.ToolCall) {
	if len(resume) == 0 {
		return nil, nil
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

	open := map[string]bool{}
	for _, m := range msgs {
		if m.Role != schema.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID == "" || answered[tc.ID] {
				continue
			}
			switch {
			case decided[tc.ID]:
				open[tc.ID] = true
				paused = append(paused, tc)
			case a.invokable[tc.Function.Name] != nil:
				uncovered = append(uncovered, tc)
			}
		}
	}
	for _, e := range resume {
		if !open[e.InterruptID] {
			logger.Warn("resume entry names no open interrupt; ignoring it",
				"interruptId", e.InterruptID)
		}
	}
	for _, tc := range uncovered {
		logger.Warn("resume does not cover a paused tool call; interrupting again",
			"tool", tc.Function.Name, "toolCallId", tc.ID)
	}
	return paused, uncovered
}

// stillOpenInterrupt re-raises the interrupt of a paused call the resume did
// not cover. The original request is not stored on the server, so the
// interrupt says only that the decision is still missing.
//
// PT: stillOpenInterrupt levanta de novo o interrupt de uma chamada pausada que
// o resume não cobriu. O pedido original não fica guardado no servidor, então o
// interrupt diz apenas que a decisão ainda falta.
func stillOpenInterrupt(tc schema.ToolCall) InterruptPoint {
	return (&InterruptError{Request: InterruptRequest{
		Message: fmt.Sprintf("the call to %q is still awaiting a decision: the resume did not answer it", tc.Function.Name),
	}}).toAGUI(tc.ID)
}

// addUsage accumulates the token usage of one model turn into the run total
// reported on the terminal event.
//
// PT: addUsage acumula o uso de tokens de um turno do modelo no total do run
// informado no evento terminal.
func addUsage(total *TokenUsage, u *schema.TokenUsage) *TokenUsage {
	if total == nil {
		total = &TokenUsage{}
	}
	add := func(dst **int64, n int) {
		if *dst == nil {
			*dst = new(int64)
		}
		**dst += int64(n)
	}
	add(&total.InputTokens, u.PromptTokens)
	add(&total.OutputTokens, u.CompletionTokens)
	add(&total.TotalTokens, u.TotalTokens)
	if n := u.PromptTokenDetails.CachedTokens; n > 0 || total.CachedInputTokens != nil {
		add(&total.CachedInputTokens, n)
	}
	if n := u.CompletionTokensDetails.ReasoningTokens; n > 0 || total.ReasoningTokens != nil {
		add(&total.ReasoningTokens, n)
	}
	return total
}

// isCancelled reports whether the human declined the call.
//
// PT: isCancelled informa se o humano recusou a chamada.
func isCancelled(resume []ResumeEntry, toolCallID string) bool {
	for _, e := range resume {
		if e.InterruptID == toolCallID && e.Status == ResumeStatusCancelled {
			return true
		}
	}
	return false
}

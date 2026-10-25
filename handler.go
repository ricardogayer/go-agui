package agui

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	wire "github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// Runner is everything a transport needs to execute a run.
//
// An *[Agent] is already a Runner. Implement it yourself when you want your own
// loop while keeping the SSE, lifecycle and panic handling of [Handler].
//
// RUN_STARTED has already been emitted when Run is called; do not repeat it.
// Returning an error is enough to end the run: if no terminal event has been
// emitted, the Handler emits RUN_ERROR. Emitting your own terminal event and
// returning nil (or the error, for logging) is also valid — [Emitter.Terminated]
// guarantees that two will not go out.
//
// PT: Runner é tudo o que um transporte precisa para executar um run.
//
// Um *[Agent] já é um Runner. Implemente-o você mesmo quando quiser o próprio
// laço mantendo o SSE, o ciclo de vida e o tratamento de pânico do [Handler].
//
// RUN_STARTED já foi emitido quando Run é chamado; não o repita. Devolver um
// erro é suficiente para encerrar o run: se nenhum evento terminal tiver sido
// emitido, o Handler emite RUN_ERROR. Emitir o próprio terminal e devolver nil
// (ou o erro, para registro) também é válido — [Emitter.Terminated] garante que
// não sairão dois.
type Runner interface {
	Run(ctx context.Context, emit *Emitter, in *RunAgentInput) error
}

// RunnerFunc adapts a function to the [Runner] interface.
//
// PT: RunnerFunc adapta uma função à interface [Runner].
type RunnerFunc func(ctx context.Context, emit *Emitter, in *RunAgentInput) error

// Run implements [Runner].
//
// PT: Run implementa [Runner].
func (f RunnerFunc) Run(ctx context.Context, emit *Emitter, in *RunAgentInput) error {
	return f(ctx, emit, in)
}

type handlerOptions struct {
	logger       *slog.Logger
	shutdownCtx  context.Context
	onDropped    DroppedFunc
	maxBodyBytes int64
	noProtobuf   bool
	rejectMajor  bool
}

// HandlerOption configures the [Handler].
//
// PT: HandlerOption configura o [Handler].
type HandlerOption func(*handlerOptions)

// WithHandlerLogger sets the logger for the handler and the SSE writer.
//
// PT: WithHandlerLogger define o logger do handler e do writer SSE.
func WithHandlerLogger(l *slog.Logger) HandlerOption {
	return func(o *handlerOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithShutdownContext ties the runs to the process shutdown context, so that a
// SIGTERM cancels the in-flight streams instead of killing them midway.
//
// PT: WithShutdownContext liga os runs ao contexto de desligamento do processo,
// para que um SIGTERM cancele os streams em voo em vez de matá-los no meio.
func WithShutdownContext(ctx context.Context) HandlerOption {
	return func(o *handlerOptions) {
		if ctx != nil {
			o.shutdownCtx = ctx
		}
	}
}

// WithOnDropped registers the callback for an event dropped due to an
// encoding/validation failure. The default logs at ERROR level.
//
// PT: WithOnDropped registra o callback de evento descartado por falha de
// codificação/validação. O padrão registra em log no nível ERROR.
func WithOnDropped(f DroppedFunc) HandlerOption {
	return func(o *handlerOptions) { o.onDropped = f }
}

// WithMaxBodyBytes limits the size of the POST body. Above the limit the
// response is 413 and the Runner is not called.
//
// The default is no limit, because AG-UI resends the ENTIRE history on every
// request and the reasonable ceiling depends on the application. On an exposed
// server, set one: without a limit, any client can make the decode allocate
// without bound.
//
// PT: WithMaxBodyBytes limita o tamanho do corpo do POST. Acima do limite a
// resposta é 413 e o Runner não é chamado.
//
// O padrão é sem limite, porque o AG-UI reenvia o histórico INTEIRO a cada
// requisição e o teto razoável depende da aplicação. Num servidor exposto,
// defina um: sem limite, qualquer cliente pode fazer o decode alocar sem teto.
func WithMaxBodyBytes(n int64) HandlerOption {
	return func(o *handlerOptions) {
		if n > 0 {
			o.maxBodyBytes = n
		}
	}
}

// WithoutProtobuf makes the handler always answer with SSE, ignoring a client
// that asks for the HTTP + Protobuf binding. By default the binding is
// negotiated through the Accept header, as AG-UI 1.0 prescribes: Protobuf when
// the client admits application/vnd.ag-ui.event+proto (explicitly or through
// a wildcard), SSE otherwise.
//
// PT: WithoutProtobuf faz o handler responder sempre com SSE, ignorando um
// cliente que peça o binding HTTP + Protobuf. Por padrão o binding é negociado
// pelo cabeçalho Accept, como o AG-UI 1.0 prescreve: Protobuf quando o cliente
// admite application/vnd.ag-ui.event+proto (explicitamente ou por curinga),
// SSE caso contrário.
func WithoutProtobuf() HandlerOption {
	return func(o *handlerOptions) { o.noProtobuf = true }
}

// WithRejectOtherMajorVersions makes the handler answer HTTP 400 to a client
// that declares a protocolVersion of another major version. By default such
// a client is served, with a warning in the log.
//
// PT: WithRejectOtherMajorVersions faz o handler responder HTTP 400 a um
// cliente que declare um protocolVersion de outra versão major. Por padrão
// esse cliente é atendido, com um aviso no log.
func WithRejectOtherMajorVersions() HandlerOption {
	return func(o *handlerOptions) { o.rejectMajor = true }
}

// Handler returns the http.Handler that speaks AG-UI 1.0: it receives a
// RunAgentInput in JSON and responds with an event stream — SSE
// (`text/event-stream`) or, when the client negotiates it through Accept, the
// HTTP + Protobuf binding.
//
// It takes care of the input (read under the AG-UI processing model: unknown
// fields dropped with a warning, a malformed known value refused with HTTP
// 400 before any stream; retired forms converted; protocol version
// negotiated), headers, the run lifecycle (ids, cancellation, panic) and the
// terminal event. Every event is validated against the official 1.0 schema
// and sequence rules before it is written. Everything application-specific
// lives behind the [Runner] interface.
//
// To mount it on a *gin.Engine, use the ginx package — or, without it,
// `r.POST("/", gin.WrapH(agui.Handler(ag)))`.
//
// PT: Handler devolve o http.Handler que fala AG-UI 1.0: recebe um
// RunAgentInput em JSON e responde com um stream de eventos — SSE
// (`text/event-stream`) ou, quando o cliente o negocia pelo Accept, o binding
// HTTP + Protobuf.
//
// Cuida do input (lido sob o modelo de processamento do AG-UI: campos
// desconhecidos descartados com aviso, valor conhecido malformado recusado
// com HTTP 400 antes de qualquer stream; formas aposentadas convertidas;
// versão do protocolo negociada), dos cabeçalhos, do ciclo de vida do run
// (ids, cancelamento, pânico) e do evento terminal. Todo evento é validado
// contra o schema oficial 1.0 e as regras de sequência antes de ser escrito.
// Tudo o que é da aplicação fica atrás da interface [Runner].
//
// Para montar num *gin.Engine, use o pacote ginx — ou, sem ele,
// `r.POST("/", gin.WrapH(agui.Handler(ag)))`.
func Handler(r Runner, opts ...HandlerOption) http.Handler {
	cfg := &handlerOptions{logger: slog.Default(), shutdownCtx: context.Background()}
	for _, o := range opts {
		o(cfg)
	}
	maxBody := cfg.maxBodyBytes
	if maxBody <= 0 {
		maxBody = 1 << 62 // no limit (PT: sem limite)
	}
	reader := server.NewAgentHandler(nil, server.Options{
		MaxBodyBytes:             maxBody,
		RejectOtherMajorVersions: cfg.rejectMajor,
	})

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		in, warnings, err := reader.ReadInput(req)
		for _, warn := range warnings {
			cfg.logger.Warn("run input", "warning", warn.String())
		}
		if err != nil {
			writeRequestError(w, err, cfg.logger)
			return
		}

		// threadId and runId are required by the schema but may arrive empty.
		// PT: threadId e runId são exigidos pelo schema mas podem chegar vazios.
		threadID := in.ThreadID
		if threadID == "" {
			threadID = uuid.NewString()
		}
		runID := in.RunID
		if runID == "" {
			runID = uuid.NewString()
		}
		logger := cfg.logger.With("thread", threadID, "run", runID)

		// The server's WriteTimeout would cut a long stream midway: clear it.
		// PT: O WriteTimeout do servidor cortaria um stream longo no meio: zere-o.
		rc := http.NewResponseController(w)
		if err := rc.SetWriteDeadline(time.Time{}); err != nil &&
			!errors.Is(err, http.ErrNotSupported) {
			logger.Warn("could not clear the stream write deadline", "error", err)
		}

		var out *stream.SSE
		if cfg.noProtobuf {
			out = stream.New(w, stream.ForThread(threadID))
		} else {
			out = stream.Negotiate(w, req, stream.ForThread(threadID))
		}
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		_ = rc.Flush() // commits the headers before the first event (PT: compromete os cabeçalhos antes do primeiro evento)

		reqCtx, cancelReq := mergeCancel(req.Context(), cfg.shutdownCtx)
		defer cancelReq()
		runCtx, cancel := context.WithCancel(reqCtx)
		defer cancel()

		onDrop := cfg.onDropped
		if onDrop == nil {
			onDrop = func(_ context.Context, ev Event, err error) {
				logger.Error("AG-UI event dropped",
					"type", ev.Type(), "error", err)
			}
		}
		emit := NewEmitter(w,
			WithEmitterContext(runCtx),
			WithEmitterIDs(threadID, runID),
			WithEmitterParentRunID(wire.Deref(in.ParentRunID)),
			WithEmitterCancel(cancel),
			WithEmitterStream(out),
			WithEmitterDropped(onDrop),
		)

		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("handler panicked",
					"panic", rec, "stack", string(debug.Stack()))
				emit.RunError("the agent crashed while handling this run")
			}
		}()

		emit.RunStarted()
		err = r.Run(runCtx, emit, in)

		if !emit.Terminated() {
			if err != nil {
				logger.Error("runner failed without emitting a terminal event", "error", err)
				emit.RunError("the agent failed to complete this run")
			} else {
				logger.Warn("runner finished without emitting a terminal event; emitting RUN_FINISHED")
				emit.RunFinished()
			}
		} else if err != nil {
			logger.Warn("run finished with error", "error", err)
		}
		if e := emit.Err(); e != nil {
			logger.Info("stream closed before the end", "error", e)
		}
	})
}

// writeRequestError answers a request refused before the stream: 400 for a
// malformed input, 405/413/415 for the transport-level refusals.
//
// PT: writeRequestError responde uma requisição recusada antes do stream: 400
// para um input malformado, 405/413/415 para as recusas de transporte.
func writeRequestError(w http.ResponseWriter, err error, logger *slog.Logger) {
	status, resp := http.StatusBadRequest, server.ErrorResponse{Error: "invalid request body"}
	var re *server.RequestError
	if errors.As(err, &re) {
		status, resp.Error = re.Status, re.Message
		if re.Err != nil {
			resp.Details = re.Err.Error()
		}
	}
	logger.Warn("request refused before the stream", "status", status, "error", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// Health returns the /health handler, with the same body as the Python adapter.
//
// PT: Health devolve o handler de /health, com o mesmo corpo do adaptador Python.
func Health(agentName string) http.Handler {
	body, _ := json.Marshal(map[string]any{
		"status": "ok",
		"agent":  map[string]any{"name": agentName},
	})
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

// mergeCancel returns a context cancelled when either of the two parents is
// cancelled. It joins "client disconnected" (request context) and "process
// shutting down" (shutdown context) into a single thing.
//
// PT: mergeCancel devolve um contexto cancelado quando qualquer um dos dois pais é
// cancelado. Junta "cliente desconectou" (contexto da requisição) e "processo
// desligando" (contexto de shutdown) numa coisa só.
func mergeCancel(primary, secondary context.Context) (context.Context, context.CancelFunc) {
	if secondary == nil || secondary.Done() == nil {
		return context.WithCancel(primary)
	}
	ctx, cancel := context.WithCancel(primary)
	stop := context.AfterFunc(secondary, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

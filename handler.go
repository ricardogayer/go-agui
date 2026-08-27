package agui

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
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

// Handler returns the http.Handler that speaks AG-UI over SSE: it receives a
// RunAgentInput in JSON and responds with `text/event-stream`.
//
// It takes care of decoding, headers, the run lifecycle (ids, cancellation,
// panic) and the terminal event. Everything application-specific lives behind
// the [Runner] interface.
//
// To mount it on a *gin.Engine, use the ginx package — or, without it,
// `r.POST("/", gin.WrapH(agui.Handler(ag)))`.
//
// PT: Handler devolve o http.Handler que fala AG-UI sobre SSE: recebe um
// RunAgentInput em JSON e responde `text/event-stream`.
//
// Cuida da decodificação, dos cabeçalhos, do ciclo de vida do run
// (ids, cancelamento, pânico) e do evento terminal. Tudo o que é da aplicação
// fica atrás da interface [Runner].
//
// Para montar num *gin.Engine, use o pacote ginx — ou, sem ele,
// `r.POST("/", gin.WrapH(agui.Handler(ag)))`.
func Handler(r Runner, opts ...HandlerOption) http.Handler {
	cfg := &handlerOptions{logger: slog.Default(), shutdownCtx: context.Background()}
	for _, o := range opts {
		o(cfg)
	}
	sw := sse.NewSSEWriter().WithLogger(cfg.logger)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body := req.Body
		if cfg.maxBodyBytes > 0 {
			body = http.MaxBytesReader(w, req.Body, cfg.maxBodyBytes)
		}
		var in RunAgentInput
		if err := json.NewDecoder(body).Decode(&in); err != nil {
			status := http.StatusBadRequest
			msg := "invalid request body"
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
				msg = "request body too large"
			}
			cfg.logger.Warn("invalid request body", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
			return
		}

		// The Python adapter accepts empty threadId/runId; here we generate
		// valid ids, because the Go SDK rejects events with those fields empty.
		//
		// PT: O adaptador Python aceita threadId/runId vazios; aqui geramos ids
		// válidos, porque o SDK Go recusa eventos com esses campos vazios.
		threadID := in.ThreadID
		if threadID == "" {
			threadID = events.GenerateThreadID()
		}
		runID := in.RunID
		if runID == "" {
			runID = events.GenerateRunID()
		}
		logger := cfg.logger.With("thread", threadID, "run", runID)

		rc := http.NewResponseController(w)
		// An SSE run is long by nature. Without clearing the write deadline, an
		// http.Server with WriteTimeout — the recommended configuration in any
		// production guide — would cut the stream in the middle of the response.
		// ErrNotSupported happens when the wrapped writer does not expose
		// Unwrap(); in that case there is no deadline to clear and moving on is
		// the right thing.
		//
		// PT: Um run SSE é longo por natureza. Sem zerar o prazo de escrita, um
		// http.Server com WriteTimeout — a configuração recomendada em
		// qualquer guia de produção — cortaria o stream no meio da resposta.
		// ErrNotSupported acontece quando o writer embrulhado não expõe
		// Unwrap(); nesse caso não há prazo a zerar e seguir é o certo.
		if err := rc.SetWriteDeadline(time.Time{}); err != nil &&
			!errors.Is(err, http.ErrNotSupported) {
			logger.Warn("could not clear the stream write deadline", "error", err)
		}

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// Mirrors NGINX's `proxy_buffering off`: without this a proxy in front
		// holds the frames and streaming turns into batching.
		//
		// PT: Espelha o `proxy_buffering off` do NGINX: sem isto um proxy à frente
		// segura os quadros e o streaming vira lote.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		_ = rc.Flush() // commits the headers before the first event (PT: compromete os cabeçalhos antes do primeiro evento)

		// The run context dies when the client disconnects (req.Context()) and
		// also when the process shuts down. The Emitter additionally cancels it
		// on the first write that fails.
		//
		// PT: O contexto do run morre quando o cliente desconecta
		// (req.Context()) e também quando o processo desliga. O Emitter o
		// cancela adicionalmente na primeira escrita que falha.
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
			WithEmitterCancel(cancel),
			WithEmitterSSEWriter(sw),
			WithEmitterDropped(onDrop),
		)

		// A panic inside the agent loop (provider bug, nil deref) would escape
		// through the handler goroutine and truncate the stream without a
		// terminal event. It becomes RUN_ERROR.
		//
		// PT: Um pânico dentro do laço do agente (bug do provedor, nil deref) sairia
		// pela goroutine do handler e truncaria o stream sem evento terminal.
		// Vira RUN_ERROR.
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("handler panicked",
					"panic", rec, "stack", string(debug.Stack()))
				emit.RunError("the agent crashed while handling this run")
			}
		}()

		emit.RunStarted()
		err := r.Run(runCtx, emit, &in)

		// Protocol invariant: every run ends with exactly one terminal event. A
		// Runner that returns an error without having emitted its own would
		// leave the client hanging on a stream that simply stops.
		//
		// PT: Invariante do protocolo: todo run termina com exatamente um evento
		// terminal. Um Runner que devolve erro sem ter emitido o seu deixaria o
		// cliente pendurado num stream que simplesmente para.
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

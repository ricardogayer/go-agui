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

// Runner é tudo o que um transporte precisa para executar um run.
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

// RunnerFunc adapta uma função à interface [Runner].
type RunnerFunc func(ctx context.Context, emit *Emitter, in *RunAgentInput) error

// Run implementa [Runner].
func (f RunnerFunc) Run(ctx context.Context, emit *Emitter, in *RunAgentInput) error {
	return f(ctx, emit, in)
}

type handlerOptions struct {
	logger       *slog.Logger
	shutdownCtx  context.Context
	onDropped    DroppedFunc
	maxBodyBytes int64
}

// HandlerOption configura o [Handler].
type HandlerOption func(*handlerOptions)

// WithHandlerLogger define o logger do handler e do writer SSE.
func WithHandlerLogger(l *slog.Logger) HandlerOption {
	return func(o *handlerOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithShutdownContext liga os runs ao contexto de desligamento do processo,
// para que um SIGTERM cancele os streams em voo em vez de matá-los no meio.
func WithShutdownContext(ctx context.Context) HandlerOption {
	return func(o *handlerOptions) {
		if ctx != nil {
			o.shutdownCtx = ctx
		}
	}
}

// WithOnDropped registra o callback de evento descartado por falha de
// codificação/validação. O padrão registra em log no nível ERROR.
func WithOnDropped(f DroppedFunc) HandlerOption {
	return func(o *handlerOptions) { o.onDropped = f }
}

// WithMaxBodyBytes limita o tamanho do corpo do POST. Acima do limite a
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

// Handler devolve o http.Handler que fala AG-UI sobre SSE: recebe um
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
			cfg.logger.Warn("corpo de requisição inválido", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
			return
		}

		// O adaptador Python aceita threadId/runId vazios; aqui geramos ids
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
		// Um run SSE é longo por natureza. Sem zerar o prazo de escrita, um
		// http.Server com WriteTimeout — a configuração recomendada em
		// qualquer guia de produção — cortaria o stream no meio da resposta.
		// ErrNotSupported acontece quando o writer embrulhado não expõe
		// Unwrap(); nesse caso não há prazo a zerar e seguir é o certo.
		if err := rc.SetWriteDeadline(time.Time{}); err != nil &&
			!errors.Is(err, http.ErrNotSupported) {
			logger.Warn("não foi possível remover o prazo de escrita do stream", "error", err)
		}

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// Espelha o `proxy_buffering off` do NGINX: sem isto um proxy à frente
		// segura os quadros e o streaming vira lote.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		_ = rc.Flush() // compromete os cabeçalhos antes do primeiro evento

		// O contexto do run morre quando o cliente desconecta
		// (req.Context()) e também quando o processo desliga. O Emitter o
		// cancela adicionalmente na primeira escrita que falha.
		reqCtx, cancelReq := mergeCancel(req.Context(), cfg.shutdownCtx)
		defer cancelReq()
		runCtx, cancel := context.WithCancel(reqCtx)
		defer cancel()

		onDrop := cfg.onDropped
		if onDrop == nil {
			onDrop = func(_ context.Context, ev Event, err error) {
				logger.Error("evento AG-UI descartado",
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

		// Um pânico dentro do laço do agente (bug do provedor, nil deref) sairia
		// pela goroutine do handler e truncaria o stream sem evento terminal.
		// Vira RUN_ERROR.
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("handler entrou em pânico",
					"panic", rec, "stack", string(debug.Stack()))
				emit.RunError("the agent crashed while handling this run")
			}
		}()

		emit.RunStarted()
		err := r.Run(runCtx, emit, &in)

		// Invariante do protocolo: todo run termina com exatamente um evento
		// terminal. Um Runner que devolve erro sem ter emitido o seu deixaria o
		// cliente pendurado num stream que simplesmente para.
		if !emit.Terminated() {
			if err != nil {
				logger.Error("runner falhou sem emitir evento terminal", "error", err)
				emit.RunError("the agent failed to complete this run")
			} else {
				logger.Warn("runner terminou sem emitir evento terminal; emitindo RUN_FINISHED")
				emit.RunFinished()
			}
		} else if err != nil {
			logger.Warn("run terminou com erro", "error", err)
		}

		if e := emit.Err(); e != nil {
			logger.Info("stream encerrado antes do fim", "error", e)
		}
	})
}

// Health devolve o handler de /health, com o mesmo corpo do adaptador Python.
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

// mergeCancel devolve um contexto cancelado quando qualquer um dos dois pais é
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

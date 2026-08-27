// Package ginx mounts an AG-UI agent on a Gin router.
//
// The core (`agui`) speaks http.Handler and knows no framework; this package is
// the convenience layer for Gin users, and it is a SEPARATE MODULE so that Gin
// does not enter the go.mod of whoever uses chi, echo or plain net/http.
//
// Register the two routes of the contract on an existing router:
//
//	r := gin.New()
//	ginx.Register(r, ag, "my_agent")
//
// Or bring everything up, which is what most people want:
//
//	log.Fatal(ginx.ListenAndServe(ctx, ":8000", ag, "my_agent"))
//
// PT: Package ginx monta um agente AG-UI num roteador Gin.
//
// O núcleo (`agui`) fala http.Handler e não conhece framework nenhum; este
// pacote é a camada de conveniência para quem usa Gin, e é um MÓDULO À PARTE
// para que o Gin não entre no go.mod de quem usa chi, echo ou net/http puro.
//
// Os exemplos acima mostram como registrar as duas rotas do contrato num
// roteador existente, ou subir tudo, que é o que a maioria quer.
package ginx

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	agui "github.com/ricardogayer/go-agui"
)

// Handler adapts the AG-UI handler to Gin.
//
// gin.ResponseWriter implements http.Flusher and http.ResponseWriter, so SSE
// streaming goes through the wrap without loss.
//
// PT: Handler adapta o handler AG-UI para o Gin.
//
// O gin.ResponseWriter implementa http.Flusher e http.ResponseWriter, então o
// streaming SSE atravessa o wrap sem perda.
func Handler(r agui.Runner, opts ...agui.HandlerOption) gin.HandlerFunc {
	return gin.WrapH(agui.Handler(r, opts...))
}

// Health adapts the healthcheck handler to Gin.
//
// PT: Health adapta o handler de healthcheck para o Gin.
func Health(agentName string) gin.HandlerFunc {
	return gin.WrapH(agui.Health(agentName))
}

// Register mounts `POST /` and `GET /health` on the router.
//
// It does NOT handle `//health` — the path a probe written against the Python
// backend uses, and which Gin's router refuses to register directly ("handlers
// are already registered for path '/health'"). Wrap the engine with
// [agui.NormalizeHealthPath] yourself, which is what [ListenAndServe] does.
//
// PT: Register monta `POST /` e `GET /health` no roteador.
//
// NÃO trata `//health` — o caminho que uma sonda escrita contra o backend
// Python usa, e que o roteador do Gin recusa registrar diretamente ("handlers
// are already registered for path '/health'"). Envolva o engine com
// [agui.NormalizeHealthPath] você mesmo, que é o que [ListenAndServe] faz.
func Register(r gin.IRouter, runner agui.Runner, agentName string, opts ...agui.HandlerOption) {
	r.POST("/", Handler(runner, opts...))
	r.GET("/health", Health(agentName))
}

// Engine returns a ready *gin.Engine, in release mode, with Recovery, the two
// routes and — optionally — CORS.
//
// PT: Engine devolve um *gin.Engine pronto, em modo release, com Recovery, as duas
// rotas e — opcionalmente — CORS.
func Engine(runner agui.Runner, agentName string, opts ...Option) *gin.Engine {
	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	if cfg.cors {
		r.Use(CORS())
	}
	Register(r, runner, agentName, cfg.handler...)
	return r
}

// ListenAndServe brings the agent up on an HTTP server and shuts it down
// gracefully when ctx is cancelled.
//
// PT: ListenAndServe sobe o agente num servidor HTTP e o desliga graciosamente
// quando ctx é cancelado.
func ListenAndServe(ctx context.Context, addr string, runner agui.Runner, agentName string, opts ...Option) error {
	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}
	// The runs are tied to ctx by default, so in-flight streams end on
	// shutdown; a WithShutdownContext passed by the caller wins.
	//
	// PT: Os runs ficam ligados a ctx por padrão, para os streams em curso
	// terminarem no desligamento; um WithShutdownContext do chamador prevalece.
	opts = append([]Option{WithHandlerOptions(agui.WithShutdownContext(ctx))}, opts...)
	engine := Engine(runner, agentName, opts...)
	return agui.ListenAndServe(ctx, addr,
		agui.NormalizeHealthPath(engine), cfg.serve...)
}

type options struct {
	cors    bool
	handler []agui.HandlerOption
	serve   []agui.ServeOption
}

// Option configures this package's router and server.
//
// PT: Option configura o roteador e o servidor deste pacote.
type Option func(*options)

// WithCORS turns on a permissive CORS, to debug the backend straight from the
// browser.
//
// Off by default: the CopilotKit runtime calls the backend server-side, so
// there is no cross-origin in the normal path.
//
// PT: WithCORS liga um CORS permissivo, para depurar o backend direto do navegador.
//
// Desligado por padrão: o runtime CopilotKit chama o backend server-side, então
// não há origem cruzada no caminho normal.
func WithCORS() Option {
	return func(o *options) { o.cors = true }
}

// WithHandlerOptions passes options on to the AG-UI handler (logger, shutdown
// context, dropped-event callback, max body size).
//
// PT: WithHandlerOptions repassa opções ao handler AG-UI (logger, contexto de
// desligamento, callback de evento descartado, tamanho máximo do corpo).
func WithHandlerOptions(opts ...agui.HandlerOption) Option {
	return func(o *options) { o.handler = append(o.handler, opts...) }
}

// WithServeOptions passes options on to the HTTP server.
//
// PT: WithServeOptions repassa opções ao servidor HTTP.
func WithServeOptions(opts ...agui.ServeOption) Option {
	return func(o *options) { o.serve = append(o.serve, opts...) }
}

// CORS returns the permissive CORS middleware.
//
// PT: CORS devolve o middleware de CORS permissivo.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Cache-Control")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

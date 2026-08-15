// Package ginx monta um agente AG-UI num roteador Gin.
//
// O núcleo (`agui`) fala http.Handler e não conhece framework nenhum; este
// pacote é a camada de conveniência para quem usa Gin, e é um MÓDULO À PARTE
// para que o Gin não entre no go.mod de quem usa chi, echo ou net/http puro.
//
// Registrar as duas rotas do contrato num roteador existente:
//
//	r := gin.New()
//	ginx.Register(r, ag, "meu_agente")
//
// Ou subir tudo, que é o que a maioria quer:
//
//	log.Fatal(ginx.ListenAndServe(ctx, ":8000", ag, "meu_agente"))
package ginx

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	agui "github.com/ricardogayer/go-agui"
)

// Handler adapta o handler AG-UI para o Gin.
//
// O gin.ResponseWriter implementa http.Flusher e http.ResponseWriter, então o
// streaming SSE atravessa o wrap sem perda.
func Handler(r agui.Runner, opts ...agui.HandlerOption) gin.HandlerFunc {
	return gin.WrapH(agui.Handler(r, opts...))
}

// Health adapta o handler de healthcheck para o Gin.
func Health(agentName string) gin.HandlerFunc {
	return gin.WrapH(agui.Health(agentName))
}

// Register monta `POST /` e `GET /health` no roteador.
//
// Também instala a normalização de `//health` — o caminho que uma sonda
// escrita contra o backend Python usa, e que o roteador do Gin recusa registrar
// diretamente ("handlers are already registered for path '/health'"), porque
// ele normaliza a barra dobrada antes de casar a rota. Ver
// [agui.NormalizeHealthPath].
func Register(r gin.IRouter, runner agui.Runner, agentName string, opts ...agui.HandlerOption) {
	r.POST("/", Handler(runner, opts...))
	r.GET("/health", Health(agentName))
}

// Engine devolve um *gin.Engine pronto, em modo release, com Recovery, as duas
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

// ListenAndServe sobe o agente num servidor HTTP e o desliga graciosamente
// quando ctx é cancelado.
func ListenAndServe(ctx context.Context, addr string, runner agui.Runner, agentName string, opts ...Option) error {
	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}
	engine := Engine(runner, agentName, opts...)
	return agui.ListenAndServe(ctx, addr,
		agui.NormalizeHealthPath(engine), cfg.serve...)
}

type options struct {
	cors    bool
	handler []agui.HandlerOption
	serve   []agui.ServeOption
}

// Option configura o roteador e o servidor deste pacote.
type Option func(*options)

// WithCORS liga um CORS permissivo, para depurar o backend direto do navegador.
//
// Desligado por padrão: o runtime CopilotKit chama o backend server-side, então
// não há origem cruzada no caminho normal.
func WithCORS() Option {
	return func(o *options) { o.cors = true }
}

// WithHandlerOptions repassa opções ao handler AG-UI (logger, contexto de
// desligamento, callback de evento descartado).
func WithHandlerOptions(opts ...agui.HandlerOption) Option {
	return func(o *options) { o.handler = append(o.handler, opts...) }
}

// WithServeOptions repassa opções ao servidor HTTP.
func WithServeOptions(opts ...agui.ServeOption) Option {
	return func(o *options) { o.serve = append(o.serve, opts...) }
}

// CORS devolve o middleware de CORS permissivo.
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

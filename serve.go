package agui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Mux mounts the routes of the AG-UI contract:
//
//	POST /        → the run (SSE)
//	GET  /health  → the healthcheck
//	GET  //health → same, rewritten to /health
//
// About the double slash: the Python adapter mounts the healthcheck at
// `path + "/health"`; with path="/" that is literally `//health`, and that is
// the path any probe written against the Python backend uses.
//
// Registering both forms would be more explicit, but it is not possible: both
// http.ServeMux and Gin's router reject a pattern with an "unclean" path — the
// ServeMux panics with "non-CONNECT pattern with unclean path can never
// match". Hence the rewrite, which happens before routing.
//
// PT: Mux monta as rotas do contrato AG-UI (ver a tabela acima).
//
// Sobre a barra dobrada: o adaptador Python monta o healthcheck em
// `path + "/health"`; com path="/" isso dá literalmente `//health`, e é esse o
// caminho que qualquer sonda escrita contra o backend Python usa.
//
// Registrar as duas formas seria mais explícito, mas não é possível: tanto o
// http.ServeMux quanto o roteador do Gin recusam um padrão com caminho "sujo"
// — o ServeMux entra em pânico com "non-CONNECT pattern with unclean path can
// never match". Daí a reescrita, que acontece antes do roteamento.
func Mux(r Runner, agentName string, opts ...HandlerOption) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /", Handler(r, opts...))
	mux.Handle("GET /health", Health(agentName))
	return NormalizeHealthPath(mux)
}

// NormalizeHealthPath rewrites `//health` to `/health` before routing.
//
// Exported because whoever builds their own router — a *gin.Engine, for
// example — needs it for the same reason, and it cannot be a route. See [Mux].
//
// PT: NormalizeHealthPath reescreve `//health` para `/health` antes do roteamento.
//
// Exportada porque quem monta o próprio roteador — um *gin.Engine, por exemplo
// — precisa dela pelo mesmo motivo, e ela não tem como ser uma rota. Ver [Mux].
func NormalizeHealthPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "//health" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/health"
			next.ServeHTTP(w, r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe starts an HTTP server on addr and shuts it down gracefully
// when ctx is cancelled.
//
// There is no WriteTimeout, and that is deliberate: an SSE run lasts as long as
// the conversation and a write deadline would cut it midway. ReadHeaderTimeout
// is present, which is what protects against clients that never finish
// sending their headers.
//
// This function does NOT register flags nor read the command line — a library
// that calls flag.Parse breaks the caller's FlagSet, and `go test`. See
// [CheckHealth] for the container probe.
//
// PT: ListenAndServe sobe um servidor HTTP em addr e o desliga graciosamente quando
// ctx é cancelado.
//
// Não há WriteTimeout, e isso é deliberado: um run SSE dura o tempo da conversa
// e um prazo de escrita o cortaria no meio. ReadHeaderTimeout está presente,
// que é o que protege contra clientes que nunca terminam de enviar os headers.
//
// Esta função NÃO registra flags nem lê a linha de comando — uma biblioteca que
// chama flag.Parse quebra o FlagSet de quem a usa, e o `go test`. Ver
// [CheckHealth] para a sonda de container.
func ListenAndServe(ctx context.Context, addr string, h http.Handler, opts ...ServeOption) error {
	cfg := &serveOptions{
		logger:          slog.Default(),
		shutdownTimeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(cfg)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		cfg.logger.Info("server is up", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	cfg.logger.Info("signal received; draining connections")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown did not drain in time: %w", err)
	}
	return nil
}

type serveOptions struct {
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// ServeOption configures [ListenAndServe].
//
// PT: ServeOption configura [ListenAndServe].
type ServeOption func(*serveOptions)

// WithServeLogger sets the server logger.
//
// PT: WithServeLogger define o logger do servidor.
func WithServeLogger(l *slog.Logger) ServeOption {
	return func(o *serveOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithShutdownTimeout sets how long the shutdown waits for in-flight
// connections. The default is 10s.
//
// PT: WithShutdownTimeout define quanto tempo o desligamento espera pelas conexões
// em voo. O padrão é 10s.
func WithShutdownTimeout(d time.Duration) ServeOption {
	return func(o *serveOptions) {
		if d > 0 {
			o.shutdownTimeout = d
		}
	}
}

// CheckHealth hits a server's /health and returns an error if it is not
// healthy.
//
// It exists for the probe of distroless containers, which have neither a shell
// nor curl: the binary itself becomes the probe. In the application's main:
//
//	var probe = flag.Bool("healthcheck", false, "queries the local /health and exits 0/1")
//	// ...
//	if *probe {
//		if err := agui.CheckHealth("http://127.0.0.1:" + port + "/health"); err != nil {
//			fmt.Fprintln(os.Stderr, err)
//			os.Exit(1)
//		}
//		return
//	}
//
// This tests the real endpoint, unlike a healthcheck that only opens a TCP
// socket and reports "healthy" with the agent broken.
//
// PT: CheckHealth bate no /health de um servidor e devolve erro se ele não estiver
// saudável.
//
// Existe para a sonda de containers distroless, que não têm shell nem curl: o
// próprio binário vira a sonda. No main da aplicação, use o padrão do exemplo
// acima.
//
// Isto testa o endpoint de verdade, diferente de um healthcheck que só abre um
// socket TCP e reporta "healthy" com o agente quebrado.
func CheckHealth(url string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

// Addr builds the listen address from HOST and PORT, falling back to the
// defaults when they are not set.
//
// It is the convenience that avoids rewriting the same environment read in
// every main; whoever wants to control it builds the addr themselves.
//
// PT: Addr monta o endereço de escuta a partir de HOST e PORT, caindo nos padrões
// quando não estiverem definidos.
//
// É a conveniência que evita reescrever a mesma leitura de ambiente em todo
// main; quem quiser controlar isso monta o addr sozinho.
func Addr(defaultPort int) string {
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := defaultPort
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 65535 {
			port = n
		}
	}
	return host + ":" + strconv.Itoa(port)
}

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

// Mux monta as rotas do contrato AG-UI:
//
//	POST /        → o run (SSE)
//	GET  /health  → o healthcheck
//	GET  //health → idem, reescrito para /health
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

// NormalizeHealthPath reescreve `//health` para `/health` antes do roteamento.
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

// ListenAndServe sobe um servidor HTTP em addr e o desliga graciosamente quando
// ctx é cancelado.
//
// Não há WriteTimeout, e isso é deliberado: um run SSE dura o tempo da conversa
// e um prazo de escrita o cortaria no meio. ReadHeaderTimeout está presente,
// que é o que protege contra conexões ociosas.
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
		cfg.logger.Info("servidor no ar", "addr", addr)
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

	cfg.logger.Info("sinal recebido; drenando conexões")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("desligamento não drenou a tempo: %w", err)
	}
	return nil
}

type serveOptions struct {
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// ServeOption configura [ListenAndServe].
type ServeOption func(*serveOptions)

// WithServeLogger define o logger do servidor.
func WithServeLogger(l *slog.Logger) ServeOption {
	return func(o *serveOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithShutdownTimeout define quanto tempo o desligamento espera pelas conexões
// em voo. O padrão é 10s.
func WithShutdownTimeout(d time.Duration) ServeOption {
	return func(o *serveOptions) {
		if d > 0 {
			o.shutdownTimeout = d
		}
	}
}

// CheckHealth bate no /health de um servidor e devolve erro se ele não estiver
// saudável.
//
// Existe para a sonda de containers distroless, que não têm shell nem curl: o
// próprio binário vira a sonda. No main da aplicação:
//
//	var probe = flag.Bool("healthcheck", false, "consulta o /health local e sai 0/1")
//	// ...
//	if *probe {
//		if err := agui.CheckHealth("http://127.0.0.1:" + porta + "/health"); err != nil {
//			fmt.Fprintln(os.Stderr, err)
//			os.Exit(1)
//		}
//		return
//	}
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

// Addr monta o endereço de escuta a partir de HOST e PORT, caindo nos padrões
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

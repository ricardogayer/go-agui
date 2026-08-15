package agui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
)

func TestCorpoInvalidoViraBadRequest(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			t.Fatal("o runner não deveria ser chamado")
			return nil
		}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{isto não é json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperava 400", rec.Code)
	}
}

func TestCorpoAcimaDoLimiteVira413(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			t.Fatal("o runner não deveria ser chamado")
			return nil
		}), agui.WithMaxBodyBytes(64))

	corpo := `{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"` +
		strings.Repeat("x", 256) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(corpo))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperava 413", rec.Code)
	}
}

func TestPanicoDoRunnerViraRunError(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			panic("bug no provedor")
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("oi"))

	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunError)
	aguitest.RequireBalanced(t, evs)
}

func TestRunnerQueDevolveErroSemTerminalGanhaUmRunError(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			return context.DeadlineExceeded
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("oi"))

	// O invariante: todo run termina com exatamente um evento terminal. Um
	// Runner que esquece o seu deixaria o cliente pendurado num stream que
	// simplesmente para.
	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunError)
}

func TestRunnerQueTerminaSozinhoNaoRecebeTerminalDuplicado(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return context.Canceled // erro só para registro
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("oi"))

	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunFinished)
	aguitest.RequireBalanced(t, evs)
}

func TestThreadIDERunIDVaziosSaoGerados(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			// O SDK recusa eventos com esses campos vazios; se não fossem
			// gerados, nem o RUN_STARTED sairia.
			if e.ThreadID() == "" || e.RunID() == "" {
				t.Error("threadId/runId deveriam ter sido gerados")
			}
			e.RunFinished()
			return nil
		}))

	in := aguitest.Input("oi")
	in.ThreadID, in.RunID = "", ""

	evs := aguitest.Collect(t, h, in)
	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunFinished)
}

func TestCabecalhosDeStreamingSaemAntesDoPrimeiroEvento(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return nil
		}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"threadId":"t","runId":"r"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	esperados := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		// Espelha o proxy_buffering off do NGINX: sem isto o streaming vira lote.
		"X-Accel-Buffering": "no",
	}
	for k, v := range esperados {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("cabeçalho %s = %q, esperava %q", k, got, v)
		}
	}
}

func TestMuxAtendeAsDuasFormasDoHealth(t *testing.T) {
	mux := agui.Mux(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return nil
		}), "agente_teste")

	// `//health` é o caminho que uma sonda escrita contra o backend Python usa:
	// o adaptador monta o healthcheck em path + "/health", e com path="/" isso
	// dá literalmente duas barras.
	for _, caminho := range []string{"/health", "//health"} {
		req := httptest.NewRequest(http.MethodGet, caminho, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, esperava 200", caminho, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "agente_teste") {
			t.Errorf("GET %s não devolveu o nome do agente: %s", caminho, rec.Body.String())
		}
	}
}

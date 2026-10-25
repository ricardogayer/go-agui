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

func TestInvalidBodyBecomesBadRequest(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			t.Fatal("the runner should not be called")
			return nil
		}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{this is not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestBodyOverTheLimitBecomes413(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			t.Fatal("the runner should not be called")
			return nil
		}), agui.WithMaxBodyBytes(64))

	body := `{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"` +
		strings.Repeat("x", 256) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestRunnerPanicBecomesRunError(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			panic("bug in the provider")
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("hi"))

	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunError)
	aguitest.RequireBalanced(t, evs)
}

func TestRunnerReturningErrorWithoutTerminalGetsARunError(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			return context.DeadlineExceeded
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("hi"))

	// The invariant: every run ends with exactly one terminal event. A Runner
	// that forgets its own would leave the client hanging on a stream that
	// simply stops.
	//
	// PT: O invariante: todo run termina com exatamente um evento terminal. Um
	// Runner que esquece o seu deixaria o cliente pendurado num stream que
	// simplesmente para.
	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunError)
}

func TestRunnerThatTerminatesItselfGetsNoDuplicateTerminal(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return context.Canceled // error only for the record (PT: erro só para registro)
		}))

	evs := aguitest.Collect(t, h, aguitest.Input("hi"))

	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunFinished)
	aguitest.RequireBalanced(t, evs)
}

func TestEmptyThreadIDAndRunIDAreGenerated(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			// The SDK rejects events with these fields empty; if they were not
			// generated, not even the RUN_STARTED would go out.
			//
			// PT: O SDK recusa eventos com esses campos vazios; se não fossem
			// gerados, nem o RUN_STARTED sairia.
			if e.ThreadID() == "" || e.RunID() == "" {
				t.Error("threadId/runId should have been generated")
			}
			e.RunFinished()
			return nil
		}))

	in := aguitest.Input("hi")
	in.ThreadID, in.RunID = "", ""

	evs := aguitest.Collect(t, h, in)
	aguitest.RequireSequence(t, evs, agui.EventRunStarted, agui.EventRunFinished)
}

func TestStreamingHeadersAreSentBeforeTheFirstEvent(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return nil
		}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"threadId":"t","runId":"r","messages":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	want := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		// Mirrors NGINX's proxy_buffering off: without it streaming turns into batching.
		//
		// PT: Espelha o proxy_buffering off do NGINX: sem isto o streaming vira lote.
		"X-Accel-Buffering": "no",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
}

func TestMuxServesBothHealthPaths(t *testing.T) {
	mux := agui.Mux(agui.RunnerFunc(
		func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
			e.RunFinished()
			return nil
		}), "test_agent")

	// `//health` is the path a probe written against the Python backend uses:
	// the adapter mounts the healthcheck at path + "/health", and with path="/"
	// that gives literally two slashes.
	//
	// PT: `//health` é o path que uma sonda escrita contra o backend Python usa:
	// o adaptador monta o healthcheck em path + "/health", e com path="/" isso
	// dá literalmente duas barras.
	for _, path := range []string{"/health", "//health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "test_agent") {
			t.Errorf("GET %s did not return the agent name: %s", path, rec.Body.String())
		}
	}
}

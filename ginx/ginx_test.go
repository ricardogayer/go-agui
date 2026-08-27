package ginx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
	"github.com/ricardogayer/go-agui/ginx"
)

func runner() agui.Runner {
	return agui.RunnerFunc(func(_ context.Context, e *agui.Emitter, _ *agui.RunAgentInput) error {
		e.TextStart("m1")
		e.TextContent("m1", "hello from Gin")
		e.TextEnd("m1")
		e.RunFinished()
		return nil
	})
}

func TestSSEStreamingPassesThroughGin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ginx.Register(r, runner(), "test_agent")

	evs := aguitest.Collect(t, r, aguitest.Input("hi"))

	aguitest.RequireSequence(t, evs,
		agui.EventRunStarted,
		agui.EventTextMessageStart, agui.EventTextMessageChunk, agui.EventTextMessageEnd,
		agui.EventRunFinished)
	aguitest.RequireBalanced(t, evs)

	if got := aguitest.ConcatenatedText(evs); got != "hello from Gin" {
		t.Fatalf("text = %q", got)
	}
}

func TestEngineServesBothHealthPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Engine alone does not normalize; ListenAndServe is what does it. Here we
	// assemble the same stack by hand to exercise both paths.
	//
	// PT: O Engine sozinho não normaliza; quem faz isso é o ListenAndServe. Aqui
	// montamos o mesmo empilhamento à mão para exercitar os dois caminhos.
	h := agui.NormalizeHealthPath(ginx.Engine(runner(), "test_agent"))

	for _, path := range []string{"/health", "//health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, expected 200 (body: %s)", path, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), "test_agent") {
			t.Errorf("GET %s = %s", path, rec.Body.String())
		}
	}
}

func TestCORSAnswersPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := ginx.Engine(runner(), "test_agent", ginx.WithCORS())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, expected 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}

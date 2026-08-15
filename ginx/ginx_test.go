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
		e.TextContent("m1", "olá do Gin")
		e.TextEnd("m1")
		e.RunFinished()
		return nil
	})
}

func TestOStreamingSSEAtravessaOGin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ginx.Register(r, runner(), "agente_teste")

	evs := aguitest.Collect(t, r, aguitest.Input("oi"))

	aguitest.RequireSequence(t, evs,
		agui.EventRunStarted,
		agui.EventTextMessageStart, agui.EventTextMessageChunk, agui.EventTextMessageEnd,
		agui.EventRunFinished)
	aguitest.RequireBalanced(t, evs)

	if got := aguitest.TextoConcatenado(evs); got != "olá do Gin" {
		t.Fatalf("texto = %q", got)
	}
}

func TestEngineAtendeOHealthNasDuasFormas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// O Engine sozinho não normaliza; quem faz isso é o ListenAndServe. Aqui
	// montamos o mesmo empilhamento à mão para exercitar os dois caminhos.
	h := agui.NormalizeHealthPath(ginx.Engine(runner(), "agente_teste"))

	for _, caminho := range []string{"/health", "//health"} {
		req := httptest.NewRequest(http.MethodGet, caminho, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, esperava 200 (corpo: %s)", caminho, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), "agente_teste") {
			t.Errorf("GET %s = %s", caminho, rec.Body.String())
		}
	}
}

func TestCORSRespondeAoPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := ginx.Engine(runner(), "agente_teste", ginx.WithCORS())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, esperava 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("faltou o cabeçalho de CORS")
	}
}

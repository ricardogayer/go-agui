// Command quickstart é o menor agente AG-UI possível: chat com streaming, sem
// tools e sem estado.
//
// Rodar:
//
//	export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
//	go run ./examples/quickstart
//
// Testar sem subir runtime nenhum:
//
//	curl -N -X POST localhost:8000 -H 'content-type: application/json' \
//	  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"oi"}]}'
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/ginx"
	"github.com/ricardogayer/go-agui/model"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	m := agui.Must(model.FromEnv(ctx))
	ag := agui.Must(agui.New(ctx, m,
		agui.WithSystemPrompt("Você é um assistente útil e direto.")))

	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "quickstart"))
}

// Command quickstart is the smallest possible AG-UI agent: streaming chat, with
// no tools and no state.
//
// Run:
//
//	export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
//	go run ./examples/quickstart
//
// Test without bringing up any runtime:
//
//	curl -N -X POST localhost:8000 -H 'content-type: application/json' \
//	  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"hi"}]}'
//
// PT: Command quickstart é o menor agente AG-UI possível: chat com streaming, sem
// tools e sem estado.
//
// Os exemplos acima mostram como rodar e como testar sem subir runtime nenhum.
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
		agui.WithSystemPrompt("You are a helpful, direct assistant.")))

	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "quickstart"))
}

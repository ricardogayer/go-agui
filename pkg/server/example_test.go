package server_test

import (
	"context"
	"net/http"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

func myAgent(ctx context.Context, in *agui.RunAgentInput, sse *stream.SSE) error {
	if err := sse.RunStarted(in.ThreadID, in.RunID); err != nil {
		return err
	}
	if _, err := sse.TextMessage("Olá! Você disse: " + server.LastUserText(in)); err != nil {
		return err
	}
	return sse.RunFinished(in.ThreadID, in.RunID)
}

func Example() {
	http.Handle("/agent", server.NewAgentHandler(server.RunnerFunc(myAgent)))
	http.Handle("/agent/capabilities", server.CapabilitiesHandler(agui.AgentCapabilities{
		Reasoning: &agui.ReasoningCapabilities{Supported: agui.Ptr(true), Streaming: agui.Ptr(true)},
	}))
	// http.ListenAndServe(":8080", nil)
}

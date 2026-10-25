package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/FrontendToolReinvocationTest.cs.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/FrontendToolReinvocationTest.cs::FrontendToolReinvocationTest.ReinvokedClientTool_SurfacesAgain_WithFreshResult
func TestCsFrontendToolReinvocationTest_ReinvokedClientTool_SurfacesAgain_WithFreshResult(t *testing.T) {
	rounds := 0
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		rounds++
		must(s.RunStarted(in.ThreadID, in.RunID))
		switch rounds {
		case 1:
			must(s.ToolCall("call_l1", "get_user_location", "{}"))
		case 2:
			must(s.ToolCall("call_l2", "get_user_location", "{}"))
		default:
			_, err := s.TextMessage("Done.")
			must(err)
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "where am I?")
	var results []string
	text := runClientToolLoop(t, c, []agui.Tool{clientTool("get_user_location", "Gets the user's GPS location (live)")}, func(agui.ToolCall) string {
		r := fmt.Sprintf("Amsterdam #%d", len(results)+1)
		results = append(results, r)
		return r
	})
	// The client tool runs on the client both times, fresh.
	if strings.Join(results, ",") != "Amsterdam #1,Amsterdam #2" || rounds != 3 || !strings.Contains(text, "Done.") {
		t.Fatalf("results %v, rounds %d, text %q", results, rounds, text)
	}
}

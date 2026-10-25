package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/MixedToolInvocationIntegrationTest.cs.

import (
	"context"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/MixedToolInvocationIntegrationTest.cs::MixedToolInvocationIntegrationTest.FICC_ApprovalFlow_DirectTest
func TestCsMixedToolInvocationIntegrationTest_FICC_ApprovalFlow_DirectTest(t *testing.T) {
	t.Skip(naFICC)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/MixedToolInvocationIntegrationTest.cs::MixedToolInvocationIntegrationTest.MixedInvocation_TwoTurnFlow_EmitsToolCallsThenServerResults
func TestCsMixedToolInvocationIntegrationTest_MixedInvocation_TwoTurnFlow_EmitsToolCallsThenServerResults(t *testing.T) {
	// Turn 1: the model calls a client tool (get_user_location) and a server
	// tool (get_weather); the run ends as a success with both unanswered.
	// Turn 2: the client answers its tool; the server executes its own call
	// and streams the TOOL_CALL_RESULT, then the final text.
	r := newSampleRun(t, "MixedToolInvocationIntegration")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	tools := r.baselineTools(1)
	res := r.runTurn(1, client.RunInput{Tools: tools})
	r.assertRequest(1, res)
	if !res.Succeeded() || len(res.PendingToolCallIDs) != 2 {
		t.Fatalf("status %s pending %v", res.Status, res.PendingToolCallIDs)
	}
	// The application executes only the calls to its own tools.
	clientToolInvoked := false
	declared := map[string]bool{}
	for _, tl := range tools {
		declared[tl.Name] = true
	}
	turn2 := r.baselineMessages(2)
	answer := turn2[len(turn2)-1].(*agui.ToolMessage)
	for _, call := range toolCallsOf(r.client.Store.Messages) {
		if declared[call.Function.Name] {
			clientToolInvoked = true
			r.client.AddMessage(&agui.ToolMessage{ID: answer.ID, ToolCallID: call.ID, Content: answer.Content})
		}
	}
	if !clientToolInvoked {
		t.Fatal("the client tool was not invoked")
	}
	res = r.runTurn(2, client.RunInput{Tools: tools})
	r.assertRequest(2, res)
	if !res.Succeeded() || single[*agui.ToolCallResultEvent](t, res.Events).ToolCallID != toolCallsOf(r.client.Store.Messages)[1].ID {
		t.Fatalf("turn 2: %s %v", res.Status, types(res.Events))
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/MixedToolInvocationIntegrationTest.cs::MixedToolInvocationIntegrationTest.MixedInvocation_ServerOnlyToolCalls_NoApprovalFlow
func TestCsMixedToolInvocationIntegrationTest_MixedInvocation_ServerOnlyToolCalls_NoApprovalFlow(t *testing.T) {
	serverToolInvoked := false
	getWeather := func(city string) string { serverToolInvoked = true; return "Weather in " + city + ": 18C, cloudy" }
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		// The model calls only the server tool; the agent executes it in
		// its own tool loop, in the same run.
		must(s.ToolCall("call_w1", "get_weather", `{"city":"London"}`))
		must(s.ToolCallResult("call_w1", "call_w1", getWeather("London")))
		_, err := s.TextMessage("The weather in London is 18C and cloudy.")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "What's the weather in London?")
	res := c.runOK(t, client.RunInput{Tools: []agui.Tool{clientTool("get_user_location", "Gets location")}})
	if !serverToolInvoked {
		t.Fatal("server tool not invoked")
	}
	if got := assistantText(res.Events); got != "The weather in London is 18C and cloudy." {
		t.Fatalf("text %q", got)
	}
	if !res.Succeeded() || len(res.PendingToolCallIDs) != 0 || len(ofType[*agui.RunFinishedEvent](res.Events)) != 1 {
		t.Fatalf("status %s pending %v", res.Status, res.PendingToolCallIDs)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/MixedToolInvocationIntegrationTest.cs::MixedToolInvocationIntegrationTest.MixedInvocation_ClientOnlyToolCalls_TwoTurnFlow
func TestCsMixedToolInvocationIntegrationTest_MixedInvocation_ClientOnlyToolCalls_TwoTurnFlow(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		if tms := toolMessagesOf(in.Messages); len(tms) == 1 && tms[0].ToolCallID == "call_loc1" {
			_, err := s.TextMessage("You are in Amsterdam!")
			must(err)
		} else {
			must(s.ToolCall("call_loc1", "get_user_location", "{}"))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Where am I?")
	clientToolInvoked := false
	tools := []agui.Tool{clientTool("get_user_location", "Gets location")}
	text := runClientToolLoop(t, c, tools, func(call agui.ToolCall) string {
		clientToolInvoked = true
		return "Amsterdam, Netherlands"
	})
	if !clientToolInvoked || text != "You are in Amsterdam!" {
		t.Fatalf("invoked %v, text %q", clientToolInvoked, text)
	}
}

// runClientToolLoop is the application loop AGUIChatClient's function
// invocation performs: run, execute the pending calls to the application's
// own tools, answer them in the thread, and run again until nothing is
// pending. It returns the text the runs streamed.
func runClientToolLoop(t *testing.T, c *testClient, tools []agui.Tool, exec func(agui.ToolCall) string) string {
	t.Helper()
	declared := map[string]bool{}
	for _, tl := range tools {
		declared[tl.Name] = true
	}
	var text string
	for round := 0; round < 5; round++ {
		res, err := c.Run(context.Background(), client.RunInput{Tools: tools}, nil)
		if err != nil {
			t.Fatal(err)
		}
		text += assistantText(res.Events)
		answered := 0
		for _, id := range res.PendingToolCallIDs {
			for _, call := range toolCallsOf(c.Store.Messages) {
				if call.ID == id && declared[call.Function.Name] {
					c.AddMessage(&agui.ToolMessage{ID: "result-" + id, ToolCallID: id, Content: agui.TextContent(exec(call))})
					answered++
				}
			}
		}
		if answered == 0 {
			return text
		}
	}
	t.Fatal("tool loop did not settle")
	return ""
}

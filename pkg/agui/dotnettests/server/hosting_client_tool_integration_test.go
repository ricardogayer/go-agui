package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/ClientToolIntegrationTest.cs.
//
// AGUIChatClient wraps MEAI function invocation: client tools travel in
// RunAgentInput.tools, a call to one is executed by the client and answered
// on the next run, and a call to any other tool surfaces to the application
// unexecuted. The Go client exposes the same protocol facts (the tools sent,
// the calls a run left pending); runClientToolLoop is the application loop.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ClientToolIntegrationTest.cs::ClientToolIntegrationTest.ClientTools_SentInRunAgentInput_ConvertedToAITools
func TestCsClientToolIntegrationTest_ClientTools_SentInRunAgentInput_ConvertedToAITools(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var captured []agui.Tool
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			captured = in.Tools
			must(s.RunStarted(in.ThreadID, in.RunID))
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), format)
		c.say("u1", "What's the weather?")
		c.runOK(t, client.RunInput{Tools: []agui.Tool{{
			Name: "get_weather", Description: "Gets the weather for a location",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"location": map[string]any{"type": "string"}}},
		}}})
		if len(captured) != 1 || captured[0].Name != "get_weather" {
			t.Fatalf("tools %+v", captured)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ClientToolIntegrationTest.cs::ClientToolIntegrationTest.ClientToolAutoInvocation_ServerRequestsClientTool_FunctionInvokedByFunctionInvokingChatClient
func TestCsClientToolIntegrationTest_ClientToolAutoInvocation_ServerRequestsClientTool_FunctionInvokedByFunctionInvokingChatClient(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		turn := 0
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			turn++
			must(s.RunStarted(in.ThreadID, in.RunID))
			if turn == 1 {
				must(s.ToolCall("call-1", "get_weather", `{"location":"Seattle"}`))
			} else {
				if tms := toolMessagesOf(in.Messages); len(tms) != 1 || tms[0].ToolCallID != "call-1" || tms[0].Content.String() != "Sunny in Seattle" {
					t.Errorf("turn 2 did not carry the tool result: %+v", tms)
				}
				_, err := s.TextMessage("It's sunny in Seattle!")
				must(err)
			}
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), format)
		c.say("u1", "What's the weather in Seattle?")
		invocations := 0
		text := runClientToolLoop(t, c, []agui.Tool{clientTool("get_weather", "Gets the weather for a location")}, func(call agui.ToolCall) string {
			invocations++
			if !strings.Contains(call.Function.Arguments, "Seattle") {
				t.Errorf("arguments %q", call.Function.Arguments)
			}
			return "Sunny in Seattle"
		})
		if invocations != 1 || !strings.Contains(text, "sunny") {
			t.Fatalf("invocations %d, text %q", invocations, text)
		}
		return c
	})
}

// serverToolPassThrough: the run calls a tool the application did not
// declare; the call surfaces to the application and is not executed.
func serverToolPassThrough(t *testing.T, format, toolName, args string, declared agui.Tool) *testClient {
	c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", toolName, args))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
	c.say("u1", "Do something")
	executed := 0
	runClientToolLoop(t, c, []agui.Tool{declared}, func(agui.ToolCall) string { executed++; return "" })
	if executed != 0 {
		t.Fatal("a call to an undeclared tool was executed")
	}
	calls := toolCallsOf(c.Store.Messages)
	if len(calls) != 1 || calls[0].Function.Name != toolName || calls[0].ID != "call-1" {
		t.Fatalf("calls %+v", calls)
	}
	return c
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ClientToolIntegrationTest.cs::ClientToolIntegrationTest.ServerToolPassThrough_NotInClientTools_YieldedAsFunctionCallContent
func TestCsClientToolIntegrationTest_ServerToolPassThrough_NotInClientTools_YieldedAsFunctionCallContent(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		return serverToolPassThrough(t, format, "server_only_tool", `{"key":"value"}`, clientTool("unrelated_client_tool", "A client tool not related to the server tool"))
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ClientToolIntegrationTest.cs::ClientToolIntegrationTest.ServerToolPassThrough_WithClientTools_ServerToolNotAutoInvoked
func TestCsClientToolIntegrationTest_ServerToolPassThrough_WithClientTools_ServerToolNotAutoInvoked(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		return serverToolPassThrough(t, format, "server_tool", `{"data":"test"}`, clientTool("client_tool", "A client tool"))
	})
}

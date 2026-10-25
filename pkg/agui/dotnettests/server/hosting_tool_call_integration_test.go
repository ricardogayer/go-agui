package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs.
//
// AGUIChatClient folds a tool call's events into one FunctionCallContent at
// TOOL_CALL_END; the Go consumer delivers the events and assembles the call
// on the assistant message in its Store.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// dummyClientTool is the C# DummyClientTool.
var dummyClientTool = clientTool("_dummy_client_tool", "Dummy client tool for testing")

func toolCallClient(t *testing.T, format string, emit func(s *stream.SSE, must func(error))) *testClient {
	c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		emit(s, must)
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
	c.say("u1", "Hi")
	return c
}

func argsMap(t *testing.T, call agui.ToolCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &m); err != nil {
		t.Fatalf("arguments %q: %v", call.Function.Arguments, err)
	}
	return m
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_FunctionCallContent_MapsToToolCallEvents
func TestCsToolCallIntegrationTest_PostRun_FunctionCallContent_MapsToToolCallEvents(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := toolCallClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.ToolCall("call-1", "get_weather", `{"location":"Seattle"}`))
		})
		res := c.runOK(t, client.RunInput{Tools: []agui.Tool{dummyClientTool}})
		assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs,
			agui.EventTypeToolCallEnd, agui.EventTypeRunFinished)
		if res.Events[3].(*agui.ToolCallEndEvent).ToolCallID != "call-1" {
			t.Fatal("end id")
		}
		calls := toolCallsOf(c.Store.Messages)
		if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Function.Name != "get_weather" || argsMap(t, calls[0])["location"] != "Seattle" {
			t.Fatalf("calls %+v", calls)
		}
		if !res.Succeeded() {
			t.Fatalf("status %s", res.Status)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_SequentialToolCalls_EmitsAllEventsInOrder
func TestCsToolCallIntegrationTest_PostRun_SequentialToolCalls_EmitsAllEventsInOrder(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := toolCallClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.ToolCall("c1", "get_weather", `{"location":"Seattle"}`))
			must(s.ToolCall("c2", "get_time", `{"timezone":"PST"}`))
		})
		res := c.runOK(t, client.RunInput{Tools: []agui.Tool{dummyClientTool}})
		ends := ofType[*agui.ToolCallEndEvent](res.Events)
		if len(ends) != 2 || ends[0].ToolCallID != "c1" || ends[1].ToolCallID != "c2" {
			t.Fatalf("events %v", types(res.Events))
		}
		calls := toolCallsOf(c.Store.Messages)
		if len(calls) != 2 || calls[0].Function.Name != "get_weather" || calls[1].Function.Name != "get_time" {
			t.Fatalf("calls %+v", calls)
		}
		if !res.Succeeded() {
			t.Fatalf("status %s", res.Status)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_ToolCallInterleavedWithText_BothEmitted
func TestCsToolCallIntegrationTest_PostRun_ToolCallInterleavedWithText_BothEmitted(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := toolCallClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.TextStart("m1"))
			must(s.TextContent("m1", "Let me check the weather. "))
			must(s.TextEnd("m1"))
			must(s.ToolCallStartIn("m1", "call-1", "get_weather"))
			must(s.ToolCallArgs("call-1", `{"location":"Seattle"}`))
			must(s.ToolCallEnd("call-1"))
		})
		res := c.runOK(t, client.RunInput{Tools: []agui.Tool{dummyClientTool}})
		if d := single[*agui.TextMessageContentEvent](t, res.Events).Delta; d != "Let me check the weather. " {
			t.Fatalf("delta %q", d)
		}
		if indexOf(res.Events, agui.EventTypeTextMessageContent) > indexOf(res.Events, agui.EventTypeToolCallEnd) {
			t.Fatalf("order %v", types(res.Events))
		}
		// Text and call land on the same assistant message.
		var am *agui.AssistantMessage
		for _, m := range c.Store.Messages {
			if v, ok := m.(*agui.AssistantMessage); ok {
				am = v
			}
		}
		if am == nil || agui.Deref(am.Content) != "Let me check the weather. " || len(am.ToolCalls) != 1 || am.ToolCalls[0].Function.Name != "get_weather" {
			t.Fatalf("assistant message %+v", am)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_ToolCallResultEvent_StreamedAsToolCallUpdate
func TestCsToolCallIntegrationTest_PostRun_ToolCallResultEvent_StreamedAsToolCallUpdate(t *testing.T) {
	c := toolCallClient(t, formatJSON, func(s *stream.SSE, must func(error)) {
		must(s.ToolCall("call-1", "get_weather", `{"location":"Seattle"}`))
		must(s.ToolCallResult("call-1", "call-1", `{"temp":72}`))
	})
	res := c.runOK(t, client.RunInput{Tools: []agui.Tool{dummyClientTool}})
	calls := toolCallsOf(c.Store.Messages)
	if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Function.Name != "get_weather" {
		t.Fatalf("calls %+v", calls)
	}
	results := toolMessagesOf(c.Store.Messages)
	if len(results) != 1 || results[0].ToolCallID != "call-1" || results[0].Content.String() != `{"temp":72}` {
		t.Fatalf("results %+v", results)
	}
	// Answered by the producer: nothing is left pending.
	if len(res.PendingToolCallIDs) != 0 {
		t.Fatalf("pending %v", res.PendingToolCallIDs)
	}
	assertOneRun(t, res)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_ToolCallWithNullArgs_MapsToFunctionCallContentWithNullArgs
func TestCsToolCallIntegrationTest_PostRun_ToolCallWithNullArgs_MapsToFunctionCallContentWithNullArgs(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := toolCallClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.ToolCallStart("call-1", "ping"))
			must(s.ToolCallEnd("call-1"))
		})
		res := c.runOK(t, client.RunInput{Tools: []agui.Tool{dummyClientTool}})
		if end := single[*agui.ToolCallEndEvent](t, res.Events); end.ToolCallID != "call-1" {
			t.Fatalf("end %+v", end)
		}
		calls := toolCallsOf(c.Store.Messages)
		if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Function.Name != "ping" || calls[0].Function.Arguments != "" {
			t.Fatalf("calls %+v", calls)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ToolCallIntegrationTest.cs::ToolCallIntegrationTest.PostRun_ContinuationStreamsGenuinelyNewToolCall
func TestCsToolCallIntegrationTest_PostRun_ContinuationStreamsGenuinelyNewToolCall(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			must(s.ToolCall("call-new", "next_step", `{"value":42}`))
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), format)
		c.AddMessage(&agui.AssistantMessage{ID: "a1", ToolCalls: []agui.ToolCall{agui.NewToolCall("call-existing", "confirm", "{}")}})
		c.AddMessage(&agui.ToolMessage{ID: "t1", ToolCallID: "call-existing", Content: agui.TextContent(`{"choice":"yes"}`)})
		res := c.runOK(t, client.RunInput{Tools: []agui.Tool{clientTool("confirm", "Confirms an action")}})
		assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs,
			agui.EventTypeToolCallEnd, agui.EventTypeRunFinished)
		if res.Events[3].(*agui.ToolCallEndEvent).ToolCallID != "call-new" {
			t.Fatal("end id")
		}
		calls := toolCallsOf(c.Store.Messages)
		last := calls[len(calls)-1]
		if last.ID != "call-new" || last.Function.Name != "next_step" || argsMap(t, last)["value"] != float64(42) {
			t.Fatalf("calls %+v", calls)
		}
		// Only the genuinely new call is pending.
		if len(res.PendingToolCallIDs) != 1 || res.PendingToolCallIDs[0] != "call-new" {
			t.Fatalf("pending %v", res.PendingToolCallIDs)
		}
		return c
	})
}

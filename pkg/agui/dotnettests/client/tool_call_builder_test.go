package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs.
//
// The .NET ToolCallBuilder assembles TOOL_CALL_* events into MEAI
// FunctionCallContent (parsing the argument text into a dictionary) and
// buffers updates until every call's result arrives. The Go analogue is the
// consumer pipeline + client.Store: the call is a ToolCall on an assistant
// message whose Function.Arguments is the concatenated argument text, and a
// TOOL_CALL_RESULT becomes a tool message. Arguments stay text: schema.mdx
// (FunctionCall.arguments) says they are "Kept as written because a model can
// emit arguments that are not valid JSON", so where .NET parses (and throws on
// non-JSON) the Go tests parse the stored text themselves, and the .NET
// "throws on invalid JSON" cases are adapted to that rule.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func tcStart(id, name string) *agui.ToolCallStartEvent {
	return &agui.ToolCallStartEvent{ToolCallID: id, ToolCallName: name}
}

func tcArgs(id, delta string) *agui.ToolCallArgsEvent {
	return &agui.ToolCallArgsEvent{ToolCallID: id, Delta: delta}
}

func tcEnd(id string) *agui.ToolCallEndEvent { return &agui.ToolCallEndEvent{ToolCallID: id} }

func tcResult(msgID, callID, content string) *agui.ToolCallResultEvent {
	return &agui.ToolCallResultEvent{MessageID: msgID, ToolCallID: callID, Content: agui.TextContent(content)}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.EndToolCall_ReturnsFunctionCallContent
func TestCsToolCallBuilderTest_EndToolCall_ReturnsFunctionCallContent(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t, tcStart("call1", "get_weather"), tcArgs("call1", `{"location":"NYC"}`), tcEnd("call1"))
	calls := f.toolCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].ID != "call1" || calls[0].Function.Name != "get_weather" {
		t.Errorf("call = %+v", calls[0])
	}
	if got := parseArgs(t, calls[0].Function.Arguments)["location"]; got != "NYC" {
		t.Errorf("location = %v", got)
	}
	rs := f.delivered[0].(*agui.RunStartedEvent)
	if rs.ThreadID != "thread1" || rs.RunID != "run1" {
		t.Errorf("run ids %s/%s", rs.ThreadID, rs.RunID)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.AppendArgs_AccumulatesMultipleDeltas
func TestCsToolCallBuilderTest_AppendArgs_AccumulatesMultipleDeltas(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "test"), tcArgs("c1", `{"a":`), tcArgs("c1", "1}"), tcEnd("c1"))
	_, tc := f.toolCall(t, "c1")
	if tc.Function.Arguments != `{"a":1}` {
		t.Fatalf("arguments = %q", tc.Function.Arguments)
	}
	if _, ok := parseArgs(t, tc.Function.Arguments)["a"]; !ok {
		t.Fatal("arguments lack a")
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.EndToolCall_ThrowsForUnknownToolCallId
func TestCsToolCallBuilderTest_EndToolCall_ThrowsForUnknownToolCallId(t *testing.T) {
	f := started(t, "t", "r")
	err := f.push(tcEnd("unknown"))
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("want a rejection naming the id, got %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.AppendArgs_ThrowsForUnknownToolCallId
func TestCsToolCallBuilderTest_AppendArgs_ThrowsForUnknownToolCallId(t *testing.T) {
	f := started(t, "t", "r")
	err := f.push(tcArgs("unknown", "test"))
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("want a rejection naming the id, got %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.EndToolCall_WithEmptyArgs_ReturnsNullArguments
//
// .NET: null Arguments. AG-UI: FunctionCall.arguments is a required string; a
// call that received no ARGS has the empty argument text.
func TestCsToolCallBuilderTest_EndToolCall_WithEmptyArgs_ReturnsNullArguments(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "no_args"), tcEnd("c1"))
	calls := f.toolCalls()
	if len(calls) != 1 || calls[0].Function.Name != "no_args" {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].Function.Arguments != "" {
		t.Fatalf("arguments = %q, want empty", calls[0].Function.Arguments)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.MultipleToolCalls_TrackedIndependently
func TestCsToolCallBuilderTest_MultipleToolCalls_TrackedIndependently(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		tcStart("c1", "tool_a"), tcStart("c2", "tool_b"),
		tcArgs("c1", `{"x":1}`), tcArgs("c2", `{"y":2}`),
		tcEnd("c1"), tcEnd("c2"))
	calls := f.toolCalls()
	if len(calls) != 2 || calls[0].Function.Name != "tool_a" || calls[1].Function.Name != "tool_b" {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].Function.Arguments != `{"x":1}` || calls[1].Function.Arguments != `{"y":2}` {
		t.Fatalf("arguments mixed up: %+v", calls)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.InvalidJsonArgs_ThrowsJsonException
//
// Adapted: .NET throws a JsonException when the argument text is not JSON.
// The specification requires the opposite (schema.mdx, FunctionCall.arguments:
// "Kept as written because a model can emit arguments that are not valid JSON,
// and losing them at the protocol boundary would hide the fault from the
// consumer that has to handle it"; TOOL_CALL_ARGS.delta: "the protocol does
// not validate it"). The stream must be accepted and the text kept verbatim.
func TestCsToolCallBuilderTest_InvalidJsonArgs_ThrowsJsonException(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "test"), tcArgs("c1", "not valid json"), tcEnd("c1"))
	if _, tc := f.toolCall(t, "c1"); tc.Function.Arguments != "not valid json" {
		t.Fatalf("arguments = %q", tc.Function.Arguments)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.SequentialToolCalls_CanReuseBuilder
func TestCsToolCallBuilderTest_SequentialToolCalls_CanReuseBuilder(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t,
		tcStart("c1", "tool1"), tcArgs("c1", `{"k":"v1"}`), tcEnd("c1"),
		tcStart("c2", "tool2"), tcArgs("c2", `{"k":"v2"}`), tcEnd("c2"))
	calls := f.toolCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].ID != "c1" || calls[0].Function.Name != "tool1" || calls[1].ID != "c2" || calls[1].Function.Name != "tool2" {
		t.Fatalf("calls = %+v", calls)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.Args_WithNestedJsonObjects_ParsesCorrectly
func TestCsToolCallBuilderTest_Args_WithNestedJsonObjects_ParsesCorrectly(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "test"), tcArgs("c1", `{"name":"test","count":42,"active":true,"tags":null}`), tcEnd("c1"))
	_, tc := f.toolCall(t, "c1")
	args := parseArgs(t, tc.Function.Arguments)
	if args["name"] != "test" || jsonString(args["count"]) != "42" || args["active"] != true {
		t.Fatalf("args = %v", args)
	}
	if v, ok := args["tags"]; !ok || v != nil {
		t.Fatalf("tags = %v (present %v), want an explicit null", v, ok)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.Args_WithJsonArray_ReturnsJsonElement
func TestCsToolCallBuilderTest_Args_WithJsonArray_ReturnsJsonElement(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "test"), tcArgs("c1", `{"items":[1,2,3]}`), tcEnd("c1"))
	_, tc := f.toolCall(t, "c1")
	items, ok := parseArgs(t, tc.Function.Arguments)["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("items = %v", items)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.Args_AsJsonArrayRoot_ReturnsNullArguments
//
// Adapted like InvalidJsonArgs: .NET throws because a root-level array is not
// an arguments dictionary; the protocol carries the text as written.
func TestCsToolCallBuilderTest_Args_AsJsonArrayRoot_ReturnsNullArguments(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "test"), tcArgs("c1", "[1,2,3]"), tcEnd("c1"))
	if _, tc := f.toolCall(t, "c1"); tc.Function.Arguments != "[1,2,3]" {
		t.Fatalf("arguments = %q", tc.Function.Arguments)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.IsBuffering_TrueAfterEndToolCall_FalseAfterFlush
//
// .NET buffers a call's update until its result arrives. Go's analogue of
// "awaiting a result" is Store.PendingToolCallIDs: the run's calls not yet
// answered by TOOL_CALL_RESULT (what a consumer derives when RUN_FINISHED does
// not name them). Go has no update buffer, so a started-but-open call already
// counts as unanswered.
func TestCsToolCallBuilderTest_IsBuffering_TrueAfterEndToolCall_FalseAfterFlush(t *testing.T) {
	f := started(t, "t", "r")
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Fatalf("pending before any call: %v", p)
	}
	f.mustPush(t, tcStart("c1", "tool"), tcEnd("c1"))
	if p := f.store.PendingToolCallIDs(); !eqStrings(p, []string{"c1"}) {
		t.Fatalf("pending after END = %v, want [c1]", p)
	}
	f.mustPush(t, tcResult("res-1", "c1", "ok"))
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Fatalf("pending after the result = %v", p)
	}
}

// resultFollowsCall checks that the tool message answering callID comes after
// the assistant message carrying the call.
func resultFollowsCall(t *testing.T, f *feed, callID string) *agui.ToolMessage {
	t.Helper()
	callAt, resultAt := -1, -1
	var tm *agui.ToolMessage
	for i, m := range f.store.Messages {
		switch x := m.(type) {
		case *agui.AssistantMessage:
			for _, tc := range x.ToolCalls {
				if tc.ID == callID {
					callAt = i
				}
			}
		case *agui.ToolMessage:
			if x.ToolCallID == callID {
				resultAt, tm = i, x
			}
		}
	}
	if callAt < 0 || resultAt < 0 || resultAt < callAt {
		t.Fatalf("call %s at %d, result at %d", callID, callAt, resultAt)
	}
	return tm
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.AddResult_SingleToolCall_FlushesImmediately
func TestCsToolCallBuilderTest_AddResult_SingleToolCall_FlushesImmediately(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t, tcStart("c1", "get_weather"), tcArgs("c1", `{"loc":"NYC"}`), tcEnd("c1"), tcResult("res-1", "c1", "sunny"))
	_, tc := f.toolCall(t, "c1")
	if tc.Function.Name != "get_weather" {
		t.Errorf("call = %+v", tc)
	}
	if tm := resultFollowsCall(t, f, "c1"); tm.Content.String() != "sunny" {
		t.Errorf("result = %q", tm.Content.String())
	}
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Errorf("pending = %v", p)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.AddResult_MultipleToolCalls_FlushesWhenAllResolved
//
// .NET emits FCC(c1), FCC(c2), FRC(c1), FRC(c2) once both results are in. In
// Go both calls are on one assistant message (parentMessageId), and each
// result lands after the call it answers, in arrival order.
func TestCsToolCallBuilderTest_AddResult_MultipleToolCalls_FlushesWhenAllResolved(t *testing.T) {
	f := started(t, "thread1", "run1")
	a := tcStart("c1", "tool_a")
	a.ParentMessageID = agui.Ptr("asst-1")
	b := tcStart("c2", "tool_b")
	b.ParentMessageID = agui.Ptr("asst-1")
	f.mustPush(t, a, tcEnd("c1"), b, tcEnd("c2"), tcResult("res-1", "c1", "result_a"))
	if p := f.store.PendingToolCallIDs(); !eqStrings(p, []string{"c2"}) {
		t.Fatalf("pending after the first result = %v, want [c2]", p)
	}
	f.mustPush(t, tcResult("res-2", "c2", "result_b"))
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Fatalf("pending = %v", p)
	}
	var shape []string
	for _, m := range f.store.Messages {
		switch x := m.(type) {
		case *agui.AssistantMessage:
			for _, tc := range x.ToolCalls {
				shape = append(shape, "call:"+tc.ID)
			}
		case *agui.ToolMessage:
			shape = append(shape, "result:"+x.ToolCallID)
		}
	}
	if want := []string{"call:c1", "call:c2", "result:c1", "result:c2"}; !eqStrings(shape, want) {
		t.Fatalf("store order %v, want %v", shape, want)
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.BufferUpdate_PreservesOrderWithToolCalls
//
// .NET buffers the text update behind the pending call: FCC, text, FRC. The
// Go client delivers events in arrival order (the protocol's order), which is
// that same order here; and the result still attaches to its call.
func TestCsToolCallBuilderTest_BufferUpdate_PreservesOrderWithToolCalls(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t,
		tcStart("c1", "tool"), tcEnd("c1"),
		textStart("m1", agui.TextMessageRoleAssistant), textContent("m1", "thinking..."), textEnd("m1"),
		tcResult("res-1", "c1", "done"))
	var order []string
	for _, e := range f.delivered {
		switch x := e.(type) {
		case *agui.ToolCallEndEvent:
			order = append(order, "call")
		case *agui.TextMessageContentEvent:
			order = append(order, "text:"+x.Delta)
		case *agui.ToolCallResultEvent:
			order = append(order, "result")
		}
	}
	if want := []string{"call", "text:thinking...", "result"}; !eqStrings(order, want) {
		t.Fatalf("delivery order %v, want %v", order, want)
	}
	if tm := resultFollowsCall(t, f, "c1"); tm.Content.String() != "done" {
		t.Errorf("result = %q", tm.Content.String())
	}
}

// C#: tests/AGUI.Client.UnitTests/ToolCallBuilderTest.cs::ToolCallBuilderTest.AddResult_SequentialBatches_FlushIndependently
func TestCsToolCallBuilderTest_AddResult_SequentialBatches_FlushIndependently(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t, tcStart("c1", "tool_a"), tcEnd("c1"), tcResult("res-1", "c1", "result_a"))
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Fatalf("pending after batch 1 = %v", p)
	}
	f.mustPush(t, tcStart("c2", "tool_b"), tcEnd("c2"))
	if p := f.store.PendingToolCallIDs(); !eqStrings(p, []string{"c2"}) {
		t.Fatalf("pending in batch 2 = %v, want [c2]", p)
	}
	f.mustPush(t, tcResult("res-2", "c2", "result_b"))
	if p := f.store.PendingToolCallIDs(); len(p) != 0 {
		t.Fatalf("pending after batch 2 = %v", p)
	}
	resultFollowsCall(t, f, "c1")
	resultFollowsCall(t, f, "c2")
}

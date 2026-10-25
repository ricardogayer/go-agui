package client_rules_test

import (
	"fmt"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ────────────────────────────────────────────────
// Text message lifecycle rules
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_ValidLifecycle_ProducesTextUpdates
func TestCsProtocolRuleTest_TextMessage_ValidLifecycle_ProducesTextUpdates(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "Hello "),
		textContent("m1", "world"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Hello ", "world"})
	m := o.message(t, "m1").(*agui.AssistantMessage)
	eq(t, "assembled", agui.Deref(m.Content), "Hello world")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_ConcurrentWithDifferentIds_Succeeds
func TestCsProtocolRuleTest_TextMessage_ConcurrentWithDifferentIds_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "Hello"),
		textStart("m2"),
		textContent("m2", "World"),
		textEnd("m1"),
		textEnd("m2"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Hello", "World"})
	eq(t, "m1", agui.Deref(o.message(t, "m1").(*agui.AssistantMessage).Content), "Hello")
	eq(t, "m2", agui.Deref(o.message(t, "m2").(*agui.AssistantMessage).Content), "World")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_DuplicateId_Throws
func TestCsProtocolRuleTest_TextMessage_DuplicateId_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), textStart("m1"), textContent("m1", "Hello"), textStart("m1"))
	o.protocolRejected(t, "m1", "already in progress")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_EndForUnstartedId_Throws
func TestCsProtocolRuleTest_TextMessage_EndForUnstartedId_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), textStart("m1"), textContent("m1", "Hello"), textEnd("wrong-id"))
	o.protocolRejected(t, "wrong-id", "No active text message")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_ContentBeforeStart_Throws
func TestCsProtocolRuleTest_TextMessage_ContentBeforeStart_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), textContent("m1", "Hello"))
	o.protocolRejected(t, "m1", "No active text message")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_EndBeforeStart_Throws
func TestCsProtocolRuleTest_TextMessage_EndBeforeStart_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), textEnd("m1"))
	o.protocolRejected(t, "m1", "TEXT_MESSAGE_END")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_SequentialMessages_Succeeds
func TestCsProtocolRuleTest_TextMessage_SequentialMessages_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"), textContent("m1", "First"), textEnd("m1"),
		textStart("m2"), textContent("m2", "Second"), textEnd("m2"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"First", "Second"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.TextMessage_UserRole_PreservesRole
func TestCsProtocolRuleTest_TextMessage_UserRole_PreservesRole(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("TEXT_MESSAGE_START", "messageId", "m1", "role", "user"),
		textContent("m1", "echo"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"echo"})
	u, ok := o.message(t, "m1").(*agui.UserMessage)
	if !ok {
		t.Fatalf("m1 is %T, want a user message", o.message(t, "m1"))
	}
	eq(t, "role", u.MessageRole(), agui.RoleUser)
	eq(t, "content", u.Content.String(), "echo")
}

// ────────────────────────────────────────────────
// Tool call lifecycle rules
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_ValidLifecycle_ProducesFunctionCallContent
func TestCsProtocolRuleTest_ToolCall_ValidLifecycle_ProducesFunctionCallContent(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "get_weather"),
		toolArgs("tc1", `{"city":`),
		toolArgs("tc1", `"NYC"}`),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "get_weather")
	eq(t, "id", calls[0].ID, "tc1")
	eq(t, "city", fmt.Sprint(args(t, calls[0])["city"]), "NYC")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_ConcurrentWithDifferentIds_AllComplete
func TestCsProtocolRuleTest_ToolCall_ConcurrentWithDifferentIds_AllComplete(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "weather"),
		toolStart("tc2", "search"),
		toolArgs("tc1", `{"loc":"NYC"}`),
		toolArgs("tc2", `{"q":"test"}`),
		toolEnd("tc2"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	calls := o.calls()
	eq(t, "tool calls", len(calls), 2)
	byID := map[string]string{}
	for _, c := range calls {
		byID[c.ID] = c.Function.Name
	}
	eq(t, "tc1", byID["tc1"], "weather")
	eq(t, "tc2", byID["tc2"], "search")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_ArgsBeforeStart_Throws
func TestCsProtocolRuleTest_ToolCall_ArgsBeforeStart_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), toolArgs("nonexistent", "{}"))
	o.protocolRejected(t, "nonexistent", "No active tool call")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_EndBeforeStart_Throws
func TestCsProtocolRuleTest_ToolCall_EndBeforeStart_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), toolEnd("nonexistent"))
	o.protocolRejected(t, "nonexistent", "TOOL_CALL_END")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_DuplicateId_Throws
func TestCsProtocolRuleTest_ToolCall_DuplicateId_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), toolStart("tc1", "first"), toolStart("tc1", "second"))
	o.protocolRejected(t, "tc1", "already in progress")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_EmptyArgs_ProducesNullArguments
//
// FunctionCallContent.Arguments == null is MEAI's spelling of "no argument
// text"; the Go tool call carries the empty argument string.
func TestCsProtocolRuleTest_ToolCall_EmptyArgs_ProducesNullArguments(t *testing.T) {
	o := feed(t, runStarted("r1"), toolStart("tc1", "no_args"), toolEnd("tc1"), runFinished("r1"))
	o.accepted(t)
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "no_args")
	eq(t, "arguments", calls[0].Function.Arguments, "")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_SequentialCalls_AllComplete
func TestCsProtocolRuleTest_ToolCall_SequentialCalls_AllComplete(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "first"), toolArgs("tc1", "{}"), toolEnd("tc1"),
		toolStart("tc2", "second"), toolArgs("tc2", "{}"), toolEnd("tc2"),
		runFinished("r1"),
	)
	o.accepted(t)
	var names []string
	for _, c := range o.calls() {
		names = append(names, c.Function.Name)
	}
	eqStrings(t, "tool calls", names, []string{"first", "second"})
}

// ────────────────────────────────────────────────
// Interleaving / nesting rules
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Interleaving_ToolCallDuringTextMessage_BothComplete
func TestCsProtocolRuleTest_Interleaving_ToolCallDuringTextMessage_BothComplete(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "Let me check "),
		toolStart("tc1", "weather"),
		toolArgs("tc1", `{"city":"NYC"}`),
		toolEnd("tc1"),
		textContent("m1", "the weather."),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Let me check ", "the weather."})
	eq(t, "m1", agui.Deref(o.message(t, "m1").(*agui.AssistantMessage).Content), "Let me check the weather.")
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "weather")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Interleaving_TextMessageDuringToolCall_BothComplete
func TestCsProtocolRuleTest_Interleaving_TextMessageDuringToolCall_BothComplete(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search"),
		toolArgs("tc1", `{"q":`),
		textStart("m1"),
		textContent("m1", "Searching..."),
		textEnd("m1"),
		toolArgs("tc1", `"test"}`),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Searching..."})
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "search")
	eq(t, "q", fmt.Sprint(args(t, calls[0])["q"]), "test")
}

// ────────────────────────────────────────────────
// Meta events (allowed in any context)
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MetaEvents_StateSnapshotDuringTextMessage_PassesThrough
func TestCsProtocolRuleTest_MetaEvents_StateSnapshotDuringTextMessage_PassesThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		ev("STATE_SNAPSHOT", "snapshot", raw(`{"count":1}`)),
		textContent("m1", "Hello"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "STATE_SNAPSHOT delivered", o.count(agui.EventTypeStateSnapshot), 1)
	eqStrings(t, "texts", o.texts(), []string{"Hello"})
	// Go keeps a state store (.NET has none): the snapshot replaced the state.
	st, _ := o.agent.Store.State.(map[string]any)
	eq(t, "state.count", fmt.Sprint(st["count"]), "1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MetaEvents_StateDeltaDuringToolCall_PassesThrough
//
// The delta replaces /x in a state that has none. .NET has no state store, so
// it only passes the event through (corpus state-delta-unappliable-warns-and-
// keeps.json, expectOverrides.dotnet). The Go store applies deltas; per the
// spec an unappliable patch warns and keeps the prior state — the stream is
// not failed.
func TestCsProtocolRuleTest_MetaEvents_StateDeltaDuringToolCall_PassesThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "calc"),
		ev("STATE_DELTA", "delta", raw(`[{"op":"replace","path":"/x","value":5}]`)),
		toolArgs("tc1", "{}"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "STATE_DELTA delivered", o.count(agui.EventTypeStateDelta), 1)
	eq(t, "tool calls", len(o.calls()), 1)
	if len(o.warnings) == 0 {
		t.Errorf("the unappliable delta was not announced")
	}
	if o.agent.Store.State != nil {
		t.Errorf("prior (empty) state not kept: %v", o.agent.Store.State)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MetaEvents_CustomEventAnyContext_PassesThrough
//
// The C# CustomEvent{Value = null} serialises "value": null, which is a JSON
// value and so satisfies the required `value`.
func TestCsProtocolRuleTest_MetaEvents_CustomEventAnyContext_PassesThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("CUSTOM", "name", "ui_hint", "value", raw(`{"action":"highlight"}`)),
		textStart("m1"),
		ev("CUSTOM", "name", "progress", "value", nil),
		textContent("m1", "Hi"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	var names []string
	for _, e := range o.of(agui.EventTypeCustom) {
		names = append(names, e.(*agui.CustomEvent).Name)
	}
	eqStrings(t, "CUSTOM names", names, []string{"ui_hint", "progress"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MetaEvents_RawEventDuringToolCall_PassesThrough
func TestCsProtocolRuleTest_MetaEvents_RawEventDuringToolCall_PassesThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "run"),
		ev("RAW", "event", raw(`{"debug":true}`)),
		toolArgs("tc1", "{}"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RAW delivered", o.count(agui.EventTypeRaw), 1)
	eq(t, "tool calls", len(o.calls()), 1)
}

// ────────────────────────────────────────────────
// Reasoning events
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Reasoning_FullLifecycle_ProducesReasoningEvents
//
// The C# events are built with default ids, which serialise as
// "messageId": "" (a valid string); the role of REASONING_MESSAGE_START is the
// fixed "reasoning".
func TestCsProtocolRuleTest_Reasoning_FullLifecycle_ProducesReasoningEvents(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", ""),
		ev("REASONING_MESSAGE_START", "messageId", "", "role", "reasoning"),
		ev("REASONING_MESSAGE_CONTENT", "messageId", "", "delta", "Thinking..."),
		ev("REASONING_MESSAGE_END", "messageId", ""),
		ev("REASONING_END", "messageId", ""),
		textStart("m1"),
		textContent("m1", "Answer"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "REASONING_START", o.count(agui.EventTypeReasoningStart), 1)
	eq(t, "REASONING_END", o.count(agui.EventTypeReasoningEnd), 1)
	contents := o.of(agui.EventTypeReasoningMessageContent)
	if len(contents) != 1 || contents[0].(*agui.ReasoningMessageContentEvent).Delta != "Thinking..." {
		t.Errorf("reasoning content: %v", contents)
	}
	eqStrings(t, "texts", o.texts(), []string{"Answer"})
}

// ────────────────────────────────────────────────
// Activity events
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Activity_SnapshotAndDelta_PassThrough
//
// The C# delta leaves ActivityType unset, which serialises as "" (the field
// is required). .NET has no activity reducer (corpus
// activity-snapshot-then-delta.json, expectOverrides.dotnet); Go materialises
// the activity message and applies the patch, as the spec requires.
func TestCsProtocolRuleTest_Activity_SnapshotAndDelta_PassThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "PLAN", "content", raw(`{"status":"running"}`)),
		ev("ACTIVITY_DELTA", "messageId", "a1", "activityType", "", "patch", raw(`[{"op":"replace","path":"/status","value":"done"}]`)),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "ACTIVITY_SNAPSHOT delivered", o.count(agui.EventTypeActivitySnapshot), 1)
	eq(t, "ACTIVITY_DELTA delivered", o.count(agui.EventTypeActivityDelta), 1)
	a, ok := o.message(t, "a1").(*agui.ActivityMessage)
	if !ok {
		t.Fatalf("a1 is %T", o.message(t, "a1"))
	}
	eq(t, "status", fmt.Sprint(a.Content["status"]), "done")
}

// ────────────────────────────────────────────────
// Complex scenarios
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Complex_FiveConcurrentToolCalls_AllComplete
func TestCsProtocolRuleTest_Complex_FiveConcurrentToolCalls_AllComplete(t *testing.T) {
	evs := []string{runStarted("r1")}
	for i := 0; i < 5; i++ {
		evs = append(evs, toolStart(fmt.Sprintf("tc%d", i), fmt.Sprintf("tool_%d", i)))
	}
	for round := 0; round < 3; round++ {
		for i := 0; i < 5; i++ {
			delta := `{"r":`
			if round == 1 {
				delta = fmt.Sprint(i)
			} else if round == 2 {
				delta = "}"
			}
			evs = append(evs, toolArgs(fmt.Sprintf("tc%d", i), delta))
		}
	}
	for i := 0; i < 5; i++ {
		evs = append(evs, toolEnd(fmt.Sprintf("tc%d", i)))
	}
	evs = append(evs, runFinished("r1"))

	o := feed(t, evs...)
	o.accepted(t)
	calls := o.calls()
	eq(t, "tool calls", len(calls), 5)
	for i := 0; i < 5; i++ {
		var call *agui.ToolCall
		for j := range calls {
			if calls[j].ID == fmt.Sprintf("tc%d", i) {
				call = &calls[j]
			}
		}
		if call == nil {
			t.Fatalf("tc%d missing", i)
		}
		eq(t, "name", call.Function.Name, fmt.Sprintf("tool_%d", i))
		eq(t, "r", fmt.Sprint(args(t, *call)["r"]), fmt.Sprint(i))
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Complex_TextAndToolCallsInterleaved_AllComplete
func TestCsProtocolRuleTest_Complex_TextAndToolCallsInterleaved_AllComplete(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "I'll help. "),
		toolStart("tc1", "search"),
		toolArgs("tc1", `{"q":"info"}`),
		textContent("m1", "Searching..."),
		toolEnd("tc1"),
		textEnd("m1"),
		textStart("m2"),
		textContent("m2", "Found it!"),
		textEnd("m2"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"I'll help. ", "Searching...", "Found it!"})
	var names []string
	for _, c := range o.calls() {
		names = append(names, c.Function.Name)
	}
	eqStrings(t, "tool calls", names, []string{"search"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Complex_MixedEventsFullScenario_ProducesAllOutputs
func TestCsProtocolRuleTest_Complex_MixedEventsFullScenario_ProducesAllOutputs(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("STATE_SNAPSHOT", "snapshot", raw(`{"phase":"init"}`)),
		ev("CUSTOM", "name", "ui", "value", raw(`{"hint":"show_spinner"}`)),
		ev("REASONING_START", "messageId", ""),
		ev("REASONING_MESSAGE_START", "messageId", "", "role", "reasoning"),
		ev("REASONING_MESSAGE_CONTENT", "messageId", "", "delta", "Planning"),
		ev("REASONING_MESSAGE_END", "messageId", ""),
		ev("REASONING_END", "messageId", ""),
		stepStarted("generate"),
		textStart("m1"),
		textContent("m1", "Here"),
		textEnd("m1"),
		stepFinished("generate"),
		toolStart("tc1", "fetch"),
		toolArgs("tc1", "{}"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	for _, et := range []agui.EventType{agui.EventTypeRunStarted, agui.EventTypeStateSnapshot, agui.EventTypeCustom, agui.EventTypeReasoningMessageContent, agui.EventTypeRunFinished} {
		if o.count(et) == 0 {
			t.Errorf("%s not delivered", et)
		}
	}
	eqStrings(t, "texts", o.texts(), []string{"Here"})
	var names []string
	for _, c := range o.calls() {
		names = append(names, c.Function.Name)
	}
	eqStrings(t, "tool calls", names, []string{"fetch"})
}

package capabilities_test

// Port of sdks/python/tests/test_subagent.py.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func by(id string) agui.Attribution { return agui.Attribution{SubagentRunID: p(id)} }

func assertSubagentRunID(t *testing.T, v any, want string) {
	t.Helper()
	d := wire(t, v)
	if d["subagentRunId"] != want {
		t.Fatalf("subagentRunId on the wire = %v, want %q (%v)", d["subagentRunId"], want, d)
	}
	assertNoKey(t, d, "subagent_run_id")
}

// ---------------------------------------------------------------------------
// TestSubagentMessageAttribution
// ---------------------------------------------------------------------------

// Python: tests/test_subagent.py::TestSubagentMessageAttribution::test_assistant_message_accepts_subagent_run_id
func TestPyTestSubagentMessageAttribution_TestAssistantMessageAcceptsSubagentRunId(t *testing.T) {
	msg := &agui.AssistantMessage{ID: "m1", Content: p("hi"), SubagentRunID: p("sub-1")}
	if agui.Deref(msg.SubagentRunID) != "sub-1" {
		t.Fatal("subagentRunId not set")
	}
	assertSubagentRunID(t, msg, "sub-1")
}

// Python: tests/test_subagent.py::TestSubagentMessageAttribution::test_tool_and_reasoning_messages_accept_subagent_run_id
func TestPyTestSubagentMessageAttribution_TestToolAndReasoningMessagesAcceptSubagentRunId(t *testing.T) {
	tool := &agui.ToolMessage{ID: "t1", Content: agui.TextContent("ok"), ToolCallID: "tc1", SubagentRunID: p("sub-2")}
	if agui.Deref(tool.SubagentRunID) != "sub-2" {
		t.Fatal("tool subagentRunId not set")
	}
	assertSubagentRunID(t, tool, "sub-2")
	reasoning := &agui.ReasoningMessage{ID: "r1", Content: "x", SubagentRunID: p("sub-3")}
	if agui.Deref(reasoning.SubagentRunID) != "sub-3" {
		t.Fatal("reasoning subagentRunId not set")
	}
	assertSubagentRunID(t, reasoning, "sub-3")
	for name, m := range map[string]any{"ToolMessage": tool, "ReasoningMessage": reasoning} {
		if err := agui.ValidateValue(name, wire(t, m)); err != nil {
			t.Errorf("schema %s rejects %v: %v", name, wire(t, m), err)
		}
	}
}

// Python: tests/test_subagent.py::TestSubagentMessageAttribution::test_subagent_run_id_optional
func TestPyTestSubagentMessageAttribution_TestSubagentRunIdOptional(t *testing.T) {
	msg := &agui.AssistantMessage{ID: "m2", Content: p("hi")}
	if msg.SubagentRunID != nil {
		t.Fatal("subagentRunId set by default")
	}
	assertNoKey(t, wire(t, msg), "subagentRunId")
}

// ---------------------------------------------------------------------------
// TestSubagentEventAttribution
// ---------------------------------------------------------------------------

// Python: tests/test_subagent.py::TestSubagentEventAttribution::test_creation_and_standalone_events_accept_subagent_run_id
func TestPyTestSubagentEventAttribution_TestCreationAndStandaloneEventsAcceptSubagentRunId(t *testing.T) {
	e := &agui.TextMessageStartEvent{Attribution: by("sub-1"), MessageID: "m1"}
	if agui.Deref(e.SubagentRunID) != "sub-1" || agui.SubagentOf(e) != "sub-1" {
		t.Fatal("TextMessageStart subagentRunId not set")
	}
	assertSubagentRunID(t, e, "sub-1")
	d := &agui.StateDeltaEvent{Attribution: by("sub-2"), Delta: agui.JSONPatch{}}
	if agui.Deref(d.SubagentRunID) != "sub-2" {
		t.Fatal("StateDelta subagentRunId not set")
	}
	assertSubagentRunID(t, d, "sub-2")
}

// Python: tests/test_subagent.py::TestSubagentEventAttribution::test_all_chunk_events_accept_subagent_run_id
func TestPyTestSubagentEventAttribution_TestAllChunkEventsAcceptSubagentRunId(t *testing.T) {
	text := &agui.TextMessageChunkEvent{Attribution: by("sub-7"), MessageID: p("m1")}
	if agui.Deref(text.SubagentRunID) != "sub-7" {
		t.Fatal("text chunk")
	}
	assertSubagentRunID(t, text, "sub-7")
	tool := &agui.ToolCallChunkEvent{Attribution: by("sub-8"), ToolCallID: p("tc1")}
	if agui.Deref(tool.SubagentRunID) != "sub-8" {
		t.Fatal("tool chunk")
	}
	assertSubagentRunID(t, tool, "sub-8")
	reasoning := &agui.ReasoningMessageChunkEvent{Attribution: by("sub-9"), MessageID: p("r1"), Delta: p("thinking")}
	if agui.Deref(reasoning.SubagentRunID) != "sub-9" {
		t.Fatal("reasoning chunk")
	}
	assertSubagentRunID(t, reasoning, "sub-9")
	for _, e := range []agui.Event{text, tool, reasoning} {
		if err := agui.ValidateEvent(e); err != nil {
			t.Errorf("%s: schema rejects attributed chunk: %v", e.Type(), err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestSubagentLifecycleEvents
// ---------------------------------------------------------------------------

// Python: tests/test_subagent.py::TestSubagentLifecycleEvents::test_started_finished_error
func TestPyTestSubagentLifecycleEvents_TestStartedFinishedError(t *testing.T) {
	s := &agui.SubagentStartedEvent{SubagentRunID: "s1", Name: "R", Description: p("d"), ParentSubagentRunID: p("s0")}
	if s.Type() != agui.EventTypeSubagentStarted || agui.Deref(s.ParentSubagentRunID) != "s0" {
		t.Fatalf("unexpected %+v", s)
	}
	sd := wire(t, s)
	if sd["type"] != "SUBAGENT_STARTED" || sd["subagentRunId"] != "s1" || sd["parentSubagentRunId"] != "s0" {
		t.Fatalf("started wire = %v", sd)
	}

	f := &agui.SubagentFinishedEvent{SubagentRunID: "s1"}
	if f.Type() != agui.EventTypeSubagentFinished {
		t.Fatal("finished type")
	}
	fd := wire(t, f)
	if fd["type"] != "SUBAGENT_FINISHED" || fd["subagentRunId"] != "s1" {
		t.Fatalf("finished wire = %v", fd)
	}

	e := &agui.SubagentErrorEvent{SubagentRunID: "s1", Message: "boom", Code: p("E1")}
	if e.Message != "boom" {
		t.Fatal("error message")
	}
	ed := wire(t, e)
	if ed["subagentRunId"] != "s1" || ed["code"] != "E1" {
		t.Fatalf("error wire = %v", ed)
	}
	for _, ev := range []agui.Event{s, f, e} {
		if err := agui.ValidateEvent(ev); err != nil {
			t.Errorf("%s: %v", ev.Type(), err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestSubagentFinishedOutcome
// ---------------------------------------------------------------------------

// Python: tests/test_subagent.py::TestSubagentFinishedOutcome::test_outcome_roundtrip_and_camel_case_wire_form
func TestPyTestSubagentFinishedOutcome_TestOutcomeRoundtripAndCamelCaseWireForm(t *testing.T) {
	suspended := &agui.SubagentFinishedEvent{
		SubagentRunID: "s1",
		Outcome:       &agui.SubagentSuspendedOutcome{InterruptIDs: []string{"int-1"}},
	}
	w := wire(t, suspended)
	assertJSONEqual(t, w["outcome"], generic(t, `{"type": "suspended", "interruptIds": ["int-1"]}`))

	raw, _ := json.Marshal(w)
	var back agui.SubagentFinishedEvent
	mustUnmarshal(t, string(raw), &back)
	so, ok := back.Outcome.(*agui.SubagentSuspendedOutcome)
	if !ok || back.Outcome.OutcomeType() != "suspended" {
		t.Fatalf("outcome = %#v", back.Outcome)
	}
	if !reflect.DeepEqual(so.InterruptIDs, []string{"int-1"}) {
		t.Fatalf("interruptIds = %v", so.InterruptIDs)
	}

	var success agui.SubagentFinishedEvent
	mustUnmarshal(t, `{"type": "SUBAGENT_FINISHED", "subagentRunId": "s1", "outcome": {"type": "success"}}`, &success)
	if _, ok := success.Outcome.(*agui.SubagentSuccessOutcome); !ok {
		t.Fatalf("outcome = %#v, want *SubagentSuccessOutcome", success.Outcome)
	}

	// Legacy: omitted (or null) outcome stays nil.
	var legacy agui.SubagentFinishedEvent
	mustUnmarshal(t, `{"type": "SUBAGENT_FINISHED", "subagentRunId": "s1", "outcome": null}`, &legacy)
	if legacy.Outcome != nil {
		t.Fatalf("outcome = %#v, want nil", legacy.Outcome)
	}
}

// Python: tests/test_subagent.py::TestSubagentFinishedOutcome::test_interrupt_carries_the_raising_subagent
func TestPyTestSubagentFinishedOutcome_TestInterruptCarriesTheRaisingSubagent(t *testing.T) {
	var owned agui.Interrupt
	mustUnmarshal(t, `{"id": "int-1", "reason": "hitl", "subagentRunId": "tools:s1"}`, &owned)
	if agui.Deref(owned.SubagentRunID) != "tools:s1" {
		t.Fatalf("subagentRunId = %v", owned.SubagentRunID)
	}
	if wire(t, owned)["subagentRunId"] != "tools:s1" {
		t.Fatal("subagentRunId lost on the wire")
	}
	var root agui.Interrupt
	mustUnmarshal(t, `{"id": "int-2", "reason": "hitl"}`, &root)
	if root.SubagentRunID != nil {
		t.Fatalf("subagentRunId = %v, want nil", *root.SubagentRunID)
	}
}

package usage_test

// Port of sdks/python/tests/test_run_finished_event.py.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func parseRunFinished(t *testing.T, doc string) *agui.RunFinishedEvent {
	t.Helper()
	ev, err := agui.ParseEvent([]byte(doc))
	if err != nil {
		t.Fatalf("ParseEvent(%s): %v", doc, err)
	}
	e, ok := ev.(*agui.RunFinishedEvent)
	if !ok {
		t.Fatalf("decoded %T, want *RunFinishedEvent", ev)
	}
	return e
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_legacy_event_with_no_outcome
func TestPyRunFinishedEventTest_LegacyEventWithNoOutcome(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1"}
	if e.Outcome != nil {
		t.Fatalf("outcome = %v, want absent", e.Outcome)
	}
	if e.Result != nil {
		t.Fatalf("result = %v, want absent", e.Result)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_legacy_event_with_result_only
func TestPyRunFinishedEventTest_LegacyEventWithResultOnly(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1", Result: map[string]any{"ok": true}}
	if e.Outcome != nil {
		t.Fatalf("outcome = %v, want absent", e.Outcome)
	}
	if !reflect.DeepEqual(e.Result, map[string]any{"ok": true}) {
		t.Fatalf("result = %v", e.Result)
	}
	assertJSON(t, e, `{"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1", "result": {"ok": true}}`)
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_explicit_success_outcome
func TestPyRunFinishedEventTest_ExplicitSuccessOutcome(t *testing.T) {
	e := agui.RunFinishedEvent{
		ThreadID: "t-1", RunID: "r-1",
		Outcome: &agui.SuccessOutcome{},
		Result:  map[string]any{"ok": true},
	}
	if _, ok := e.Outcome.(*agui.SuccessOutcome); !ok {
		t.Fatalf("outcome is %T", e.Outcome)
	}
	if e.Outcome.OutcomeType() != "success" {
		t.Fatalf("outcome type = %q", e.Outcome.OutcomeType())
	}
	if !reflect.DeepEqual(e.Result, map[string]any{"ok": true}) {
		t.Fatalf("result = %v", e.Result)
	}
	if got := object(t, e)["outcome"]; !reflect.DeepEqual(got, map[string]any{"type": "success"}) {
		t.Fatalf("serialized outcome = %v", got)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_explicit_interrupt_outcome
func TestPyRunFinishedEventTest_ExplicitInterruptOutcome(t *testing.T) {
	e := agui.RunFinishedEvent{
		ThreadID: "t-1", RunID: "r-1",
		Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{ID: "int-1", Reason: "tool_call"}}},
	}
	o, ok := e.Outcome.(*agui.InterruptOutcome)
	if !ok {
		t.Fatalf("outcome is %T", e.Outcome)
	}
	if o.OutcomeType() != "interrupt" {
		t.Fatalf("outcome type = %q", o.OutcomeType())
	}
	if len(o.Interrupts) != 1 {
		t.Fatalf("len(interrupts) = %d", len(o.Interrupts))
	}
	if err := agui.ValidateEvent(&e); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_explicit_cancelled_outcome
func TestPyRunFinishedEventTest_ExplicitCancelledOutcome(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1", Outcome: &agui.CancelledOutcome{}}
	if _, ok := e.Outcome.(*agui.CancelledOutcome); !ok {
		t.Fatalf("outcome is %T", e.Outcome)
	}
	if e.Outcome.OutcomeType() != "cancelled" {
		t.Fatalf("outcome type = %q", e.Outcome.OutcomeType())
	}
	if e.Result != nil {
		t.Fatalf("result = %v, want absent", e.Result)
	}
	if got := object(t, e)["outcome"]; !reflect.DeepEqual(got, map[string]any{"type": "cancelled"}) {
		t.Fatalf("serialized outcome = %v", got)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_cancelled_outcome_via_dict_discriminator
func TestPyRunFinishedEventTest_CancelledOutcomeViaDictDiscriminator(t *testing.T) {
	e := parseRunFinished(t, `{"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1", "outcome": {"type": "cancelled"}}`)
	if _, ok := e.Outcome.(*agui.CancelledOutcome); !ok {
		t.Fatalf("outcome is %T, want *CancelledOutcome", e.Outcome)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_cancelled_outcome_is_exported_from_core
//
// Python checks the import path and __all__. The Go equivalent of "exported
// from the package" is that agui.CancelledOutcome is an exported member of the
// RunFinishedOutcome union (checked at compile time below) and that the
// package's outcome decoder resolves the "cancelled" discriminator to it.
func TestPyRunFinishedEventTest_CancelledOutcomeIsExportedFromCore(t *testing.T) {
	var _ agui.RunFinishedOutcome = (*agui.CancelledOutcome)(nil)
	if agui.OutcomeCancelled != "cancelled" {
		t.Fatalf("OutcomeCancelled = %q", agui.OutcomeCancelled)
	}
	o, err := agui.UnmarshalRunFinishedOutcome([]byte(`{"type":"cancelled"}`))
	if err != nil {
		t.Fatalf("UnmarshalRunFinishedOutcome: %v", err)
	}
	if _, ok := o.(*agui.CancelledOutcome); !ok {
		t.Fatalf("decoded %T", o)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_outcome_via_dict_discriminator
func TestPyRunFinishedEventTest_OutcomeViaDictDiscriminator(t *testing.T) {
	e := parseRunFinished(t, `{
		"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1",
		"outcome": {"type": "interrupt", "interrupts": [{"id": "int-1", "reason": "tool_call"}]}
	}`)
	o, ok := e.Outcome.(*agui.InterruptOutcome)
	if !ok {
		t.Fatalf("outcome is %T, want *InterruptOutcome", e.Outcome)
	}
	if len(o.Interrupts) != 1 {
		t.Fatalf("len(interrupts) = %d", len(o.Interrupts))
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_interrupt_outcome_rejects_empty_interrupts
func TestPyRunFinishedEventTest_InterruptOutcomeRejectsEmptyInterrupts(t *testing.T) {
	// Go cannot refuse construction, so the producer path refuses it: the
	// outcome will not marshal, and the schema rejects the document.
	if b, err := json.Marshal(agui.InterruptOutcome{Interrupts: []agui.Interrupt{}}); err == nil {
		t.Fatalf("an interrupt outcome with no interrupts marshaled: %s", b)
	}
	mustReject(t, "RunFinishedInterruptOutcome", `{"type": "interrupt", "interrupts": []}`)
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_interrupt_outcome_via_dict_rejects_empty
func TestPyRunFinishedEventTest_InterruptOutcomeViaDictRejectsEmpty(t *testing.T) {
	doc := `{"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1", "outcome": {"type": "interrupt", "interrupts": []}}`
	mustReject(t, "RunFinishedEvent", doc)
	if _, err := agui.ParseEvent([]byte(doc)); err == nil {
		t.Fatalf("ParseEvent accepted an interrupt outcome with no interrupts")
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_camel_case_serialization
func TestPyRunFinishedEventTest_CamelCaseSerialization(t *testing.T) {
	e := agui.RunFinishedEvent{
		ThreadID: "t-1", RunID: "r-1",
		Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{
			{ID: "int-1", Reason: "tool_call", ToolCallID: agui.Ptr("tc-1")},
		}},
	}
	d := object(t, e)
	if d["threadId"] != "t-1" {
		t.Fatalf("threadId = %v", d["threadId"])
	}
	outcome := d["outcome"].(map[string]any)
	if outcome["type"] != "interrupt" {
		t.Fatalf("outcome.type = %v", outcome["type"])
	}
	first := outcome["interrupts"].([]any)[0].(map[string]any)
	if first["toolCallId"] != "tc-1" {
		t.Fatalf("interrupts[0].toolCallId = %v", first["toolCallId"])
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_success_outcome_with_pending_tool_call_ids
func TestPyRunFinishedEventTest_SuccessOutcomeWithPendingToolCallIds(t *testing.T) {
	e := agui.RunFinishedEvent{
		ThreadID: "t-1", RunID: "r-1",
		Outcome: &agui.SuccessOutcome{PendingToolCallIDs: []string{"tc-1", "tc-2"}},
	}
	o, ok := e.Outcome.(*agui.SuccessOutcome)
	if !ok {
		t.Fatalf("outcome is %T", e.Outcome)
	}
	if !reflect.DeepEqual(o.PendingToolCallIDs, []string{"tc-1", "tc-2"}) {
		t.Fatalf("pendingToolCallIds = %v", o.PendingToolCallIDs)
	}
	assertJSON(t, object(t, e)["outcome"], `{"type": "success", "pendingToolCallIds": ["tc-1", "tc-2"]}`)
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_success_outcome_omits_pending_tool_call_ids_when_unset
func TestPyRunFinishedEventTest_SuccessOutcomeOmitsPendingToolCallIdsWhenUnset(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1", Outcome: &agui.SuccessOutcome{}}
	assertJSON(t, object(t, e)["outcome"], `{"type": "success"}`)
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_pending_tool_call_ids_via_dict
func TestPyRunFinishedEventTest_PendingToolCallIdsViaDict(t *testing.T) {
	e := parseRunFinished(t, `{
		"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1",
		"outcome": {"type": "success", "pendingToolCallIds": ["tc-1"]}
	}`)
	o, ok := e.Outcome.(*agui.SuccessOutcome)
	if !ok {
		t.Fatalf("outcome is %T", e.Outcome)
	}
	if !reflect.DeepEqual(o.PendingToolCallIDs, []string{"tc-1"}) {
		t.Fatalf("pendingToolCallIds = %v", o.PendingToolCallIDs)
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_pending_tool_call_ids_rejects_non_string_items
func TestPyRunFinishedEventTest_PendingToolCallIdsRejectsNonStringItems(t *testing.T) {
	doc := `{"type": "RUN_FINISHED", "threadId": "t-1", "runId": "r-1", "outcome": {"type": "success", "pendingToolCallIds": [42]}}`
	mustReject(t, "RunFinishedEvent", doc)
	if _, err := agui.UnmarshalEvent([]byte(doc)); err == nil {
		t.Fatalf("typed decode accepted a non-string pending tool call id")
	}
}

// Python: tests/test_run_finished_event.py::RunFinishedEventTest::test_legacy_event_serialization_omits_outcome
func TestPyRunFinishedEventTest_LegacyEventSerializationOmitsOutcome(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1"}
	if _, ok := object(t, e)["outcome"]; ok {
		t.Fatalf("outcome serialized for a legacy event")
	}
}

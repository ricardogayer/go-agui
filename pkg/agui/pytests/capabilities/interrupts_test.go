package capabilities_test

// Port of sdks/python/tests/test_interrupts.py.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func roundTripResume(t *testing.T, r agui.ResumeEntry) agui.ResumeEntry {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out agui.ResumeEntry
	mustUnmarshal(t, string(raw), &out)
	return out
}

// ---------------------------------------------------------------------------
// InterruptTest
// ---------------------------------------------------------------------------

// Python: tests/test_interrupts.py::InterruptTest::test_required_fields_only
func TestPyInterruptTest_TestRequiredFieldsOnly(t *testing.T) {
	i := agui.Interrupt{ID: "int-1", Reason: "tool_call"}
	if i.ID != "int-1" || i.Reason != "tool_call" || i.Message != nil || i.ToolCallID != nil {
		t.Fatalf("unexpected %+v", i)
	}
	if err := agui.ValidateValue("Interrupt", wire(t, i)); err != nil {
		t.Fatalf("schema rejects required-only interrupt: %v", err)
	}
}

// Python: tests/test_interrupts.py::InterruptTest::test_all_optional_fields
func TestPyInterruptTest_TestAllOptionalFields(t *testing.T) {
	i := agui.Interrupt{
		ID:             "int-1",
		Reason:         "input_required",
		Message:        p("Approve?"),
		ToolCallID:     p("tc-1"),
		ResponseSchema: map[string]any{"type": "object"},
		ExpiresAt:      p("2099-01-01T00:00:00Z"),
		Metadata:       agui.Metadata{"foo": "bar"},
	}
	if agui.Deref(i.ToolCallID) != "tc-1" {
		t.Fatalf("toolCallId = %v", i.ToolCallID)
	}
	if !reflect.DeepEqual(i.ResponseSchema, map[string]any{"type": "object"}) {
		t.Fatalf("responseSchema = %v", i.ResponseSchema)
	}
	if err := agui.ValidateValue("Interrupt", wire(t, i)); err != nil {
		t.Fatalf("schema rejects fully populated interrupt: %v", err)
	}
}

// Python: tests/test_interrupts.py::InterruptTest::test_alias_camel_case_on_serialization
func TestPyInterruptTest_TestAliasCamelCaseOnSerialization(t *testing.T) {
	d := wire(t, agui.Interrupt{ID: "int-1", Reason: "tool_call", ToolCallID: p("tc-1")})
	assertHasKey(t, d, "toolCallId")
	assertNoKey(t, d, "tool_call_id")
}

// Python: tests/test_interrupts.py::InterruptTest::test_parse_from_camel_case
func TestPyInterruptTest_TestParseFromCamelCase(t *testing.T) {
	var i agui.Interrupt
	mustUnmarshal(t, `{"id": "int-1", "reason": "tool_call", "toolCallId": "tc-1"}`, &i)
	if agui.Deref(i.ToolCallID) != "tc-1" {
		t.Fatalf("toolCallId = %v", i.ToolCallID)
	}
}

// Python: tests/test_interrupts.py::InterruptTest::test_rejects_missing_id
func TestPyInterruptTest_TestRejectsMissingId(t *testing.T) {
	if err := agui.Validate("Interrupt", []byte(`{"reason": "tool_call"}`)); err == nil {
		t.Fatal("interrupt without id accepted")
	}
}

// Python: tests/test_interrupts.py::InterruptTest::test_rejects_missing_reason
func TestPyInterruptTest_TestRejectsMissingReason(t *testing.T) {
	if err := agui.Validate("Interrupt", []byte(`{"id": "int-1"}`)); err == nil {
		t.Fatal("interrupt without reason accepted")
	}
}

// ---------------------------------------------------------------------------
// ResumeEntryTest
// ---------------------------------------------------------------------------

// Python: tests/test_interrupts.py::ResumeEntryTest::test_resolved_with_payload
func TestPyResumeEntryTest_TestResolvedWithPayload(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}}
	if r.Status != "resolved" {
		t.Fatalf("status = %q", r.Status)
	}
	if !reflect.DeepEqual(r.Payload, map[string]any{"approved": true}) {
		t.Fatalf("payload = %v", r.Payload)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryTest::test_cancelled_without_payload
func TestPyResumeEntryTest_TestCancelledWithoutPayload(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusCancelled}
	if r.Status != "cancelled" || r.Payload != nil {
		t.Fatalf("unexpected %+v", r)
	}
	if err := agui.ValidateValue("ResumeEntry", wire(t, r)); err != nil {
		t.Fatalf("schema rejects cancelled entry: %v", err)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryTest::test_rejects_unknown_status
func TestPyResumeEntryTest_TestRejectsUnknownStatus(t *testing.T) {
	if err := agui.Validate("ResumeEntry", []byte(`{"interruptId": "int-1", "status": "denied"}`)); err == nil {
		t.Fatal("status \"denied\" accepted")
	}
}

// Python: tests/test_interrupts.py::ResumeEntryTest::test_rejects_missing_interrupt_id
func TestPyResumeEntryTest_TestRejectsMissingInterruptId(t *testing.T) {
	if err := agui.Validate("ResumeEntry", []byte(`{"status": "resolved"}`)); err == nil {
		t.Fatal("resume entry without interruptId accepted")
	}
}

// Python: tests/test_interrupts.py::ResumeEntryTest::test_alias_camel_case_on_serialization
func TestPyResumeEntryTest_TestAliasCamelCaseOnSerialization(t *testing.T) {
	d := wire(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}})
	assertHasKey(t, d, "interruptId")
	assertNoKey(t, d, "interrupt_id")
}

// Python: tests/test_interrupts.py::ResumeEntryTest::test_parse_from_camel_case
func TestPyResumeEntryTest_TestParseFromCamelCase(t *testing.T) {
	var r agui.ResumeEntry
	mustUnmarshal(t, `{"interruptId": "int-1", "status": "cancelled"}`, &r)
	if r.InterruptID != "int-1" || r.Status != agui.ResumeStatusCancelled {
		t.Fatalf("unexpected %+v", r)
	}
}

// ---------------------------------------------------------------------------
// ResumeEntryMetadataTest
// ---------------------------------------------------------------------------

const valueShapes = `{
  "nullValue": null,
  "string": "afterModel-review",
  "number": 42,
  "float": 1.5,
  "boolean": true,
  "emptyArray": [],
  "array": [1, "two", null, {"nested": true}],
  "emptyObject": {},
  "nested": {"signature": {"alg": "ed25519", "hash": "abc"}, "tags": ["a", "b"]}
}`

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_absent_by_default
func TestPyResumeEntryMetadataTest_TestAbsentByDefault(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved}
	if r.Metadata != nil {
		t.Fatalf("metadata = %v", r.Metadata)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_json_round_trip_of_every_value_shape
func TestPyResumeEntryMetadataTest_TestJsonRoundTripOfEveryValueShape(t *testing.T) {
	shapes := agui.Metadata(obj(t, generic(t, valueShapes)))
	restored := roundTripResume(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Metadata: shapes})
	if !reflect.DeepEqual(restored.Metadata, agui.Metadata(obj(t, generic(t, valueShapes)))) {
		t.Fatalf("metadata changed: %v", restored.Metadata)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_empty_object_round_trips_distinct_from_absent
func TestPyResumeEntryMetadataTest_TestEmptyObjectRoundTripsDistinctFromAbsent(t *testing.T) {
	restored := roundTripResume(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Metadata: agui.Metadata{}})
	if restored.Metadata == nil || len(restored.Metadata) != 0 {
		t.Fatalf("metadata = %#v, want empty non-nil map", restored.Metadata)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_explicit_null_reads_back_as_absent
func TestPyResumeEntryMetadataTest_TestExplicitNullReadsBackAsAbsent(t *testing.T) {
	var r agui.ResumeEntry
	mustUnmarshal(t, `{"interruptId": "int-1", "status": "resolved", "metadata": null}`, &r)
	if r.Metadata != nil {
		t.Fatalf("metadata = %#v", r.Metadata)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_plain_model_dump_json_round_trips
func TestPyResumeEntryMetadataTest_TestPlainModelDumpJsonRoundTrips(t *testing.T) {
	restored := roundTripResume(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusCancelled})
	if restored.Metadata != nil {
		t.Fatalf("metadata = %#v", restored.Metadata)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_absent_metadata_serializes_without_the_key
func TestPyResumeEntryMetadataTest_TestAbsentMetadataSerializesWithoutTheKey(t *testing.T) {
	d := wire(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}})
	assertNoKey(t, d, "metadata")
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_exclude_none_preserves_null_values_under_keys
func TestPyResumeEntryMetadataTest_TestExcludeNonePreservesNullValuesUnderKeys(t *testing.T) {
	d := wire(t, agui.ResumeEntry{
		InterruptID: "int-1",
		Status:      agui.ResumeStatusResolved,
		Metadata:    agui.Metadata{"source": "ui", "nullValue": nil},
	})
	assertJSONEqual(t, d["metadata"], generic(t, `{"source": "ui", "nullValue": null}`))
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_carried_on_cancelled_entries_too
func TestPyResumeEntryMetadataTest_TestCarriedOnCancelledEntriesToo(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusCancelled, Metadata: agui.Metadata{"reason": "timeout"}}
	if !reflect.DeepEqual(r.Metadata, agui.Metadata{"reason": "timeout"}) {
		t.Fatalf("metadata = %v", r.Metadata)
	}
	if err := agui.ValidateValue("ResumeEntry", wire(t, r)); err != nil {
		t.Fatalf("schema rejects cancelled entry with metadata: %v", err)
	}
}

// Python: tests/test_interrupts.py::ResumeEntryMetadataTest::test_reaches_the_agent_through_run_agent_input
func TestPyResumeEntryMetadataTest_TestReachesTheAgentThroughRunAgentInput(t *testing.T) {
	var in agui.RunAgentInput
	mustUnmarshal(t, `{
		"threadId": "t-1",
		"runId": "r-1",
		"state": {},
		"messages": [],
		"tools": [],
		"context": [],
		"forwardedProps": {},
		"resume": [{
			"interruptId": "generic-1",
			"status": "resolved",
			"payload": {"approved": true},
			"metadata": {"ag-ui": {}, "definitionId": "review-plan", "key": "afterModel-review"}
		}]
	}`, &in)
	if len(in.Resume) != 1 {
		t.Fatalf("resume = %+v", in.Resume)
	}
	want := agui.Metadata{"ag-ui": map[string]any{}, "definitionId": "review-plan", "key": "afterModel-review"}
	if !reflect.DeepEqual(in.Resume[0].Metadata, want) {
		t.Fatalf("metadata = %v", in.Resume[0].Metadata)
	}
}

// ---------------------------------------------------------------------------
// RunAgentInputResumeTest
// ---------------------------------------------------------------------------

func baseInput() agui.RunAgentInput {
	return agui.RunAgentInput{
		ThreadID:       "t-1",
		RunID:          "r-1",
		State:          map[string]any{},
		Messages:       agui.Messages{},
		Tools:          []agui.Tool{},
		Context:        []agui.Context{},
		ForwardedProps: map[string]any{},
	}
}

// Python: tests/test_interrupts.py::RunAgentInputResumeTest::test_without_resume
func TestPyRunAgentInputResumeTest_TestWithoutResume(t *testing.T) {
	in := baseInput()
	if in.Resume != nil {
		t.Fatalf("resume = %v", in.Resume)
	}
	assertNoKey(t, wire(t, in), "resume")
}

// Python: tests/test_interrupts.py::RunAgentInputResumeTest::test_with_resume
func TestPyRunAgentInputResumeTest_TestWithResume(t *testing.T) {
	in := baseInput()
	in.Resume = []agui.ResumeEntry{
		{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}},
		{InterruptID: "int-2", Status: agui.ResumeStatusCancelled},
	}
	if len(in.Resume) != 2 || in.Resume[0].Status != "resolved" {
		t.Fatalf("resume = %+v", in.Resume)
	}
	if err := agui.ValidateValue("RunAgentInput", wire(t, in)); err != nil {
		t.Fatalf("schema rejects run input with resume: %v", err)
	}
}

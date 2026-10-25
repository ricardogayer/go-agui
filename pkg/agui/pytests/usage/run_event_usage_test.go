package usage_test

// Port of sdks/python/tests/test_run_event_usage.py.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ---------------------------------------------------------------------------
// Shared helpers for the usage_test package.
// ---------------------------------------------------------------------------

// generic marshals v with encoding/json (the Go equivalent of
// model_dump(by_alias=True)) and decodes it back into plain JSON values.
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return parse(t, string(b))
}

// parse decodes a JSON document into plain JSON values.
func parse(t *testing.T, doc string) any {
	t.Helper()
	var out any
	if err := json.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("decode %s: %v", doc, err)
	}
	return out
}

// object is generic(v) asserted to be a JSON object.
func object(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := generic(t, v).(map[string]any)
	if !ok {
		t.Fatalf("%T did not marshal to a JSON object", v)
	}
	return m
}

// assertJSON checks that v marshals to exactly the JSON document want (same
// keys present and absent, same values).
func assertJSON(t *testing.T, v any, want string) {
	t.Helper()
	got := generic(t, v)
	exp := parse(t, want)
	if !reflect.DeepEqual(got, exp) {
		gb, _ := json.Marshal(got)
		t.Fatalf("serialized JSON mismatch\n got: %s\nwant: %s", gb, want)
	}
}

// mustValidate asserts that the schema accepts doc as definition.
func mustValidate(t *testing.T, definition, doc string) {
	t.Helper()
	if err := agui.Validate(definition, []byte(doc)); err != nil {
		t.Fatalf("Validate(%q, %s) rejected a valid document: %v", definition, doc, err)
	}
}

// mustReject asserts that the strict schema check rejects doc as definition
// (the Go counterpart of a Pydantic ValidationError).
func mustReject(t *testing.T, definition, doc string) {
	t.Helper()
	if err := agui.Validate(definition, []byte(doc)); err == nil {
		t.Fatalf("Validate(%q, %s) accepted a document the protocol forbids", definition, doc)
	}
}

// countFields are the five count fields TokenUsageCountConstraintsTest walks,
// in wire spelling, each with an accessor on the Go struct.
var countFields = []struct {
	wire string
	get  func(*agui.TokenUsage) *int64
	set  func(*agui.TokenUsage, int64)
}{
	{"inputTokens", func(u *agui.TokenUsage) *int64 { return u.InputTokens }, func(u *agui.TokenUsage, v int64) { u.InputTokens = &v }},
	{"outputTokens", func(u *agui.TokenUsage) *int64 { return u.OutputTokens }, func(u *agui.TokenUsage, v int64) { u.OutputTokens = &v }},
	{"totalTokens", func(u *agui.TokenUsage) *int64 { return u.TotalTokens }, func(u *agui.TokenUsage, v int64) { u.TotalTokens = &v }},
	{"reasoningTokens", func(u *agui.TokenUsage) *int64 { return u.ReasoningTokens }, func(u *agui.TokenUsage, v int64) { u.ReasoningTokens = &v }},
	{"cachedInputTokens", func(u *agui.TokenUsage) *int64 { return u.CachedInputTokens }, func(u *agui.TokenUsage, v int64) { u.CachedInputTokens = &v }},
}

// ---------------------------------------------------------------------------
// TokenUsageTest
// ---------------------------------------------------------------------------

// Python: tests/test_run_event_usage.py::TokenUsageTest::test_full_entry
func TestPyTokenUsageTest_FullEntry(t *testing.T) {
	u := agui.TokenUsage{
		Provider:          agui.Ptr("anthropic"),
		Model:             agui.Ptr("claude-sonnet-4"),
		InputTokens:       agui.Ptr[int64](100),
		OutputTokens:      agui.Ptr[int64](50),
		TotalTokens:       agui.Ptr[int64](150),
		ReasoningTokens:   agui.Ptr[int64](20),
		CachedInputTokens: agui.Ptr[int64](10),
	}
	if agui.Deref(u.Provider) != "anthropic" {
		t.Fatalf("provider = %v", u.Provider)
	}
	if u.TotalTokens == nil || *u.TotalTokens != 150 {
		t.Fatalf("totalTokens = %v", u.TotalTokens)
	}
	b, _ := json.Marshal(u)
	mustValidate(t, "TokenUsage", string(b))
}

// Python: tests/test_run_event_usage.py::TokenUsageTest::test_all_fields_optional
func TestPyTokenUsageTest_AllFieldsOptional(t *testing.T) {
	var u agui.TokenUsage
	if u.InputTokens != nil {
		t.Fatalf("inputTokens = %v, want absent", *u.InputTokens)
	}
	// The empty entry is a valid TokenUsage on the wire.
	assertJSON(t, u, `{}`)
	mustValidate(t, "TokenUsage", `{}`)
}

// Python: tests/test_run_event_usage.py::TokenUsageTest::test_camel_case_serialization
func TestPyTokenUsageTest_CamelCaseSerialization(t *testing.T) {
	u := agui.TokenUsage{InputTokens: agui.Ptr[int64](5), CachedInputTokens: agui.Ptr[int64](2)}
	assertJSON(t, u, `{"inputTokens": 5, "cachedInputTokens": 2}`)
}

// Python: tests/test_run_event_usage.py::TokenUsageTest::test_rejects_non_numeric
func TestPyTokenUsageTest_RejectsNonNumeric(t *testing.T) {
	doc := `{"inputTokens": "lots"}`
	mustReject(t, "TokenUsage", doc)
	var u agui.TokenUsage
	if err := json.Unmarshal([]byte(doc), &u); err == nil {
		t.Fatalf("typed decode accepted a string count: %+v", u)
	}
}

// ---------------------------------------------------------------------------
// RunFinishedUsageTest
// ---------------------------------------------------------------------------

// Python: tests/test_run_event_usage.py::RunFinishedUsageTest::test_legacy_event_has_no_usage
func TestPyRunFinishedUsageTest_LegacyEventHasNoUsage(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1"}
	if e.Usage != nil {
		t.Fatalf("usage = %v, want absent", e.Usage)
	}
}

// Python: tests/test_run_event_usage.py::RunFinishedUsageTest::test_usage_from_camel_case_dict
func TestPyRunFinishedUsageTest_UsageFromCamelCaseDict(t *testing.T) {
	doc := `{
		"type": "RUN_FINISHED",
		"threadId": "t-1",
		"runId": "r-1",
		"usage": [
			{"provider": "openai", "model": "gpt-4o", "inputTokens": 100, "totalTokens": 120},
			{"provider": "openai", "model": "gpt-4o-mini", "inputTokens": 10}
		]
	}`
	ev, err := agui.ParseEvent([]byte(doc))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	e, ok := ev.(*agui.RunFinishedEvent)
	if !ok {
		t.Fatalf("decoded %T, want *RunFinishedEvent", ev)
	}
	if len(e.Usage) != 2 {
		t.Fatalf("len(usage) = %d, want 2", len(e.Usage))
	}
	if agui.Deref(e.Usage[1].Model) != "gpt-4o-mini" {
		t.Fatalf("usage[1].model = %v", e.Usage[1].Model)
	}
}

// Python: tests/test_run_event_usage.py::RunFinishedUsageTest::test_legacy_serialization_omits_usage
func TestPyRunFinishedUsageTest_LegacySerializationOmitsUsage(t *testing.T) {
	e := agui.RunFinishedEvent{ThreadID: "t-1", RunID: "r-1"}
	if _, ok := object(t, e)["usage"]; ok {
		t.Fatalf("usage present in %v", object(t, e))
	}
}

// ---------------------------------------------------------------------------
// RunErrorUsageTest
// ---------------------------------------------------------------------------

// Python: tests/test_run_event_usage.py::RunErrorUsageTest::test_no_usage
func TestPyRunErrorUsageTest_NoUsage(t *testing.T) {
	e := agui.RunErrorEvent{Message: "boom"}
	if e.Usage != nil {
		t.Fatalf("usage = %v, want absent", e.Usage)
	}
	if _, ok := object(t, e)["usage"]; ok {
		t.Fatalf("usage serialized for an event without usage")
	}
}

// Python: tests/test_run_event_usage.py::RunErrorUsageTest::test_partial_usage_on_failure
func TestPyRunErrorUsageTest_PartialUsageOnFailure(t *testing.T) {
	doc := `{"type": "RUN_ERROR", "message": "boom", "usage": [{"provider": "anthropic", "inputTokens": 100}]}`
	ev, err := agui.ParseEvent([]byte(doc))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	e := ev.(*agui.RunErrorEvent)
	if len(e.Usage) != 1 || e.Usage[0].InputTokens == nil || *e.Usage[0].InputTokens != 100 {
		t.Fatalf("usage = %+v", e.Usage)
	}
}

// ---------------------------------------------------------------------------
// TokenUsageCountConstraintsTest
// ---------------------------------------------------------------------------

// Python: tests/test_run_event_usage.py::TokenUsageCountConstraintsTest::test_rejects_negative_counts
//
// Go's int64 can hold a negative count, so the constructor cannot refuse it;
// the producer-side guard is schema validation, which stream.SSE.Send runs on
// every event. Both the bare entry and an event carrying it must be refused.
func TestPyTokenUsageCountConstraintsTest_RejectsNegativeCounts(t *testing.T) {
	for _, f := range countFields {
		t.Run(f.wire, func(t *testing.T) {
			mustReject(t, "TokenUsage", `{"`+f.wire+`": -1}`)
			var u agui.TokenUsage
			f.set(&u, -1)
			ev := &agui.RunFinishedEvent{ThreadID: "t", RunID: "r", Usage: []agui.TokenUsage{u}}
			if err := agui.ValidateEvent(ev); err == nil {
				t.Fatalf("ValidateEvent accepted %s = -1", f.wire)
			}
		})
	}
}

// Python: tests/test_run_event_usage.py::TokenUsageCountConstraintsTest::test_accepts_zero_and_positive_counts
func TestPyTokenUsageCountConstraintsTest_AcceptsZeroAndPositiveCounts(t *testing.T) {
	for _, f := range countFields {
		t.Run(f.wire, func(t *testing.T) {
			for _, n := range []int64{0, 7} {
				doc := `{"` + f.wire + `": ` + itoa(n) + `}`
				mustValidate(t, "TokenUsage", doc)
				var u agui.TokenUsage
				if err := json.Unmarshal([]byte(doc), &u); err != nil {
					t.Fatalf("decode %s: %v", doc, err)
				}
				if got := f.get(&u); got == nil || *got != n {
					t.Fatalf("%s = %v, want %d", f.wire, got, n)
				}
			}
		})
	}
}

// Python: tests/test_run_event_usage.py::TokenUsageCountConstraintsTest::test_still_rejects_fractional_counts
func TestPyTokenUsageCountConstraintsTest_StillRejectsFractionalCounts(t *testing.T) {
	mustReject(t, "TokenUsage", `{"inputTokens": 1.5}`)
}

// Python: tests/test_run_event_usage.py::TokenUsageCountConstraintsTest::test_counts_remain_optional
func TestPyTokenUsageCountConstraintsTest_CountsRemainOptional(t *testing.T) {
	u := agui.TokenUsage{Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o")}
	for _, f := range countFields {
		if got := f.get(&u); got != nil {
			t.Fatalf("%s = %d, want absent", f.wire, *got)
		}
	}
	mustValidate(t, "TokenUsage", `{"provider": "openai", "model": "gpt-4o"}`)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

package usage_test

// Port of sdks/python/tests/test_token_usage.py.
//
// The Python file tests two SDK helpers, token_usage_from_langchain_metadata
// (a vendor-metadata -> TokenUsage mapper) and aggregate_token_usage. The Go
// library has neither (grep: no mapper or aggregator in pkg/agui, pkg/client,
// pkg/adapter, pkg/stream, pkg/server), so tests that exercise only that
// mapping or aggregation logic are skipped as N/A. Where a test pins a
// protocol rule that the Go library does enforce — which values a TokenUsage
// count may hold, the wire's safe-integer bound, that an absent count differs
// from zero, that the entry carries no content — the Go equivalent is tested:
// the Go producer's guard is the schema check stream.SSE.Send runs on every
// event (agui.Validate / agui.ValidateEvent), and the consumer's is the
// client pipeline.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

const naMapper = "N/A in Go: the Go library has no vendor-metadata -> TokenUsage mapper (Python token_usage_from_langchain_metadata)"
const naAggregate = "N/A in Go: the Go library has no TokenUsage aggregation helper (Python aggregate_token_usage)"

// ---------------------------------------------------------------------------
// TokenUsageFromLangChainMetadataTest
// ---------------------------------------------------------------------------

// Python: tests/test_token_usage.py::TokenUsageFromLangChainMetadataTest::test_maps_core_and_detail_fields
func TestPyTokenUsageFromLangChainMetadataTest_MapsCoreAndDetailFields(t *testing.T) {
	t.Skip(naMapper)
}

// Python: tests/test_token_usage.py::TokenUsageFromLangChainMetadataTest::test_returns_none_for_missing_or_empty_metadata
func TestPyTokenUsageFromLangChainMetadataTest_ReturnsNoneForMissingOrEmptyMetadata(t *testing.T) {
	t.Skip(naMapper)
}

// Python: tests/test_token_usage.py::TokenUsageFromLangChainMetadataTest::test_omits_absent_fields_entirely
//
// The mapper half is N/A; the serialization half — an entry with one count
// serializes to exactly that count — is the Go type's job.
func TestPyTokenUsageFromLangChainMetadataTest_OmitsAbsentFieldsEntirely(t *testing.T) {
	assertJSON(t, agui.TokenUsage{InputTokens: agui.Ptr[int64](5)}, `{"inputTokens": 5}`)
}

// Python: tests/test_token_usage.py::TokenUsageFromLangChainMetadataTest::test_reads_attribute_shaped_metadata
func TestPyTokenUsageFromLangChainMetadataTest_ReadsAttributeShapedMetadata(t *testing.T) {
	t.Skip(naMapper + "; object-shaped (attribute) vendor payloads are a Python duck-typing concern")
}

// ---------------------------------------------------------------------------
// TokenUsageGuardTest — Go equivalent: the value is refused on the wire
// (strict schema check), while the entry without it stays valid.
// ---------------------------------------------------------------------------

// rejectedCount asserts the TokenUsage document `bad` is refused by the
// schema and by typed decoding, and that `good` (the same entry with the bad
// count left out, which is what the Python mapper produces) is valid.
func rejectedCount(t *testing.T, bad, good string) {
	t.Helper()
	mustReject(t, "TokenUsage", bad)
	var u agui.TokenUsage
	if err := json.Unmarshal([]byte(bad), &u); err == nil {
		// A typed decode that silently accepts is only a problem when the
		// strict check would not catch it; ParseEvent always runs both.
		ev := `{"type":"RUN_FINISHED","threadId":"t","runId":"r","usage":[` + bad + `]}`
		if _, err := agui.ParseEvent([]byte(ev)); err == nil {
			t.Fatalf("ParseEvent accepted usage %s", bad)
		}
	}
	if good != "" {
		mustValidate(t, "TokenUsage", good)
	}
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_drops_string_counts
func TestPyTokenUsageGuardTest_DropsStringCounts(t *testing.T) {
	rejectedCount(t, `{"provider": "openai", "inputTokens": "100", "outputTokens": 5}`,
		`{"provider": "openai", "outputTokens": 5}`)
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_drops_none_counts
func TestPyTokenUsageGuardTest_DropsNoneCounts(t *testing.T) {
	// A whole-field null is absence spelled wrong (basic/index.mdx "Absent
	// means absent"); inputTokens is not in the legacy-null registry.
	rejectedCount(t, `{"inputTokens": null, "outputTokens": 5}`, `{"outputTokens": 5}`)
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_drops_nan_and_infinity
func TestPyTokenUsageGuardTest_DropsNanAndInfinity(t *testing.T) {
	t.Skip("N/A in Go: NaN and Infinity cannot be held by the *int64 count fields nor written as JSON numbers, so there is nothing to drop; the mapper that drops them has no Go counterpart")
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_drops_negative_and_fractional_counts
func TestPyTokenUsageGuardTest_DropsNegativeAndFractionalCounts(t *testing.T) {
	rejectedCount(t, `{"inputTokens": -1, "totalTokens": 4}`, `{"totalTokens": 4}`)
	rejectedCount(t, `{"outputTokens": 1.5, "totalTokens": 4}`, `{"totalTokens": 4}`)
	// The Go producer cannot write a fractional count (int64), but it can
	// write a negative one; the producer-side check refuses it.
	ev := &agui.RunFinishedEvent{ThreadID: "t", RunID: "r", Usage: []agui.TokenUsage{
		{InputTokens: agui.Ptr[int64](-1), TotalTokens: agui.Ptr[int64](4)},
	}}
	if err := agui.ValidateEvent(ev); err == nil {
		t.Fatalf("ValidateEvent accepted a negative count")
	}
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_drops_booleans
func TestPyTokenUsageGuardTest_DropsBooleans(t *testing.T) {
	rejectedCount(t, `{"inputTokens": true}`, "")
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_accepts_integral_float
//
// Go equivalent: 12.0 is an integer under the schema (JSON Schema "integer"
// is any number with a zero fractional part), so a consumer must accept an
// event carrying it and read the count as 12.
func TestPyTokenUsageGuardTest_AcceptsIntegralFloat(t *testing.T) {
	mustValidate(t, "TokenUsage", `{"inputTokens": 12.0}`)
	p := &client.Pipeline{Store: client.NewStore(nil, nil)}
	if _, err := p.Push([]byte(`{"type":"RUN_STARTED","threadId":"t","runId":"r"}`)); err != nil {
		t.Fatalf("RUN_STARTED: %v", err)
	}
	evs, err := p.Push([]byte(`{"type":"RUN_FINISHED","threadId":"t","runId":"r","usage":[{"inputTokens":12.0}]}`))
	if err != nil {
		t.Fatalf("consumer rejected a schema-valid RUN_FINISHED with inputTokens 12.0: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("delivered %d events", len(evs))
	}
	u := evs[0].(*agui.RunFinishedEvent).Usage
	if len(u) != 1 || u[0].InputTokens == nil || *u[0].InputTokens != 12 {
		t.Fatalf("usage = %+v, want inputTokens 12", u)
	}
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_an_integer_too_large_to_be_a_float_is_dropped_not_raised
func TestPyTokenUsageGuardTest_AnIntegerTooLargeToBeAFloatIsDroppedNotRaised(t *testing.T) {
	huge := "1" + strings.Repeat("0", 1000) // 10**1000
	rejectedCount(t, `{"inputTokens": `+huge+`, "outputTokens": 5}`, `{"outputTokens": 5}`)
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_a_huge_integer_in_every_slot_is_dropped_not_raised
func TestPyTokenUsageGuardTest_AHugeIntegerInEverySlotIsDroppedNotRaised(t *testing.T) {
	pow := func(n int) string { return "1" + strings.Repeat("0", n) }
	for field, v := range map[string]string{
		"inputTokens": pow(400), "outputTokens": "-" + pow(400), "totalTokens": pow(1000),
		"reasoningTokens": pow(500), "cachedInputTokens": pow(600),
	} {
		t.Run(field, func(t *testing.T) {
			rejectedCount(t, `{"provider": "openai", "`+field+`": `+v+`}`, `{"provider": "openai"}`)
		})
	}
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_counts_beyond_the_safe_integer_wire_range_are_dropped
func TestPyTokenUsageGuardTest_CountsBeyondTheSafeIntegerWireRangeAreDropped(t *testing.T) {
	const maxSafe = int64(1)<<53 - 1
	rejectedCount(t, `{"inputTokens": 9007199254740992, "outputTokens": 9007199254740991}`,
		`{"outputTokens": 9007199254740991}`)
	// The Go producer can hold 2**53 in an int64; the producer-side check
	// refuses it, and accepts 2**53 - 1.
	bad := &agui.RunFinishedEvent{ThreadID: "t", RunID: "r", Usage: []agui.TokenUsage{
		{InputTokens: agui.Ptr(maxSafe + 1), OutputTokens: agui.Ptr(maxSafe)},
	}}
	if err := agui.ValidateEvent(bad); err == nil {
		t.Fatalf("ValidateEvent accepted inputTokens = 2**53")
	}
	good := &agui.RunFinishedEvent{ThreadID: "t", RunID: "r", Usage: []agui.TokenUsage{{OutputTokens: agui.Ptr(maxSafe)}}}
	if err := agui.ValidateEvent(good); err != nil {
		t.Fatalf("ValidateEvent rejected outputTokens = 2**53-1: %v", err)
	}
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_a_float_beyond_the_wire_range_is_dropped
func TestPyTokenUsageGuardTest_AFloatBeyondTheWireRangeIsDropped(t *testing.T) {
	rejectedCount(t, `{"inputTokens": 1e300, "outputTokens": 2}`, `{"outputTokens": 2}`)
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_guards_nested_detail_fields
func TestPyTokenUsageGuardTest_GuardsNestedDetailFields(t *testing.T) {
	t.Skip(naMapper + "; the nested input_token_details/output_token_details containers are LangChain's vendor shape, not a protocol one")
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_tolerates_non_object_detail_containers
func TestPyTokenUsageGuardTest_ToleratesNonObjectDetailContainers(t *testing.T) {
	t.Skip(naMapper + "; the detail containers are LangChain's vendor shape, not a protocol one")
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_returns_none_when_no_usable_count_survives
func TestPyTokenUsageGuardTest_ReturnsNoneWhenNoUsableCountSurvives(t *testing.T) {
	t.Skip(naMapper + "; deciding whether to emit an entry at all is the mapper's decision")
}

// Python: tests/test_token_usage.py::TokenUsageGuardTest::test_reports_a_measured_zero_as_zero
func TestPyTokenUsageGuardTest_ReportsAMeasuredZeroAsZero(t *testing.T) {
	u := agui.TokenUsage{OutputTokens: agui.Ptr[int64](0)}
	assertJSON(t, u, `{"outputTokens": 0}`)
	var back agui.TokenUsage
	if err := json.Unmarshal([]byte(`{"outputTokens": 0}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.OutputTokens == nil || *back.OutputTokens != 0 || back.InputTokens != nil {
		t.Fatalf("decoded %+v: a measured zero must stay distinct from absent", back)
	}
}

// ---------------------------------------------------------------------------
// TokenUsageContentSafetyTest
// ---------------------------------------------------------------------------

var contentBearing = map[string]any{
	"prompt":     "what is the capital of France?",
	"completion": "Paris",
	"messages":   []any{map[string]any{"role": "user", "content": "secret"}},
	"text":       "secret",
	"content":    "secret",
	"input":      "secret",
	"output":     "secret",
	"thread_id":  "t-1",
	"run_id":     "r-1",
	"user_id":    "u-1",
	"api_key":    "sk-live-xxx",
}

// Python: tests/test_token_usage.py::TokenUsageContentSafetyTest::test_content_bearing_fields_are_not_copied
func TestPyTokenUsageContentSafetyTest_ContentBearingFieldsAreNotCopied(t *testing.T) {
	t.Skip(naMapper + "; that no vendor field is copied is a property of the mapper (the Go type's closedness is covered by TestPyTokenUsageContentSafetyTest_TheMapperIsTheGuardBecauseTheTypeAllowsExtras)")
}

// Python: tests/test_token_usage.py::TokenUsageContentSafetyTest::test_the_mapper_is_the_guard_because_the_type_allows_extras
//
// Adapted (b): in Python, ConfiguredBaseModel is extra="allow", so a direct
// TokenUsage(prompt=...) keeps the content-bearing field and only the mapper
// stands guard. The 1.0 schema closes TokenUsage (unevaluatedProperties:
// false; "nothing content-bearing or identifying"), so in Go the type itself
// is the guard: the struct has no place for an extra, decoding drops it, the
// strict check rejects it and the processing model strips it. The mapper
// half of the Python test is N/A (no Go mapper).
func TestPyTokenUsageContentSafetyTest_TheMapperIsTheGuardBecauseTheTypeAllowsExtras(t *testing.T) {
	allowed := map[string]bool{
		"provider": true, "model": true, "inputTokens": true, "outputTokens": true,
		"totalTokens": true, "reasoningTokens": true, "cachedInputTokens": true, "cacheWriteInputTokens": true,
	}
	smuggledDoc := map[string]any{"inputTokens": 1, "provider": "openai", "model": "gpt-4o"}
	for k, v := range contentBearing {
		smuggledDoc[k] = v
	}
	doc, _ := json.Marshal(smuggledDoc)

	mustReject(t, "TokenUsage", string(doc))

	var u agui.TokenUsage
	if err := json.Unmarshal(doc, &u); err != nil {
		t.Fatalf("decode: %v", err)
	}
	serialized, _ := json.Marshal(u)
	for k := range object(t, u) {
		if !allowed[k] {
			t.Fatalf("TokenUsage serialized non-count key %q", k)
		}
	}
	for k, v := range contentBearing {
		if strings.Contains(string(serialized), `"`+k+`"`) {
			t.Fatalf("serialized entry carries key %q: %s", k, serialized)
		}
		if s, ok := v.(string); ok && strings.Contains(string(serialized), s) {
			t.Fatalf("serialized entry echoes %q: %s", s, serialized)
		}
	}

	res, err := agui.Sanitize("TokenUsage", doc, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	for k := range res.Value.(map[string]any) {
		if !allowed[k] {
			t.Fatalf("Sanitize kept content-bearing key %q", k)
		}
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("stripping content-bearing keys produced no warning")
	}
}

// ---------------------------------------------------------------------------
// AggregateTokenUsageTest
// ---------------------------------------------------------------------------

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_returns_empty_for_empty_input
func TestPyAggregateTokenUsageTest_ReturnsEmptyForEmptyInput(t *testing.T) { t.Skip(naAggregate) }

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_sums_entries_for_the_same_provider_and_model
func TestPyAggregateTokenUsageTest_SumsEntriesForTheSameProviderAndModel(t *testing.T) {
	t.Skip(naAggregate)
}

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_sums_the_cache_breakdown_like_every_other_count
func TestPyAggregateTokenUsageTest_SumsTheCacheBreakdownLikeEveryOtherCount(t *testing.T) {
	t.Skip(naAggregate)
}

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_keeps_distinct_models_separate_in_first_seen_order
func TestPyAggregateTokenUsageTest_KeepsDistinctModelsSeparateInFirstSeenOrder(t *testing.T) {
	t.Skip(naAggregate)
}

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_distinct_providers_stay_separate
func TestPyAggregateTokenUsageTest_DistinctProvidersStaySeparate(t *testing.T) { t.Skip(naAggregate) }

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_unlabelled_entries_group_together
func TestPyAggregateTokenUsageTest_UnlabelledEntriesGroupTogether(t *testing.T) { t.Skip(naAggregate) }

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_a_field_only_some_members_report_is_summed_over_those_members
func TestPyAggregateTokenUsageTest_AFieldOnlySomeMembersReportIsSummedOverThoseMembers(t *testing.T) {
	t.Skip(naAggregate)
}

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_a_field_no_member_reports_stays_unset
func TestPyAggregateTokenUsageTest_AFieldNoMemberReportsStaysUnset(t *testing.T) { t.Skip(naAggregate) }

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_a_reported_zero_survives_aggregation
func TestPyAggregateTokenUsageTest_AReportedZeroSurvivesAggregation(t *testing.T) {
	t.Skip(naAggregate)
}

// Python: tests/test_token_usage.py::AggregateTokenUsageTest::test_does_not_mutate_its_inputs
func TestPyAggregateTokenUsageTest_DoesNotMutateItsInputs(t *testing.T) { t.Skip(naAggregate) }

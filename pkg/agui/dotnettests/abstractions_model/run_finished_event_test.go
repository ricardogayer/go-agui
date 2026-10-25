package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func decodeRunFinished(t *testing.T, data string) agui.RunFinishedEvent {
	t.Helper()
	var e agui.RunFinishedEvent
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return e
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_WithOutcomeAndResult
func TestCsRunFinishedEventTest_Serialization_WithOutcomeAndResult(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Outcome: &agui.SuccessOutcome{}, Result: map[string]any{"answer": 42}})
	assertEq(t, o["type"], "RUN_FINISHED", "type")
	assertEq(t, o["threadId"], "t1", "threadId")
	assertEq(t, o["runId"], "r1", "runId")
	assertEq(t, objOf(t, o["outcome"])["type"], "success", "outcome.type")
	assertEq(t, objOf(t, o["result"])["answer"], float64(42), "result.answer")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_WithInterrupt
func TestCsRunFinishedEventTest_Serialization_WithInterrupt(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{
		ThreadID: "t1",
		Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{
			{ID: "int-1", Reason: "input_required", Message: agui.Ptr("need_input")},
		}},
	})
	assertEq(t, o["type"], "RUN_FINISHED", "type")
	outcome := objOf(t, o["outcome"])
	assertEq(t, outcome["type"], "interrupt", "outcome.type")
	interrupts := arrOf(t, outcome["interrupts"])
	if len(interrupts) != 1 {
		t.Fatalf("expected 1 interrupt, got %v", interrupts)
	}
	assertEq(t, objOf(t, interrupts[0])["id"], "int-1", "interrupts[0].id")
	assertEq(t, objOf(t, interrupts[0])["message"], "need_input", "interrupts[0].message")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_WithCancelled
func TestCsRunFinishedEventTest_Serialization_WithCancelled(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Outcome: &agui.CancelledOutcome{}})
	assertEq(t, o["type"], "RUN_FINISHED", "type")
	outcome := objOf(t, o["outcome"])
	assertEq(t, outcome["type"], "cancelled", "outcome.type")
	assertOmitted(t, outcome, "interrupts")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Deserialization_WithCancelled
func TestCsRunFinishedEventTest_Deserialization_WithCancelled(t *testing.T) {
	e := decodeRunFinished(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"cancelled"}}`)
	if _, ok := e.Outcome.(*agui.CancelledOutcome); !ok {
		t.Fatalf("expected *agui.CancelledOutcome, got %T", e.Outcome)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_WithPendingToolCallIds
func TestCsRunFinishedEventTest_Serialization_WithPendingToolCallIds(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Outcome: &agui.SuccessOutcome{PendingToolCallIDs: []string{"tc-1", "tc-2"}}})
	outcome := objOf(t, o["outcome"])
	assertEq(t, outcome["type"], "success", "outcome.type")
	assertEq(t, arrOf(t, outcome["pendingToolCallIds"]), []any{"tc-1", "tc-2"}, "pendingToolCallIds")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_SuccessOmitsPendingToolCallIdsWhenUnset
func TestCsRunFinishedEventTest_Serialization_SuccessOmitsPendingToolCallIdsWhenUnset(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Outcome: &agui.SuccessOutcome{}})
	assertOmitted(t, objOf(t, o["outcome"]), "pendingToolCallIds")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Deserialization_WithPendingToolCallIds
func TestCsRunFinishedEventTest_Deserialization_WithPendingToolCallIds(t *testing.T) {
	e := decodeRunFinished(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"success","pendingToolCallIds":["tc-1"]}}`)
	s, ok := e.Outcome.(*agui.SuccessOutcome)
	if !ok {
		t.Fatalf("expected *agui.SuccessOutcome, got %T", e.Outcome)
	}
	assertEq(t, s.PendingToolCallIDs, []string{"tc-1"}, "pendingToolCallIds")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_OmitsNullProperties
func TestCsRunFinishedEventTest_Serialization_OmitsNullProperties(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{})
	assertEq(t, o["type"], "RUN_FINISHED", "type")
	assertEq(t, o["threadId"], "", "threadId")
	assertEq(t, o["runId"], "", "runId")
	assertOmitted(t, o, "outcome", "result", "usage")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_WithUsage
func TestCsRunFinishedEventTest_Serialization_WithUsage(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Usage: []agui.TokenUsage{{
		Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"),
		InputTokens: agui.Ptr[int64](11), OutputTokens: agui.Ptr[int64](22), TotalTokens: agui.Ptr[int64](33),
		ReasoningTokens: agui.Ptr[int64](44), CachedInputTokens: agui.Ptr[int64](55), CacheWriteInputTokens: agui.Ptr[int64](66),
	}}})
	usage := arrOf(t, o["usage"])
	if len(usage) != 1 {
		t.Fatalf("expected one usage entry, got %v", usage)
	}
	u := objOf(t, usage[0])
	assertEq(t, u["provider"], "openai", "provider")
	assertEq(t, u["model"], "gpt-4o", "model")
	assertEq(t, u["inputTokens"], float64(11), "inputTokens")
	assertEq(t, u["outputTokens"], float64(22), "outputTokens")
	assertEq(t, u["totalTokens"], float64(33), "totalTokens")
	assertEq(t, u["reasoningTokens"], float64(44), "reasoningTokens")
	assertEq(t, u["cachedInputTokens"], float64(55), "cachedInputTokens")
	assertEq(t, u["cacheWriteInputTokens"], float64(66), "cacheWriteInputTokens")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Serialization_UsageOmitsUnreportedCounts
func TestCsRunFinishedEventTest_Serialization_UsageOmitsUnreportedCounts(t *testing.T) {
	o := obj(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Usage: []agui.TokenUsage{{InputTokens: agui.Ptr[int64](7)}}})
	entry := objOf(t, arrOf(t, o["usage"])[0])
	assertEq(t, entry["inputTokens"], float64(7), "inputTokens")
	assertOmitted(t, entry, "provider", "model", "outputTokens", "totalTokens", "reasoningTokens", "cachedInputTokens")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunFinishedEventTest.cs::RunFinishedEventTest.Deserialization_WithUsage
func TestCsRunFinishedEventTest_Deserialization_WithUsage(t *testing.T) {
	e := decodeRunFinished(t, `{
	  "type": "RUN_FINISHED",
	  "threadId": "t1",
	  "runId": "r1",
	  "usage": [{ "provider": "anthropic", "model": "claude-opus-4", "inputTokens": 5, "outputTokens": 6 }]
	}`)
	if len(e.Usage) != 1 {
		t.Fatalf("expected one usage entry, got %+v", e.Usage)
	}
	u := e.Usage[0]
	assertEq(t, agui.Deref(u.Provider), "anthropic", "provider")
	assertEq(t, agui.Deref(u.Model), "claude-opus-4", "model")
	assertEq(t, agui.Deref(u.InputTokens), int64(5), "inputTokens")
	assertEq(t, agui.Deref(u.OutputTokens), int64(6), "outputTokens")
	if u.TotalTokens != nil {
		t.Errorf("totalTokens must be absent, got %d", *u.TotalTokens)
	}
}

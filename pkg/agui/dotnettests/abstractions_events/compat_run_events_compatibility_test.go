package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func runFixture(t *testing.T, i int) agui.Event {
	t.Helper()
	return decodeFixtureEvent(t, loadFixture(t, "run-events.json")[i])
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunStartedEvent_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunStartedEvent_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunStartedEvent](t, runFixture(t, 0))
	eq(t, "ThreadID", typed.ThreadID, "thread-1234")
	eq(t, "RunID", typed.RunID, "run-5678")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunFinishedEvent_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunFinishedEvent_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunFinishedEvent](t, runFixture(t, 1))
	eq(t, "ThreadID", typed.ThreadID, "thread-1234")
	eq(t, "RunID", typed.RunID, "run-5678")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunErrorEvent_WithMessage_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunErrorEvent_WithMessage_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunErrorEvent](t, runFixture(t, 2))
	eq(t, "Message", typed.Message, "Failed to execute tool call")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunErrorEvent_WithCode_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunErrorEvent_WithCode_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunErrorEvent](t, runFixture(t, 3))
	eq(t, "Message", typed.Message, "API request failed")
	eq(t, "Code", agui.Deref(typed.Code), "API_ERROR")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.StepStartedEvent_WithTimestamp_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_StepStartedEvent_WithTimestamp_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StepStartedEvent](t, runFixture(t, 4))
	eq(t, "StepName", typed.StepName, "data_analysis")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.StepStartedEvent_Minimal_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_StepStartedEvent_Minimal_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StepStartedEvent](t, runFixture(t, 5))
	eq(t, "StepName", typed.StepName, "process_payment")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.StepFinishedEvent_WithTimestamp_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_StepFinishedEvent_WithTimestamp_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StepFinishedEvent](t, runFixture(t, 6))
	eq(t, "StepName", typed.StepName, "data_analysis")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.StepFinishedEvent_Minimal_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_StepFinishedEvent_Minimal_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StepFinishedEvent](t, runFixture(t, 7))
	eq(t, "StepName", typed.StepName, "process_payment")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunFinishedEvent_WithUsage_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunFinishedEvent_WithUsage_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunFinishedEvent](t, runFixture(t, 8))
	if len(typed.Usage) != 2 {
		t.Fatalf("len(Usage) = %d, want 2", len(typed.Usage))
	}
	u0, u1 := typed.Usage[0], typed.Usage[1]
	eq(t, "Usage[0].Provider", agui.Deref(u0.Provider), "openai")
	eq(t, "Usage[0].Model", agui.Deref(u0.Model), "gpt-4o")
	eq(t, "Usage[0].InputTokens", agui.Deref(u0.InputTokens), int64(11))
	eq(t, "Usage[0].OutputTokens", agui.Deref(u0.OutputTokens), int64(22))
	eq(t, "Usage[0].TotalTokens", agui.Deref(u0.TotalTokens), int64(33))
	eq(t, "Usage[0].ReasoningTokens", agui.Deref(u0.ReasoningTokens), int64(44))
	eq(t, "Usage[0].CachedInputTokens", agui.Deref(u0.CachedInputTokens), int64(55))
	eq(t, "Usage[1].Provider", agui.Deref(u1.Provider), "anthropic")
	eq(t, "Usage[1].InputTokens", agui.Deref(u1.InputTokens), int64(1))
	if u1.OutputTokens != nil {
		t.Errorf("Usage[1].OutputTokens = %d, want nil", *u1.OutputTokens)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.RunErrorEvent_WithUsage_DeserializesFromTypeScriptPayload
func TestCsRunEventsCompatibilityTest_RunErrorEvent_WithUsage_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.RunErrorEvent](t, runFixture(t, 9))
	eq(t, "Message", typed.Message, "API request failed")
	if len(typed.Usage) != 1 {
		t.Fatalf("len(Usage) = %d, want 1", len(typed.Usage))
	}
	eq(t, "Usage[0].Provider", agui.Deref(typed.Usage[0].Provider), "openai")
	eq(t, "Usage[0].InputTokens", agui.Deref(typed.Usage[0].InputTokens), int64(120))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/RunEventsCompatibilityTest.cs::RunEventsCompatibilityTest.AllRunEvents_RoundTrip_ProduceSameJson
//
// The C# body only compares types; as the name promises, Go also checks the
// re-serialised JSON equals the fixture.
func TestCsRunEventsCompatibilityTest_AllRunEvents_RoundTrip_ProduceSameJson(t *testing.T) {
	fx := loadFixture(t, "run-events.json")
	assertRoundTripPreservesType(t, fx)
	for i, raw := range fx {
		e := decodeFixtureEvent(t, raw)
		var want any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		eq(t, fmt.Sprintf("fixture %d JSON", i), generic(t, e), want)
	}
}

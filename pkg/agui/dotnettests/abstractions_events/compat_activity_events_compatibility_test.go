package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/ActivityEventsCompatibilityTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ActivityEventsCompatibilityTest.cs::ActivityEventsCompatibilityTest.ActivitySnapshot_WithReplaceTrue_DeserializesFromTypeScriptPayload
func TestCsActivityEventsCompatibilityTest_ActivitySnapshot_WithReplaceTrue_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "activity-events.json")
	typed := asType[*agui.ActivitySnapshotEvent](t, decodeFixtureEvent(t, fx[0]))
	eq(t, "MessageID", typed.MessageID, "msg_activity")
	eq(t, "ActivityType", typed.ActivityType, "PLAN")
	eq(t, "len(tasks)", arrLen(t, typed.Content["tasks"]), 1)
	eq(t, "tasks[0]", at(t, typed.Content, "tasks", 0), "search")
	eq(t, "Replace", typed.Replace != nil && *typed.Replace, true)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ActivityEventsCompatibilityTest.cs::ActivityEventsCompatibilityTest.ActivitySnapshot_WithReplaceFalse_DeserializesFromTypeScriptPayload
func TestCsActivityEventsCompatibilityTest_ActivitySnapshot_WithReplaceFalse_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "activity-events.json")
	typed := asType[*agui.ActivitySnapshotEvent](t, decodeFixtureEvent(t, fx[1]))
	eq(t, "MessageID", typed.MessageID, "msg_activity")
	eq(t, "ActivityType", typed.ActivityType, "PLAN")
	eq(t, "len(tasks)", arrLen(t, typed.Content["tasks"]), 0)
	// An explicit false is distinct from absent (absent means true).
	if typed.Replace == nil || *typed.Replace {
		t.Errorf("Replace = %v, want explicit false", typed.Replace)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ActivityEventsCompatibilityTest.cs::ActivityEventsCompatibilityTest.ActivityDelta_DeserializesFromTypeScriptPayload
func TestCsActivityEventsCompatibilityTest_ActivityDelta_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "activity-events.json")
	typed := asType[*agui.ActivityDeltaEvent](t, decodeFixtureEvent(t, fx[2]))
	eq(t, "MessageID", typed.MessageID, "msg_activity")
	eq(t, "ActivityType", typed.ActivityType, "PLAN")
	if len(typed.Patch) != 1 {
		t.Fatalf("len(Patch) = %d, want 1", len(typed.Patch))
	}
	eq(t, "Patch[0].Op", typed.Patch[0].Op, "replace")
	eq(t, "Patch[0].Path", typed.Patch[0].Path, "/tasks/0")
	eq(t, "Patch[0].Value", typed.Patch[0].Value, "✓ search")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ActivityEventsCompatibilityTest.cs::ActivityEventsCompatibilityTest.AllActivityEvents_RoundTrip_PreservesType
func TestCsActivityEventsCompatibilityTest_AllActivityEvents_RoundTrip_PreservesType(t *testing.T) {
	assertRoundTripPreservesType(t, loadFixture(t, "activity-events.json"))
}

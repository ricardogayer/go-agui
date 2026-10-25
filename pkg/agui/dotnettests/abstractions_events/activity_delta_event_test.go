package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ActivityDeltaEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ActivityDeltaEventTest.cs::ActivityDeltaEventTest.Serialize_IncludesTypeAndProperties
func TestCsActivityDeltaEventTest_Serialize_IncludesTypeAndProperties(t *testing.T) {
	evt := &agui.ActivityDeltaEvent{
		MessageID:    "msg_activity",
		ActivityType: "PLAN",
		Patch:        agui.JSONPatch{agui.Replace("/tasks/0", "✓ search")},
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "ACTIVITY_DELTA")
	eq(t, "messageId", root["messageId"], "msg_activity")
	eq(t, "activityType", root["activityType"], "PLAN")
	eq(t, "len(patch)", arrLen(t, root["patch"]), 1)
	eq(t, "patch[0].op", at(t, root, "patch", 0, "op"), "replace")
}

// C#: tests/AGUI.Abstractions.UnitTests/ActivityDeltaEventTest.cs::ActivityDeltaEventTest.Deserialize_RoundTrips
func TestCsActivityDeltaEventTest_Deserialize_RoundTrips(t *testing.T) {
	evt := &agui.ActivityDeltaEvent{
		MessageID:    "msg_activity",
		ActivityType: "PLAN",
		Patch: agui.JSONPatch{
			agui.Replace("/tasks/0", "done"),
			agui.Add("/tasks/-", "new task"),
		},
	}
	got := roundTrip(t, evt)
	eq(t, "MessageID", got.MessageID, "msg_activity")
	eq(t, "ActivityType", got.ActivityType, "PLAN")
	if len(got.Patch) != 2 {
		t.Fatalf("len(Patch) = %d, want 2", len(got.Patch))
	}
	eq(t, "Patch[0].Op", got.Patch[0].Op, "replace")
	eq(t, "Patch[1].Op", got.Patch[1].Op, "add")
}

// C#: tests/AGUI.Abstractions.UnitTests/ActivityDeltaEventTest.cs::ActivityDeltaEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsActivityDeltaEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"ACTIVITY_DELTA","messageId":"msg1","activityType":"PLAN","patch":[{"op":"replace","path":"/tasks/0","value":"done"}]}`)
	delta := asType[*agui.ActivityDeltaEvent](t, evt)
	eq(t, "MessageID", delta.MessageID, "msg1")
	eq(t, "ActivityType", delta.ActivityType, "PLAN")
	eq(t, "len(Patch)", len(delta.Patch), 1)
}

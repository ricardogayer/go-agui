package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ActivitySnapshotEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ActivitySnapshotEventTest.cs::ActivitySnapshotEventTest.Serialize_IncludesTypeAndProperties
func TestCsActivitySnapshotEventTest_Serialize_IncludesTypeAndProperties(t *testing.T) {
	evt := &agui.ActivitySnapshotEvent{
		MessageID:    "msg_activity",
		ActivityType: "PLAN",
		Content:      map[string]any{"tasks": []string{"search", "process"}},
		Replace:      agui.Ptr(true),
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "ACTIVITY_SNAPSHOT")
	eq(t, "messageId", root["messageId"], "msg_activity")
	eq(t, "activityType", root["activityType"], "PLAN")
	eq(t, "len(content.tasks)", arrLen(t, at(t, root, "content", "tasks")), 2)
	eq(t, "replace", root["replace"], true)
}

// C#: tests/AGUI.Abstractions.UnitTests/ActivitySnapshotEventTest.cs::ActivitySnapshotEventTest.Serialize_OmitsReplaceWhenNull
func TestCsActivitySnapshotEventTest_Serialize_OmitsReplaceWhenNull(t *testing.T) {
	evt := &agui.ActivitySnapshotEvent{
		MessageID:    "msg1",
		ActivityType: "SEARCH",
		Content:      map[string]any{"query": "test"},
	}
	root := serialize(t, evt)
	if has(root, "replace") {
		t.Errorf("replace present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ActivitySnapshotEventTest.cs::ActivitySnapshotEventTest.Deserialize_RoundTrips
func TestCsActivitySnapshotEventTest_Deserialize_RoundTrips(t *testing.T) {
	evt := &agui.ActivitySnapshotEvent{
		MessageID:    "msg_activity",
		ActivityType: "PLAN",
		Content:      map[string]any{"tasks": []string{"search"}},
		Replace:      agui.Ptr(true),
	}
	got := roundTrip(t, evt)
	eq(t, "MessageID", got.MessageID, "msg_activity")
	eq(t, "ActivityType", got.ActivityType, "PLAN")
	if got.Replace == nil || !*got.Replace {
		t.Errorf("Replace = %v, want true", got.Replace)
	}
	eq(t, "len(Content.tasks)", arrLen(t, got.Content["tasks"]), 1)
}

// C#: tests/AGUI.Abstractions.UnitTests/ActivitySnapshotEventTest.cs::ActivitySnapshotEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsActivitySnapshotEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"ACTIVITY_SNAPSHOT","messageId":"msg1","activityType":"PLAN","content":{"tasks":[]}}`)
	snapshot := asType[*agui.ActivitySnapshotEvent](t, evt)
	eq(t, "MessageID", snapshot.MessageID, "msg1")
	eq(t, "ActivityType", snapshot.ActivityType, "PLAN")
}

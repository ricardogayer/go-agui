package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/StateSnapshotEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/StateSnapshotEventTest.cs::StateSnapshotEventTest.Serialize_IncludesTypeAndSnapshot
func TestCsStateSnapshotEventTest_Serialize_IncludesTypeAndSnapshot(t *testing.T) {
	root := serialize(t, &agui.StateSnapshotEvent{Snapshot: map[string]any{"counter": 0, "name": "test"}})
	eq(t, "type", root["type"], "STATE_SNAPSHOT")
	eq(t, "snapshot.counter", at(t, root, "snapshot", "counter"), float64(0))
	eq(t, "snapshot.name", at(t, root, "snapshot", "name"), "test")
}

// C#: tests/AGUI.Abstractions.UnitTests/StateSnapshotEventTest.cs::StateSnapshotEventTest.Deserialize_RoundTrips
func TestCsStateSnapshotEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.StateSnapshotEvent{Snapshot: map[string]any{"counter": 42}})
	eq(t, "Snapshot.counter", at(t, got.Snapshot, "counter"), float64(42))
}

// C#: tests/AGUI.Abstractions.UnitTests/StateSnapshotEventTest.cs::StateSnapshotEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsStateSnapshotEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"STATE_SNAPSHOT","snapshot":{"counter":10}}`)
	snapshot := asType[*agui.StateSnapshotEvent](t, evt)
	eq(t, "Snapshot.counter", at(t, snapshot.Snapshot, "counter"), float64(10))
}

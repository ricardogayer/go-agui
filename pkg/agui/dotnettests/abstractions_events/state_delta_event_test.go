package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/StateDeltaEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/StateDeltaEventTest.cs::StateDeltaEventTest.Serialize_IncludesTypeAndDelta
func TestCsStateDeltaEventTest_Serialize_IncludesTypeAndDelta(t *testing.T) {
	root := serialize(t, &agui.StateDeltaEvent{Delta: agui.JSONPatch{agui.Replace("/counter", 1)}})
	eq(t, "type", root["type"], "STATE_DELTA")
	eq(t, "len(delta)", arrLen(t, root["delta"]), 1)
	eq(t, "delta[0].op", at(t, root, "delta", 0, "op"), "replace")
	eq(t, "delta[0].path", at(t, root, "delta", 0, "path"), "/counter")
	eq(t, "delta[0].value", at(t, root, "delta", 0, "value"), float64(1))
}

// C#: tests/AGUI.Abstractions.UnitTests/StateDeltaEventTest.cs::StateDeltaEventTest.Deserialize_RoundTrips
func TestCsStateDeltaEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.StateDeltaEvent{Delta: agui.JSONPatch{
		agui.Add("/items/-", "item1"),
		agui.Remove("/temp"),
	}})
	if len(got.Delta) != 2 {
		t.Fatalf("len(Delta) = %d, want 2", len(got.Delta))
	}
	eq(t, "Delta[0].Op", got.Delta[0].Op, "add")
	eq(t, "Delta[0].Path", got.Delta[0].Path, "/items/-")
	eq(t, "Delta[1].Op", got.Delta[1].Op, "remove")
	eq(t, "Delta[1].Path", got.Delta[1].Path, "/temp")
}

// C#: tests/AGUI.Abstractions.UnitTests/StateDeltaEventTest.cs::StateDeltaEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsStateDeltaEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"STATE_DELTA","delta":[{"op":"replace","path":"/counter","value":5}]}`)
	delta := asType[*agui.StateDeltaEvent](t, evt)
	if len(delta.Delta) != 1 {
		t.Fatalf("len(Delta) = %d, want 1", len(delta.Delta))
	}
	eq(t, "Delta[0].Op", delta.Delta[0].Op, "replace")
	eq(t, "Delta[0].Path", delta.Delta[0].Path, "/counter")
}

// C#: tests/AGUI.Abstractions.UnitTests/StateDeltaEventTest.cs::StateDeltaEventTest.Serialize_MoveOperation_IncludesFrom
func TestCsStateDeltaEventTest_Serialize_MoveOperation_IncludesFrom(t *testing.T) {
	root := serialize(t, &agui.StateDeltaEvent{Delta: agui.JSONPatch{agui.Move("/a", "/b")}})
	op := at(t, root, "delta", 0)
	eq(t, "op", at(t, op, "op"), "move")
	eq(t, "path", at(t, op, "path"), "/b")
	eq(t, "from", at(t, op, "from"), "/a")
}

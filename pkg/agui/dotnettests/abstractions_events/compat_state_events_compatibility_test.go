package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs.

import (
	"math"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func stateFixture(t *testing.T, i int) agui.Event {
	t.Helper()
	return decodeFixtureEvent(t, loadFixture(t, "state-events.json")[i])
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateSnapshot_Basic_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateSnapshot_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateSnapshotEvent](t, stateFixture(t, 0))
	s := typed.Snapshot
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
	eq(t, "counter", at(t, s, "counter"), float64(42))
	eq(t, "len(items)", arrLen(t, at(t, s, "items")), 3)
	eq(t, "items[0]", at(t, s, "items", 0), "apple")
	eq(t, "config.enabled", at(t, s, "config", "enabled"), true)
	eq(t, "config.maxRetries", at(t, s, "config", "maxRetries"), float64(3))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateSnapshot_Empty_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateSnapshot_Empty_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateSnapshotEvent](t, stateFixture(t, 1))
	m, ok := typed.Snapshot.(map[string]any)
	if !ok {
		t.Fatalf("Snapshot is %T, want an object", typed.Snapshot)
	}
	eq(t, "len(snapshot)", len(m), 0)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateSnapshot_SpecialValues_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateSnapshot_SpecialValues_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateSnapshotEvent](t, stateFixture(t, 2))
	s := typed.Snapshot
	if v := at(t, s, "nullValue"); v != nil {
		t.Errorf("nullValue = %#v, want null", v)
	}
	eq(t, "emptyString", at(t, s, "emptyString"), "")
	eq(t, "zero", at(t, s, "zero"), float64(0))
	eq(t, "negativeNumber", at(t, s, "negativeNumber"), float64(-123))
	if f, _ := at(t, s, "floatNumber").(float64); math.Abs(f-3.14159) > 1e-5 {
		t.Errorf("floatNumber = %v, want 3.14159", f)
	}
	eq(t, "len(emptyArray)", arrLen(t, at(t, s, "emptyArray")), 0)
	eq(t, "emptyObject", at(t, s, "emptyObject"), map[string]any{})
	eq(t, "boolValues.true", at(t, s, "boolValues", "true"), true)
	eq(t, "boolValues.false", at(t, s, "boolValues", "false"), false)
	eq(t, "dateString", at(t, s, "dateString"), "2023-01-15T10:30:00.000Z")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateDelta_Basic_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateDelta_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateDeltaEvent](t, stateFixture(t, 3))
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
	if len(typed.Delta) != 2 {
		t.Fatalf("len(Delta) = %d, want 2", len(typed.Delta))
	}
	eq(t, "Delta[0].Op", typed.Delta[0].Op, "add")
	eq(t, "Delta[0].Path", typed.Delta[0].Path, "/counter")
	eq(t, "Delta[0].Value", generic(t, typed.Delta[0].Value), float64(42))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateDelta_AllJsonPatchOperations_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateDelta_AllJsonPatchOperations_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateDeltaEvent](t, stateFixture(t, 4))
	d := typed.Delta
	if len(d) != 6 {
		t.Fatalf("len(Delta) = %d, want 6", len(d))
	}
	// add
	eq(t, "add.op", d[0].Op, "add")
	eq(t, "add.path", d[0].Path, "/users/123")
	eq(t, "add.value.name", at(t, generic(t, d[0].Value), "name"), "John")
	// remove
	eq(t, "remove.op", d[1].Op, "remove")
	eq(t, "remove.path", d[1].Path, "/users/456")
	// replace
	eq(t, "replace.op", d[2].Op, "replace")
	eq(t, "replace.value", d[2].Value, "Jane Doe")
	// move
	eq(t, "move.op", d[3].Op, "move")
	eq(t, "move.from", d[3].From, "/users/old")
	eq(t, "move.path", d[3].Path, "/users/new")
	// copy
	eq(t, "copy.op", d[4].Op, "copy")
	eq(t, "copy.from", d[4].From, "/templates/default")
	// test
	eq(t, "test.op", d[5].Op, "test")
	eq(t, "test.value", d[5].Value, true)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.StateDelta_Empty_DeserializesFromTypeScriptPayload
func TestCsStateEventsCompatibilityTest_StateDelta_Empty_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.StateDeltaEvent](t, stateFixture(t, 5))
	eq(t, "len(Delta)", len(typed.Delta), 0)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/StateEventsCompatibilityTest.cs::StateEventsCompatibilityTest.AllStateEvents_RoundTrip_PreservesType
func TestCsStateEventsCompatibilityTest_AllStateEvents_RoundTrip_PreservesType(t *testing.T) {
	assertRoundTripPreservesType(t, loadFixture(t, "state-events.json"))
}

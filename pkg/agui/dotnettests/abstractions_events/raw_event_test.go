package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/RawEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/RawEventTest.cs::RawEventTest.Serialize_IncludesTypeEventAndSource
func TestCsRawEventTest_Serialize_IncludesTypeEventAndSource(t *testing.T) {
	evt := &agui.RawEvent{
		Event:  map[string]any{"action": "button_click", "elementId": "submit-btn"},
		Source: agui.Ptr("frontend"),
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "RAW")
	eq(t, "source", root["source"], "frontend")
	eq(t, "event.action", at(t, root, "event", "action"), "button_click")
}

// C#: tests/AGUI.Abstractions.UnitTests/RawEventTest.cs::RawEventTest.Serialize_OmitsSourceWhenNull
func TestCsRawEventTest_Serialize_OmitsSourceWhenNull(t *testing.T) {
	evt := &agui.RawEvent{Event: map[string]any{"data": "test"}}
	root := serialize(t, evt)
	if has(root, "source") {
		t.Errorf("source present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/RawEventTest.cs::RawEventTest.Deserialize_RoundTrips
func TestCsRawEventTest_Deserialize_RoundTrips(t *testing.T) {
	evt := &agui.RawEvent{
		Event:  map[string]any{"type": "user_action", "timestamp": 1234567890},
		Source: agui.Ptr("backend"),
	}
	got := roundTrip(t, evt)
	eq(t, "Source", agui.Deref(got.Source), "backend")
	eq(t, "Event.type", at(t, got.Event, "type"), "user_action")
}

// C#: tests/AGUI.Abstractions.UnitTests/RawEventTest.cs::RawEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsRawEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"RAW","event":{"data":"test"},"source":"external"}`)
	raw := asType[*agui.RawEvent](t, evt)
	eq(t, "Source", agui.Deref(raw.Source), "external")
	eq(t, "Event.data", at(t, raw.Event, "data"), "test")
}

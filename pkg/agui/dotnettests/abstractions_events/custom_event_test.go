package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/CustomEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/CustomEventTest.cs::CustomEventTest.Serialize_IncludesTypeNameAndValue
func TestCsCustomEventTest_Serialize_IncludesTypeNameAndValue(t *testing.T) {
	evt := &agui.CustomEvent{
		Name:  "user_preference_updated",
		Value: map[string]any{"theme": "dark", "fontSize": "medium"},
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "CUSTOM")
	eq(t, "name", root["name"], "user_preference_updated")
	eq(t, "value.theme", at(t, root, "value", "theme"), "dark")
}

// C#: tests/AGUI.Abstractions.UnitTests/CustomEventTest.cs::CustomEventTest.Deserialize_RoundTrips
func TestCsCustomEventTest_Deserialize_RoundTrips(t *testing.T) {
	evt := &agui.CustomEvent{
		Name:  "analytics_event",
		Value: map[string]any{"action": "click", "count": 5},
	}
	got := roundTrip(t, evt)
	eq(t, "Name", got.Name, "analytics_event")
	eq(t, "Value.action", at(t, got.Value, "action"), "click")
	eq(t, "Value.count", at(t, got.Value, "count"), float64(5))
}

// C#: tests/AGUI.Abstractions.UnitTests/CustomEventTest.cs::CustomEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsCustomEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"CUSTOM","name":"test_event","value":{"key":"val"}}`)
	custom := asType[*agui.CustomEvent](t, evt)
	eq(t, "Name", custom.Name, "test_event")
	eq(t, "Value.key", at(t, custom.Value, "key"), "val")
}

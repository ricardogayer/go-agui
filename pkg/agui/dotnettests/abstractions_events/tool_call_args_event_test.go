package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ToolCallArgsEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallArgsEventTest.cs::ToolCallArgsEventTest.Serialize_IncludesTypeAndToolCallIdAndDelta
func TestCsToolCallArgsEventTest_Serialize_IncludesTypeAndToolCallIdAndDelta(t *testing.T) {
	root := serialize(t, &agui.ToolCallArgsEvent{ToolCallID: "call-1", Delta: `{"location":`})
	eq(t, "type", root["type"], "TOOL_CALL_ARGS")
	eq(t, "toolCallId", root["toolCallId"], "call-1")
	eq(t, "delta", root["delta"], `{"location":`)
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallArgsEventTest.cs::ToolCallArgsEventTest.Deserialize_RoundTrips
func TestCsToolCallArgsEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ToolCallArgsEvent{ToolCallID: "call-2", Delta: `"Seattle"}`})
	eq(t, "ToolCallID", got.ToolCallID, "call-2")
	eq(t, "Delta", got.Delta, `"Seattle"}`)
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallArgsEventTest.cs::ToolCallArgsEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsToolCallArgsEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TOOL_CALL_ARGS","toolCallId":"call-3","delta":"test"}`)
	typed := asType[*agui.ToolCallArgsEvent](t, evt)
	eq(t, "ToolCallID", typed.ToolCallID, "call-3")
	eq(t, "Delta", typed.Delta, "test")
}

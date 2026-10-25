package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ToolCallEndEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallEndEventTest.cs::ToolCallEndEventTest.Serialize_IncludesTypeAndToolCallId
func TestCsToolCallEndEventTest_Serialize_IncludesTypeAndToolCallId(t *testing.T) {
	root := serialize(t, &agui.ToolCallEndEvent{ToolCallID: "call-1"})
	eq(t, "type", root["type"], "TOOL_CALL_END")
	eq(t, "toolCallId", root["toolCallId"], "call-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallEndEventTest.cs::ToolCallEndEventTest.Deserialize_RoundTrips
func TestCsToolCallEndEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ToolCallEndEvent{ToolCallID: "call-2"})
	eq(t, "ToolCallID", got.ToolCallID, "call-2")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallEndEventTest.cs::ToolCallEndEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsToolCallEndEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TOOL_CALL_END","toolCallId":"call-3"}`)
	typed := asType[*agui.ToolCallEndEvent](t, evt)
	eq(t, "ToolCallID", typed.ToolCallID, "call-3")
}

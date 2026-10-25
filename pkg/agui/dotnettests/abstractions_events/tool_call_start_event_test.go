package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs::ToolCallStartEventTest.Serialize_IncludesTypeAndToolCallIdAndName
func TestCsToolCallStartEventTest_Serialize_IncludesTypeAndToolCallIdAndName(t *testing.T) {
	root := serialize(t, &agui.ToolCallStartEvent{ToolCallID: "call-1", ToolCallName: "get_weather"})
	eq(t, "type", root["type"], "TOOL_CALL_START")
	eq(t, "toolCallId", root["toolCallId"], "call-1")
	eq(t, "toolCallName", root["toolCallName"], "get_weather")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs::ToolCallStartEventTest.Serialize_IncludesParentMessageId_WhenSet
func TestCsToolCallStartEventTest_Serialize_IncludesParentMessageId_WhenSet(t *testing.T) {
	root := serialize(t, &agui.ToolCallStartEvent{ToolCallID: "call-1", ToolCallName: "get_weather", ParentMessageID: agui.Ptr("msg-1")})
	eq(t, "parentMessageId", root["parentMessageId"], "msg-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs::ToolCallStartEventTest.Deserialize_RoundTrips
func TestCsToolCallStartEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ToolCallStartEvent{ToolCallID: "call-2", ToolCallName: "search"})
	eq(t, "ToolCallID", got.ToolCallID, "call-2")
	eq(t, "ToolCallName", got.ToolCallName, "search")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs::ToolCallStartEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsToolCallStartEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TOOL_CALL_START","toolCallId":"call-3","toolCallName":"get_weather"}`)
	typed := asType[*agui.ToolCallStartEvent](t, evt)
	eq(t, "ToolCallID", typed.ToolCallID, "call-3")
	eq(t, "ToolCallName", typed.ToolCallName, "get_weather")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallStartEventTest.cs::ToolCallStartEventTest.ToolCallStartEvent_Serialization_OmitsParentMessageId_WhenNull
func TestCsToolCallStartEventTest_ToolCallStartEvent_Serialization_OmitsParentMessageId_WhenNull(t *testing.T) {
	root := serialize(t, &agui.ToolCallStartEvent{ToolCallID: "call-123", ToolCallName: "get_weather"})
	if has(root, "parentMessageId") {
		t.Errorf("parentMessageId present: %v", root)
	}
}

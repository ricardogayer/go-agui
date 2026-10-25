package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs::ToolCallResultEventTest.Serialize_IncludesTypeAndToolCallIdAndContent
func TestCsToolCallResultEventTest_Serialize_IncludesTypeAndToolCallIdAndContent(t *testing.T) {
	root := serialize(t, &agui.ToolCallResultEvent{ToolCallID: "call-1", MessageID: "msg-1", Content: agui.TextContent(`{"temp":72}`)})
	eq(t, "type", root["type"], "TOOL_CALL_RESULT")
	eq(t, "toolCallId", root["toolCallId"], "call-1")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "content", root["content"], `{"temp":72}`)
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs::ToolCallResultEventTest.Serialize_IncludesRole_WhenSet
func TestCsToolCallResultEventTest_Serialize_IncludesRole_WhenSet(t *testing.T) {
	root := serialize(t, &agui.ToolCallResultEvent{ToolCallID: "call-1", MessageID: "msg-1", Content: agui.TextContent("result-data"), Role: agui.Ptr(agui.RoleTool)})
	eq(t, "role", root["role"], "tool")
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs::ToolCallResultEventTest.Serialize_OmitsRole_WhenNull
func TestCsToolCallResultEventTest_Serialize_OmitsRole_WhenNull(t *testing.T) {
	root := serialize(t, &agui.ToolCallResultEvent{ToolCallID: "call-1", MessageID: "msg-1", Content: agui.TextContent("ok")})
	if has(root, "role") {
		t.Errorf("role present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs::ToolCallResultEventTest.Deserialize_RoundTrips
func TestCsToolCallResultEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ToolCallResultEvent{ToolCallID: "call-2", MessageID: "msg-2", Content: agui.TextContent("success"), Role: agui.Ptr(agui.RoleTool)})
	eq(t, "ToolCallID", got.ToolCallID, "call-2")
	eq(t, "MessageID", got.MessageID, "msg-2")
	eq(t, "Content.IsParts", got.Content.IsParts(), false)
	eq(t, "Content", got.Content.String(), "success")
	eq(t, "Role", agui.Deref(got.Role), agui.RoleTool)
}

// C#: tests/AGUI.Abstractions.UnitTests/ToolCallResultEventTest.cs::ToolCallResultEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsToolCallResultEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TOOL_CALL_RESULT","toolCallId":"call-3","messageId":"msg-3","content":"done"}`)
	typed := asType[*agui.ToolCallResultEvent](t, evt)
	eq(t, "ToolCallID", typed.ToolCallID, "call-3")
	eq(t, "MessageID", typed.MessageID, "msg-3")
	eq(t, "Content.IsParts", typed.Content.IsParts(), false)
	eq(t, "Content", typed.Content.String(), "done")
}

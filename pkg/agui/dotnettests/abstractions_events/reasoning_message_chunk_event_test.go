package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningMessageChunkEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageChunkEventTest.cs::ReasoningMessageChunkEventTest.Serialize_IncludesTypeAndOptionalFields
func TestCsReasoningMessageChunkEventTest_Serialize_IncludesTypeAndOptionalFields(t *testing.T) {
	root := serialize(t, &agui.ReasoningMessageChunkEvent{MessageID: agui.Ptr("msg-1"), Delta: agui.Ptr("chunk")})
	eq(t, "type", root["type"], "REASONING_MESSAGE_CHUNK")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "delta", root["delta"], "chunk")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageChunkEventTest.cs::ReasoningMessageChunkEventTest.Serialize_OmitsNullOptionalFields
func TestCsReasoningMessageChunkEventTest_Serialize_OmitsNullOptionalFields(t *testing.T) {
	root := serialize(t, &agui.ReasoningMessageChunkEvent{})
	eq(t, "type", root["type"], "REASONING_MESSAGE_CHUNK")
	if has(root, "messageId") {
		t.Errorf("messageId present: %v", root)
	}
	if has(root, "delta") {
		t.Errorf("delta present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageChunkEventTest.cs::ReasoningMessageChunkEventTest.Deserialize_RoundTrips
func TestCsReasoningMessageChunkEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningMessageChunkEvent{MessageID: agui.Ptr("msg-2"), Delta: agui.Ptr("data")})
	eq(t, "MessageID", agui.Deref(got.MessageID), "msg-2")
	eq(t, "Delta", agui.Deref(got.Delta), "data")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageChunkEventTest.cs::ReasoningMessageChunkEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningMessageChunkEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_MESSAGE_CHUNK","messageId":"msg-3","delta":"text"}`)
	chunk := asType[*agui.ReasoningMessageChunkEvent](t, evt)
	eq(t, "MessageID", agui.Deref(chunk.MessageID), "msg-3")
	eq(t, "Delta", agui.Deref(chunk.Delta), "text")
}

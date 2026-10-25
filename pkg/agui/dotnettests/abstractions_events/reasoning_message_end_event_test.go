package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningMessageEndEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageEndEventTest.cs::ReasoningMessageEndEventTest.Serialize_IncludesTypeAndMessageId
func TestCsReasoningMessageEndEventTest_Serialize_IncludesTypeAndMessageId(t *testing.T) {
	root := serialize(t, &agui.ReasoningMessageEndEvent{MessageID: "msg-1"})
	eq(t, "type", root["type"], "REASONING_MESSAGE_END")
	eq(t, "messageId", root["messageId"], "msg-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageEndEventTest.cs::ReasoningMessageEndEventTest.Deserialize_RoundTrips
func TestCsReasoningMessageEndEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningMessageEndEvent{MessageID: "msg-2"})
	eq(t, "MessageID", got.MessageID, "msg-2")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageEndEventTest.cs::ReasoningMessageEndEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningMessageEndEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_MESSAGE_END","messageId":"msg-3"}`)
	end := asType[*agui.ReasoningMessageEndEvent](t, evt)
	eq(t, "MessageID", end.MessageID, "msg-3")
}

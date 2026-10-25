package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningEndEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEndEventTest.cs::ReasoningEndEventTest.Serialize_IncludesTypeAndMessageId
func TestCsReasoningEndEventTest_Serialize_IncludesTypeAndMessageId(t *testing.T) {
	root := serialize(t, &agui.ReasoningEndEvent{MessageID: "msg-1"})
	eq(t, "type", root["type"], "REASONING_END")
	eq(t, "messageId", root["messageId"], "msg-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEndEventTest.cs::ReasoningEndEventTest.Deserialize_RoundTrips
func TestCsReasoningEndEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningEndEvent{MessageID: "msg-2"})
	eq(t, "MessageID", got.MessageID, "msg-2")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEndEventTest.cs::ReasoningEndEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningEndEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_END","messageId":"msg-3"}`)
	end := asType[*agui.ReasoningEndEvent](t, evt)
	eq(t, "MessageID", end.MessageID, "msg-3")
}

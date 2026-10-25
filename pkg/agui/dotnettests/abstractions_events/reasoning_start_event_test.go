package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningStartEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningStartEventTest.cs::ReasoningStartEventTest.Serialize_IncludesTypeAndMessageId
func TestCsReasoningStartEventTest_Serialize_IncludesTypeAndMessageId(t *testing.T) {
	root := serialize(t, &agui.ReasoningStartEvent{MessageID: "msg-1"})
	eq(t, "type", root["type"], "REASONING_START")
	eq(t, "messageId", root["messageId"], "msg-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningStartEventTest.cs::ReasoningStartEventTest.Deserialize_RoundTrips
func TestCsReasoningStartEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningStartEvent{MessageID: "msg-2"})
	eq(t, "MessageID", got.MessageID, "msg-2")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningStartEventTest.cs::ReasoningStartEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningStartEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_START","messageId":"msg-3"}`)
	end := asType[*agui.ReasoningStartEvent](t, evt)
	eq(t, "MessageID", end.MessageID, "msg-3")
}

package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningMessageContentEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageContentEventTest.cs::ReasoningMessageContentEventTest.Serialize_IncludesTypeMessageIdAndDelta
func TestCsReasoningMessageContentEventTest_Serialize_IncludesTypeMessageIdAndDelta(t *testing.T) {
	root := serialize(t, &agui.ReasoningMessageContentEvent{MessageID: "msg-1", Delta: "thinking..."})
	eq(t, "type", root["type"], "REASONING_MESSAGE_CONTENT")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "delta", root["delta"], "thinking...")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageContentEventTest.cs::ReasoningMessageContentEventTest.Deserialize_RoundTrips
func TestCsReasoningMessageContentEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningMessageContentEvent{MessageID: "msg-2", Delta: "step 1"})
	eq(t, "MessageID", got.MessageID, "msg-2")
	eq(t, "Delta", got.Delta, "step 1")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageContentEventTest.cs::ReasoningMessageContentEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningMessageContentEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_MESSAGE_CONTENT","messageId":"msg-3","delta":"analyze"}`)
	content := asType[*agui.ReasoningMessageContentEvent](t, evt)
	eq(t, "MessageID", content.MessageID, "msg-3")
	eq(t, "Delta", content.Delta, "analyze")
}

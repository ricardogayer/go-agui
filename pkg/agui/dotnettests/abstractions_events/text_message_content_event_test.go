package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/TextMessageContentEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageContentEventTest.cs::TextMessageContentEventTest.Serialize_IncludesTypeAndMessageIdAndDelta
func TestCsTextMessageContentEventTest_Serialize_IncludesTypeAndMessageIdAndDelta(t *testing.T) {
	root := serialize(t, &agui.TextMessageContentEvent{MessageID: "msg-1", Delta: "Hello "})
	eq(t, "type", root["type"], "TEXT_MESSAGE_CONTENT")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "delta", root["delta"], "Hello ")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageContentEventTest.cs::TextMessageContentEventTest.Deserialize_RoundTrips
func TestCsTextMessageContentEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.TextMessageContentEvent{MessageID: "msg-1", Delta: "world"})
	eq(t, "MessageID", got.MessageID, "msg-1")
	eq(t, "Delta", got.Delta, "world")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageContentEventTest.cs::TextMessageContentEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsTextMessageContentEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TEXT_MESSAGE_CONTENT","messageId":"msg-1","delta":"hi"}`)
	textContent := asType[*agui.TextMessageContentEvent](t, evt)
	eq(t, "MessageID", textContent.MessageID, "msg-1")
	eq(t, "Delta", textContent.Delta, "hi")
}

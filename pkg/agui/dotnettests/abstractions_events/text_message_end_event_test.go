package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/TextMessageEndEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageEndEventTest.cs::TextMessageEndEventTest.Serialize_IncludesTypeAndMessageId
func TestCsTextMessageEndEventTest_Serialize_IncludesTypeAndMessageId(t *testing.T) {
	root := serialize(t, &agui.TextMessageEndEvent{MessageID: "msg-1"})
	eq(t, "type", root["type"], "TEXT_MESSAGE_END")
	eq(t, "messageId", root["messageId"], "msg-1")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageEndEventTest.cs::TextMessageEndEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsTextMessageEndEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TEXT_MESSAGE_END","messageId":"msg-1"}`)
	textEnd := asType[*agui.TextMessageEndEvent](t, evt)
	eq(t, "MessageID", textEnd.MessageID, "msg-1")
}

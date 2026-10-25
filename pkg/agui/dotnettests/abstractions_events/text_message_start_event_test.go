package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func role(r agui.TextMessageRole) *agui.TextMessageRole { return &r }

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Serialize_IncludesTypeAndMessageIdAndRole
func TestCsTextMessageStartEventTest_Serialize_IncludesTypeAndMessageIdAndRole(t *testing.T) {
	root := serialize(t, &agui.TextMessageStartEvent{MessageID: "msg-1", Role: role(agui.TextMessageRoleAssistant)})
	eq(t, "type", root["type"], "TEXT_MESSAGE_START")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "role", root["role"], "assistant")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Deserialize_RoundTrips
func TestCsTextMessageStartEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.TextMessageStartEvent{MessageID: "msg-2", Role: role(agui.TextMessageRoleUser)})
	eq(t, "MessageID", got.MessageID, "msg-2")
	eq(t, "Role", agui.Deref(got.Role), agui.TextMessageRoleUser)
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsTextMessageStartEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"TEXT_MESSAGE_START","messageId":"msg-3","role":"assistant"}`)
	textStart := asType[*agui.TextMessageStartEvent](t, evt)
	eq(t, "MessageID", textStart.MessageID, "msg-3")
	eq(t, "Role", agui.Deref(textStart.Role), agui.TextMessageRoleAssistant)
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Serialize_WithName_IncludesName
func TestCsTextMessageStartEventTest_Serialize_WithName_IncludesName(t *testing.T) {
	root := serialize(t, &agui.TextMessageStartEvent{MessageID: "msg-4", Role: role(agui.TextMessageRoleAssistant), Name: agui.Ptr("TestAgent")})
	eq(t, "name", root["name"], "TestAgent")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Serialize_WithoutName_OmitsName
func TestCsTextMessageStartEventTest_Serialize_WithoutName_OmitsName(t *testing.T) {
	root := serialize(t, &agui.TextMessageStartEvent{MessageID: "msg-5", Role: role(agui.TextMessageRoleAssistant)})
	if has(root, "name") {
		t.Errorf("name present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Deserialize_WithName_RoundTrips
func TestCsTextMessageStartEventTest_Deserialize_WithName_RoundTrips(t *testing.T) {
	var evt agui.TextMessageStartEvent
	if err := json.Unmarshal([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"msg-6","role":"assistant","name":"MyAgent"}`), &evt); err != nil {
		t.Fatal(err)
	}
	eq(t, "MessageID", evt.MessageID, "msg-6")
	eq(t, "Name", agui.Deref(evt.Name), "MyAgent")
}

// C#: tests/AGUI.Abstractions.UnitTests/TextMessageStartEventTest.cs::TextMessageStartEventTest.Deserialize_WithoutName_NameIsNull
func TestCsTextMessageStartEventTest_Deserialize_WithoutName_NameIsNull(t *testing.T) {
	var evt agui.TextMessageStartEvent
	if err := json.Unmarshal([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"msg-7","role":"assistant"}`), &evt); err != nil {
		t.Fatal(err)
	}
	if evt.Name != nil {
		t.Errorf("Name = %q, want nil", *evt.Name)
	}
}

package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningMessageStartEventTest.cs.
//
// In Go, REASONING_MESSAGE_START has no settable Role: the schema fixes
// role to the constant "reasoning" (required), so MarshalJSON always writes
// it. The C# Role property assertions are carried by the serialised form.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageStartEventTest.cs::ReasoningMessageStartEventTest.Serialize_IncludesTypeMessageIdAndRole
func TestCsReasoningMessageStartEventTest_Serialize_IncludesTypeMessageIdAndRole(t *testing.T) {
	root := serialize(t, &agui.ReasoningMessageStartEvent{MessageID: "msg-1"})
	eq(t, "type", root["type"], "REASONING_MESSAGE_START")
	eq(t, "messageId", root["messageId"], "msg-1")
	eq(t, "role", root["role"], "reasoning")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageStartEventTest.cs::ReasoningMessageStartEventTest.Deserialize_RoundTrips
func TestCsReasoningMessageStartEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.ReasoningMessageStartEvent{MessageID: "msg-2"})
	eq(t, "MessageID", got.MessageID, "msg-2")
	eq(t, "role (re-serialised)", serialize(t, got)["role"], "reasoning")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageStartEventTest.cs::ReasoningMessageStartEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningMessageStartEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_MESSAGE_START","messageId":"msg-3","role":"reasoning"}`)
	start := asType[*agui.ReasoningMessageStartEvent](t, evt)
	eq(t, "MessageID", start.MessageID, "msg-3")
	b, _ := json.Marshal(start)
	eq(t, "role (re-serialised)", toObject(t, b)["role"], "reasoning")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningMessageStartEventTest.cs::ReasoningMessageStartEventTest.Role_DefaultsToReasoning
func TestCsReasoningMessageStartEventTest_Role_DefaultsToReasoning(t *testing.T) {
	evt := &agui.ReasoningMessageStartEvent{MessageID: "msg-4"}
	eq(t, "role", serialize(t, evt)["role"], "reasoning")
}

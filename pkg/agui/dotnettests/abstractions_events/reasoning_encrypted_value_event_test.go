package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/ReasoningEncryptedValueEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEncryptedValueEventTest.cs::ReasoningEncryptedValueEventTest.Serialize_IncludesAllFields
func TestCsReasoningEncryptedValueEventTest_Serialize_IncludesAllFields(t *testing.T) {
	evt := &agui.ReasoningEncryptedValueEvent{
		Subtype:        "tool-call",
		EntityID:       "entity-1",
		EncryptedValue: "enc-abc123",
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "REASONING_ENCRYPTED_VALUE")
	eq(t, "subtype", root["subtype"], "tool-call")
	eq(t, "entityId", root["entityId"], "entity-1")
	eq(t, "encryptedValue", root["encryptedValue"], "enc-abc123")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEncryptedValueEventTest.cs::ReasoningEncryptedValueEventTest.Deserialize_RoundTrips
func TestCsReasoningEncryptedValueEventTest_Deserialize_RoundTrips(t *testing.T) {
	evt := &agui.ReasoningEncryptedValueEvent{
		Subtype:        "message",
		EntityID:       "entity-2",
		EncryptedValue: "enc-xyz789",
	}
	got := roundTrip(t, evt)
	eq(t, "Subtype", got.Subtype, agui.EncryptedValueSubtypeMessage)
	eq(t, "EntityID", got.EntityID, "entity-2")
	eq(t, "EncryptedValue", got.EncryptedValue, "enc-xyz789")
}

// C#: tests/AGUI.Abstractions.UnitTests/ReasoningEncryptedValueEventTest.cs::ReasoningEncryptedValueEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsReasoningEncryptedValueEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	evt := viaBaseEvent(t, `{"type":"REASONING_ENCRYPTED_VALUE","subtype":"tool-call","entityId":"e1","encryptedValue":"val"}`)
	encrypted := asType[*agui.ReasoningEncryptedValueEvent](t, evt)
	eq(t, "Subtype", encrypted.Subtype, agui.EncryptedValueSubtypeToolCall)
	eq(t, "EntityID", encrypted.EntityID, "e1")
	eq(t, "EncryptedValue", encrypted.EncryptedValue, "val")
}

package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func messageFixture(t *testing.T, i int) agui.Event {
	t.Helper()
	return decodeFixtureEvent(t, loadFixture(t, "message-events.json")[i])
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_WithTimestamp_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_WithTimestamp_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 0))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleAssistant)
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_Minimal_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_Minimal_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 1))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleAssistant)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_UserRole_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_UserRole_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 2))
	eq(t, "MessageID", typed.MessageID, "test-msg")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleUser)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_DeveloperRole_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_DeveloperRole_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 3))
	eq(t, "MessageID", typed.MessageID, "test-msg-developer")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleDeveloper)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_SystemRole_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_SystemRole_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 4))
	eq(t, "MessageID", typed.MessageID, "test-msg-system")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleSystem)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageStart_WithName_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageStart_WithName_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, messageFixture(t, 5))
	eq(t, "MessageID", typed.MessageID, "test-msg-named")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleAssistant)
	eq(t, "Name", agui.Deref(typed.Name), "TestAgent")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageContent_Basic_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageContent_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageContentEvent](t, messageFixture(t, 6))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Delta", typed.Delta, "Hello, how can I help you today?")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageContent_SpecialCharacters_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageContent_SpecialCharacters_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageContentEvent](t, messageFixture(t, 7))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	if !strings.Contains(typed.Delta, "\U0001F680") { // rocket emoji
		t.Errorf("Delta %q lacks the rocket emoji", typed.Delta)
	}
	if !strings.Contains(typed.Delta, "ñ") { // ñ
		t.Errorf("Delta %q lacks ñ", typed.Delta)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.TextMessageEnd_DeserializesFromTypeScriptPayload
func TestCsMessageEventsCompatibilityTest_TextMessageEnd_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.TextMessageEndEvent](t, messageFixture(t, 8))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/MessageEventsCompatibilityTest.cs::MessageEventsCompatibilityTest.AllMessageEvents_RoundTrip_PreservesType
func TestCsMessageEventsCompatibilityTest_AllMessageEvents_RoundTrip_PreservesType(t *testing.T) {
	assertRoundTripPreservesType(t, loadFixture(t, "message-events.json"))
}

package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func toolFixture(t *testing.T, i int) agui.Event {
	t.Helper()
	return decodeFixtureEvent(t, loadFixture(t, "tool-call-events.json")[i])
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallStart_Basic_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallStart_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallStartEvent](t, toolFixture(t, 0))
	eq(t, "ToolCallID", typed.ToolCallID, "tool-1")
	eq(t, "ToolCallName", typed.ToolCallName, "get_weather")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallStart_WithParentMessage_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallStart_WithParentMessage_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallStartEvent](t, toolFixture(t, 1))
	eq(t, "ToolCallID", typed.ToolCallID, "tool-1")
	eq(t, "ToolCallName", typed.ToolCallName, "search_database")
	eq(t, "ParentMessageID", agui.Deref(typed.ParentMessageID), "msg-123")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallArgs_Basic_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallArgs_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallArgsEvent](t, toolFixture(t, 2))
	eq(t, "ToolCallID", typed.ToolCallID, "tool-1")
	eq(t, "Delta", typed.Delta, `{"location":"San Francisco"}`)
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallArgs_ComplexJson_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallArgs_ComplexJson_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallArgsEvent](t, toolFixture(t, 3))
	eq(t, "ToolCallID", typed.ToolCallID, "db-query-tool-123")
	for _, want := range []string{"SELECT * FROM users", `"age":{"min":18,"max":65}`} {
		if !strings.Contains(typed.Delta, want) {
			t.Errorf("Delta lacks %q: %s", want, typed.Delta)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallArgs_PartialJson_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallArgs_PartialJson_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallArgsEvent](t, toolFixture(t, 4))
	eq(t, "ToolCallID", typed.ToolCallID, "streaming-tool")
	eq(t, "Delta", typed.Delta, `{"location":"San Fran`)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallEnd_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallEnd_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallEndEvent](t, toolFixture(t, 5))
	eq(t, "ToolCallID", typed.ToolCallID, "tool-1")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.ToolCallResult_DeserializesFromTypeScriptPayload
func TestCsToolCallEventsCompatibilityTest_ToolCallResult_DeserializesFromTypeScriptPayload(t *testing.T) {
	typed := asType[*agui.ToolCallResultEvent](t, toolFixture(t, 6))
	eq(t, "ToolCallID", typed.ToolCallID, "tc-1")
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Content.IsParts", typed.Content.IsParts(), false)
	eq(t, "Content", typed.Content.String(), `{"ok":true}`)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ToolCallEventsCompatibilityTest.cs::ToolCallEventsCompatibilityTest.AllToolCallEvents_RoundTrip_PreservesType
func TestCsToolCallEventsCompatibilityTest_AllToolCallEvents_RoundTrip_PreservesType(t *testing.T) {
	assertRoundTripPreservesType(t, loadFixture(t, "tool-call-events.json"))
}

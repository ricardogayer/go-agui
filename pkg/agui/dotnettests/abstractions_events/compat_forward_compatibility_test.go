package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs.
//
// The .NET models parse unknown properties leniently into extension data. In
// Go the processing model (Sanitize) is the layer that reads a newer peer's
// event: unknown properties are stripped with a warning naming the path and
// the event itself is delivered. Each test therefore also checks that the
// strict 1.0 schema rejects the extra field, that a warning names it, and
// that it does not survive into the decoded value.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// decodeForward decodes fixture i of forward-compatibility.json through the
// processing model and checks the extra fields were stripped with warnings.
func decodeForward(t *testing.T, i int, extras ...string) agui.Event {
	t.Helper()
	raw := loadFixture(t, "forward-compatibility.json")[i]
	if err := agui.Validate("Event", raw); err == nil {
		t.Errorf("strict 1.0 schema accepted undeclared properties %v", extras)
	}
	e, ws := decodeAsEvent(t, raw)
	for _, x := range extras {
		found := false
		for _, w := range ws {
			if w.Path == "/"+x {
				found = true
			}
		}
		if !found {
			t.Errorf("no warning names /%s: %v", x, ws)
		}
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range extras {
		if strings.Contains(string(b), x) {
			t.Errorf("stripped property %s survived: %s", x, b)
		}
	}
	return e
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.RunStarted_WithExtraFields_DeserializesWithoutError
func TestCsForwardCompatibilityTest_RunStarted_WithExtraFields_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.RunStartedEvent](t, decodeForward(t, 0, "futureField", "anotherFutureField"))
	eq(t, "ThreadID", typed.ThreadID, "t1")
	eq(t, "RunID", typed.RunID, "r1")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.TextMessageStart_WithExtraNestedFields_DeserializesWithoutError
func TestCsForwardCompatibilityTest_TextMessageStart_WithExtraNestedFields_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.TextMessageStartEvent](t, decodeForward(t, 1, "extraNested"))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Role", agui.Deref(typed.Role), agui.TextMessageRoleAssistant)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.TextMessageContent_WithExtraArrayField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_TextMessageContent_WithExtraArrayField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.TextMessageContentEvent](t, decodeForward(t, 2, "unknownArray"))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "Delta", typed.Delta, "hello")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.ToolCallStart_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_ToolCallStart_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.ToolCallStartEvent](t, decodeForward(t, 3, "futureToolField"))
	eq(t, "ToolCallID", typed.ToolCallID, "tc-1")
	eq(t, "ToolCallName", typed.ToolCallName, "test")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.StateSnapshot_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_StateSnapshot_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.StateSnapshotEvent](t, decodeForward(t, 4, "additionalMetadata"))
	eq(t, "snapshot.key", at(t, typed.Snapshot, "key"), "value")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.StateDelta_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_StateDelta_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.StateDeltaEvent](t, decodeForward(t, 5, "futureFlag"))
	eq(t, "len(Delta)", len(typed.Delta), 1)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.CustomEvent_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_CustomEvent_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.CustomEvent](t, decodeForward(t, 6, "futureCustomField"))
	eq(t, "Name", typed.Name, "test")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.RawEvent_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_RawEvent_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.RawEvent](t, decodeForward(t, 7, "futureRawField"))
	eq(t, "Source", agui.Deref(typed.Source), "test")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.ActivitySnapshot_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_ActivitySnapshot_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.ActivitySnapshotEvent](t, decodeForward(t, 8, "futureActivityField"))
	eq(t, "MessageID", typed.MessageID, "msg-1")
	eq(t, "ActivityType", typed.ActivityType, "PLAN")
	eq(t, "Replace", typed.Replace != nil && *typed.Replace, true)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.RunFinished_WithExtraField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_RunFinished_WithExtraField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.RunFinishedEvent](t, decodeForward(t, 9, "futureOutcome"))
	eq(t, "ThreadID", typed.ThreadID, "t1")
	eq(t, "RunID", typed.RunID, "r1")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.RunError_WithExtraNestedField_DeserializesWithoutError
func TestCsForwardCompatibilityTest_RunError_WithExtraNestedField_DeserializesWithoutError(t *testing.T) {
	typed := asType[*agui.RunErrorEvent](t, decodeForward(t, 10, "futureErrorDetails"))
	eq(t, "Message", typed.Message, "fail")
	eq(t, "Code", agui.Deref(typed.Code), "ERR")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/ForwardCompatibilityTest.cs::ForwardCompatibilityTest.AllForwardCompatibilityEvents_CanDeserialize
func TestCsForwardCompatibilityTest_AllForwardCompatibilityEvents_CanDeserialize(t *testing.T) {
	for i, raw := range loadFixture(t, "forward-compatibility.json") {
		e, _ := decodeAsEvent(t, raw)
		if e == nil || e.Type() == "" {
			t.Errorf("event at index %d has no type", i)
		}
	}
}

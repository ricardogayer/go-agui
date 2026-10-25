package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs.
//
// Every schema event crosses the binary transport. An envelope carrying a
// variant this build was not compiled against decodes to an explicit
// unknown-event error (protobuf.ErrUnknownEventType, the .NET
// AGUIUnknownEventTypeException) rather than to some other event; malformed
// input is never confused with it.

import (
	"errors"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.FormerlyUnsupportedEvent_RoundTrips
func TestCsUnsupportedEventTest_FormerlyUnsupportedEvent_RoundTrips(t *testing.T) {
	rows := []agui.Event{
		&agui.ReasoningStartEvent{MessageID: "m1"},
		&agui.ReasoningEndEvent{MessageID: "m1"},
		&agui.ReasoningMessageStartEvent{MessageID: "m1"},
		&agui.ReasoningMessageContentEvent{MessageID: "m1", Delta: "d"},
		&agui.ReasoningMessageEndEvent{MessageID: "m1"},
		&agui.ReasoningMessageChunkEvent{MessageID: agui.Ptr("m1"), Delta: agui.Ptr("d")},
		&agui.ReasoningEncryptedValueEvent{Subtype: agui.EncryptedValueSubtypeMessage, EntityID: "m1", EncryptedValue: "v"},
		&agui.ActivitySnapshotEvent{MessageID: "m1", ActivityType: "a", Content: map[string]any{}},
		&agui.ActivityDeltaEvent{MessageID: "m1", ActivityType: "a", Patch: agui.JSONPatch{}},
		&agui.ToolCallResultEvent{MessageID: "m1", ToolCallID: "c1", Content: agui.TextContent("ok")},
		&agui.TextMessageChunkEvent{MessageID: agui.Ptr("m1"), Delta: agui.Ptr("d")},
		&agui.ToolCallChunkEvent{ToolCallID: agui.Ptr("c1"), Delta: agui.Ptr("{")},
	}
	for _, e := range rows {
		t.Run(string(e.Type()), func(t *testing.T) {
			eq(t, "Type", decode(t, encode(t, e)).Type(), e.Type())
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_SubagentEvent_RoundTrips
func TestCsUnsupportedEventTest_Decode_SubagentEvent_RoundTrips(t *testing.T) {
	envelopes := map[string][]byte{
		"SUBAGENT_STARTED":  wrap(armSubagentStarted, etSubagentStarted, sf(2, "s1"), sf(3, "researcher")),
		"SUBAGENT_FINISHED": wrap(armSubagentFinished, etSubagentFinished, sf(2, "s1")),
		"SUBAGENT_ERROR":    wrap(armSubagentError, etSubagentError, sf(2, "s1"), sf(3, "boom")),
	}
	for _, kind := range []string{"SUBAGENT_STARTED", "SUBAGENT_FINISHED", "SUBAGENT_ERROR"} {
		t.Run(kind, func(t *testing.T) {
			decoded := decode(t, envelopes[kind])
			eq(t, "Type", string(decoded.Type()), kind)
			eq(t, "SubagentRunID", agui.SubagentOf(decoded), "s1")
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_EnvelopeWithOnlyUnknownTags_ThrowsUnknownEventType
func TestCsUnsupportedEventTest_Decode_EnvelopeWithOnlyUnknownTags_ThrowsUnknownEventType(t *testing.T) {
	// Field 900, length-delimited, empty payload: a variant far outside the
	// envelope this build knows.
	_, err := protobuf.Decode([]byte{0xE2, 0x38, 0x00})
	if !errors.Is(err, protobuf.ErrUnknownEventType) {
		t.Fatalf("got %v, want ErrUnknownEventType", err)
	}
	if protobuf.IsMalformed(err) {
		t.Fatal("an event from the future is not malformed")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_EmptyEnvelope_ThrowsInvalidData
func TestCsUnsupportedEventTest_Decode_EmptyEnvelope_ThrowsInvalidData(t *testing.T) {
	mustBeMalformed(t, []byte{}, "")
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_KnownTagWithUnreadablePayload_ThrowsInvalidData
func TestCsUnsupportedEventTest_Decode_KnownTagWithUnreadablePayload_ThrowsInvalidData(t *testing.T) {
	// Envelope field 1 (TEXT_MESSAGE_START) carried as a varint.
	mustBeMalformed(t, []byte{0x08, 0x01}, "")
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_KnownTagCarriedAsGroup_ThrowsInvalidData
func TestCsUnsupportedEventTest_Decode_KnownTagCarriedAsGroup_ThrowsInvalidData(t *testing.T) {
	// Envelope field 1 as a start group, then its end group.
	mustBeMalformed(t, []byte{0x0B, 0x0C}, "")
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Decode_UnknownVarintOnly_ThrowsInvalidData
func TestCsUnsupportedEventTest_Decode_UnknownVarintOnly_ThrowsInvalidData(t *testing.T) {
	// Field 900, varint, value 1: every event arm is length-delimited, so this
	// is broken bytes, not an event to learn about later.
	mustBeMalformed(t, []byte{0xE0, 0x38, 0x01}, "")
}

// C#: tests/AGUI.Protobuf.UnitTests/UnsupportedEventTest.cs::UnsupportedEventTest.Encode_Null_Throws
func TestCsUnsupportedEventTest_Encode_Null_Throws(t *testing.T) {
	if b, err := protobuf.Encode(nil); err == nil {
		t.Fatalf("Encode(nil) = %x, want an error", b)
	}
	if b, err := protobuf.EncodeJSON([]byte("null")); err == nil {
		t.Fatalf("EncodeJSON(null) = %x, want an error", b)
	}
}

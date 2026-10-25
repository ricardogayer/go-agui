package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/MetadataTest.cs.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const metadataValueShapesJSON = `{
  "nullValue": null,
  "string": "finish_reason",
  "number": 42,
  "float": 1.5,
  "boolean": true,
  "emptyArray": [],
  "array": [1, "two", null, { "nested": true }],
  "emptyObject": {},
  "nested": { "usage": { "input": 10, "output": 20 }, "tags": ["a", "b"] }
}`

// reservedMetadataKey is AGUIMetadata.ReservedKey: the key the spec reserves
// for the protocol's own use.
const reservedMetadataKey = "ag-ui"

func valueShapes(t *testing.T, doc string) agui.Metadata {
	t.Helper()
	var m agui.Metadata
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func messageMetadata(t *testing.T, m agui.Message) agui.Metadata {
	t.Helper()
	switch x := m.(type) {
	case *agui.DeveloperMessage:
		return x.Metadata
	case *agui.SystemMessage:
		return x.Metadata
	case *agui.AssistantMessage:
		return x.Metadata
	case *agui.UserMessage:
		return x.Metadata
	case *agui.ToolMessage:
		return x.Metadata
	case *agui.ActivityMessage:
		return x.Metadata
	case *agui.ReasoningMessage:
		return x.Metadata
	}
	t.Fatalf("unknown message type %T", m)
	return nil
}

func setMessageMetadata(t *testing.T, m agui.Message, md agui.Metadata) {
	t.Helper()
	switch x := m.(type) {
	case *agui.DeveloperMessage:
		x.Metadata = md
	case *agui.SystemMessage:
		x.Metadata = md
	case *agui.AssistantMessage:
		x.Metadata = md
	case *agui.UserMessage:
		x.Metadata = md
	case *agui.ToolMessage:
		x.Metadata = md
	case *agui.ActivityMessage:
		x.Metadata = md
	case *agui.ReasoningMessage:
		x.Metadata = md
	default:
		t.Fatalf("unknown message type %T", m)
	}
}

// allMessageRoles is the AllMessageRoles member data.
func allMessageRoles() []agui.Message {
	return []agui.Message{
		&agui.DeveloperMessage{ID: "1", Content: "c"},
		&agui.SystemMessage{ID: "1", Content: "c"},
		&agui.AssistantMessage{ID: "1", Content: agui.Ptr("c")},
		&agui.UserMessage{ID: "1", Content: agui.TextContent("c")},
		&agui.ToolMessage{ID: "1", Content: agui.TextContent("c"), ToolCallID: "tc1"},
		&agui.ActivityMessage{ID: "1", ActivityType: "PLAN", Content: map[string]any{}},
		&agui.ReasoningMessage{ID: "1", Content: "c"},
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Event_SerializesEveryValueShape
func TestCsMetadataTest_Event_SerializesEveryValueShape(t *testing.T) {
	evt := &agui.TextMessageStartEvent{BaseEvent: agui.BaseEvent{Metadata: valueShapes(t, metadataValueShapesJSON)}, MessageID: "m1"}
	md := objOf(t, obj(t, evt)["metadata"])
	if v, ok := md["nullValue"]; !ok || v != nil {
		t.Errorf("nullValue must be written as null, got %v", md)
	}
	assertEq(t, md["string"], "finish_reason", "string")
	assertEq(t, md["number"], float64(42), "number")
	assertEq(t, md["float"], 1.5, "float")
	assertEq(t, md["boolean"], true, "boolean")
	assertEq(t, len(arrOf(t, md["emptyArray"])), 0, "emptyArray length")
	assertEq(t, len(arrOf(t, md["array"])), 4, "array length")
	assertEq(t, objOf(t, objOf(t, md["nested"])["usage"])["input"], float64(10), "nested.usage.input")
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Event_RoundTripsMetadata
func TestCsMetadataTest_Event_RoundTripsMetadata(t *testing.T) {
	evt := &agui.TextMessageStartEvent{BaseEvent: agui.BaseEvent{Metadata: valueShapes(t, metadataValueShapesJSON)}, MessageID: "m1"}
	back := roundTripEvent(t, evt).(*agui.TextMessageStartEvent)
	if back.Metadata == nil {
		t.Fatal("metadata lost")
	}
	if !jsonEqual(t, mustJSON(t, back.Metadata), []byte(metadataValueShapesJSON)) {
		t.Fatalf("metadata changed: %s", mustJSON(t, back.Metadata))
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Event_OmitsAbsentMetadataFromTheWire
func TestCsMetadataTest_Event_OmitsAbsentMetadataFromTheWire(t *testing.T) {
	assertOmitted(t, obj(t, &agui.TextMessageStartEvent{MessageID: "m1"}), "metadata")
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Event_ReadsExplicitNullAsAbsent
func TestCsMetadataTest_Event_ReadsExplicitNullAsAbsent(t *testing.T) {
	var e agui.TextMessageStartEvent
	if err := json.Unmarshal([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"m1","metadata":null}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Metadata != nil {
		t.Fatalf("explicit null must read as absent, got %v", e.Metadata)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Event_DistinguishesEmptyObjectFromAbsent
func TestCsMetadataTest_Event_DistinguishesEmptyObjectFromAbsent(t *testing.T) {
	var e agui.TextMessageStartEvent
	if err := json.Unmarshal([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"m1","metadata":{}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Metadata == nil {
		t.Fatal("an empty object must not read as absent")
	}
	if len(e.Metadata) != 0 {
		t.Fatalf("expected an empty object, got %v", e.Metadata)
	}
	// And it survives re-serialization as {}, distinct from omission.
	if md, ok := obj(t, &e)["metadata"]; !ok || len(objOf(t, md)) != 0 {
		t.Fatalf("empty metadata object not written back as {}")
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.NonMessageEvent_CarriesMetadata
func TestCsMetadataTest_NonMessageEvent_CarriesMetadata(t *testing.T) {
	evt := &agui.RunFinishedEvent{
		BaseEvent: agui.BaseEvent{Metadata: valueShapes(t, `{"usage":{"total":100}}`)},
		ThreadID:  "t1", RunID: "r1",
	}
	back := roundTripEvent(t, evt).(*agui.RunFinishedEvent)
	if back.Metadata == nil {
		t.Fatal("metadata lost")
	}
	assertEq(t, objOf(t, back.Metadata["usage"])["total"], float64(100), "usage.total")
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Message_RoundTripsMetadataForEveryRole
func TestCsMetadataTest_Message_RoundTripsMetadataForEveryRole(t *testing.T) {
	for _, m := range allMessageRoles() {
		t.Run(string(m.MessageRole()), func(t *testing.T) {
			setMessageMetadata(t, m, valueShapes(t, metadataValueShapesJSON))
			back := roundTripMessage(t, m)
			md := messageMetadata(t, back)
			if md == nil {
				t.Fatal("metadata lost")
			}
			if !jsonEqual(t, mustJSON(t, md), []byte(metadataValueShapesJSON)) {
				t.Fatalf("metadata changed: %s", mustJSON(t, md))
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Message_OmitsAbsentMetadataForEveryRole
func TestCsMetadataTest_Message_OmitsAbsentMetadataForEveryRole(t *testing.T) {
	for _, m := range allMessageRoles() {
		t.Run(string(m.MessageRole()), func(t *testing.T) {
			assertOmitted(t, obj(t, m), "metadata")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Message_ReadsExplicitNullAsAbsentForEveryRole
func TestCsMetadataTest_Message_ReadsExplicitNullAsAbsentForEveryRole(t *testing.T) {
	for _, m := range allMessageRoles() {
		t.Run(string(m.MessageRole()), func(t *testing.T) {
			js := strings.TrimSuffix(string(mustJSON(t, m)), "}") + `,"metadata":null}`
			back, err := agui.UnmarshalMessage([]byte(js))
			if err != nil {
				t.Fatalf("decode %s: %v", js, err)
			}
			if md := messageMetadata(t, back); md != nil {
				t.Fatalf("explicit null must read as absent, got %v", md)
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/MetadataTest.cs::MetadataTest.Message_CarriesTheReservedKey
func TestCsMetadataTest_Message_CarriesTheReservedKey(t *testing.T) {
	msg := &agui.AssistantMessage{
		ID: "1", Content: agui.Ptr("hello"),
		Metadata: valueShapes(t, `{"`+reservedMetadataKey+`":{"usage":{"input":1}}}`),
	}
	back := roundTripMessage(t, msg).(*agui.AssistantMessage)
	if back.Metadata == nil {
		t.Fatal("metadata lost")
	}
	reserved := objOf(t, back.Metadata[reservedMetadataKey])
	assertEq(t, objOf(t, reserved["usage"])["input"], float64(1), "ag-ui.usage.input")
}

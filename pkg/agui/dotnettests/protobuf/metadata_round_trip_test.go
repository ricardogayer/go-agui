package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs: metadata
// survives the binary transport with every value shape intact, including
// nulls under a key, and an absent object stays distinguishable from an
// empty one. ProtoEventMapper.ToProto/FromProto is the encoder/decoder pair.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

const valueShapesJSON = `{
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

func valueShapes(t *testing.T) agui.Metadata { return agui.Metadata(obj(t, valueShapesJSON)) }

func get(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m := asType[map[string]any](t, v)
			var ok bool
			if v, ok = m[k]; !ok {
				t.Fatalf("missing key %q", k)
			}
		case int:
			v = asType[[]any](t, v)[k]
		}
	}
	return v
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.Event_PreservesEveryValueShape
func TestCsMetadataRoundTripTest_Event_PreservesEveryValueShape(t *testing.T) {
	e := &agui.TextMessageStartEvent{MessageID: "m1"}
	e.Metadata = valueShapes(t)
	restored := roundTrip(t, e)
	if restored.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	md := map[string]any(restored.Metadata)
	if v := get(t, md, "nullValue"); v != nil {
		t.Fatalf("nullValue = %v, want null", v)
	}
	eq(t, "string", get(t, md, "string"), any("finish_reason"))
	eq(t, "number", get(t, md, "number"), any(42.0))
	eq(t, "float", get(t, md, "float"), any(1.5))
	eq(t, "boolean", get(t, md, "boolean"), any(true))
	eq(t, "len(emptyArray)", len(asType[[]any](t, get(t, md, "emptyArray"))), 0)
	eq(t, "len(array)", len(asType[[]any](t, get(t, md, "array"))), 4)
	if v := get(t, md, "array", 2); v != nil {
		t.Fatalf("array[2] = %v, want null", v)
	}
	eq(t, "nested.usage.input", get(t, md, "nested", "usage", "input"), any(10.0))
	eq(t, "nested.tags[1]", get(t, md, "nested", "tags", 1), any("b"))
	// The whole document, including the empty object, survives.
	assertJSONEqual(t, parseJSON(t, valueShapesJSON), md)
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.Event_KeepsAbsentMetadataAbsent
func TestCsMetadataRoundTripTest_Event_KeepsAbsentMetadataAbsent(t *testing.T) {
	restored := roundTrip(t, &agui.TextMessageStartEvent{MessageID: "m1"})
	if restored.Metadata != nil {
		t.Fatalf("Metadata = %v, want nil", restored.Metadata)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.Event_DistinguishesEmptyObjectFromAbsent
func TestCsMetadataRoundTripTest_Event_DistinguishesEmptyObjectFromAbsent(t *testing.T) {
	e := &agui.TextMessageStartEvent{MessageID: "m1"}
	e.Metadata = agui.Metadata{}
	restored := roundTrip(t, e)
	if restored.Metadata == nil {
		t.Fatal("an empty metadata object came back absent")
	}
	eq(t, "len(Metadata)", len(restored.Metadata), 0)
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.NonMessageEvent_CarriesMetadata
func TestCsMetadataRoundTripTest_NonMessageEvent_CarriesMetadata(t *testing.T) {
	e := &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"}
	e.Metadata = agui.Metadata(obj(t, `{"usage":{"total":100}}`))
	restored := roundTrip(t, e)
	eq(t, "usage.total", get(t, map[string]any(restored.Metadata), "usage", "total"), any(100.0))
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.MessagesSnapshot_CarriesPerMessageMetadataWithoutLeaking
func TestCsMetadataRoundTripTest_MessagesSnapshot_CarriesPerMessageMetadataWithoutLeaking(t *testing.T) {
	restored := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("a"), Metadata: valueShapes(t)},
		&agui.AssistantMessage{ID: "m2", Content: agui.Ptr("b")},
	}})
	first := asType[*agui.AssistantMessage](t, restored.Messages[0])
	eq(t, "number", get(t, map[string]any(first.Metadata), "number"), any(42.0))
	// The second message had none and must not gain any.
	if md := asType[*agui.AssistantMessage](t, restored.Messages[1]).Metadata; md != nil {
		t.Fatalf("second message gained metadata %v", md)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.Message_PreservesTheReservedKey
func TestCsMetadataRoundTripTest_Message_PreservesTheReservedKey(t *testing.T) {
	restored := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("a"),
			Metadata: agui.Metadata(obj(t, `{"ag-ui":{"usage":{"input":1}}}`))},
	}})
	md := map[string]any(asType[*agui.AssistantMessage](t, restored.Messages[0]).Metadata)
	eq(t, "ag-ui.usage.input", get(t, md, "ag-ui", "usage", "input"), any(1.0))
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.NullKindMetadata_IsTreatedAsAbsent
func TestCsMetadataRoundTripTest_NullKindMetadata_IsTreatedAsAbsent(t *testing.T) {
	// A null standing in for the object reads as absent, the null-as-absent
	// rule the other SDKs apply. (The Go model cannot hold it — nil is
	// absent — so the document carries the explicit null.)
	doc := roundTripDoc(t, `{"type":"TEXT_MESSAGE_START","messageId":"m1","metadata":null}`)
	if v, ok := doc["metadata"]; ok {
		t.Fatalf("metadata = %v, want absent", v)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.NonObjectMetadata_FailsLoudlyInsteadOfBeingSilentlyDropped
func TestCsMetadataRoundTripTest_NonObjectMetadata_FailsLoudlyInsteadOfBeingSilentlyDropped(t *testing.T) {
	// Metadata is an object. The Go model cannot hold anything else
	// (agui.Metadata is a map), but EncodeJSON accepts any document, and the
	// encoder must not silently drop a non-object value — that would make it
	// survive JSON and vanish over protobuf. .NET throws; the TypeScript
	// reference encodes "as given" with a console warning naming the path.
	// Either loud answer satisfies the rule: an error, or a warning.
	for _, j := range []string{`[1,2,3]`, `"a string"`, `42`, `true`} {
		t.Run(j, func(t *testing.T) {
			var warnings []string
			b, err := protobuf.EncodeJSON([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"m1","metadata":`+j+`}`),
				protobuf.Options{Warn: func(s string) { warnings = append(warnings, s) }})
			if err == nil && len(warnings) == 0 {
				t.Fatalf("non-object metadata %s was silently dropped: encoded %x with no error and no warning", j, b)
			}
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/MetadataRoundTripTest.cs::MetadataRoundTripTest.ToolCall_CarriesItsOwnMetadataIndependently
func TestCsMetadataRoundTripTest_ToolCall_CarriesItsOwnMetadataIndependently(t *testing.T) {
	// A tool call is not a message, so it carries its own metadata.
	tc1 := agui.NewToolCall("tc1", "a", "{}")
	tc1.Metadata = valueShapes(t)
	tc2 := agui.NewToolCall("tc2", "b", "{}") // carries none, so a leak would surface
	restored := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr(""), ToolCalls: []agui.ToolCall{tc1, tc2}},
	}})
	msg := asType[*agui.AssistantMessage](t, restored.Messages[0])
	eq(t, "toolCalls[0].number", get(t, map[string]any(msg.ToolCalls[0].Metadata), "number"), any(42.0))
	if msg.ToolCalls[1].Metadata != nil {
		t.Fatalf("toolCalls[1] gained metadata %v", msg.ToolCalls[1].Metadata)
	}
	if msg.Metadata != nil {
		t.Fatalf("the message gained metadata %v", msg.Metadata)
	}
}

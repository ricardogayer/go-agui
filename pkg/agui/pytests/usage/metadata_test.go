package usage_test

// Port of sdks/python/tests/test_metadata.py.

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// valueShapesJSON is VALUE_SHAPES: every JSON shape the protocol promises
// survives a round trip.
const valueShapesJSON = `{
	"nullValue": null,
	"string": "finish_reason",
	"number": 42,
	"float": 1.5,
	"boolean": true,
	"emptyArray": [],
	"array": [1, "two", null, {"nested": true}],
	"emptyObject": {},
	"nested": {"usage": {"input": 10, "output": 20}, "tags": ["a", "b"]}
}`

// aguiMetadataKey is the protocol-reserved metadata key (Python's
// AGUI_METADATA_KEY); the Go library exports no constant for it.
const aguiMetadataKey = "ag-ui"

// valueShapes returns a fresh VALUE_SHAPES map, decoded from JSON so it
// compares equal to what a JSON round trip yields.
func valueShapes(t *testing.T) agui.Metadata {
	t.Helper()
	var m agui.Metadata
	if err := json.Unmarshal([]byte(valueShapesJSON), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func roundTripEvent(t *testing.T, e agui.Event) agui.Event {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := agui.ParseEvent(b)
	if err != nil {
		t.Fatalf("ParseEvent(%s): %v", b, err)
	}
	return out
}

func roundTripMessage(t *testing.T, m agui.Message) agui.Message {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := agui.UnmarshalMessage(b)
	if err != nil {
		t.Fatalf("UnmarshalMessage(%s): %v", b, err)
	}
	return out
}

// encodeAfterRunStart sends e through the official producer path
// (stream.SSE.Send, after the RUN_STARTED it requires) and returns the
// decoded JSON payload of e's SSE frame.
func encodeAfterRunStart(t *testing.T, e agui.Event) (map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	s := stream.NewWriter(&buf)
	if err := s.RunStarted("t1", "r1"); err != nil {
		t.Fatalf("RunStarted: %v", err)
	}
	if err := s.Send(e); err != nil {
		t.Fatalf("Send: %v", err)
	}
	r := sse.NewReader(&buf)
	var last []byte
	for {
		data, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		last = data
	}
	var payload map[string]any
	if err := json.Unmarshal(last, &payload); err != nil {
		t.Fatalf("frame payload %s: %v", last, err)
	}
	return payload, string(last)
}

// ---------------------------------------------------------------------------
// TestMetadataOnEvents
// ---------------------------------------------------------------------------

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_accepts_every_value_shape
func TestPyTestMetadataOnEvents_AcceptsEveryValueShape(t *testing.T) {
	e := &agui.TextMessageStartEvent{MessageID: "m1", BaseEvent: agui.BaseEvent{Metadata: valueShapes(t)}}
	if !reflect.DeepEqual(e.Metadata, valueShapes(t)) {
		t.Fatalf("metadata = %v", e.Metadata)
	}
	if err := agui.ValidateEvent(e); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_absent_by_default
func TestPyTestMetadataOnEvents_AbsentByDefault(t *testing.T) {
	e := agui.TextMessageStartEvent{MessageID: "m1"}
	if e.Metadata != nil {
		t.Fatalf("metadata = %v, want absent", e.Metadata)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_accepts_empty_object
func TestPyTestMetadataOnEvents_AcceptsEmptyObject(t *testing.T) {
	e := &agui.CustomEvent{Name: "n", Value: 1, BaseEvent: agui.BaseEvent{Metadata: agui.Metadata{}}}
	if e.Metadata == nil || len(e.Metadata) != 0 {
		t.Fatalf("metadata = %#v, want {}", e.Metadata)
	}
	// An empty object is a present value, distinct from absent.
	got := roundTripEvent(t, e).Base().Metadata
	if got == nil || len(got) != 0 {
		t.Fatalf("round-tripped metadata = %#v, want {}", got)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_explicit_null_reads_back_as_absent
//
// Adapted (b): Python accepts "metadata": null because its own
// model_dump_json() can emit it. The spec (basic/metadata.mdx) says the object
// "MUST NOT be null", and the Go producer never writes it. Lenient typed
// decoding still reads a null as absent; the strict schema check rejects it.
func TestPyTestMetadataOnEvents_ExplicitNullReadsBackAsAbsent(t *testing.T) {
	doc := `{"type": "TEXT_MESSAGE_START", "messageId": "m1", "metadata": null}`
	ev, err := agui.UnmarshalEvent([]byte(doc))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	if md := ev.Base().Metadata; md != nil {
		t.Fatalf("metadata = %v, want absent", md)
	}
	mustReject(t, "Event", doc)
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_plain_model_dump_json_round_trips
func TestPyTestMetadataOnEvents_PlainModelDumpJsonRoundTrips(t *testing.T) {
	original := &agui.TextMessageStartEvent{MessageID: "m1"}
	if _, ok := object(t, original)["metadata"]; ok {
		t.Fatalf("plain marshal wrote metadata for an event without it")
	}
	if md := roundTripEvent(t, original).Base().Metadata; md != nil {
		t.Fatalf("metadata = %v, want absent", md)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_json_round_trip
func TestPyTestMetadataOnEvents_JsonRoundTrip(t *testing.T) {
	e := &agui.ToolCallResultEvent{
		MessageID: "m1", ToolCallID: "tc1", Content: agui.TextContent("done"),
		BaseEvent: agui.BaseEvent{Metadata: valueShapes(t)},
	}
	got := roundTripEvent(t, e).Base().Metadata
	if !reflect.DeepEqual(got, valueShapes(t)) {
		t.Fatalf("round-tripped metadata = %v", got)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnEvents::test_non_message_event_carries_metadata
func TestPyTestMetadataOnEvents_NonMessageEventCarriesMetadata(t *testing.T) {
	e := &agui.RunFinishedEvent{
		ThreadID: "t1", RunID: "r1",
		BaseEvent: agui.BaseEvent{Metadata: agui.Metadata{"usage": map[string]any{"total": 100}}},
	}
	if !reflect.DeepEqual(e.Metadata, agui.Metadata{"usage": map[string]any{"total": 100}}) {
		t.Fatalf("metadata = %v", e.Metadata)
	}
	assertJSON(t, object(t, e)["metadata"], `{"usage": {"total": 100}}`)
}

// ---------------------------------------------------------------------------
// TestMetadataOnMessages
// ---------------------------------------------------------------------------

type messageCase struct {
	name string
	make func(md agui.Metadata) agui.Message
	get  func(m agui.Message) agui.Metadata
}

func messageCases() []messageCase {
	return []messageCase{
		{"DeveloperMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.DeveloperMessage{ID: "1", Content: "c", Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.DeveloperMessage).Metadata }},
		{"SystemMessage",
			func(md agui.Metadata) agui.Message { return &agui.SystemMessage{ID: "1", Content: "c", Metadata: md} },
			func(m agui.Message) agui.Metadata { return m.(*agui.SystemMessage).Metadata }},
		{"AssistantMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.AssistantMessage{ID: "1", Content: agui.Ptr("c"), Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.AssistantMessage).Metadata }},
		{"UserMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.UserMessage{ID: "1", Content: agui.TextContent("c"), Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.UserMessage).Metadata }},
		{"ToolMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.ToolMessage{ID: "1", Content: agui.TextContent("c"), ToolCallID: "tc1", Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.ToolMessage).Metadata }},
		{"ActivityMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.ActivityMessage{ID: "1", ActivityType: "PLAN", Content: map[string]any{}, Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.ActivityMessage).Metadata }},
		{"ReasoningMessage",
			func(md agui.Metadata) agui.Message {
				return &agui.ReasoningMessage{ID: "1", Content: "c", Metadata: md}
			},
			func(m agui.Message) agui.Metadata { return m.(*agui.ReasoningMessage).Metadata }},
	}
}

// Python: tests/test_metadata.py::TestMetadataOnMessages::test_all_message_types_carry_metadata
func TestPyTestMetadataOnMessages_AllMessageTypesCarryMetadata(t *testing.T) {
	for _, c := range messageCases() {
		t.Run(c.name, func(t *testing.T) {
			m := c.make(valueShapes(t))
			if !reflect.DeepEqual(c.get(m), valueShapes(t)) {
				t.Fatalf("metadata = %v", c.get(m))
			}
			b, _ := json.Marshal(m)
			if err := agui.Validate("Message", b); err != nil {
				t.Fatalf("Validate(Message, %s): %v", b, err)
			}
		})
	}
}

// Python: tests/test_metadata.py::TestMetadataOnMessages::test_all_message_types_allow_absent_metadata
func TestPyTestMetadataOnMessages_AllMessageTypesAllowAbsentMetadata(t *testing.T) {
	for _, c := range messageCases() {
		t.Run(c.name, func(t *testing.T) {
			if md := c.get(c.make(nil)); md != nil {
				t.Fatalf("metadata = %v, want absent", md)
			}
		})
	}
}

// Python: tests/test_metadata.py::TestMetadataOnMessages::test_all_message_types_read_null_metadata_as_absent
//
// Python passes metadata=None to the constructor; the Go counterpart is
// decoding a message whose metadata is JSON null (lenient typed decode).
func TestPyTestMetadataOnMessages_AllMessageTypesReadNullMetadataAsAbsent(t *testing.T) {
	for _, c := range messageCases() {
		t.Run(c.name, func(t *testing.T) {
			m := object(t, c.make(nil))
			m["metadata"] = nil
			b, _ := json.Marshal(m)
			out, err := agui.UnmarshalMessage(b)
			if err != nil {
				t.Fatalf("UnmarshalMessage(%s): %v", b, err)
			}
			if md := c.get(out); md != nil {
				t.Fatalf("metadata = %v, want absent", md)
			}
		})
	}
}

// Python: tests/test_metadata.py::TestMetadataOnMessages::test_all_message_types_survive_a_plain_json_round_trip
func TestPyTestMetadataOnMessages_AllMessageTypesSurviveAPlainJsonRoundTrip(t *testing.T) {
	for _, c := range messageCases() {
		t.Run(c.name, func(t *testing.T) {
			restored := roundTripMessage(t, c.make(nil))
			if md := c.get(restored); md != nil {
				t.Fatalf("metadata = %v, want absent", md)
			}
		})
	}
}

// Python: tests/test_metadata.py::TestMetadataOnMessages::test_message_json_round_trip
func TestPyTestMetadataOnMessages_MessageJsonRoundTrip(t *testing.T) {
	md := agui.Metadata{aguiMetadataKey: map[string]any{"usage": map[string]any{"input": float64(1)}}}
	m := &agui.AssistantMessage{ID: "1", Content: agui.Ptr("hello"), Metadata: md}
	restored := roundTripMessage(t, m).(*agui.AssistantMessage)
	if !reflect.DeepEqual(restored.Metadata, md) {
		t.Fatalf("metadata = %v, want %v", restored.Metadata, md)
	}
}

// ---------------------------------------------------------------------------
// TestMetadataOnToolCalls
// ---------------------------------------------------------------------------

// Python: tests/test_metadata.py::TestMetadataOnToolCalls::test_tool_call_carries_metadata
func TestPyTestMetadataOnToolCalls_ToolCallCarriesMetadata(t *testing.T) {
	call := agui.ToolCall{ID: "tc1", Function: agui.FunctionCall{Name: "search", Arguments: "{}"}, Metadata: valueShapes(t)}
	if !reflect.DeepEqual(call.Metadata, valueShapes(t)) {
		t.Fatalf("metadata = %v", call.Metadata)
	}
}

// Python: tests/test_metadata.py::TestMetadataOnToolCalls::test_tool_call_metadata_is_optional
func TestPyTestMetadataOnToolCalls_ToolCallMetadataIsOptional(t *testing.T) {
	call := agui.NewToolCall("tc1", "search", "{}")
	if call.Metadata != nil {
		t.Fatalf("metadata = %v, want absent", call.Metadata)
	}
	if _, ok := object(t, call)["metadata"]; ok {
		t.Fatalf("absent tool-call metadata serialized")
	}
}

// Python: tests/test_metadata.py::TestMetadataOnToolCalls::test_tool_calls_keep_their_metadata_separate
func TestPyTestMetadataOnToolCalls_ToolCallsKeepTheirMetadataSeparate(t *testing.T) {
	first := agui.NewToolCall("tc1", "a", "{}")
	first.Metadata = agui.Metadata{"phase": "one"}
	m := &agui.AssistantMessage{ID: "m1", ToolCalls: []agui.ToolCall{first, agui.NewToolCall("tc2", "b", "{}")}}
	restored := roundTripMessage(t, m).(*agui.AssistantMessage)
	if !reflect.DeepEqual(restored.ToolCalls[0].Metadata, agui.Metadata{"phase": "one"}) {
		t.Fatalf("toolCalls[0].metadata = %v", restored.ToolCalls[0].Metadata)
	}
	if restored.ToolCalls[1].Metadata != nil {
		t.Fatalf("toolCalls[1].metadata = %v, want absent", restored.ToolCalls[1].Metadata)
	}
	if restored.Metadata != nil {
		t.Fatalf("message metadata = %v, want absent", restored.Metadata)
	}
}

// ---------------------------------------------------------------------------
// TestMetadataEncoding (EventEncoder -> stream.SSE, the Go producer path)
// ---------------------------------------------------------------------------

// Python: tests/test_metadata.py::TestMetadataEncoding::test_absent_metadata_is_omitted_from_the_wire
func TestPyTestMetadataEncoding_AbsentMetadataIsOmittedFromTheWire(t *testing.T) {
	payload, _ := encodeAfterRunStart(t, &agui.TextMessageStartEvent{MessageID: "m1"})
	if _, ok := payload["metadata"]; ok {
		t.Fatalf("metadata on the wire: %v", payload)
	}
}

// Python: tests/test_metadata.py::TestMetadataEncoding::test_nested_nulls_survive_exclude_none
func TestPyTestMetadataEncoding_NestedNullsSurviveExcludeNone(t *testing.T) {
	const mdJSON = `{"usage": {"cost": null, "tokens": {"input": null}}, "trace": [null, {"span": null}]}`
	var md agui.Metadata
	if err := json.Unmarshal([]byte(mdJSON), &md); err != nil {
		t.Fatal(err)
	}
	payload, _ := encodeAfterRunStart(t, &agui.TextMessageStartEvent{MessageID: "m1", BaseEvent: agui.BaseEvent{Metadata: md}})
	if !reflect.DeepEqual(payload["metadata"], parse(t, mdJSON)) {
		t.Fatalf("metadata on the wire = %v, want %s", payload["metadata"], mdJSON)
	}
}

// Python: tests/test_metadata.py::TestMetadataEncoding::test_present_metadata_is_encoded
func TestPyTestMetadataEncoding_PresentMetadataIsEncoded(t *testing.T) {
	md := agui.Metadata{"source": "openai", "nullValue": nil}
	payload, _ := encodeAfterRunStart(t, &agui.TextMessageStartEvent{MessageID: "m1", BaseEvent: agui.BaseEvent{Metadata: md}})
	if !reflect.DeepEqual(payload["metadata"], map[string]any{"source": "openai", "nullValue": nil}) {
		t.Fatalf("metadata on the wire = %v", payload["metadata"])
	}
}

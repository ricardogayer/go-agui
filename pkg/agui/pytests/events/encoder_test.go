package events_test

// Port of the official AG-UI Python SDK tests/test_encoder.py.
//
// The Python EventEncoder writes one event as "data: <json>\n\n". The Go
// analogue is json.Marshal of the event followed by sse.Writer.WriteData
// (encodeSSE in events_test.go); pkg/stream adds schema and ordering checks
// on top of exactly that framing.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// =============================================================================
// TestEventEncoder
// =============================================================================

// Python: tests/test_encoder.py::TestEventEncoder::test_encoder_initialization
func TestPyTestEventEncoder_EncoderInitialization(t *testing.T) {
	var buf bytes.Buffer
	if sse.NewWriter(&buf) == nil {
		t.Fatal("sse.NewWriter returned nil")
	}
	if stream.NewWriter(&buf) == nil {
		t.Fatal("stream.NewWriter returned nil")
	}
	// AGUI_MEDIA_TYPE in Python is the protobuf media type; the encoder
	// accepts it and still answers SSE, as the Go producer does.
	eq(t, "ProtobufContentType", sse.ProtobufContentType, "application/vnd.ag-ui.event+proto")
	eq(t, "ContentType", sse.ContentType, "text/event-stream")
}

// Python: tests/test_encoder.py::TestEventEncoder::test_encode_method
//
// Python encodes a bare BaseEvent(type=RAW); Go has no bare envelope type,
// so the zero RAW event (NewEvent) with only a timestamp is used.
func TestPyTestEventEncoder_EncodeMethod(t *testing.T) {
	now := time.Now().UnixMilli()
	e, _ := agui.NewEvent(agui.EventTypeRaw)
	e.Base().Timestamp = &now
	encoded := encodeSSE(t, e)
	eq(t, "frame", encoded, "data: "+string(mustJSON(t, e))+"\n\n")
	for _, s := range []string{`"type":`, `"timestamp":`} {
		if !strings.Contains(encoded, s) {
			t.Errorf("frame %q lacks %s", encoded, s)
		}
	}
	for _, s := range []string{`"rawEvent":`, `"raw_event":`} {
		if strings.Contains(encoded, s) {
			t.Errorf("frame %q contains %s", encoded, s)
		}
	}
}

// Python: tests/test_encoder.py::TestEventEncoder::test_encode_sse_method
func TestPyTestEventEncoder_EncodeSseMethod(t *testing.T) {
	e := &agui.TextMessageContentEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_123", Delta: "Hello, world!"}
	encoded := encodeSSE(t, e)
	if !strings.HasPrefix(encoded, "data: ") || !strings.HasSuffix(encoded, "\n\n") {
		t.Fatalf("not an SSE frame: %q", encoded)
	}
	d := sseJSON(t, encoded)
	eq(t, "type", d["type"], "TEXT_MESSAGE_CONTENT")
	eq(t, "messageId", d["messageId"], "msg_123")
	eq(t, "delta", d["delta"], "Hello, world!")
	eq(t, "timestamp", d["timestamp"], float64(ts))
	present(t, d, "messageId")
	absent(t, d, "message_id")
}

// Python: tests/test_encoder.py::TestEventEncoder::test_encode_with_different_event_types
func TestPyTestEventEncoder_EncodeWithDifferentEventTypes(t *testing.T) {
	base, _ := agui.NewEvent(agui.EventTypeRaw)
	base.Base().Timestamp = tsPtr()
	if enc := encodeSSE(t, base); !strings.Contains(enc, `"type":"RAW"`) {
		t.Errorf("frame %q lacks \"type\":\"RAW\"", enc)
	}

	content := &agui.TextMessageContentEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_456", Delta: "Testing different events"}
	enc := encodeSSE(t, content)
	for _, s := range []string{`"type":"TEXT_MESSAGE_CONTENT"`, `"messageId":"msg_456"`, `"delta":"Testing different events"`} {
		if !strings.Contains(enc, s) {
			t.Errorf("frame %q lacks %s", enc, s)
		}
	}
	d := sseJSON(t, enc)
	present(t, d, "messageId")
	absent(t, d, "message_id")
}

// Python: tests/test_encoder.py::TestEventEncoder::test_null_value_exclusion
func TestPyTestEventEncoder_NullValueExclusion(t *testing.T) {
	base, _ := agui.NewEvent(agui.EventTypeRaw)
	base.Base().Timestamp = tsPtr()
	base.Base().RawEvent = nil
	d := sseJSON(t, encodeSSE(t, base))
	present(t, d, "type", "timestamp")
	absent(t, d, "rawEvent")

	tc := &agui.ToolCallStartEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, ToolCallID: "call_123", ToolCallName: "test_tool", ParentMessageID: nil}
	d = sseJSON(t, encodeSSE(t, tc))
	present(t, d, "toolCallId", "toolCallName")
	absent(t, d, "parentMessageId")
}

// Python: tests/test_encoder.py::TestEventEncoder::test_round_trip_serialization
func TestPyTestEventEncoder_RoundTripSerialization(t *testing.T) {
	orig := agui.ToolCallStartEvent{
		BaseEvent:       agui.BaseEvent{Timestamp: tsPtr()},
		ToolCallID:      "call_abc123",
		ToolCallName:    "search_tool",
		ParentMessageID: agui.Ptr("msg_parent_456"),
	}
	data := mustJSON(t, orig)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	present(t, m, "toolCallId", "toolCallName", "parentMessageId")
	absent(t, m, "tool_call_id", "tool_call_name", "parent_message_id")

	var got agui.ToolCallStartEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	eq(t, "Type", got.Type(), orig.Type())
	eq(t, "ToolCallID", got.ToolCallID, orig.ToolCallID)
	eq(t, "ToolCallName", got.ToolCallName, orig.ToolCallName)
	eq(t, "ParentMessageID", *got.ParentMessageID, *orig.ParentMessageID)
	eq(t, "Timestamp", *got.Timestamp, *orig.Timestamp)
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("round trip differs:\n got %#v\nwant %#v", got, orig)
	}
}

// =============================================================================
// TestWireNullParity
// =============================================================================

// wire is the _wire helper: encode one event as SSE and decode its payload.
func wire(t *testing.T, e agui.Event) map[string]any {
	t.Helper()
	return sseJSON(t, encodeSSE(t, e))
}

// Python: tests/test_encoder.py::TestWireNullParity::test_required_null_payload_survives
func TestPyTestWireNullParity_RequiredNullPayloadSurvives(t *testing.T) {
	eq(t, "CUSTOM wire", wire(t, &agui.CustomEvent{Name: "hb", Value: nil}),
		map[string]any{"type": "CUSTOM", "name": "hb", "value": nil})
	w := wire(t, &agui.StateSnapshotEvent{Snapshot: nil})
	present(t, w, "snapshot")
	if w["snapshot"] != nil {
		t.Errorf("snapshot = %v, want null", w["snapshot"])
	}
}

// Python: tests/test_encoder.py::TestWireNullParity::test_nested_patch_null_value_survives
func TestPyTestWireNullParity_NestedPatchNullValueSurvives(t *testing.T) {
	var delta agui.JSONPatch
	if err := json.Unmarshal([]byte(`[{"op":"add","path":"/cleared","value":null}]`), &delta); err != nil {
		t.Fatal(err)
	}
	w := wire(t, &agui.StateDeltaEvent{Delta: delta})
	eq(t, "delta[0]", w["delta"].([]any)[0], map[string]any{"op": "add", "path": "/cleared", "value": nil})

	// The same through the constructor helper.
	w = wire(t, &agui.StateDeltaEvent{Delta: agui.JSONPatch{agui.Add("/cleared", nil)}})
	eq(t, "delta[0] (agui.Add)", w["delta"].([]any)[0], map[string]any{"op": "add", "path": "/cleared", "value": nil})
}

// Python: tests/test_encoder.py::TestWireNullParity::test_optional_none_stays_absent
func TestPyTestWireNullParity_OptionalNoneStaysAbsent(t *testing.T) {
	w := wire(t, &agui.TextMessageStartEvent{MessageID: "m", Name: nil})
	absent(t, w, "name", "role")
}

// Python: tests/test_encoder.py::TestWireNullParity::test_null_valued_unknown_field_is_data
//
// Adapted (Python-specific behaviour): the Python models keep unknown
// top-level keys (xTrace) as extras and re-emit them. Go's typed events have
// no extras; the spec (basic/processing.mdx, "What a consumer does with
// unrecognised material") requires an unknown property on a closed event to
// be stripped with a warning before application code, while members of the
// open JSON Patch operation objects MUST be preserved (RFC 6902 section 4).
// So the Go test asserts: the patch operation's unknown "vendor": null and
// required "value": null survive decode and re-encode; Sanitize strips
// xTrace with a warning naming its path and keeps "vendor".
func TestPyTestWireNullParity_NullValuedUnknownFieldIsData(t *testing.T) {
	doc := []byte(`{"type":"STATE_DELTA","delta":[{"op":"add","path":"/x","value":null,"vendor":null}],"xTrace":null}`)

	e, err := agui.UnmarshalEvent(doc)
	if err != nil {
		t.Fatal(err)
	}
	w := wire(t, e)
	eq(t, "delta[0]", w["delta"].([]any)[0], map[string]any{"op": "add", "path": "/x", "value": nil, "vendor": nil})

	res, err := agui.Sanitize("Event", doc, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if res.Dropped {
		t.Fatal("Sanitize dropped the whole event")
	}
	v := res.Value.(map[string]any)
	absent(t, v, "xTrace")
	eq(t, "sanitized delta[0]", generic(t, v["delta"].([]any)[0]), map[string]any{"op": "add", "path": "/x", "value": nil, "vendor": nil})
	warned := false
	for _, wn := range res.Warnings {
		if strings.Contains(wn.String(), "xTrace") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning names xTrace: %v", res.Warnings)
	}
}

// Python: tests/test_encoder.py::TestWireNullParity::test_model_valued_extra_gets_the_same_walk
//
// Go events have no extras; the open channel a Go caller can put an
// arbitrary value into is metadata. An event value placed there must keep
// its required null on the wire.
func TestPyTestWireNullParity_ModelValuedExtraGetsTheSameWalk(t *testing.T) {
	outer := &agui.StepStartedEvent{StepName: "s"}
	outer.Metadata = agui.Metadata{"vendor": &agui.CustomEvent{Name: "inner", Value: nil}}
	w := wire(t, outer)
	eq(t, "metadata.vendor", w["metadata"].(map[string]any)["vendor"],
		map[string]any{"type": "CUSTOM", "name": "inner", "value": nil})
}

// Python: tests/test_encoder.py::TestWireNullParity::test_model_nested_in_containers_gets_the_walk
//
// Go analogue: an event value nested in maps and slices inside an any-typed
// field (RAW's event) and inside metadata.
func TestPyTestWireNullParity_ModelNestedInContainersGetsTheWalk(t *testing.T) {
	want := map[string]any{"type": "CUSTOM", "name": "inner", "value": nil}

	outer := &agui.StepStartedEvent{StepName: "s"}
	outer.Metadata = agui.Metadata{"vendor": map[string]any{"wrapped": []any{agui.CustomEvent{Name: "inner", Value: nil}}}}
	w := wire(t, outer)
	got := w["metadata"].(map[string]any)["vendor"].(map[string]any)["wrapped"].([]any)[0]
	eq(t, "metadata.vendor.wrapped[0]", got, want)

	raw := &agui.RawEvent{Event: map[string]any{"wrapped": []any{&agui.CustomEvent{Name: "inner", Value: nil}}}}
	w = wire(t, raw)
	eq(t, "event.wrapped[0]", w["event"].(map[string]any)["wrapped"].([]any)[0], want)
}

// Python: tests/test_encoder.py::TestWireNullParity::test_root_model_extras_unwrap
func TestPyTestWireNullParity_RootModelExtrasUnwrap(t *testing.T) {
	t.Skip("N/A in Go: pydantic RootModel wrapping is a Pydantic construct; Go has no root-model wrapper to unwrap (a plain slice is covered by ModelNestedInContainersGetsTheWalk)")
}

// Python: tests/test_encoder.py::TestWireNullParity::test_null_under_a_metadata_key_is_data
func TestPyTestWireNullParity_NullUnderAMetadataKeyIsData(t *testing.T) {
	w := wire(t, &agui.TextMessageStartEvent{MessageID: "m", BaseEvent: agui.BaseEvent{Metadata: agui.Metadata{"finish": nil}}})
	eq(t, "metadata", w["metadata"], map[string]any{"finish": nil})
}

// Python: tests/test_encoder.py::TestWireNullParity::test_move_operation_keeps_the_rfc_member_name
//
// Python's second assertion (plain model_dump without aliases) has no Go
// counterpart: Go has a single serialisation, checked by the first.
func TestPyTestWireNullParity_MoveOperationKeepsTheRfcMemberName(t *testing.T) {
	var delta agui.JSONPatch
	if err := json.Unmarshal([]byte(`[{"op":"move","path":"/a","from":"/b"}]`), &delta); err != nil {
		t.Fatal(err)
	}
	eq(t, "From", delta[0].From, "/b")
	w := wire(t, &agui.StateDeltaEvent{Delta: delta})
	eq(t, "delta[0].from", w["delta"].([]any)[0].(map[string]any)["from"], "/b")
	eq(t, "Move helper", generic(t, agui.Move("/b", "/a")), map[string]any{"op": "move", "path": "/a", "from": "/b"})
}

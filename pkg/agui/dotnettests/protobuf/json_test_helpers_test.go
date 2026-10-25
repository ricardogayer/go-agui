package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/JsonTestHelpers.cs: shared helpers
// for the port of the .NET SDK's Protobuf binding tests (no test methods).
//
// JsonTestHelpers.Parse / AssertEqual compare JSON structurally with numbers
// as doubles; parseJSON / assertJSONEqual do the same here. The rest builds
// the hand-rolled protobuf the C# tests get from the generated
// AGUI.ProtocolBuffers types (Proto.Event and friends), using the frozen
// field numbers of pkg/protobuf/proto/*.proto.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

// parseJSON parses a JSON text into a generic value (numbers as float64).
func parseJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// generic normalises any Go value to its generic JSON form.
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return out
}

// assertJSONEqual is JsonTestHelpers.AssertEqual: deep structural equality.
func assertJSONEqual(t *testing.T, want, got any) {
	t.Helper()
	w, g := generic(t, want), generic(t, got)
	if !reflect.DeepEqual(w, g) {
		wb, _ := json.Marshal(w)
		gb, _ := json.Marshal(g)
		t.Fatalf("Expected JSON '%s' but got '%s'.", wb, gb)
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func deref[T any](t *testing.T, what string, p *T) T {
	t.Helper()
	if p == nil {
		t.Fatalf("%s is nil", what)
	}
	return *p
}

func isNil[T any](t *testing.T, what string, p *T) {
	t.Helper()
	if p != nil {
		t.Fatalf("%s = %v, want nil", what, *p)
	}
}

func asType[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	if !ok {
		var zero T
		t.Fatalf("got %T, want %T", v, zero)
	}
	return got
}

// encode is AGUIProtobuf.Encode.
func encode(t *testing.T, e agui.Event) []byte {
	t.Helper()
	b, err := protobuf.Encode(e)
	if err != nil {
		t.Fatalf("encode %s: %v", e.Type(), err)
	}
	return b
}

// encodeDoc encodes a JSON event document.
func encodeDoc(t *testing.T, doc string) []byte {
	t.Helper()
	b, err := protobuf.EncodeJSON([]byte(doc))
	if err != nil {
		t.Fatalf("encode %s: %v", doc, err)
	}
	return b
}

// decode is AGUIProtobuf.Decode, to the Go event.
func decode(t *testing.T, b []byte) agui.Event {
	t.Helper()
	e, err := protobuf.Decode(b)
	if err != nil {
		t.Fatalf("decode %x: %v", b, err)
	}
	return e
}

// decodeDoc decodes to the JSON event document, as a generic object.
func decodeDoc(t *testing.T, b []byte) map[string]any {
	t.Helper()
	doc, err := protobuf.DecodeJSON(b)
	if err != nil {
		t.Fatalf("decode %x: %v", b, err)
	}
	return asType[map[string]any](t, parseJSON(t, string(doc)))
}

// roundTrip is the C# RoundTrip<T>: encode, decode, assert the type.
func roundTrip[T agui.Event](t *testing.T, e T) T {
	t.Helper()
	b := encode(t, e)
	if len(b) == 0 {
		t.Fatalf("%s encoded to no bytes", e.Type())
	}
	return asType[T](t, decode(t, b))
}

// roundTripDoc round-trips a JSON event document through the wire.
func roundTripDoc(t *testing.T, doc string) map[string]any {
	t.Helper()
	return decodeDoc(t, encodeDoc(t, doc))
}

// mustBeMalformed asserts that decoding fails as malformed (InvalidDataException).
func mustBeMalformed(t *testing.T, b []byte, contains string) {
	t.Helper()
	doc, err := protobuf.DecodeJSON(b)
	if err == nil {
		t.Fatalf("decoded %s, want a malformed-frame error", doc)
	}
	if !protobuf.IsMalformed(err) {
		t.Fatalf("got %v, want a malformed-frame error", err)
	}
	if contains != "" && !bytes.Contains([]byte(err.Error()), []byte(contains)) {
		t.Fatalf("error %q does not mention %q", err, contains)
	}
}

// framed writes events as length-prefixed frames (AGUIProtobuf.WriteFramed).
func framed(t *testing.T, payloads ...[]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := protobuf.NewFrameWriter(&buf)
	for _, p := range payloads {
		if err := w.WriteFrame(p); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

// =============================================================================
// Hand-built protobuf (the C# tests' Proto.* messages)
// =============================================================================

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ld is a length-delimited field (a nested message, a string, bytes).
func ld(num int, payload ...[]byte) []byte {
	b := protowire.AppendTag(nil, protowire.Number(num), protowire.BytesType)
	return protowire.AppendBytes(b, cat(payload...))
}

// sf is a string field.
func sf(num int, s string) []byte { return ld(num, []byte(s)) }

// vf is a varint field.
func vf(num int, v uint64) []byte {
	b := protowire.AppendTag(nil, protowire.Number(num), protowire.VarintType)
	return protowire.AppendVarint(b, v)
}

// EventType numbers (events.proto).
const (
	etTextMessageStart        = 0
	etStateSnapshot           = 6
	etStateDelta              = 7
	etMessagesSnapshot        = 8
	etRunStarted              = 11
	etRunFinished             = 12
	etStepStarted             = 14
	etStepFinished            = 15
	etSubagentStarted         = 16
	etSubagentFinished        = 17
	etSubagentError           = 18
	etToolCallResult          = 21
	etActivitySnapshot        = 22
	etReasoningEncryptedValue = 30
)

// Envelope arms (Event oneof, events.proto).
const (
	armStateSnapshot           = 7
	armStateDelta              = 8
	armMessagesSnapshot        = 9
	armRunStarted              = 12
	armRunFinished             = 13
	armStepStarted             = 15
	armStepFinished            = 16
	armSubagentStarted         = 19
	armSubagentFinished        = 20
	armSubagentError           = 21
	armToolCallResult          = 22
	armActivitySnapshot        = 23
	armReasoningEncryptedValue = 31
)

// baseEvent is a BaseEvent field (field 1 of every event) declaring typ;
// proto3 writes no enum zero, as Google.Protobuf does not.
func baseEvent(typ int, extra ...[]byte) []byte {
	var body []byte
	if typ != 0 {
		body = vf(1, uint64(typ))
	}
	return ld(1, cat(body, cat(extra...)))
}

// wrap is Proto.Event{<arm> = <event of typ with fields>}.
func wrap(arm, typ int, fields ...[]byte) []byte {
	return ld(arm, baseEvent(typ), cat(fields...))
}

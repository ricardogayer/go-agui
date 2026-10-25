package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs.
//
// ProtoValueConverter is the .NET JsonElement <-> google.protobuf.Value
// converter. Go's converter is internal to pkg/protobuf, so each test carries
// the JSON value through a Value-typed wire field — CUSTOM.value — reads the
// Value's kind straight off the encoded bytes (KindCase) and checks the value
// decodes back unchanged (ToJsonElement).

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// google.protobuf.Value kind field numbers (struct.proto).
const (
	kindNull   = 1
	kindNumber = 2
	kindString = 3
	kindBool   = 4
	kindStruct = 5
	kindList   = 6
)

// fields returns the top-level fields of a message: number -> raw value
// (payload for length-delimited, varint value as 8 bytes LE for others).
func fieldsOf(t *testing.T, b []byte) map[protowire.Number][]byte {
	t.Helper()
	out := map[protowire.Number][]byte{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			t.Fatalf("bad tag in %x", b)
		}
		b = b[n:]
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			t.Fatalf("bad field %d in %x", num, b)
		}
		switch typ {
		case protowire.BytesType:
			v, _ := protowire.ConsumeBytes(b)
			out[num] = v
		case protowire.VarintType:
			v, _ := protowire.ConsumeVarint(b)
			out[num] = protowire.AppendFixed64(nil, v)
		case protowire.Fixed64Type:
			v, _ := protowire.ConsumeFixed64(b)
			out[num] = protowire.AppendFixed64(nil, v)
		default:
			out[num] = b[:m]
		}
		b = b[m:]
	}
	return out
}

// valueOnTheWire encodes the JSON value as CUSTOM.value and returns the
// encoded google.protobuf.Value message and the value decoded back.
func valueOnTheWire(t *testing.T, value string) (map[protowire.Number][]byte, any) {
	t.Helper()
	b := encodeDoc(t, `{"type":"CUSTOM","name":"v","value":`+value+`}`)
	custom, ok := fieldsOf(t, b)[11] // Event.custom
	if !ok {
		t.Fatalf("no CUSTOM arm in %x", b)
	}
	v, ok := fieldsOf(t, custom)[3] // CustomEvent.value
	if !ok {
		t.Fatalf("no value field in %x", custom)
	}
	doc := decodeDoc(t, b)
	got, ok := doc["value"]
	if !ok {
		t.Fatalf("value absent after decoding: %v", doc)
	}
	return fieldsOf(t, v), got
}

func kindOf(t *testing.T, value map[protowire.Number][]byte) protowire.Number {
	t.Helper()
	if len(value) != 1 {
		t.Fatalf("a Value carries exactly one kind, got %d", len(value))
	}
	for k := range value {
		return k
	}
	return 0
}

func le64(b []byte) uint64 { v, _ := protowire.ConsumeFixed64(b); return v }

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.String_RoundTrips
func TestCsProtoValueConverterTest_String_RoundTrips(t *testing.T) {
	value, back := valueOnTheWire(t, `"hello"`)
	eq(t, "KindCase", kindOf(t, value), kindString)
	eq(t, "StringValue", string(value[kindString]), "hello")
	assertJSONEqual(t, parseJSON(t, `"hello"`), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.Number_RoundTrips
func TestCsProtoValueConverterTest_Number_RoundTrips(t *testing.T) {
	value, back := valueOnTheWire(t, `42.5`)
	eq(t, "KindCase", kindOf(t, value), kindNumber)
	eq(t, "NumberValue", math.Float64frombits(le64(value[kindNumber])), 42.5)
	assertJSONEqual(t, parseJSON(t, `42.5`), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.IntegerNumber_RoundTrips
func TestCsProtoValueConverterTest_IntegerNumber_RoundTrips(t *testing.T) {
	value, back := valueOnTheWire(t, `42`)
	eq(t, "NumberValue", math.Float64frombits(le64(value[kindNumber])), 42.0)
	assertJSONEqual(t, parseJSON(t, `42`), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.Bool_RoundTrips
func TestCsProtoValueConverterTest_Bool_RoundTrips(t *testing.T) {
	for _, row := range []struct {
		json     string
		expected bool
	}{{"true", true}, {"false", false}} {
		t.Run(row.json, func(t *testing.T) {
			value, back := valueOnTheWire(t, row.json)
			eq(t, "KindCase", kindOf(t, value), kindBool)
			eq(t, "BoolValue", le64(value[kindBool]) != 0, row.expected)
			assertJSONEqual(t, parseJSON(t, row.json), back)
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.Null_RoundTrips
func TestCsProtoValueConverterTest_Null_RoundTrips(t *testing.T) {
	value, back := valueOnTheWire(t, `null`)
	eq(t, "KindCase", kindOf(t, value), kindNull)
	if back != nil {
		t.Fatalf("decoded %v, want JSON null", back)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.Object_RoundTrips
func TestCsProtoValueConverterTest_Object_RoundTrips(t *testing.T) {
	const j = `{"a":1,"b":"x","c":true,"d":null}`
	value, back := valueOnTheWire(t, j)
	eq(t, "KindCase", kindOf(t, value), kindStruct)
	assertJSONEqual(t, parseJSON(t, j), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.Array_RoundTrips
func TestCsProtoValueConverterTest_Array_RoundTrips(t *testing.T) {
	const j = `[1,"two",false,null]`
	value, back := valueOnTheWire(t, j)
	eq(t, "KindCase", kindOf(t, value), kindList)
	assertJSONEqual(t, parseJSON(t, j), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.NestedStructure_RoundTrips
func TestCsProtoValueConverterTest_NestedStructure_RoundTrips(t *testing.T) {
	const j = `{"items":[{"id":1,"tags":["a","b"]},{"id":2,"nested":{"deep":[true,null,3.14]}}],"count":2}`
	_, back := valueOnTheWire(t, j)
	assertJSONEqual(t, parseJSON(t, j), back)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.ToValueOrNull_ReturnsNullForNull
func TestCsProtoValueConverterTest_ToValueOrNull_ReturnsNullForNull(t *testing.T) {
	// An absent optional JSON value (RUN_FINISHED.result) is no Value at all
	// on the wire — not a null-kind Value.
	b := encodeDoc(t, `{"type":"RUN_FINISHED","threadId":"t","runId":"r"}`)
	rf := fieldsOf(t, b)[13] // Event.run_finished
	if _, ok := fieldsOf(t, rf)[4]; ok {
		t.Fatalf("RunFinishedEvent.result written for an absent value: %x", rf)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtoValueConverterTest.cs::ProtoValueConverterTest.ToJsonElementOrNull_ReturnsNullForNull
func TestCsProtoValueConverterTest_ToJsonElementOrNull_ReturnsNullForNull(t *testing.T) {
	// No Value on the wire decodes to an absent member, not to JSON null.
	b := wrap(armRunFinished, etRunFinished, sf(2, "t"), sf(3, "r"))
	doc := decodeDoc(t, b)
	if v, ok := doc["result"]; ok {
		raw, _ := json.Marshal(v)
		t.Fatalf("result decoded as %s, want absent", raw)
	}
	if raw, _ := json.Marshal(doc); strings.Contains(string(raw), "null") {
		t.Fatalf("decoded %s carries a null", raw)
	}
}

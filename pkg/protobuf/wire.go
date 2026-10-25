package protobuf

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"

	"google.golang.org/protobuf/encoding/protowire"
)

// errMalformed reports bytes that are not a well-formed AG-UI event frame.
var errMalformed = errors.New("ag-ui/protobuf: malformed event frame")

func malformed(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errMalformed, fmt.Sprintf(format, args...))
}

// =============================================================================
// Encoding
// =============================================================================
//
// Fields are written in field-number order. A non-optional scalar holding its
// default is omitted, as proto3 requires; an optional one present in the
// document is written whatever its value; a message, Value or Struct present
// in the document is always written, even when empty.

func encodeMessage(name string, o *object) ([]byte, error) {
	msg, ok := schema[name]
	if !ok {
		return nil, fmt.Errorf("ag-ui/protobuf: unknown wire message %s", name)
	}
	var b []byte
	for _, f := range msg.fields {
		v, ok := o.get(f.name)
		if !ok || (v == nil && f.optional && !(name == "JsonPatchOperation" && f.name == "value")) {
			// A whole optional null reads as an absent field: the retired
			// spelling the reference encoders also omit. A patch operation's
			// value is the exception — there null is the value being written.
			continue
		}
		var err error
		if f.repeated {
			arr, isArr := v.([]any)
			if !isArr {
				return nil, fmt.Errorf("ag-ui/protobuf: %s.%s must be a list", name, f.name)
			}
			for _, x := range arr {
				if b, err = appendField(b, f, x, true); err != nil {
					return nil, fmt.Errorf("%s.%s: %w", name, f.name, err)
				}
			}
			continue
		}
		if b, err = appendField(b, f, v, f.optional || f.oneof); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", name, f.name, err)
		}
	}
	return b, nil
}

func appendField(b []byte, f *field, v any, always bool) ([]byte, error) {
	switch f.kind {
	case kString:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected a string")
		}
		if !always && s == "" {
			return b, nil
		}
		b = protowire.AppendTag(b, f.num, protowire.BytesType)
		return protowire.AppendString(b, s), nil
	case kInt64, kEnum:
		n, err := toInt64(v)
		if err != nil {
			return nil, err
		}
		if !always && n == 0 {
			return b, nil
		}
		b = protowire.AppendTag(b, f.num, protowire.VarintType)
		return protowire.AppendVarint(b, uint64(n)), nil
	case kBool:
		x, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected a boolean")
		}
		if !always && !x {
			return b, nil
		}
		b = protowire.AppendTag(b, f.num, protowire.VarintType)
		return protowire.AppendVarint(b, protowire.EncodeBool(x)), nil
	case kDouble:
		x, err := toFloat(v)
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, f.num, protowire.Fixed64Type)
		return protowire.AppendFixed64(b, math.Float64bits(x)), nil
	case kMessage:
		var payload []byte
		var err error
		switch f.msg {
		case valueMsg:
			payload, err = encodeValue(v)
		case structMsg:
			o, ok := v.(*object)
			if !ok {
				return nil, fmt.Errorf("expected an object")
			}
			payload, err = encodeStruct(o)
		default:
			o, ok := v.(*object)
			if !ok {
				return nil, fmt.Errorf("expected an object")
			}
			payload, err = encodeMessage(f.msg, o)
		}
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, f.num, protowire.BytesType)
		return protowire.AppendBytes(b, payload), nil
	}
	return nil, fmt.Errorf("unsupported field kind")
}

// encodeValue writes any JSON value as a google.protobuf.Value.
func encodeValue(v any) ([]byte, error) {
	var b []byte
	switch t := v.(type) {
	case nil:
		b = protowire.AppendTag(b, 1, protowire.VarintType)
		return protowire.AppendVarint(b, 0), nil
	case json.Number, float64, int, int64:
		x, err := toFloat(t)
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, 2, protowire.Fixed64Type)
		return protowire.AppendFixed64(b, math.Float64bits(x)), nil
	case string:
		b = protowire.AppendTag(b, 3, protowire.BytesType)
		return protowire.AppendString(b, t), nil
	case bool:
		b = protowire.AppendTag(b, 4, protowire.VarintType)
		return protowire.AppendVarint(b, protowire.EncodeBool(t)), nil
	case *object:
		payload, err := encodeStruct(t)
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, 5, protowire.BytesType)
		return protowire.AppendBytes(b, payload), nil
	case []any:
		var list []byte
		for _, x := range t {
			item, err := encodeValue(x)
			if err != nil {
				return nil, err
			}
			list = protowire.AppendTag(list, 1, protowire.BytesType)
			list = protowire.AppendBytes(list, item)
		}
		b = protowire.AppendTag(b, 6, protowire.BytesType)
		return protowire.AppendBytes(b, list), nil
	}
	return nil, fmt.Errorf("ag-ui/protobuf: %T is not a JSON value", v)
}

// encodeStruct writes an object as a google.protobuf.Struct, its entries in
// the object's own order.
func encodeStruct(o *object) ([]byte, error) {
	var b []byte
	for _, k := range o.keys {
		value, err := encodeValue(o.vals[k])
		if err != nil {
			return nil, err
		}
		var entry []byte
		if k != "" {
			entry = protowire.AppendTag(entry, 1, protowire.BytesType)
			entry = protowire.AppendString(entry, k)
		}
		entry = protowire.AppendTag(entry, 2, protowire.BytesType)
		entry = protowire.AppendBytes(entry, value)
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendBytes(b, entry)
	}
	return b, nil
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, nil
		}
		r, ok := new(big.Rat).SetString(string(t))
		if !ok || !r.IsInt() || !r.Num().IsInt64() {
			return 0, fmt.Errorf("%s is not a 64-bit integer", t)
		}
		return r.Num().Int64(), nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case int32:
		return int64(t), nil
	}
	return 0, fmt.Errorf("expected an integer")
}

func toFloat(v any) (float64, error) {
	switch t := v.(type) {
	case json.Number:
		return t.Float64()
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	case int64:
		return float64(t), nil
	}
	return 0, fmt.Errorf("expected a number")
}

// =============================================================================
// Structural scan
// =============================================================================
//
// Before decoding, a frame is walked once to reject bytes no encoder writes:
// field number zero, unbalanced groups, unknown wire types, lengths past the
// end, an envelope naming two different events, and a content part or source
// carrying two different arms in one occurrence. Unknown fields are protobuf's
// to skip, and are skipped.

// strictArms are the oneof messages whose arms are mutually exclusive within
// one occurrence (the envelope is checked separately).
var strictArms = map[string]bool{"InputContent": true, "InputContentSource": true}

func scanMessage(name string, b []byte, envelope bool) error {
	msg := schema[name]
	var seenArm protowire.Number
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return malformed("bad tag in %s", name)
		}
		b = b[n:]
		f := msg.byNum[num]
		if typ == protowire.BytesType && f != nil && f.kind == kMessage {
			payload, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return malformed("bad length in %s", name)
			}
			if f.oneof && (envelope || strictArms[name]) {
				if seenArm != 0 && seenArm != num {
					return malformed("%s carries more than one arm", name)
				}
				seenArm = num
			}
			if f.msg != valueMsg && f.msg != structMsg && f.msg != listMsg && f.msg != entryMsg {
				if err := scanMessage(f.msg, payload, false); err != nil {
					return err
				}
			}
			b = b[n:]
			continue
		}
		if typ == protowire.EndGroupType {
			return malformed("unbalanced group in %s", name)
		}
		if envelope && f != nil {
			// A known arm in a wire type no arm uses names an event this
			// build knows in a shape no encoder writes.
			return malformed("envelope arm %d has wire type %d", num, typ)
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return malformed("bad field %d in %s", num, name)
		}
		b = b[n:]
	}
	return nil
}

// walksAsMessage reports whether b is a well-formed protobuf message, without
// reading inside length-delimited fields: used on the payload of an envelope
// arm this build does not know.
func walksAsMessage(b []byte) bool {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return false
		}
		b = b[n:]
		if typ == protowire.EndGroupType {
			return false
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return false
		}
		b = b[n:]
	}
	return true
}

// namesOnlyFutureArms reports whether an envelope that carried no arm this
// build knows names at least one arm from a later protocol version, and
// nothing broken.
func namesOnlyFutureArms(b []byte) bool {
	envelope := schema["Event"]
	future := 0
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return false
		}
		b = b[n:]
		known := envelope.byNum[num] != nil
		if typ == protowire.BytesType {
			payload, n := protowire.ConsumeBytes(b)
			if n < 0 || known || !walksAsMessage(payload) {
				return false
			}
			future++
			b = b[n:]
			continue
		}
		if known || typ == protowire.EndGroupType {
			return false
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return false
		}
		b = b[n:]
	}
	return future > 0
}

// =============================================================================
// Decoding
// =============================================================================
//
// Standard protobuf semantics: the last occurrence of a singular scalar wins,
// occurrences of a singular message merge (decoding their concatenation is
// exactly that merge), repeated fields accumulate, a later oneof arm replaces
// an earlier one, and fields this build does not know are skipped. A
// non-optional scalar that is absent reads as its default; an absent repeated
// field as the empty list; an absent message, Value or Struct stays absent.

func wireTypeOf(f *field) protowire.Type {
	switch f.kind {
	case kString, kMessage:
		return protowire.BytesType
	case kDouble:
		return protowire.Fixed64Type
	default:
		return protowire.VarintType
	}
}

func decodeMessage(name string, b []byte) (*object, error) {
	msg := schema[name]
	out := newObject()
	singular := map[protowire.Number][]byte{}
	var singularOrder []protowire.Number
	repeated := map[protowire.Number][]any{}
	var activeArm protowire.Number

	clearArm := func(num protowire.Number) {
		f := msg.byNum[num]
		out.del(f.name)
		delete(singular, num)
	}

	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, malformed("bad tag in %s", name)
		}
		b = b[n:]
		f := msg.byNum[num]
		if f == nil || typ != wireTypeOf(f) {
			n = protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return nil, malformed("bad field %d in %s", num, name)
			}
			b = b[n:]
			continue
		}
		if f.oneof {
			if activeArm != 0 && activeArm != num {
				clearArm(activeArm)
			}
			activeArm = num
		}
		switch typ {
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, malformed("bad length in %s", name)
			}
			b = b[n:]
			switch {
			case f.kind == kString && f.repeated:
				repeated[num] = append(repeated[num], string(v))
			case f.kind == kString:
				out.set(f.name, string(v))
			case f.repeated:
				item, present, err := decodeField(f, v)
				if err != nil {
					return nil, err
				}
				if !present {
					item = nil
				}
				repeated[num] = append(repeated[num], item)
			default:
				if _, seen := singular[num]; !seen {
					singularOrder = append(singularOrder, num)
				}
				singular[num] = append(singular[num], v...)
			}
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, malformed("bad varint in %s", name)
			}
			b = b[n:]
			switch f.kind {
			case kBool:
				out.set(f.name, protowire.DecodeBool(v))
			case kEnum:
				out.set(f.name, json.Number(fmt.Sprint(int32(v))))
			default:
				out.set(f.name, json.Number(fmt.Sprint(int64(v))))
			}
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return nil, malformed("bad fixed64 in %s", name)
			}
			b = b[n:]
			out.set(f.name, math.Float64frombits(v))
		}
	}

	for _, num := range singularOrder {
		payload, ok := singular[num]
		if !ok {
			continue // cleared by a later oneof arm
		}
		f := msg.byNum[num]
		v, present, err := decodeField(f, payload)
		if err != nil {
			return nil, err
		}
		if present {
			out.set(f.name, v)
		}
	}
	for _, f := range msg.fields {
		if items, ok := repeated[f.num]; ok {
			out.set(f.name, items)
		}
	}
	for _, f := range msg.fields {
		if _, ok := out.get(f.name); ok || f.oneof {
			continue
		}
		switch {
		case f.repeated:
			out.set(f.name, []any{})
		case f.optional || f.kind == kMessage:
		case f.kind == kString:
			out.set(f.name, "")
		case f.kind == kBool:
			out.set(f.name, false)
		case f.kind == kInt64, f.kind == kEnum:
			out.set(f.name, json.Number("0"))
		}
	}
	return out, nil
}

// decodeField decodes a message-typed field's payload; Values and Structs
// become plain JSON. present is false for an empty Value (no kind set).
func decodeField(f *field, payload []byte) (any, bool, error) {
	switch f.msg {
	case valueMsg:
		return decodeValue(payload)
	case structMsg:
		o, err := decodeStruct(payload)
		return o, err == nil, err
	case listMsg:
		l, err := decodeList(payload)
		return l, err == nil, err
	}
	o, err := decodeMessage(f.msg, payload)
	return o, err == nil, err
}

func decodeValue(b []byte) (any, bool, error) {
	w, err := decodeMessage(valueMsg, b)
	if err != nil {
		return nil, false, err
	}
	if len(w.keys) == 0 {
		return nil, false, nil
	}
	k := w.keys[len(w.keys)-1]
	v := w.vals[k]
	switch k {
	case "nullValue":
		return nil, true, nil
	case "numberValue":
		f := v.(float64)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false, malformed("a Value holds %v, which JSON cannot carry", f)
		}
		return f, true, nil
	case "structValue", "listValue", "stringValue", "boolValue":
		return v, true, nil
	}
	return nil, false, nil
}

func decodeStruct(b []byte) (*object, error) {
	w, err := decodeMessage("google.protobuf.Struct.raw", b)
	if err != nil {
		return nil, err
	}
	out := newObject()
	entries, _ := w.vals["fields"].([]any)
	for _, e := range entries {
		entry, ok := e.(*object)
		if !ok {
			continue
		}
		key, _ := entry.vals["key"].(string)
		if v, ok := entry.vals["value"]; ok {
			out.set(key, v)
		}
	}
	return out, nil
}

func decodeList(b []byte) ([]any, error) {
	w, err := decodeMessage("google.protobuf.ListValue.raw", b)
	if err != nil {
		return nil, err
	}
	items, _ := w.vals["values"].([]any)
	return items, nil
}

func init() {
	// Raw views of Struct and ListValue whose entries decode Values to JSON;
	// a Value's struct/list arms decode through decodeStruct/decodeList.
	def("google.protobuf.Struct.raw", rm(1, "fields", entryMsg))
	def("google.protobuf.ListValue.raw", rm(1, "values", valueMsg))
}

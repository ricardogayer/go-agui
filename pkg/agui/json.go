package agui

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Ptr returns a pointer to v. Optional fields of the protocol are pointers
// (or nil-able values) because an absent field and a zero value are
// different things on the wire: an optional field with no value is omitted,
// never sent as null or as an empty default.
func Ptr[T any](v T) *T { return &v }

// Deref returns *p, or the zero value when p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Metadata is the protocol's open channel: open by key, any JSON value under a
// key (including null). The object itself is either absent (nil) or an object.
type Metadata map[string]any

// withTag prepends a discriminator member to a marshaled JSON object, so the
// discriminator ("type", "role") is always the protocol's value whatever the
// Go value says.
func withTag(key, value string, obj []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	obj = bytes.TrimSpace(obj)
	if len(obj) < 2 || obj[0] != '{' {
		return nil, fmt.Errorf("ag-ui: cannot tag non-object JSON %s", obj)
	}
	var buf bytes.Buffer
	buf.Grow(len(obj) + len(key) + len(value) + 8)
	buf.WriteString(`{"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	v, _ := json.Marshal(value)
	buf.Write(v)
	if len(bytes.TrimSpace(obj[1:len(obj)-1])) > 0 {
		buf.WriteByte(',')
		buf.Write(obj[1:])
	} else {
		buf.WriteByte('}')
	}
	return buf.Bytes(), nil
}

// peekString reads one top-level string member of a JSON object.
func peekString(data []byte, key string) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	raw, ok := m[key]
	if !ok {
		return "", fmt.Errorf("ag-ui: missing %q", key)
	}
	var s string
	if isNull(raw) {
		return "", fmt.Errorf("ag-ui: %q must be a string, not null", key)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("ag-ui: %q must be a string", key)
	}
	return s, nil
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null"
}

// emptyIfNil keeps required arrays from marshaling as null.
func emptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

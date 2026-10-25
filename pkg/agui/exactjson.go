package agui

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// encoding/json matches object keys to struct fields case-insensitively, so
// on its own it would read "subAgents" (the key AG-UI 1.0 retired) into
// Subagents, or "MESSAGEID" into MessageID. The protocol's member names are
// exact: a key spelled differently is unrecognised material, never an alias.
// Every wire type therefore decodes through decodeExact, which drops the keys
// that do not match a field's JSON name exactly before decoding. Like
// encoding/json, typed decoding ignores unrecognised members; Sanitize is
// what reports them.

var exactNames sync.Map // reflect.Type -> map[string]bool

func jsonNames(t reflect.Type) map[string]bool {
	if cached, ok := exactNames.Load(t); ok {
		return cached.(map[string]bool)
	}
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k := range jsonNames(ft) {
					names[k] = true
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names[name] = true
	}
	exactNames.Store(t, names)
	return names
}

// filterExact removes the members of a JSON object whose key is not exactly
// one of t's JSON field names.
func filterExact(data []byte, t reflect.Type) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return data // not an object: let the decoder report it
	}
	names := jsonNames(t)
	changed := false
	for k := range m {
		if !names[k] {
			delete(m, k)
			changed = true
		}
	}
	if !changed {
		return data
	}
	out, err := json.Marshal(m)
	if err != nil {
		return data
	}
	return out
}

// decodeExact decodes data into v (a pointer to a struct type without its
// own UnmarshalJSON) using case-exact member names.
func decodeExact(data []byte, v any) error {
	return json.Unmarshal(filterExact(data, reflect.TypeOf(v).Elem()), v)
}

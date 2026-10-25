package agui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// JSON Patch operation names (RFC 6902).
const (
	PatchAdd     = "add"
	PatchRemove  = "remove"
	PatchReplace = "replace"
	PatchMove    = "move"
	PatchCopy    = "copy"
	PatchTest    = "test"
)

// PatchOperation is one RFC 6902 operation. Operations are open objects:
// RFC 6902 requires members an operation does not define to be ignored, not
// rejected, so they are kept in Extensions and survive a round trip (a
// "remove" carrying a leftover "value" is a valid operation).
type PatchOperation struct {
	Op   string
	Path string
	// From is the source pointer of "move" and "copy".
	From string
	// Value is the value of "add", "replace" and "test" (nil is JSON null).
	Value any
	// Extensions holds members the operation does not define.
	Extensions map[string]any
}

// MarshalJSON implements json.Marshaler.
func (o PatchOperation) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(o.Extensions)+4)
	for k, v := range o.Extensions {
		m[k] = v
	}
	m["op"] = o.Op
	m["path"] = o.Path
	switch o.Op {
	case PatchAdd, PatchReplace, PatchTest:
		m["value"] = o.Value
	case PatchMove, PatchCopy:
		m["from"] = o.From
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *PatchOperation) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("ag-ui: patch operation must be an object")
	}
	*o = PatchOperation{}
	if err := json.Unmarshal(m["op"], &o.Op); err != nil {
		return fmt.Errorf("ag-ui: patch operation: op must be a string")
	}
	if err := json.Unmarshal(m["path"], &o.Path); err != nil {
		return fmt.Errorf("ag-ui: patch operation: path must be a string")
	}
	delete(m, "op")
	delete(m, "path")
	switch o.Op {
	case PatchAdd, PatchReplace, PatchTest:
		if raw, ok := m["value"]; ok {
			if err := json.Unmarshal(raw, &o.Value); err != nil {
				return err
			}
			delete(m, "value")
		}
	case PatchMove, PatchCopy:
		if raw, ok := m["from"]; ok {
			if err := json.Unmarshal(raw, &o.From); err != nil {
				return fmt.Errorf("ag-ui: patch operation: from must be a string")
			}
			delete(m, "from")
		}
	}
	for k, raw := range m {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		if o.Extensions == nil {
			o.Extensions = map[string]any{}
		}
		o.Extensions[k] = v
	}
	return nil
}

// JSONPatch is an RFC 6902 patch: operations applied in order, atomically.
type JSONPatch []PatchOperation

// MarshalJSON implements json.Marshaler (an empty patch is []).
func (p JSONPatch) MarshalJSON() ([]byte, error) {
	return json.Marshal(emptyIfNil([]PatchOperation(p)))
}

// Add returns an "add" operation.
func Add(path string, value any) PatchOperation {
	return PatchOperation{Op: PatchAdd, Path: path, Value: value}
}

// Remove returns a "remove" operation.
func Remove(path string) PatchOperation { return PatchOperation{Op: PatchRemove, Path: path} }

// Replace returns a "replace" operation.
func Replace(path string, value any) PatchOperation {
	return PatchOperation{Op: PatchReplace, Path: path, Value: value}
}

// Move returns a "move" operation.
func Move(from, path string) PatchOperation {
	return PatchOperation{Op: PatchMove, From: from, Path: path}
}

// Copy returns a "copy" operation.
func Copy(from, path string) PatchOperation {
	return PatchOperation{Op: PatchCopy, From: from, Path: path}
}

// Test returns a "test" operation.
func Test(path string, value any) PatchOperation {
	return PatchOperation{Op: PatchTest, Path: path, Value: value}
}

// PatchError reports a well-formed patch that does not apply to the document
// it targets. Under the snapshot–delta pattern this is not fatal: the consumer
// warns and keeps its prior value.
type PatchError struct {
	Index int
	Op    string
	Path  string
	Cause string
}

func (e *PatchError) Error() string {
	return fmt.Sprintf("ag-ui: patch operation %d (%s %s) does not apply: %s", e.Index, e.Op, e.Path, e.Cause)
}

// ApplyPatch applies a patch to a generic JSON document (map[string]any,
// []any, string, float64/json.Number, bool, nil) and returns the result.
// Application is atomic: on error the input document is left untouched and
// no partial result is returned.
func ApplyPatch(doc any, patch JSONPatch) (any, error) {
	cur := deepCopy(doc)
	for i, op := range patch {
		var err error
		cur, err = applyOp(cur, op)
		if err != nil {
			return nil, &PatchError{Index: i, Op: op.Op, Path: op.Path, Cause: err.Error()}
		}
	}
	return cur, nil
}

func applyOp(doc any, op PatchOperation) (any, error) {
	switch op.Op {
	case PatchAdd:
		return addAt(doc, op.Path, deepCopy(op.Value))
	case PatchRemove:
		out, _, err := removeAt(doc, op.Path)
		return out, err
	case PatchReplace:
		out, _, err := removeAt(doc, op.Path)
		if err != nil {
			return nil, err
		}
		return addAt(out, op.Path, deepCopy(op.Value))
	case PatchMove:
		if op.From == op.Path {
			if _, err := getAt(doc, op.From); err != nil {
				return nil, err
			}
			return doc, nil
		}
		if strings.HasPrefix(op.Path, op.From+"/") {
			return nil, fmt.Errorf("cannot move a value into one of its own children")
		}
		out, v, err := removeAt(doc, op.From)
		if err != nil {
			return nil, err
		}
		return addAt(out, op.Path, v)
	case PatchCopy:
		v, err := getAt(doc, op.From)
		if err != nil {
			return nil, err
		}
		return addAt(doc, op.Path, deepCopy(v))
	case PatchTest:
		v, err := getAt(doc, op.Path)
		if err != nil {
			return nil, err
		}
		if !jsonEqual(normalizeGeneric(deepCopy(v)), normalizeGeneric(deepCopy(op.Value))) {
			return nil, fmt.Errorf("test failed: value differs")
		}
		return doc, nil
	default:
		return nil, fmt.Errorf("unrecognised operation %q", op.Op)
	}
}

func splitPointer(ptr string) ([]string, error) {
	if ptr == "" {
		return nil, nil
	}
	if ptr[0] != '/' {
		return nil, fmt.Errorf("invalid JSON pointer %q", ptr)
	}
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func arrayIndex(tok string, n int, allowEnd bool) (int, error) {
	if allowEnd && tok == "-" {
		return n, nil
	}
	// RFC 6901: "0", or digits without a leading zero — no sign.
	if tok == "" || (len(tok) > 1 && tok[0] == '0') || strings.TrimLeft(tok, "0123456789") != "" {
		return 0, fmt.Errorf("invalid array index %q", tok)
	}
	i, err := strconv.Atoi(tok)
	if err != nil || i < 0 {
		return 0, fmt.Errorf("invalid array index %q", tok)
	}
	limit := n - 1
	if allowEnd {
		limit = n
	}
	if i > limit {
		return 0, fmt.Errorf("array index %d out of bounds", i)
	}
	return i, nil
}

func getAt(doc any, ptr string) (any, error) {
	parts, err := splitPointer(ptr)
	if err != nil {
		return nil, err
	}
	cur := doc
	for _, p := range parts {
		switch t := cur.(type) {
		case map[string]any:
			v, ok := t[p]
			if !ok {
				return nil, fmt.Errorf("path %q does not exist", ptr)
			}
			cur = v
		case []any:
			i, err := arrayIndex(p, len(t), false)
			if err != nil {
				return nil, err
			}
			cur = t[i]
		default:
			return nil, fmt.Errorf("path %q does not exist", ptr)
		}
	}
	return cur, nil
}

// update walks to the parent of ptr and replaces the child via fn.
func update(doc any, parts []string, fn func(parent any, key string) (any, error)) (any, error) {
	if len(parts) == 1 {
		return fn(doc, parts[0])
	}
	switch t := doc.(type) {
	case map[string]any:
		child, ok := t[parts[0]]
		if !ok {
			return nil, fmt.Errorf("path segment %q does not exist", parts[0])
		}
		nc, err := update(child, parts[1:], fn)
		if err != nil {
			return nil, err
		}
		t[parts[0]] = nc
		return t, nil
	case []any:
		i, err := arrayIndex(parts[0], len(t), false)
		if err != nil {
			return nil, err
		}
		nc, err := update(t[i], parts[1:], fn)
		if err != nil {
			return nil, err
		}
		t[i] = nc
		return t, nil
	default:
		return nil, fmt.Errorf("path segment %q does not exist", parts[0])
	}
}

func addAt(doc any, ptr string, v any) (any, error) {
	parts, err := splitPointer(ptr)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return v, nil
	}
	return update(doc, parts, func(parent any, key string) (any, error) {
		switch t := parent.(type) {
		case map[string]any:
			t[key] = v
			return t, nil
		case []any:
			i, err := arrayIndex(key, len(t), true)
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(t)+1)
			out = append(out, t[:i]...)
			out = append(out, v)
			return append(out, t[i:]...), nil
		default:
			return nil, fmt.Errorf("cannot add to a %s", jsonTypeName(parent))
		}
	})
}

func removeAt(doc any, ptr string) (any, any, error) {
	parts, err := splitPointer(ptr)
	if err != nil {
		return nil, nil, err
	}
	if len(parts) == 0 {
		return nil, doc, nil
	}
	var removed any
	out, err := update(doc, parts, func(parent any, key string) (any, error) {
		switch t := parent.(type) {
		case map[string]any:
			v, ok := t[key]
			if !ok {
				return nil, fmt.Errorf("path %q does not exist", ptr)
			}
			removed = v
			delete(t, key)
			return t, nil
		case []any:
			i, err := arrayIndex(key, len(t), false)
			if err != nil {
				return nil, err
			}
			removed = t[i]
			out := make([]any, 0, len(t)-1)
			out = append(out, t[:i]...)
			return append(out, t[i+1:]...), nil
		default:
			return nil, fmt.Errorf("path %q does not exist", ptr)
		}
	})
	return out, removed, err
}

// deepCopy copies a generic JSON value.
func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = deepCopy(t[k])
		}
		return out
	case Metadata:
		return deepCopy(map[string]any(t))
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = deepCopy(x)
		}
		return out
	default:
		return v
	}
}

// ToGeneric converts any JSON-marshalable Go value into its generic JSON form
// (map[string]any, []any, string, json.Number, bool, nil).
func ToGeneric(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return decodeGeneric(b)
}

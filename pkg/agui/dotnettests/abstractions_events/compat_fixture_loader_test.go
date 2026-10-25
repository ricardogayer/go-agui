package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/FixtureLoader.cs
// (a helper, no test methods) plus the helpers every file of this package
// shares. The C# fixtures (embedded resources there) are copied verbatim to
// testdata/compat/.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// loadFixture is FixtureLoader.LoadFixture: the elements of a fixture array.
func loadFixture(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "compat", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var out []json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("fixture %s is not a JSON array: %v", name, err)
	}
	return out
}

// decodeAsEvent is FixtureLoader.DeserializeAsBaseEvent, done the way a Go
// consumer reads an event from a peer: Sanitize applies the processing model
// (unrecognised material stripped with a warning, malformed known values
// fatal), then the clean document is decoded into its Go type.
func decodeAsEvent(t *testing.T, raw []byte) (agui.Event, []agui.Warning) {
	t.Helper()
	res, err := agui.Sanitize("Event", raw, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("sanitize %s: %v", raw, err)
	}
	if res.Dropped {
		t.Fatalf("event dropped as unrecognised: %s (%v)", raw, res.Warnings)
	}
	clean, err := json.Marshal(res.Value)
	if err != nil {
		t.Fatalf("marshal sanitized: %v", err)
	}
	e, err := agui.UnmarshalEvent(clean)
	if err != nil {
		t.Fatalf("decode %s: %v", clean, err)
	}
	return e, res.Warnings
}

// decodeFixtureEvent decodes a fixture that is a valid AG-UI 1.0 event: it
// must pass the strict schema check and decode without any warning.
func decodeFixtureEvent(t *testing.T, raw []byte) agui.Event {
	t.Helper()
	if err := agui.Validate("Event", raw); err != nil {
		t.Fatalf("fixture rejected by the 1.0 schema: %v\n%s", err, raw)
	}
	e, ws := decodeAsEvent(t, raw)
	if len(ws) != 0 {
		t.Fatalf("unexpected warnings for a valid event: %v", ws)
	}
	return e
}

// assertRoundTripPreservesType is the body of the C# AllXxx_RoundTrip tests:
// decode, re-serialise, decode again, and compare the event type. The
// re-serialised document must also be a valid 1.0 event.
func assertRoundTripPreservesType(t *testing.T, fixtures []json.RawMessage) {
	t.Helper()
	for i, raw := range fixtures {
		e, _ := decodeAsEvent(t, raw)
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("fixture %d: re-serialise: %v", i, err)
		}
		if err := agui.Validate("Event", b); err != nil {
			t.Errorf("fixture %d: re-serialised event is not valid 1.0: %v\n%s", i, err, b)
		}
		again, err := agui.UnmarshalEvent(b)
		if err != nil {
			t.Fatalf("fixture %d: re-decode: %v", i, err)
		}
		if again.Type() != e.Type() {
			t.Errorf("fixture %d: type %s became %s", i, e.Type(), again.Type())
		}
	}
}

// asType is Assert.IsType<T>.
func asType[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	if !ok {
		var zero T
		t.Fatalf("got %T, want %T", v, zero)
	}
	return got
}

// serialize marshals a Go event (JsonSerializer.Serialize) and returns the
// document as a generic object. The output must be valid AG-UI 1.0.
func serialize(t *testing.T, e agui.Event) map[string]any {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal %T: %v", e, err)
	}
	if err := agui.Validate("Event", b); err != nil {
		t.Errorf("serialised %T is not valid 1.0: %v\n%s", e, err, b)
	}
	return toObject(t, b)
}

// toObject decodes a JSON object into a generic map (JsonDocument.Parse).
func toObject(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, b)
	}
	return m
}

// roundTrip serialises v and deserialises it into a new T, as
// JsonSerializer.Deserialize(json, typeInfo<T>) does.
func roundTrip[T any](t *testing.T, v *T) *T {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	out := new(T)
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal %T: %v\n%s", out, err, b)
	}
	return out
}

// viaBaseEvent is Deserialize(json, BaseEvent) for a valid document:
// ParseEvent validates against the 1.0 schema and decodes by "type".
func viaBaseEvent(t *testing.T, doc string) agui.Event {
	t.Helper()
	e, err := agui.ParseEvent([]byte(doc))
	if err != nil {
		t.Fatalf("ParseEvent: %v\n%s", err, doc)
	}
	return e
}

// at navigates a generic JSON value: string keys index objects, ints index
// arrays (JsonElement.GetProperty / indexer).
func at(t *testing.T, v any, path ...any) any {
	t.Helper()
	cur := v
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("at %v: %T is not an object", path, cur)
			}
			next, ok := m[k]
			if !ok {
				t.Fatalf("at %v: missing property %q", path, k)
			}
			cur = next
		case int:
			a, ok := cur.([]any)
			if !ok {
				t.Fatalf("at %v: %T is not an array", path, cur)
			}
			if k >= len(a) {
				t.Fatalf("at %v: index %d out of range (%d)", path, k, len(a))
			}
			cur = a[k]
		default:
			t.Fatalf("bad path element %v", p)
		}
	}
	return cur
}

// has reports whether an object carries a key (TryGetProperty).
func has(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

// arrLen is GetArrayLength.
func arrLen(t *testing.T, v any) int {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%T is not an array", v)
	}
	return len(a)
}

// generic turns any Go value into its generic JSON form.
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/SchemaCorpusTest.cs.
//
// The fixture corpus is the behavioural contract, and the binary transport
// must carry the same protocol as the JSON path: every valid event fixture
// round-trips through encode/decode to the same materialised event; the
// committed TypeScript bytes decode to the fixture; and this encoder
// reproduces the committed .NET bytes byte for byte. The fixtures and both
// byte corpora are vendored under pkg/agui/testdata.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

const (
	fixturesDir        = "../../testdata/spec-1.0/fixtures"
	typeScriptBytesDir = "../../testdata/proto/bytes-typescript"
	dotnetBytesDir     = "../../testdata/proto/bytes-dotnet"
)

type fixture struct{ anchor, path string }

// validEventFixtures is ValidEventFixtures: every */valid/*.json under an
// anchor ending in "Event", in order.
func validEventFixtures(t *testing.T) []fixture {
	t.Helper()
	dirs, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	var out []fixture
	for _, d := range dirs {
		if !d.IsDir() || !strings.HasSuffix(d.Name(), "Event") {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(fixturesDir, d.Name(), "valid", "*.json"))
		sort.Strings(files)
		for _, f := range files {
			out = append(out, fixture{d.Name(), f})
		}
	}
	if len(out) == 0 {
		t.Fatalf("no event fixtures under %s", fixturesDir)
	}
	return out
}

func (f fixture) name() string { return f.anchor + "__" + filepath.Base(f.path) + ".bin" }

func (f fixture) raw(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// parseFixture is ParseFixture: the fixture as its (schema-checked) Go event.
func parseFixture(t *testing.T, f fixture) agui.Event {
	t.Helper()
	e, err := agui.ParseEvent(f.raw(t))
	if err != nil {
		t.Fatalf("fixture %s: %v", f.path, err)
	}
	return e
}

// toNode is ToNode: the event as generic JSON, with a root-level null
// treated as absent.
func toNode(t *testing.T, e agui.Event) map[string]any {
	t.Helper()
	m := asType[map[string]any](t, generic(t, e))
	for k, v := range m {
		if v == nil {
			delete(m, k)
		}
	}
	return m
}

// materialiseWireArrays is MaterialiseWireArrays: the wire cannot tell an
// absent repeated field from an empty one, so decoding always materialises
// the optional RunAgentInput lists.
func materialiseWireArrays(e agui.Event) {
	if rs, ok := e.(*agui.RunStartedEvent); ok && rs.Input != nil {
		if rs.Input.Tools == nil {
			rs.Input.Tools = []agui.Tool{}
		}
		if rs.Input.Resume == nil {
			rs.Input.Resume = []agui.ResumeEntry{}
		}
		if rs.Input.Context == nil {
			rs.Input.Context = []agui.Context{}
		}
	}
}

func sameEvent(t *testing.T, what string, expected, actual agui.Event) {
	t.Helper()
	want, got := toNode(t, expected), toNode(t, actual)
	if !reflect.DeepEqual(want, got) {
		wb, _ := json.Marshal(want)
		gb, _ := json.Marshal(got)
		t.Fatalf("%s.\nexpected: %s\nactual:   %s", what, wb, gb)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SchemaCorpusTest.cs::SchemaCorpusTest.ValidFixture_RoundTripsOverTheBinaryTransport
func TestCsSchemaCorpusTest_ValidFixture_RoundTripsOverTheBinaryTransport(t *testing.T) {
	for _, f := range validEventFixtures(t) {
		t.Run(f.anchor+"/"+filepath.Base(f.path), func(t *testing.T) {
			expected := parseFixture(t, f)
			decoded := decode(t, encode(t, expected))
			materialiseWireArrays(expected)
			sameEvent(t, "binary round trip changed the event", expected, decoded)
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SchemaCorpusTest.cs::SchemaCorpusTest.TypeScriptBytes_AreReadable
func TestCsSchemaCorpusTest_TypeScriptBytes_AreReadable(t *testing.T) {
	for _, f := range validEventFixtures(t) {
		t.Run(f.anchor+"/"+filepath.Base(f.path), func(t *testing.T) {
			// TypeScript writes bytes for the complete corpus, so a missing
			// file is a broken path or a stale checkout — never a silent skip.
			b, err := os.ReadFile(filepath.Join(typeScriptBytesDir, f.name()))
			if err != nil {
				t.Fatalf("%s missing: %v", f.name(), err)
			}
			decoded := decode(t, b)
			expected := parseFixture(t, f)
			materialiseWireArrays(expected)
			sameEvent(t, f.name()+": TypeScript bytes decoded to a different event", expected, decoded)
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SchemaCorpusTest.cs::SchemaCorpusTest.DotnetBytes_MatchTheCommittedFixtures
func TestCsSchemaCorpusTest_DotnetBytes_MatchTheCommittedFixtures(t *testing.T) {
	// .NET encodes the fixture parsed into its model, whose JsonElements keep
	// the document's member order. The Go model holds open objects as maps
	// (no member order), so the order-faithful Go equivalent is the encoder
	// handed the fixture document itself (EncodeJSON); the spec's byte parity
	// is defined on the corpus documents.
	for _, f := range validEventFixtures(t) {
		t.Run(f.anchor+"/"+filepath.Base(f.path), func(t *testing.T) {
			got, err := protobuf.EncodeJSON(f.raw(t))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(dotnetBytesDir, f.name()))
			if err != nil {
				t.Fatalf("%s missing: %v", f.name(), err)
			}
			if !bytes.Equal(want, got) {
				t.Fatalf("bytes differ\n got  %s\n want %s", hex.EncodeToString(got), hex.EncodeToString(want))
			}
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SchemaCorpusTest.cs::SchemaCorpusTest.NoStaleDotnetByteFixtures
func TestCsSchemaCorpusTest_NoStaleDotnetByteFixtures(t *testing.T) {
	expected := map[string]bool{}
	for _, f := range validEventFixtures(t) {
		expected[f.name()] = true
	}
	files, err := os.ReadDir(dotnetBytesDir)
	if err != nil {
		t.Fatalf("%s is missing: %v", dotnetBytesDir, err)
	}
	var stale []string
	for _, f := range files {
		if !expected[f.Name()] {
			stale = append(stale, f.Name())
		}
	}
	if len(stale) != 0 {
		t.Fatalf("stale .NET byte fixtures: %v", stale)
	}
}

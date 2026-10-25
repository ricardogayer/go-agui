package protobuf

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
)

const (
	fixturesDir = "../agui/testdata/spec-1.0/fixtures"
	tsBytesDir  = "../agui/testdata/proto/bytes-typescript"
	netBytesDir = "../agui/testdata/proto/bytes-dotnet"
)

type corpusCase struct {
	name string // "<Definition>/<file>"
	doc  []byte
}

func eventFixtures(t *testing.T) []corpusCase {
	t.Helper()
	var cases []corpusCase
	defs, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if !strings.HasSuffix(d.Name(), "Event") {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(fixturesDir, d.Name(), "valid", "*.json"))
		sort.Strings(files)
		for _, f := range files {
			doc, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			cases = append(cases, corpusCase{name: d.Name() + "/" + filepath.Base(f), doc: doc})
		}
	}
	if len(cases) < 90 {
		t.Fatalf("found only %d event fixtures", len(cases))
	}
	return cases
}

func corpusFile(dir, name string) string {
	return filepath.Join(dir, strings.ReplaceAll(name, "/", "__")+".bin")
}

// generic decodes JSON for comparison, numbers as float64 (the wire carries
// open values as doubles).
func generic(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	return v
}

// binaryNormalised records the one normalisation the corpus itself records:
// the binary wire cannot tell an absent repeated field from an empty one, so
// RunAgentInput's optional lists decode as present and empty.
func binaryNormalised(name string, doc any) any {
	if name != "RunStartedEvent/with-input.json" {
		return doc
	}
	ev := doc.(map[string]any)
	in := ev["input"].(map[string]any)
	for _, k := range []string{"tools", "context", "resume"} {
		if _, ok := in[k]; !ok {
			in[k] = []any{}
		}
	}
	return ev
}

// TestByteCorpus encodes every valid event fixture of the official corpus and
// compares the bytes with those the TypeScript and .NET reference encoders
// committed; the specification requires every first-party encoder to
// reproduce the corpus byte for byte.
func TestByteCorpus(t *testing.T) {
	cases := eventFixtures(t)
	types := map[string]bool{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var probe struct{ Type string }
			_ = json.Unmarshal(c.doc, &probe)
			types[probe.Type] = true

			got, err := EncodeJSON(c.doc)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			for _, dir := range []string{tsBytesDir, netBytesDir} {
				want, err := os.ReadFile(corpusFile(dir, c.name))
				if err != nil {
					t.Fatalf("missing corpus file: %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("bytes differ from %s\n got  %s\n want %s", filepath.Base(dir), hex.EncodeToString(got), hex.EncodeToString(want))
				}
			}

			decoded, err := DecodeJSON(got)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			want := binaryNormalised(c.name, generic(t, c.doc))
			if !reflect.DeepEqual(generic(t, decoded), want) {
				t.Fatalf("round trip changed the event\n got  %s\n want %s", decoded, c.doc)
			}
		})
	}
	if len(types) != 31 {
		t.Errorf("the corpus covers %d event types, want 31", len(types))
	}
}

// TestNoStaleCorpusFiles: every committed byte file has a fixture.
func TestNoStaleCorpusFiles(t *testing.T) {
	want := map[string]bool{}
	for _, c := range eventFixtures(t) {
		want[filepath.Base(corpusFile("", c.name))] = true
	}
	for _, dir := range []string{tsBytesDir, netBytesDir} {
		files, _ := os.ReadDir(dir)
		for _, f := range files {
			if !want[f.Name()] {
				t.Errorf("%s/%s has no fixture", filepath.Base(dir), f.Name())
			}
		}
		if len(files) != len(want) {
			t.Errorf("%s has %d files, want %d", filepath.Base(dir), len(files), len(want))
		}
	}
}

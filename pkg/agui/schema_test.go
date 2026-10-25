package agui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixturesDir = "testdata/spec-1.0/fixtures"

type schemaFixture struct {
	definition string
	name       string
	valid      bool
	data       []byte
	expect     struct {
		Keyword          string `json:"keyword"`
		InstanceLocation string `json:"instanceLocation"`
	}
}

func loadSchemaFixtures(t *testing.T) []schemaFixture {
	t.Helper()
	var out []schemaFixture
	err := filepath.WalkDir(fixturesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".expect.json") {
			return err
		}
		rel, _ := filepath.Rel(fixturesDir, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 {
			t.Fatalf("unexpected fixture layout: %s", rel)
		}
		f := schemaFixture{definition: parts[0], name: parts[1] + "/" + parts[2], valid: parts[1] == "valid"}
		if f.data, err = os.ReadFile(path); err != nil {
			return err
		}
		if !f.valid {
			raw, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".expect.json")
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &f.expect); err != nil {
				return err
			}
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 150 {
		t.Fatalf("expected the official fixture corpus, found only %d documents", len(out))
	}
	return out
}

// TestSchemaFixtures runs every document of the official AG-UI 1.0 fixture
// corpus (spec/1.0/fixtures) through Validate: valid documents must be
// accepted and invalid ones rejected at the location the corpus names.
func TestSchemaFixtures(t *testing.T) {
	for _, f := range loadSchemaFixtures(t) {
		t.Run(f.definition+"/"+f.name, func(t *testing.T) {
			err := Validate(f.definition, f.data)
			if f.valid {
				if err != nil {
					t.Fatalf("valid fixture rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid fixture accepted (expected %s at %q)", f.expect.Keyword, f.expect.InstanceLocation)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if !sameRule(ve, f.expect.Keyword, f.expect.InstanceLocation) {
				t.Errorf("rejected for a different reason: got %s at %q (%s), expected %s at %q",
					ve.Keyword, ve.InstanceLocation, ve.Message, f.expect.Keyword, f.expect.InstanceLocation)
			}
		})
	}
}

// sameRule compares a rejection with the corpus expectation. The corpus
// records what ajv reports; for a failed union ajv may name the union or the
// discriminator inside it, so a oneOf/const pair one level apart counts.
func sameRule(ve *ValidationError, keyword, loc string) bool {
	if ve.InstanceLocation == loc && ve.Keyword == keyword {
		return true
	}
	// A union no branch accepted: a more specific error inside the selected
	// branch explains the same rejection.
	if keyword == "oneOf" && strings.HasPrefix(ve.InstanceLocation, loc+"/") {
		return true
	}
	union := map[string]bool{"oneOf": true, "const": true}
	if union[ve.Keyword] && union[keyword] {
		return ve.InstanceLocation == loc || strings.HasPrefix(loc, ve.InstanceLocation+"/") && !strings.Contains(strings.TrimPrefix(loc, ve.InstanceLocation+"/"), "/")
	}
	return false
}

package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs.
// Runs the shared cross-language fixture sdks/fixtures/agent-capabilities.json.

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const agentCapabilitiesFixture = "../../testdata/sdk-fixtures/agent-capabilities.json"

// The port runs the cases the .NET SDK runs.
const fixtureSDK = "dotnet"

type fixtureCase struct {
	Name       string          `json:"name"`
	ProducedBy []string        `json:"producedBy"`
	Input      json.RawMessage `json:"input"`
	Expected   json.RawMessage `json:"expected"`
}

func loadFixtureCases(t *testing.T, path, listKey string) (covered, skipped []fixtureCase) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var all []fixtureCase
	if err := json.Unmarshal(doc[listKey], &all); err != nil {
		t.Fatalf("parse %s.%s: %v", path, listKey, err)
	}
	for _, c := range all {
		if slices.Contains(c.ProducedBy, fixtureSDK) {
			covered = append(covered, c)
		} else {
			skipped = append(skipped, c)
		}
	}
	return covered, skipped
}

// noNulls is AssertNoNulls: it returns the paths of every null, not
// descending into the open-by-key members metadata and custom (whose values
// may be null) but rejecting a null spelled at those keys themselves.
func noNulls(v any, path string) []string {
	switch x := v.(type) {
	case nil:
		if path == "" {
			return []string{"$"}
		}
		return []string{path}
	case map[string]any:
		var out []string
		for k, val := range x {
			if k == "metadata" || k == "custom" {
				if val == nil {
					out = append(out, path+"."+k)
				}
				continue
			}
			out = append(out, noNulls(val, path+"."+k)...)
		}
		return out
	case []any:
		var out []string
		for i, val := range x {
			out = append(out, noNulls(val, path+"["+strconv.Itoa(i)+"]")...)
		}
		return out
	}
	return nil
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs::AgentCapabilitiesFixtureTest.FixtureCoversThisSdk
func TestCsAgentCapabilitiesFixtureTest_FixtureCoversThisSdk(t *testing.T) {
	covered, skipped := loadFixtureCases(t, agentCapabilitiesFixture, "cases")
	if len(covered) == 0 {
		t.Fatal("the fixture has no cases for this SDK")
	}
	if len(skipped) != 0 {
		names := make([]string, len(skipped))
		for i, c := range skipped {
			names[i] = c.Name
		}
		t.Fatalf("cases that skip %q: %v", fixtureSDK, names)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs::AgentCapabilitiesFixtureTest.NoExpectedDocumentContainsANull
func TestCsAgentCapabilitiesFixtureTest_NoExpectedDocumentContainsANull(t *testing.T) {
	covered, _ := loadFixtureCases(t, agentCapabilitiesFixture, "cases")
	for _, c := range covered {
		if nulls := noNulls(decode(t, c.Expected), ""); len(nulls) > 0 {
			t.Errorf("%s.expected: JSON null at %v", c.Name, nulls)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs::AgentCapabilitiesFixtureTest.AssertNoNullsRejectsANullUnderANonOpenKey
func TestCsAgentCapabilitiesFixtureTest_AssertNoNullsRejectsANullUnderANonOpenKey(t *testing.T) {
	if len(noNulls(decode(t, []byte(`{"custom":null}`)), "")) == 0 {
		t.Error("a null at the open key custom must be rejected")
	}
	if len(noNulls(decode(t, []byte(`{"identity":{"metadata":null}}`)), "")) == 0 {
		t.Error("a null at the open key identity.metadata must be rejected")
	}
	if n := noNulls(decode(t, []byte(`{"custom":{"anything":null}}`)), ""); len(n) != 0 {
		t.Errorf("a null value nested under an open key is data, got %v", n)
	}
	if len(noNulls(decode(t, []byte(`{"transport":{"streaming":null}}`)), "")) == 0 {
		t.Error("a null under a non-open key must be rejected")
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs::AgentCapabilitiesFixtureTest.CaseReserializesToItsExpectedJson
func TestCsAgentCapabilitiesFixtureTest_CaseReserializesToItsExpectedJson(t *testing.T) {
	covered, _ := loadFixtureCases(t, agentCapabilitiesFixture, "cases")
	for _, c := range covered {
		t.Run(c.Name, func(t *testing.T) {
			var caps agui.AgentCapabilities
			if err := json.Unmarshal(c.Input, &caps); err != nil {
				t.Fatalf("decode input: %v", err)
			}
			produced := mustJSON(t, &caps)
			if !jsonEqual(t, produced, c.Expected) {
				t.Fatalf("case %q\nexpected: %s\nproduced: %s", c.Name, c.Expected, produced)
			}
			// The produced document is also what the schema accepts.
			if err := agui.Validate("AgentCapabilities", produced); err != nil {
				t.Fatalf("schema rejects the produced document: %v", err)
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesFixtureTest.cs::AgentCapabilitiesFixtureTest.CaseReserializesToItsExpectedJsonThroughTheResolver
func TestCsAgentCapabilitiesFixtureTest_CaseReserializesToItsExpectedJsonThroughTheResolver(t *testing.T) {
	covered, _ := loadFixtureCases(t, agentCapabilitiesFixture, "cases")
	for _, c := range covered {
		t.Run(c.Name, func(t *testing.T) {
			t.Skipf("N/A in Go: exercises AGUIJsonUtilities.DefaultTypeInfoResolver as a second System.Text.Json serialization path (source-generator/AOT trimming guard); Go has a single encoding/json path, covered by CaseReserializesToItsExpectedJson (case %s)", c.Name)
		})
	}
}

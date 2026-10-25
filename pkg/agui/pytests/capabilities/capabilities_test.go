package capabilities_test

// Port of sdks/python/tests/test_capabilities.py.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ---------------------------------------------------------------------------
// Shared helpers (used by every file of this package).
// ---------------------------------------------------------------------------

// wire marshals v and decodes it back into generic JSON — the Go counterpart
// of Pydantic's model_dump(by_alias=True).
func wire(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return out
}

// generic decodes a JSON literal into generic JSON.
func generic(t *testing.T, s string) any {
	t.Helper()
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatalf("decode literal: %v", err)
	}
	return out
}

func mustUnmarshal(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("unmarshal %s: %v", s, err)
	}
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", g, w)
	}
}

func assertHasKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("key %q missing from %v", key, m)
	}
}

func assertNoKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("key %q unexpectedly present in %v", key, m)
	}
}

func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%v)", v, v)
	}
	return m
}

var (
	p = agui.Ptr[string]
	b = agui.Ptr[bool]
	n = agui.Ptr[int64]
)

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

// fullyPopulatedCapabilities mirrors fully_populated_capabilities(). The
// metadata number is float64 because encoding/json decodes JSON numbers in
// an open (map[string]any) member as float64; using that type here lets the
// round-trip test compare Go values with reflect.DeepEqual, as Python
// compares models with ==.
func fullyPopulatedCapabilities() agui.AgentCapabilities {
	return agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{
			Name:             p("agent-x"),
			Type:             p("langgraph"),
			Description:      p("Does the thing."),
			Version:          p("1.2.0"),
			Provider:         p("acme"),
			DocumentationURL: p("https://example.com/docs"),
			Metadata:         agui.Metadata{"team": "platform", "tier": float64(2)},
		},
		Transport: &agui.TransportCapabilities{
			Streaming:         b(true),
			Websocket:         b(false),
			HTTPBinary:        b(true),
			PushNotifications: b(false),
			Resumable:         b(true),
		},
		Tools: &agui.ToolsCapabilities{
			Supported: b(true),
			Items: []agui.Tool{{
				Name:        "search",
				Description: "Search the web",
				Parameters:  map[string]any{"type": "object"},
			}},
			ParallelCalls:  b(true),
			ClientProvided: b(false),
		},
		Output: &agui.OutputCapabilities{
			StructuredOutput:   b(true),
			SupportedMimeTypes: []string{"text/plain", "application/json"},
		},
		State: &agui.StateCapabilities{
			Snapshots:       b(true),
			Deltas:          b(true),
			Memory:          b(false),
			PersistentState: b(true),
		},
		MultiAgent: &agui.MultiAgentCapabilities{
			Supported:  b(true),
			Delegation: b(true),
			Handoffs:   b(false),
			Subagents:  []agui.SubagentInfo{{Name: "planner", Description: p("plans things")}},
		},
		Reasoning: &agui.ReasoningCapabilities{
			Supported: b(true),
			Streaming: b(true),
			Encrypted: b(false),
		},
		Multimodal: &agui.MultimodalCapabilities{
			Input: &agui.MultimodalInputCapabilities{
				Image: b(true), Audio: b(false), Video: b(false), PDF: b(true), File: b(true),
			},
			Output: &agui.MultimodalOutputCapabilities{Image: b(false), Audio: b(true)},
		},
		Execution: &agui.ExecutionCapabilities{
			CodeExecution:    b(true),
			Sandboxed:        b(true),
			MaxIterations:    n(10),
			MaxExecutionTime: n(30000),
		},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{
			Supported:        b(true),
			Approvals:        b(true),
			Interventions:    b(false),
			Feedback:         b(true),
			Interrupts:       b(true),
			ApproveWithEdits: b(true),
		},
		Custom: map[string]any{"integration": "langgraph", "graphId": "main"},
	}
}

const expectedWire = `{
  "identity": {
    "name": "agent-x",
    "type": "langgraph",
    "description": "Does the thing.",
    "version": "1.2.0",
    "provider": "acme",
    "documentationUrl": "https://example.com/docs",
    "metadata": {"team": "platform", "tier": 2}
  },
  "transport": {
    "streaming": true,
    "websocket": false,
    "httpBinary": true,
    "pushNotifications": false,
    "resumable": true
  },
  "tools": {
    "supported": true,
    "items": [
      {"name": "search", "description": "Search the web", "parameters": {"type": "object"}}
    ],
    "parallelCalls": true,
    "clientProvided": false
  },
  "output": {
    "structuredOutput": true,
    "supportedMimeTypes": ["text/plain", "application/json"]
  },
  "state": {"snapshots": true, "deltas": true, "memory": false, "persistentState": true},
  "multiAgent": {
    "supported": true,
    "delegation": true,
    "handoffs": false,
    "subagents": [{"name": "planner", "description": "plans things"}]
  },
  "reasoning": {"supported": true, "streaming": true, "encrypted": false},
  "multimodal": {
    "input": {"image": true, "audio": false, "video": false, "pdf": true, "file": true},
    "output": {"image": false, "audio": true}
  },
  "execution": {
    "codeExecution": true,
    "sandboxed": true,
    "maxIterations": 10,
    "maxExecutionTime": 30000
  },
  "humanInTheLoop": {
    "supported": true,
    "approvals": true,
    "interventions": false,
    "feedback": true,
    "interrupts": true,
    "approveWithEdits": true
  },
  "custom": {"integration": "langgraph", "graphId": "main"}
}`

// ---------------------------------------------------------------------------
// TestFullyPopulatedWireShape
// ---------------------------------------------------------------------------

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_dump_matches_the_wire_shape_exactly
func TestPyTestFullyPopulatedWireShape_TestDumpMatchesTheWireShapeExactly(t *testing.T) {
	dumped := wire(t, fullyPopulatedCapabilities())
	assertJSONEqual(t, any(dumped), generic(t, expectedWire))
}

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_camel_case_keys_are_the_ones_on_the_wire
func TestPyTestFullyPopulatedWireShape_TestCamelCaseKeysAreTheOnesOnTheWire(t *testing.T) {
	d := wire(t, fullyPopulatedCapabilities())
	assertHasKey(t, obj(t, d["identity"]), "documentationUrl")
	assertHasKey(t, obj(t, d["transport"]), "httpBinary")
	assertHasKey(t, obj(t, d["transport"]), "pushNotifications")
	assertHasKey(t, obj(t, d["output"]), "structuredOutput")
	assertHasKey(t, obj(t, d["output"]), "supportedMimeTypes")
	assertHasKey(t, obj(t, d["state"]), "persistentState")
	assertHasKey(t, obj(t, d["tools"]), "parallelCalls")
	assertHasKey(t, obj(t, d["tools"]), "clientProvided")
	assertHasKey(t, obj(t, d["execution"]), "codeExecution")
	assertHasKey(t, obj(t, d["execution"]), "maxIterations")
	assertHasKey(t, obj(t, d["execution"]), "maxExecutionTime")
	assertHasKey(t, obj(t, d["humanInTheLoop"]), "approveWithEdits")
	assertHasKey(t, d, "multiAgent")
	assertHasKey(t, d, "humanInTheLoop")
	assertHasKey(t, obj(t, d["identity"]), "metadata")
	assertHasKey(t, d, "custom")
	assertHasKey(t, d, "multimodal")
	assertNoKey(t, d, "multiModal")
}

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_subagents_is_one_lowercase_word
func TestPyTestFullyPopulatedWireShape_TestSubagentsIsOneLowercaseWord(t *testing.T) {
	ma := obj(t, wire(t, fullyPopulatedCapabilities())["multiAgent"])
	assertHasKey(t, ma, "subagents")
	assertNoKey(t, ma, "subAgents")
	assertNoKey(t, ma, "sub_agents")
	first := obj(t, ma["subagents"].([]any)[0])
	if first["name"] != "planner" {
		t.Fatalf("subagents[0].name = %v, want planner", first["name"])
	}
}

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_no_snake_case_leaks_into_the_wire
func TestPyTestFullyPopulatedWireShape_TestNoSnakeCaseLeaksIntoTheWire(t *testing.T) {
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch v := node.(type) {
		case map[string]any:
			for k, val := range v {
				if strings.Contains(k, "_") {
					t.Errorf("snake_case key %q serialized at %s", k, path)
				}
				walk(val, path+"."+k)
			}
		case []any:
			for i, val := range v {
				walk(val, path+"["+string(rune('0'+i))+"]")
			}
		}
	}
	walk(any(wire(t, fullyPopulatedCapabilities())), "$")
}

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_round_trip_parses_back_to_an_equal_model
func TestPyTestFullyPopulatedWireShape_TestRoundTripParsesBackToAnEqualModel(t *testing.T) {
	dumped := wire(t, fullyPopulatedCapabilities())
	raw, _ := json.Marshal(dumped)
	var parsed agui.AgentCapabilities
	mustUnmarshal(t, string(raw), &parsed)
	if want := fullyPopulatedCapabilities(); !reflect.DeepEqual(parsed, want) {
		t.Fatalf("parsed model differs:\n got %+v\nwant %+v", parsed, want)
	}
	assertJSONEqual(t, any(wire(t, parsed)), any(dumped))
}

// Python: tests/test_capabilities.py::TestFullyPopulatedWireShape::test_json_round_trip
func TestPyTestFullyPopulatedWireShape_TestJsonRoundTrip(t *testing.T) {
	text, err := json.Marshal(fullyPopulatedCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, generic(t, string(text)), generic(t, expectedWire))
	var parsed agui.AgentCapabilities
	mustUnmarshal(t, string(text), &parsed)
	if !reflect.DeepEqual(parsed, fullyPopulatedCapabilities()) {
		t.Fatalf("parsed model differs: %+v", parsed)
	}
}

// ---------------------------------------------------------------------------
// TestCrossLanguageFixture
// ---------------------------------------------------------------------------

type fixtureCase struct {
	Name       string          `json:"name"`
	ProducedBy []string        `json:"producedBy"`
	Input      json.RawMessage `json:"input"`
	Expected   json.RawMessage `json:"expected"`
}

func loadFixture(t *testing.T) []fixtureCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sdk-fixtures", "agent-capabilities.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f struct {
		Cases []fixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Cases
}

// Python: tests/test_capabilities.py::TestCrossLanguageFixture::test_fixture_covers_this_sdk
//
// The upstream fixture lists the SDKs that produce each case (typescript,
// python, dotnet); Go is not one of them, so this port does not filter by
// producedBy at all: it applies every case to Go. The intent kept is "no
// case is excluded" — the fixture has cases and the Go run covers all of
// them. The Python SDK's own coverage is asserted too, as upstream does.
func TestPyTestCrossLanguageFixture_TestFixtureCoversThisSdk(t *testing.T) {
	cases := loadFixture(t)
	if len(cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	var missing []string
	for _, c := range cases {
		found := false
		for _, s := range c.ProducedBy {
			if s == "python" {
				found = true
			}
		}
		if !found {
			missing = append(missing, c.Name)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("cases that exclude python: %v", missing)
	}
}

// Python: tests/test_capabilities.py::TestCrossLanguageFixture::test_every_case_reserializes_to_its_expected_json
func TestPyTestCrossLanguageFixture_TestEveryCaseReserializesToItsExpectedJson(t *testing.T) {
	for _, c := range loadFixture(t) {
		t.Run(c.Name, func(t *testing.T) {
			var caps agui.AgentCapabilities
			mustUnmarshal(t, string(c.Input), &caps)
			assertJSONEqual(t, any(wire(t, caps)), generic(t, string(c.Expected)))
		})
	}
}

// ---------------------------------------------------------------------------
// TestNoneFieldsAreOmitted
// ---------------------------------------------------------------------------

// Python: tests/test_capabilities.py::TestNoneFieldsAreOmitted::test_empty_agent_capabilities_serializes_to_an_empty_object
func TestPyTestNoneFieldsAreOmitted_TestEmptyAgentCapabilitiesSerializesToAnEmptyObject(t *testing.T) {
	assertJSONEqual(t, any(wire(t, agui.AgentCapabilities{})), any(map[string]any{}))
}

// Python: tests/test_capabilities.py::TestNoneFieldsAreOmitted::test_partially_declared_groups_omit_their_undeclared_fields
func TestPyTestNoneFieldsAreOmitted_TestPartiallyDeclaredGroupsOmitTheirUndeclaredFields(t *testing.T) {
	d := wire(t, agui.AgentCapabilities{
		Transport:      &agui.TransportCapabilities{Streaming: b(true)},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{Supported: b(true)},
	})
	assertJSONEqual(t, any(d), generic(t, `{"transport": {"streaming": true}, "humanInTheLoop": {"supported": true}}`))
}

// Python: tests/test_capabilities.py::TestNoneFieldsAreOmitted::test_no_null_survives_anywhere_in_a_partial_dump
func TestPyTestNoneFieldsAreOmitted_TestNoNullSurvivesAnywhereInAPartialDump(t *testing.T) {
	text, err := json.Marshal(agui.AgentCapabilities{
		Identity:   &agui.IdentityCapabilities{Name: p("agent-x")},
		MultiAgent: &agui.MultiAgentCapabilities{Subagents: []agui.SubagentInfo{{Name: "planner"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "null") {
		t.Fatalf("null in %s", text)
	}
	assertJSONEqual(t, generic(t, string(text)),
		generic(t, `{"identity": {"name": "agent-x"}, "multiAgent": {"subagents": [{"name": "planner"}]}}`))
}

// Python: tests/test_capabilities.py::TestNoneFieldsAreOmitted::test_false_and_zero_are_values_and_stay
func TestPyTestNoneFieldsAreOmitted_TestFalseAndZeroAreValuesAndStay(t *testing.T) {
	d := wire(t, agui.ExecutionCapabilities{CodeExecution: b(false), MaxIterations: n(0)})
	assertJSONEqual(t, any(d), generic(t, `{"codeExecution": false, "maxIterations": 0}`))
}

// ---------------------------------------------------------------------------
// TestSubagentRenameIsACleanBreak
// ---------------------------------------------------------------------------

// Python: tests/test_capabilities.py::TestSubagentRenameIsACleanBreak::test_old_wire_key_does_not_populate_subagents
//
// Adapted (Python-specific behaviour): Pydantic's extra="allow" keeps
// `subAgents` as an unvalidated extra and re-emits it on dump. The spec's
// processing model instead treats an undeclared property as unrecognised
// material — a strict check rejects it, a receiving peer strips it with a
// warning — and Go structs carry no extras. The intent kept is the clean
// break: the old key never populates `subagents`, and it is not laundered
// into `subagents` on the way back out.
func TestPyTestSubagentRenameIsACleanBreak_TestOldWireKeyDoesNotPopulateSubagents(t *testing.T) {
	doc := `{"supported": true, "subAgents": [{"name": "planner"}]}`
	var parsed agui.MultiAgentCapabilities
	mustUnmarshal(t, doc, &parsed)
	// Spec 1.0 changelog item 9: "the earlier `subAgents` key is not read".
	// encoding/json matches keys case-insensitively, so this currently fails.
	if parsed.Subagents != nil {
		t.Errorf("subAgents populated subagents: %+v", parsed.Subagents)
	}
	dumped := wire(t, parsed)
	assertNoKey(t, dumped, "subagents")

	if err := agui.Validate("MultiAgentCapabilities", []byte(doc)); err == nil {
		t.Fatal("strict validation accepted the retired subAgents key")
	}
	res, err := agui.Sanitize("MultiAgentCapabilities", []byte(doc), agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	assertJSONEqual(t, res.Value, generic(t, `{"supported": true}`))
	if len(res.Warnings) == 0 {
		t.Fatal("stripping subAgents produced no warning")
	}
}

// Python: tests/test_capabilities.py::TestSubagentRenameIsACleanBreak::test_old_wire_key_nested_in_agent_capabilities
func TestPyTestSubagentRenameIsACleanBreak_TestOldWireKeyNestedInAgentCapabilities(t *testing.T) {
	var caps agui.AgentCapabilities
	mustUnmarshal(t, `{"multiAgent": {"subAgents": [{"name": "planner"}]}}`, &caps)
	if caps.MultiAgent == nil {
		t.Fatal("multiAgent not parsed")
	}
	// Spec 1.0 changelog item 9: "the earlier `subAgents` key is not read".
	if caps.MultiAgent.Subagents != nil {
		t.Errorf("subAgents populated subagents: %+v", caps.MultiAgent.Subagents)
	}
}

// Python: tests/test_capabilities.py::TestSubagentRenameIsACleanBreak::test_old_attribute_name_is_gone
func TestPyTestSubagentRenameIsACleanBreak_TestOldAttributeNameIsGone(t *testing.T) {
	typ := reflect.TypeOf(agui.MultiAgentCapabilities{})
	for _, name := range []string{"SubAgents", "Sub_agents"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Errorf("MultiAgentCapabilities still has field %s", name)
		}
	}
	if _, ok := typ.FieldByName("Subagents"); !ok {
		t.Error("MultiAgentCapabilities has no Subagents field")
	}
}

// Python: tests/test_capabilities.py::TestSubagentRenameIsACleanBreak::test_old_class_name_is_a_deprecated_alias_only
func TestPyTestSubagentRenameIsACleanBreak_TestOldClassNameIsADeprecatedAliasOnly(t *testing.T) {
	t.Skip("N/A in Go: tests a Python import alias (ag_ui.core.SubAgentInfo is SubagentInfo, listed in __all__); the Go module never shipped a 0.x SubAgentInfo name to keep compatible, and the wire key/field clean break is covered by the tests above")
}

// Python: tests/test_capabilities.py::TestSubagentRenameIsACleanBreak::test_generated_classes_are_the_exported_ones
//
// Python checks that ag_ui.core re-exports the classes generated from the
// schema. The Go equivalent of "the exported types are the schema's" is that
// each Go capability type serialises to a document the schema definition of
// the same name accepts under the strict check.
func TestPyTestSubagentRenameIsACleanBreak_TestGeneratedClassesAreTheExportedOnes(t *testing.T) {
	c := fullyPopulatedCapabilities()
	values := map[string]any{
		"SubagentInfo":                 c.MultiAgent.Subagents[0],
		"IdentityCapabilities":         c.Identity,
		"TransportCapabilities":        c.Transport,
		"ToolsCapabilities":            c.Tools,
		"OutputCapabilities":           c.Output,
		"StateCapabilities":            c.State,
		"MultiAgentCapabilities":       c.MultiAgent,
		"ReasoningCapabilities":        c.Reasoning,
		"MultimodalInputCapabilities":  c.Multimodal.Input,
		"MultimodalOutputCapabilities": c.Multimodal.Output,
		"MultimodalCapabilities":       c.Multimodal,
		"ExecutionCapabilities":        c.Execution,
		"HumanInTheLoopCapabilities":   c.HumanInTheLoop,
		"AgentCapabilities":            c,
	}
	for name, v := range values {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if err := agui.Validate(name, raw); err != nil {
				t.Fatalf("schema %s rejects Go output %s: %v", name, raw, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSubagentInfo
// ---------------------------------------------------------------------------

// Python: tests/test_capabilities.py::TestSubagentInfo::test_name_is_required
func TestPyTestSubagentInfo_TestNameIsRequired(t *testing.T) {
	err := agui.Validate("SubagentInfo", []byte(`{}`))
	if err == nil {
		t.Fatal("SubagentInfo without name accepted")
	}
	var ve *agui.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("not a ValidationError: %v", err)
	}
	if ve.Keyword != "required" || !strings.Contains(ve.Message, "name") {
		t.Fatalf("error should flag the missing `name` field specifically: %+v", ve)
	}
}

// Python: tests/test_capabilities.py::TestSubagentInfo::test_description_is_optional_and_omitted_when_absent
func TestPyTestSubagentInfo_TestDescriptionIsOptionalAndOmittedWhenAbsent(t *testing.T) {
	info := agui.SubagentInfo{Name: "only-name"}
	if info.Name != "only-name" || info.Description != nil {
		t.Fatalf("unexpected %+v", info)
	}
	assertJSONEqual(t, any(wire(t, info)), generic(t, `{"name": "only-name"}`))
}

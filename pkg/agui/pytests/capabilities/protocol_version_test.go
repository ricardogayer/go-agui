package capabilities_test

// Port of sdks/python/tests/test_protocol_version.py.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ---------------------------------------------------------------------------
// TestProtocolVersionConstant
// ---------------------------------------------------------------------------

// Python: tests/test_protocol_version.py::TestProtocolVersionConstant::test_it_is_exported_from_ag_ui_core
//
// Python checks membership of ag_ui.core.__all__. In Go, export is by
// capitalisation and is enforced by the compiler: referencing
// agui.ProtocolVersion from this external test package is the check. The
// parsed form, agui.CurrentVersion, must agree with it.
func TestPyTestProtocolVersionConstant_TestItIsExportedFromAgUiCore(t *testing.T) {
	if agui.ProtocolVersion == "" {
		t.Fatal("agui.ProtocolVersion is empty")
	}
	if agui.CurrentVersion.String() != agui.ProtocolVersion {
		t.Fatalf("CurrentVersion %s != ProtocolVersion %s", agui.CurrentVersion, agui.ProtocolVersion)
	}
}

// Python: tests/test_protocol_version.py::TestProtocolVersionConstant::test_it_is_the_generated_schema_revision
//
// Python compares the re-export with the constant generated from the
// schema's $id. Go embeds the schema verbatim, so the equivalent is: the
// constant equals the version segment of the embedded schema's $id.
func TestPyTestProtocolVersionConstant_TestItIsTheGeneratedSchemaRevision(t *testing.T) {
	var s struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(agui.SchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`/spec/([^/]+)/schema\.json$`).FindStringSubmatch(s.ID)
	if m == nil {
		t.Fatalf("cannot find a version segment in $id %q", s.ID)
	}
	if m[1] != agui.ProtocolVersion {
		t.Fatalf("schema revision %q != ProtocolVersion %q", m[1], agui.ProtocolVersion)
	}
}

// Python: tests/test_protocol_version.py::TestProtocolVersionConstant::test_it_matches_the_published_grammar
func TestPyTestProtocolVersionConstant_TestItMatchesThePublishedGrammar(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+$`).MatchString(agui.ProtocolVersion) {
		t.Fatalf("ProtocolVersion %q does not match MAJOR.MINOR", agui.ProtocolVersion)
	}
	if _, err := agui.ParseVersion(agui.ProtocolVersion); err != nil {
		t.Fatalf("own version does not parse: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestConstantMatchesTypeScript
// ---------------------------------------------------------------------------

// Python: tests/test_protocol_version.py::TestConstantMatchesTypeScript::test_typescript_generates_the_same_protocol_version
//
// Like the Python test, this skips when the TypeScript sources are absent.
// The upstream checkout is not vendored in this repository; set
// AGUI_UPSTREAM_ROOT to an ag-ui checkout to run it.
func TestPyTestConstantMatchesTypeScript_TestTypescriptGeneratesTheSameProtocolVersion(t *testing.T) {
	root := os.Getenv("AGUI_UPSTREAM_ROOT")
	if root == "" {
		t.Skip("TypeScript core sources not present (set AGUI_UPSTREAM_ROOT to an ag-ui checkout)")
	}
	path := filepath.Join(root, "sdks", "typescript", "packages", "core", "src", "generated", "version.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("TypeScript core sources not present at %s", path)
	}
	m := regexp.MustCompile(`export const PROTOCOL_VERSION\s*=\s*["']([^"']+)["']`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatalf("PROTOCOL_VERSION is no longer declared in %s; the cross-SDK check has gone vacuous", path)
	}
	if m[1] != agui.ProtocolVersion {
		t.Fatalf("TypeScript %q != Go %q", m[1], agui.ProtocolVersion)
	}
}

// ---------------------------------------------------------------------------
// TestProtocolVersionOnTheWire
// ---------------------------------------------------------------------------

// Python: tests/test_protocol_version.py::TestProtocolVersionOnTheWire::test_run_started_serializes_the_declaration_as_protocol_version
func TestPyTestProtocolVersionOnTheWire_TestRunStartedSerializesTheDeclarationAsProtocolVersion(t *testing.T) {
	e := &agui.RunStartedEvent{ThreadID: "thread-1", RunID: "run-1", ProtocolVersion: p(agui.ProtocolVersion)}
	if got := wire(t, e)["protocolVersion"]; got != agui.ProtocolVersion {
		t.Fatalf("protocolVersion = %v", got)
	}
	// The constructor a Go producer uses fills the declaration in.
	if got := wire(t, agui.NewRunStartedEvent("thread-1", "run-1"))["protocolVersion"]; got != agui.ProtocolVersion {
		t.Fatalf("NewRunStartedEvent protocolVersion = %v", got)
	}
}

// Python: tests/test_protocol_version.py::TestProtocolVersionOnTheWire::test_run_started_omits_the_declaration_when_it_is_not_set
func TestPyTestProtocolVersionOnTheWire_TestRunStartedOmitsTheDeclarationWhenItIsNotSet(t *testing.T) {
	text, err := json.Marshal(&agui.RunStartedEvent{ThreadID: "thread-1", RunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "protocolVersion") {
		t.Fatalf("protocolVersion present in %s", text)
	}
}

// Python: tests/test_protocol_version.py::TestProtocolVersionOnTheWire::test_run_agent_input_serializes_the_declaration
func TestPyTestProtocolVersionOnTheWire_TestRunAgentInputSerializesTheDeclaration(t *testing.T) {
	in := agui.RunAgentInput{
		ThreadID:        "thread-1",
		RunID:           "run-1",
		State:           nil,
		Messages:        agui.Messages{},
		Tools:           []agui.Tool{},
		Context:         []agui.Context{},
		ForwardedProps:  map[string]any{},
		ProtocolVersion: p(agui.ProtocolVersion),
	}
	if got := wire(t, in)["protocolVersion"]; got != agui.ProtocolVersion {
		t.Fatalf("protocolVersion = %v", got)
	}
}

// Python: tests/test_protocol_version.py::TestProtocolVersionOnTheWire::test_the_declaration_round_trips_through_the_wire_name
func TestPyTestProtocolVersionOnTheWire_TestTheDeclarationRoundTripsThroughTheWireName(t *testing.T) {
	doc := `{"type": "RUN_STARTED", "threadId": "thread-1", "runId": "run-1", "protocolVersion": "` + agui.ProtocolVersion + `"}`
	ev, err := agui.ParseEvent([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	rs, ok := ev.(*agui.RunStartedEvent)
	if !ok {
		t.Fatalf("parsed %T", ev)
	}
	if agui.Deref(rs.ProtocolVersion) != agui.ProtocolVersion {
		t.Fatalf("protocolVersion = %v", rs.ProtocolVersion)
	}
}

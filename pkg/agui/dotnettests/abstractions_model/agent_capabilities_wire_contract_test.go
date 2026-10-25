package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs.

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// canonicalCapabilitiesJSON is the exact document buildFullyPopulated
// serializes to. Key order is the declaration order of the Go types, which
// matches the generated .NET types (and the schema).
const canonicalCapabilitiesJSON = `{"identity":{"name":"TestAgent","type":"ag-ui-dotnet","description":"A test agent","version":"1.0.0","provider":"TestOrg","documentationUrl":"https://example.com/docs","metadata":{"region":"eu-central-1","tier":3}},"transport":{"streaming":true,"websocket":false,"httpBinary":true,"pushNotifications":true,"resumable":true},"tools":{"supported":true,"items":[{"name":"search","description":"Search the web","parameters":{}}],"parallelCalls":true,"clientProvided":true},"output":{"structuredOutput":true,"supportedMimeTypes":["text/plain","application/json"]},"state":{"snapshots":true,"deltas":true,"memory":false,"persistentState":true},"multiAgent":{"supported":true,"delegation":true,"handoffs":false,"subagents":[{"name":"helper","description":"A helper agent"}]},"reasoning":{"supported":true,"streaming":true,"encrypted":false},"multimodal":{"input":{"image":true,"audio":false,"video":false,"pdf":true,"file":true},"output":{"image":true,"audio":false}},"execution":{"codeExecution":true,"sandboxed":true,"maxIterations":10,"maxExecutionTime":30000},"humanInTheLoop":{"supported":true,"approvals":true,"interventions":false,"feedback":true,"interrupts":true,"approveWithEdits":true},"custom":{"rateLimit":42}}`

var loadBearingWireKeys = []string{
	"documentationUrl", "httpBinary", "pushNotifications", "structuredOutput", "supportedMimeTypes",
	"persistentState", "parallelCalls", "clientProvided", "codeExecution", "maxIterations",
	"maxExecutionTime", "approveWithEdits", "multiAgent", "humanInTheLoop", "subagents", "metadata", "custom",
}

func buildFullyPopulated() *agui.AgentCapabilities {
	T, F := agui.Ptr(true), agui.Ptr(false)
	return &agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{
			Name: agui.Ptr("TestAgent"), Type: agui.Ptr("ag-ui-dotnet"), Description: agui.Ptr("A test agent"),
			Version: agui.Ptr("1.0.0"), Provider: agui.Ptr("TestOrg"), DocumentationURL: agui.Ptr("https://example.com/docs"),
			Metadata: agui.Metadata{"region": "eu-central-1", "tier": 3},
		},
		Transport: &agui.TransportCapabilities{Streaming: T, Websocket: F, HTTPBinary: T, PushNotifications: T, Resumable: T},
		Tools: &agui.ToolsCapabilities{
			Supported:     T,
			Items:         []agui.Tool{{Name: "search", Description: "Search the web", Parameters: map[string]any{}}},
			ParallelCalls: T, ClientProvided: T,
		},
		Output:     &agui.OutputCapabilities{StructuredOutput: T, SupportedMimeTypes: []string{"text/plain", "application/json"}},
		State:      &agui.StateCapabilities{Snapshots: T, Deltas: T, Memory: F, PersistentState: T},
		MultiAgent: &agui.MultiAgentCapabilities{Supported: T, Delegation: T, Handoffs: F, Subagents: []agui.SubagentInfo{{Name: "helper", Description: agui.Ptr("A helper agent")}}},
		Reasoning:  &agui.ReasoningCapabilities{Supported: T, Streaming: T, Encrypted: F},
		Multimodal: &agui.MultimodalCapabilities{
			Input:  &agui.MultimodalInputCapabilities{Image: T, Audio: F, Video: F, PDF: T, File: T},
			Output: &agui.MultimodalOutputCapabilities{Image: T, Audio: F},
		},
		Execution:      &agui.ExecutionCapabilities{CodeExecution: T, Sandboxed: T, MaxIterations: agui.Ptr[int64](10), MaxExecutionTime: agui.Ptr[int64](30000)},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{Supported: T, Approvals: T, Interventions: F, Feedback: T, Interrupts: T, ApproveWithEdits: T},
		Custom:         map[string]any{"rateLimit": 42},
	}
}

func collectPropertyNames(v any, into map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			into[k] = true
			collectPropertyNames(val, into)
		}
	case []any:
		for _, val := range x {
			collectPropertyNames(val, into)
		}
	}
}

func findNullValuedProperties(v any, path string) []string {
	switch x := v.(type) {
	case nil:
		return []string{path}
	case map[string]any:
		var out []string
		for k, val := range x {
			out = append(out, findNullValuedProperties(val, path+"/"+k)...)
		}
		return out
	case []any:
		var out []string
		for i, val := range x {
			out = append(out, findNullValuedProperties(val, path+"/"+strconv.Itoa(i))...)
		}
		return out
	}
	return nil
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.FullyPopulated_SerializesToTheCanonicalWireDocument
func TestCsAgentCapabilitiesWireContractTest_FullyPopulated_SerializesToTheCanonicalWireDocument(t *testing.T) {
	got := string(mustJSON(t, buildFullyPopulated()))
	if got != canonicalCapabilitiesJSON {
		t.Fatalf("not the canonical document\n got: %s\nwant: %s", got, canonicalCapabilitiesJSON)
	}
	if err := agui.Validate("AgentCapabilities", []byte(got)); err != nil {
		t.Fatalf("schema rejects the canonical document: %v", err)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.FullyPopulated_ThroughAGUIJsonUtilities_MatchesTheContextByteForByte
func TestCsAgentCapabilitiesWireContractTest_FullyPopulated_ThroughAGUIJsonUtilities_MatchesTheContextByteForByte(t *testing.T) {
	t.Skip("N/A in Go: compares two System.Text.Json paths (source-generated context vs AGUIJsonUtilities.DefaultTypeInfoResolver in caller-owned options); Go has one encoding/json path, pinned by FullyPopulated_SerializesToTheCanonicalWireDocument")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.FullyPopulated_EmitsEveryLoadBearingWireKey
func TestCsAgentCapabilitiesWireContractTest_FullyPopulated_EmitsEveryLoadBearingWireKey(t *testing.T) {
	present := map[string]bool{}
	collectPropertyNames(decode(t, mustJSON(t, buildFullyPopulated())), present)
	for _, k := range loadBearingWireKeys {
		if !present[k] {
			t.Errorf("wire key %q missing", k)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.UnsetMembers_AreOmittedRatherThanWrittenAsNull
func TestCsAgentCapabilitiesWireContractTest_UnsetMembers_AreOmittedRatherThanWrittenAsNull(t *testing.T) {
	sparse := &agui.AgentCapabilities{
		Identity:   &agui.IdentityCapabilities{Name: agui.Ptr("sparse")},
		MultiAgent: &agui.MultiAgentCapabilities{Supported: agui.Ptr(true)},
	}
	js := string(mustJSON(t, sparse))
	assertEq(t, js, `{"identity":{"name":"sparse"},"multiAgent":{"supported":true}}`, "sparse document")
	if strings.Contains(js, "null") {
		t.Errorf("null in %s", js)
	}
	if n := findNullValuedProperties(decode(t, []byte(js)), ""); len(n) != 0 {
		t.Errorf("null-valued properties: %v", n)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.CanonicalDocument_RoundTripsBackToAnEquivalentObject
func TestCsAgentCapabilitiesWireContractTest_CanonicalDocument_RoundTripsBackToAnEquivalentObject(t *testing.T) {
	var r agui.AgentCapabilities
	if err := json.Unmarshal([]byte(canonicalCapabilitiesJSON), &r); err != nil {
		t.Fatal(err)
	}
	reserialized := string(mustJSON(t, &r))
	if reserialized != canonicalCapabilitiesJSON {
		t.Fatalf("re-serialization differs\n got: %s\nwant: %s", reserialized, canonicalCapabilitiesJSON)
	}
	assertEq(t, r.Identity.Metadata["region"], "eu-central-1", "identity.metadata.region")
	assertEq(t, r.Identity.Metadata["tier"], float64(3), "identity.metadata.tier")
	assertEq(t, r.Custom["rateLimit"], float64(42), "custom.rateLimit")
	if len(r.MultiAgent.Subagents) != 1 {
		t.Fatalf("subagents: %+v", r.MultiAgent.Subagents)
	}
	assertEq(t, r.MultiAgent.Subagents[0].Name, "helper", "subagent name")
	assertEq(t, agui.Deref(r.MultiAgent.Subagents[0].Description), "A helper agent", "subagent description")
	assertEq(t, agui.Deref(r.Execution.MaxIterations), int64(10), "maxIterations")
	assertEq(t, agui.Deref(r.Execution.MaxExecutionTime), int64(30000), "maxExecutionTime")
	boolIs(t, r.HumanInTheLoop.ApproveWithEdits, true, "approveWithEdits")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesWireContractTest.cs::AgentCapabilitiesWireContractTest.LegacySubAgentsKey_IsIgnored_NotMappedOntoSubagents
//
// Go's typed decode matches member names exactly (encoding/json alone would
// fold "subAgents" onto Subagents case-insensitively). Beyond the .NET
// assertions: the strict schema rejects the retired key, and the processing
// model strips it with a warning.
func TestCsAgentCapabilitiesWireContractTest_LegacySubAgentsKey_IsIgnored_NotMappedOntoSubagents(t *testing.T) {
	legacy := []byte(`{"multiAgent":{"supported":true,"subAgents":[{"name":"helper","description":"A helper agent"}]}}`)
	var r agui.AgentCapabilities
	if err := json.Unmarshal(legacy, &r); err != nil {
		t.Fatal(err)
	}
	boolIs(t, r.MultiAgent.Supported, true, "multiAgent.supported")
	if r.MultiAgent.Subagents != nil {
		t.Fatalf("legacy subAgents key was mapped onto Subagents: %+v", r.MultiAgent.Subagents)
	}
	if err := agui.Validate("AgentCapabilities", legacy); err == nil {
		t.Error("schema must reject the retired subAgents key")
	}
	res, err := agui.Sanitize("AgentCapabilities", legacy, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("stripping subAgents must be announced with a warning")
	}

	current := []byte(`{"multiAgent":{"supported":true,"subagents":[{"name":"helper","description":"A helper agent"}]}}`)
	var c agui.AgentCapabilities
	if err := json.Unmarshal(current, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.MultiAgent.Subagents) != 1 || c.MultiAgent.Subagents[0].Name != "helper" {
		t.Fatalf("subagents: %+v", c.MultiAgent.Subagents)
	}
}

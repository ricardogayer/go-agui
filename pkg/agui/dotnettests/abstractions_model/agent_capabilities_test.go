package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func boolIs(t *testing.T, p *bool, want bool, what string) {
	t.Helper()
	if p == nil || *p != want {
		t.Errorf("%s: got %v, want %v", what, p, want)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.RoundTrips_FullCapabilities
func TestCsAgentCapabilitiesTest_RoundTrips_FullCapabilities(t *testing.T) {
	T, F := agui.Ptr(true), agui.Ptr(false)
	caps := agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{
			Name: agui.Ptr("TestAgent"), Type: agui.Ptr("ag-ui-dotnet"), Description: agui.Ptr("A test agent"),
			Version: agui.Ptr("1.0.0"), Provider: agui.Ptr("TestOrg"), DocumentationURL: agui.Ptr("https://example.com/docs"),
		},
		Transport: &agui.TransportCapabilities{Streaming: T, Websocket: F, HTTPBinary: F, PushNotifications: F, Resumable: T},
		Tools: &agui.ToolsCapabilities{
			Supported: T, ParallelCalls: T, ClientProvided: T,
			Items: []agui.Tool{{Name: "search", Description: "Search the web", Parameters: map[string]any{}}},
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
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{Supported: T, Approvals: T, Interventions: F, Feedback: T},
	}
	data := mustJSON(t, &caps)
	var r agui.AgentCapabilities
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	assertEq(t, agui.Deref(r.Identity.Name), "TestAgent", "identity.name")
	assertEq(t, agui.Deref(r.Identity.Type), "ag-ui-dotnet", "identity.type")
	assertEq(t, agui.Deref(r.Identity.Description), "A test agent", "identity.description")
	assertEq(t, agui.Deref(r.Identity.Version), "1.0.0", "identity.version")
	assertEq(t, agui.Deref(r.Identity.Provider), "TestOrg", "identity.provider")
	assertEq(t, agui.Deref(r.Identity.DocumentationURL), "https://example.com/docs", "identity.documentationUrl")

	boolIs(t, r.Transport.Streaming, true, "transport.streaming")
	boolIs(t, r.Transport.Websocket, false, "transport.websocket")
	boolIs(t, r.Transport.Resumable, true, "transport.resumable")

	boolIs(t, r.Tools.Supported, true, "tools.supported")
	boolIs(t, r.Tools.ParallelCalls, true, "tools.parallelCalls")
	boolIs(t, r.Tools.ClientProvided, true, "tools.clientProvided")
	if len(r.Tools.Items) != 1 || r.Tools.Items[0].Name != "search" {
		t.Errorf("tools.items: %+v", r.Tools.Items)
	}

	boolIs(t, r.Output.StructuredOutput, true, "output.structuredOutput")
	assertEq(t, len(r.Output.SupportedMimeTypes), 2, "output.supportedMimeTypes count")

	boolIs(t, r.State.Snapshots, true, "state.snapshots")
	boolIs(t, r.State.Deltas, true, "state.deltas")
	boolIs(t, r.State.Memory, false, "state.memory")
	boolIs(t, r.State.PersistentState, true, "state.persistentState")

	boolIs(t, r.MultiAgent.Supported, true, "multiAgent.supported")
	boolIs(t, r.MultiAgent.Delegation, true, "multiAgent.delegation")
	boolIs(t, r.MultiAgent.Handoffs, false, "multiAgent.handoffs")
	if len(r.MultiAgent.Subagents) != 1 || r.MultiAgent.Subagents[0].Name != "helper" {
		t.Errorf("multiAgent.subagents: %+v", r.MultiAgent.Subagents)
	}

	boolIs(t, r.Reasoning.Supported, true, "reasoning.supported")
	boolIs(t, r.Reasoning.Streaming, true, "reasoning.streaming")
	boolIs(t, r.Reasoning.Encrypted, false, "reasoning.encrypted")

	boolIs(t, r.Multimodal.Input.Image, true, "multimodal.input.image")
	boolIs(t, r.Multimodal.Input.Audio, false, "multimodal.input.audio")
	boolIs(t, r.Multimodal.Input.PDF, true, "multimodal.input.pdf")
	boolIs(t, r.Multimodal.Output.Image, true, "multimodal.output.image")
	boolIs(t, r.Multimodal.Output.Audio, false, "multimodal.output.audio")

	boolIs(t, r.Execution.CodeExecution, true, "execution.codeExecution")
	boolIs(t, r.Execution.Sandboxed, true, "execution.sandboxed")
	assertEq(t, agui.Deref(r.Execution.MaxIterations), int64(10), "execution.maxIterations")
	assertEq(t, agui.Deref(r.Execution.MaxExecutionTime), int64(30000), "execution.maxExecutionTime")

	boolIs(t, r.HumanInTheLoop.Supported, true, "humanInTheLoop.supported")
	boolIs(t, r.HumanInTheLoop.Approvals, true, "humanInTheLoop.approvals")
	boolIs(t, r.HumanInTheLoop.Interventions, false, "humanInTheLoop.interventions")
	boolIs(t, r.HumanInTheLoop.Feedback, true, "humanInTheLoop.feedback")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.Serialize_UsesCamelCase
func TestCsAgentCapabilitiesTest_Serialize_UsesCamelCase(t *testing.T) {
	caps := agui.AgentCapabilities{
		Identity:       &agui.IdentityCapabilities{DocumentationURL: agui.Ptr("https://example.com")},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{Supported: agui.Ptr(true)},
		MultiAgent:     &agui.MultiAgentCapabilities{Supported: agui.Ptr(true)},
	}
	js := string(mustJSON(t, &caps))
	for _, k := range []string{"documentationUrl", "humanInTheLoop", "multiAgent"} {
		if !strings.Contains(js, k) {
			t.Errorf("%s missing from %s", k, js)
		}
	}
	for _, k := range []string{"DocumentationUrl", "HumanInTheLoop", "MultiAgent"} {
		if strings.Contains(js, k) {
			t.Errorf("%s present in %s", k, js)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.Serialize_OmitsNullCategories
func TestCsAgentCapabilitiesTest_Serialize_OmitsNullCategories(t *testing.T) {
	js := string(mustJSON(t, &agui.AgentCapabilities{Identity: &agui.IdentityCapabilities{Name: agui.Ptr("Agent")}}))
	if !strings.Contains(js, "identity") {
		t.Errorf("identity missing from %s", js)
	}
	for _, k := range []string{"transport", "tools", "state", "reasoning", "multimodal", "execution", "humanInTheLoop"} {
		if strings.Contains(js, k) {
			t.Errorf("%s present in %s", k, js)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.Serialize_OmitsNullProperties
func TestCsAgentCapabilitiesTest_Serialize_OmitsNullProperties(t *testing.T) {
	o := obj(t, &agui.AgentCapabilities{Transport: &agui.TransportCapabilities{Streaming: agui.Ptr(true)}})
	transport := objOf(t, o["transport"])
	assertEq(t, transport["streaming"], true, "transport.streaming")
	assertOmitted(t, transport, "websocket", "httpBinary", "pushNotifications", "resumable")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.Deserialize_FromTypeScriptPayload
func TestCsAgentCapabilitiesTest_Deserialize_FromTypeScriptPayload(t *testing.T) {
	doc := []byte(`{
	  "identity": {"name": "my-agent", "type": "langgraph", "description": "A custom agent", "version": "1.2.0", "provider": "Acme Corp", "documentationUrl": "https://docs.example.com"},
	  "transport": {"streaming": true, "websocket": false},
	  "tools": {"supported": true, "items": [{"name": "search", "description": "Search tool", "parameters": {}}], "parallelCalls": true, "clientProvided": false},
	  "output": {"structuredOutput": true, "supportedMimeTypes": ["text/plain", "application/json"]},
	  "state": {"snapshots": true, "deltas": false, "memory": true, "persistentState": true},
	  "multiAgent": {"supported": true, "delegation": true, "handoffs": false, "subagents": [{"name": "sub-1", "description": "Sub agent 1"}]},
	  "reasoning": {"supported": true, "streaming": true, "encrypted": false},
	  "multimodal": {"input": {"image": true, "audio": false, "video": false, "pdf": true, "file": true}, "output": {"image": true, "audio": false}},
	  "execution": {"codeExecution": true, "sandboxed": true, "maxIterations": 25, "maxExecutionTime": 60000},
	  "humanInTheLoop": {"supported": true, "approvals": true, "interventions": false, "feedback": true}
	}`)
	if err := agui.Validate("AgentCapabilities", doc); err != nil {
		t.Fatalf("schema rejects the TypeScript payload: %v", err)
	}
	var r agui.AgentCapabilities
	if err := json.Unmarshal(doc, &r); err != nil {
		t.Fatal(err)
	}
	assertEq(t, agui.Deref(r.Identity.Name), "my-agent", "identity.name")
	assertEq(t, agui.Deref(r.Identity.Type), "langgraph", "identity.type")
	boolIs(t, r.Transport.Streaming, true, "transport.streaming")
	boolIs(t, r.Transport.Websocket, false, "transport.websocket")
	boolIs(t, r.Tools.Supported, true, "tools.supported")
	if len(r.Tools.Items) != 1 || r.Tools.Items[0].Name != "search" {
		t.Errorf("tools.items: %+v", r.Tools.Items)
	}
	boolIs(t, r.Tools.ParallelCalls, true, "tools.parallelCalls")
	boolIs(t, r.Tools.ClientProvided, false, "tools.clientProvided")
	boolIs(t, r.Output.StructuredOutput, true, "output.structuredOutput")
	assertEq(t, r.Output.SupportedMimeTypes, []string{"text/plain", "application/json"}, "output.supportedMimeTypes")
	boolIs(t, r.State.Snapshots, true, "state.snapshots")
	boolIs(t, r.State.Deltas, false, "state.deltas")
	boolIs(t, r.State.Memory, true, "state.memory")
	boolIs(t, r.MultiAgent.Supported, true, "multiAgent.supported")
	assertEq(t, r.MultiAgent.Subagents[0].Name, "sub-1", "multiAgent.subagents[0].name")
	boolIs(t, r.Reasoning.Supported, true, "reasoning.supported")
	boolIs(t, r.Reasoning.Streaming, true, "reasoning.streaming")
	boolIs(t, r.Reasoning.Encrypted, false, "reasoning.encrypted")
	boolIs(t, r.Multimodal.Input.Image, true, "multimodal.input.image")
	boolIs(t, r.Multimodal.Input.PDF, true, "multimodal.input.pdf")
	boolIs(t, r.Multimodal.Output.Image, true, "multimodal.output.image")
	assertEq(t, agui.Deref(r.Execution.MaxIterations), int64(25), "execution.maxIterations")
	assertEq(t, agui.Deref(r.Execution.MaxExecutionTime), int64(60000), "execution.maxExecutionTime")
	boolIs(t, r.HumanInTheLoop.Approvals, true, "humanInTheLoop.approvals")
	boolIs(t, r.HumanInTheLoop.Feedback, true, "humanInTheLoop.feedback")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.EmptyCapabilities_SerializesToEmptyObject
func TestCsAgentCapabilitiesTest_EmptyCapabilities_SerializesToEmptyObject(t *testing.T) {
	assertEq(t, string(mustJSON(t, &agui.AgentCapabilities{})), "{}", "empty capabilities")
}

// C#: tests/AGUI.Abstractions.UnitTests/AgentCapabilitiesTest.cs::AgentCapabilitiesTest.Deserialize_CustomField_RoundTrips
func TestCsAgentCapabilitiesTest_Deserialize_CustomField_RoundTrips(t *testing.T) {
	var r agui.AgentCapabilities
	if err := json.Unmarshal([]byte(`{"identity":{"name":"test"},"custom":{"rateLimit":42}}`), &r); err != nil {
		t.Fatal(err)
	}
	assertEq(t, agui.Deref(r.Identity.Name), "test", "identity.name")
	if r.Custom == nil {
		t.Fatal("custom is nil")
	}
	assertEq(t, r.Custom["rateLimit"], float64(42), "custom.rateLimit")
}

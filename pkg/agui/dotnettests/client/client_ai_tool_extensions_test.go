package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs.
//
// AsAGUITools converts MEAI AIFunctions into AG-UI Tool definitions for
// RunAgentInput.tools. Go has no AIFunction: the application declares
// agui.Tool values directly in client.RunInput.Tools. The protocol-relevant
// part — which tool definitions reach the agent, with what name, description
// and parameter schema, in what order — is asserted on the request sent.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// sentTools runs once with tools and returns the request's "tools" member
// (nil, false when absent).
func sentTools(t *testing.T, tools []agui.Tool) ([]any, bool) {
	t.Helper()
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "hi"))
	if _, err := agent.Run(t.Context(), client.RunInput{Tools: tools}, nil); err != nil {
		t.Fatal(err)
	}
	v, ok := at(stub.lastBody(t), "tools")
	if !ok {
		return nil, false
	}
	list, isList := v.([]any)
	if !isList {
		t.Fatalf("tools is not an array: %v", v)
	}
	return list, true
}

// C#: tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs::ClientAIToolExtensionsTest.AsAGUITools_WithAIFunction_ConvertsCorrectly
func TestCsClientAIToolExtensionsTest_AsAGUITools_WithAIFunction_ConvertsCorrectly(t *testing.T) {
	tools, _ := sentTools(t, []agui.Tool{{Name: "greet", Description: "Says hello"}})
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	if got := mustAt(t, tools[0], "name"); got != "greet" {
		t.Errorf("name = %v", got)
	}
	if got := mustAt(t, tools[0], "description"); got != "Says hello" {
		t.Errorf("description = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs::ClientAIToolExtensionsTest.AsAGUITools_WithParameterizedFunction_IncludesJsonSchema
func TestCsClientAIToolExtensionsTest_AsAGUITools_WithParameterizedFunction_IncludesJsonSchema(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"city": map[string]any{"type": "string"}},
		"required":   []any{"city"},
	}
	tools, _ := sentTools(t, []agui.Tool{{Name: "get_weather", Description: "Gets weather", Parameters: schema}})
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	if got := mustAt(t, tools[0], "name"); got != "get_weather" {
		t.Errorf("name = %v", got)
	}
	params, ok := mustAt(t, tools[0], "parameters").(map[string]any)
	if !ok {
		t.Fatalf("parameters is not a JSON object: %v", mustAt(t, tools[0], "parameters"))
	}
	if got := mustAt(t, params, "properties.city.type"); got != "string" {
		t.Errorf("parameters carried opaquely? properties.city.type = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs::ClientAIToolExtensionsTest.AsAGUITools_WithEmptyList_ReturnsEmpty
//
// The schema says tools "Absent means none": an empty list may be sent as []
// or omitted; either way no tool reaches the agent.
func TestCsClientAIToolExtensionsTest_AsAGUITools_WithEmptyList_ReturnsEmpty(t *testing.T) {
	if tools, _ := sentTools(t, []agui.Tool{}); len(tools) != 0 {
		t.Fatalf("tools = %v", tools)
	}
}

// C#: tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs::ClientAIToolExtensionsTest.AsAGUITools_WithNullList_ReturnsEmpty
func TestCsClientAIToolExtensionsTest_AsAGUITools_WithNullList_ReturnsEmpty(t *testing.T) {
	tools, present := sentTools(t, nil)
	if len(tools) != 0 {
		t.Fatalf("tools = %v", tools)
	}
	if present && tools == nil {
		t.Fatal("tools sent as null; absent or [] are the only ways to say none")
	}
}

// C#: tests/AGUI.Client.UnitTests/ClientAIToolExtensionsTest.cs::ClientAIToolExtensionsTest.AsAGUITools_MultipleFunctions_ConvertsAll
func TestCsClientAIToolExtensionsTest_AsAGUITools_MultipleFunctions_ConvertsAll(t *testing.T) {
	tools, _ := sentTools(t, []agui.Tool{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
	})
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	if mustAt(t, tools[0], "name") != "tool_a" || mustAt(t, tools[1], "name") != "tool_b" {
		t.Fatalf("tools out of order: %v", tools)
	}
}

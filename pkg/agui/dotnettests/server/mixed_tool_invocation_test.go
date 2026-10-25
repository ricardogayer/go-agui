package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/MixedToolInvocationTest.cs.
//
// The three tests drive Microsoft.Extensions.AI's FunctionInvokingChatClient
// with ApprovalRequiredAIFunction directly — no AG-UI event or input is
// involved. The protocol side of the same flows (approval interrupts, client
// tools answered on the next run) is ported in
// hosting_mixed_tool_invocation_integration_test.go and the samples.

import "testing"

const naFICC = "N/A in Go: exercises MEAI's FunctionInvokingChatClient/ApprovalRequiredAIFunction tool loop directly (no AG-UI events or input); Go has no MEAI and the library does not invoke tools"

// C#: tests/AGUI.Server.UnitTests/MixedToolInvocationTest.cs::MixedToolInvocationTest.MixedInvocation_ApprovalFlow_ExecutesBothToolsOnResume
func TestCsMixedToolInvocationTest_MixedInvocation_ApprovalFlow_ExecutesBothToolsOnResume(t *testing.T) {
	t.Skip(naFICC)
}

// C#: tests/AGUI.Server.UnitTests/MixedToolInvocationTest.cs::MixedToolInvocationTest.MixedInvocation_LlmReissuesClientTool_TriggersNewInterrupt
func TestCsMixedToolInvocationTest_MixedInvocation_LlmReissuesClientTool_TriggersNewInterrupt(t *testing.T) {
	t.Skip(naFICC)
}

// C#: tests/AGUI.Server.UnitTests/MixedToolInvocationTest.cs::MixedToolInvocationTest.MixedInvocation_ServerOnlyTools_ExecuteWithoutApproval
func TestCsMixedToolInvocationTest_MixedInvocation_ServerOnlyTools_ExecuteWithoutApproval(t *testing.T) {
	t.Skip(naFICC)
}

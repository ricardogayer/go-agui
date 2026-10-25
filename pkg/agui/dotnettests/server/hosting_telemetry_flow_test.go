package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/TelemetryFlowTest.cs.
//
// The three tests pin how OpenTelemetry spans nest across the .NET client
// and server (execute_tool under agui.run, client orchestration spans, trace
// correlation of interrupt/resume runs). The Go library has no tracing
// instrumentation.

import "testing"

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TelemetryFlowTest.cs::TelemetryFlowTest.BackendTool_ServerExecuteToolNestsUnderAGUIRun
func TestCsTelemetryFlowTest_BackendTool_ServerExecuteToolNestsUnderAGUIRun(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TelemetryFlowTest.cs::TelemetryFlowTest.ClientTool_ExecuteToolNestsUnderClientOrchestrateTools
func TestCsTelemetryFlowTest_ClientTool_ExecuteToolNestsUnderClientOrchestrateTools(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TelemetryFlowTest.cs::TelemetryFlowTest.HumanInTheLoop_TwoRunsCorrelate_InterruptThenSuccessWithParentRunId
func TestCsTelemetryFlowTest_HumanInTheLoop_TwoRunsCorrelate_InterruptThenSuccessWithParentRunId(t *testing.T) {
	t.Skip(naTelemetry)
}

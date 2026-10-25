package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/RunTelemetryTest.cs.
//
// Every test pins the "agui.run" OpenTelemetry span the .NET server emits
// (System.Diagnostics.ActivitySource tags, status, trace nesting). The Go
// library has no tracing instrumentation, and the protocol defines none.

import "testing"

const naTelemetry = "N/A in Go: pins the .NET server's OpenTelemetry 'agui.run' span (ActivitySource tags/status/trace nesting); the Go library has no tracing instrumentation and AG-UI defines none"

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.SuccessfulRun_EmitsAGUIRunSpan_WithIdentityTagsAndSuccessOutcome
func TestCsRunTelemetryTest_SuccessfulRun_EmitsAGUIRunSpan_WithIdentityTagsAndSuccessOutcome(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.InterruptingRun_OutcomeIsInterrupt
func TestCsRunTelemetryTest_InterruptingRun_OutcomeIsInterrupt(t *testing.T) { t.Skip(naTelemetry) }

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.ContinuationRun_SetsParentRunIdTag
func TestCsRunTelemetryTest_ContinuationRun_SetsParentRunIdTag(t *testing.T) { t.Skip(naTelemetry) }

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.Run_NestsUnderAmbientActivity_SharingTraceId
func TestCsRunTelemetryTest_Run_NestsUnderAmbientActivity_SharingTraceId(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.TwoRuns_UnderOneAmbientActivity_ShareTraceId_AndChildCarriesParentRunId
func TestCsRunTelemetryTest_TwoRuns_UnderOneAmbientActivity_ShareTraceId_AndChildCarriesParentRunId(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.FailingRun_SetsErrorStatusAndErrorType
func TestCsRunTelemetryTest_FailingRun_SetsErrorStatusAndErrorType(t *testing.T) {
	for _, row := range []string{"yieldThread=false", "yieldThread=true"} {
		t.Run(row, func(t *testing.T) { t.Skip(naTelemetry) })
	}
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.ProviderOperationCanceledException_IsRecordedAsError
func TestCsRunTelemetryTest_ProviderOperationCanceledException_IsRecordedAsError(t *testing.T) {
	for _, row := range []string{"yieldThread=false", "yieldThread=true"} {
		t.Run(row, func(t *testing.T) { t.Skip(naTelemetry) })
	}
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.CallerCancellation_IsRecordedAsCanceledAndPropagates
func TestCsRunTelemetryTest_CallerCancellation_IsRecordedAsCanceledAndPropagates(t *testing.T) {
	for _, row := range []string{"yieldThread=false", "yieldThread=true"} {
		t.Run(row, func(t *testing.T) { t.Skip(naTelemetry) })
	}
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.CancellationAfterRunError_DoesNotOverwriteErrorOutcome
func TestCsRunTelemetryTest_CancellationAfterRunError_DoesNotOverwriteErrorOutcome(t *testing.T) {
	t.Skip(naTelemetry)
}

// C#: tests/AGUI.Server.UnitTests/RunTelemetryTest.cs::RunTelemetryTest.NoListener_DoesNotCreateAGUIRunActivity
func TestCsRunTelemetryTest_NoListener_DoesNotCreateAGUIRunActivity(t *testing.T) {
	t.Skip(naTelemetry)
}

package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunTelemetryIntegrationTest.cs.
//
// Both tests pin the hosted endpoint's OpenTelemetry "agui.run" span; the Go
// library has no tracing instrumentation (see run_telemetry_test.go).

import "testing"

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunTelemetryIntegrationTest.cs::RunTelemetryIntegrationTest.PostRun_EmitsAGUIRunSpan_WithSuccessOutcome
func TestCsRunTelemetryIntegrationTest_PostRun_EmitsAGUIRunSpan_WithSuccessOutcome(t *testing.T) {
	for _, f := range []string{formatJSON, formatProtobuf} {
		t.Run(f, func(t *testing.T) { t.Skip(naTelemetry) })
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunTelemetryIntegrationTest.cs::RunTelemetryIntegrationTest.PostRun_ApprovalInterrupt_EmitsInterruptOutcome
func TestCsRunTelemetryIntegrationTest_PostRun_ApprovalInterrupt_EmitsInterruptOutcome(t *testing.T) {
	for _, f := range []string{formatJSON, formatProtobuf} {
		t.Run(f, func(t *testing.T) { t.Skip(naTelemetry) })
	}
}

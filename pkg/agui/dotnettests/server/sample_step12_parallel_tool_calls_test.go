package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step12_ParallelToolCallsTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step12_ParallelToolCallsTest.cs::Step12_ParallelToolCallsTest.PostRun_WithParallelToolCalls_InvokesBothToolsAndStreamsResults
func TestCsStep12_ParallelToolCallsTest_PostRun_WithParallelToolCalls_InvokesBothToolsAndStreamsResults(t *testing.T) {
	r := newSampleRun(t, "Step12_ParallelToolCalls")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	// Two parallel calls, two results keyed on distinct call ids.
	calls, results := toolCallsOf(r.client.Store.Messages), toolMessagesOf(r.client.Store.Messages)
	if len(calls) != 2 || len(results) != 2 {
		t.Fatalf("calls %+v results %+v", calls, results)
	}
	names := map[string]bool{calls[0].Function.Name: true, calls[1].Function.Name: true}
	if !names["get_weather"] || !names["get_current_time"] {
		t.Fatalf("calls %+v", calls)
	}
	// Outbound round trip: both results survive as distinct tool messages
	// keyed on their call ids (TOOL_CALL_RESULT.messageId is the call id).
	if results[0].ToolCallID == results[1].ToolCallID {
		t.Fatal("results share a call id")
	}
	for _, tm := range results {
		if tm.ID != tm.ToolCallID {
			t.Fatalf("tool message id %q, call id %q", tm.ID, tm.ToolCallID)
		}
	}
	in, _, err := r.client.BuildInput(client.RunInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := toolMessagesOf(in.Messages); len(got) != 2 || got[0].ID == got[1].ID {
		t.Fatalf("history sent back: %+v", got)
	}
}

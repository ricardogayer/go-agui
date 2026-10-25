package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step02_BackendToolsTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step02_BackendToolsTest.cs::Step02_BackendToolsTest.PostRun_WithBackendToolCall_InvokesToolAndStreamsResult
func TestCsStep02_BackendToolsTest_PostRun_WithBackendToolCall_InvokesToolAndStreamsResult(t *testing.T) {
	r := newSampleRun(t, "Step02_BackendTools")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	// The backend tool ran on the server: the call is answered in the same run,
	// so nothing is left pending for the application.
	calls, results := toolCallsOf(r.client.Store.Messages), toolMessagesOf(r.client.Store.Messages)
	if len(calls) != 1 || calls[0].Function.Name != "SearchRestaurants" || len(results) != 1 || results[0].ToolCallID != calls[0].ID {
		t.Fatalf("calls %+v results %+v", calls, results)
	}
	if len(res.PendingToolCallIDs) != 0 {
		t.Fatalf("pending %v", res.PendingToolCallIDs)
	}
	if lastAssistantText(r.client.Store.Messages) == "" {
		t.Fatal("no final assistant text")
	}
}

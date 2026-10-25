package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step03_FrontendToolsTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step03_FrontendToolsTest.cs::Step03_FrontendToolsTest.PostRun_WithFrontendToolCall_InvokesToolLocallyAndStreamsResult
func TestCsStep03_FrontendToolsTest_PostRun_WithFrontendToolCall_InvokesToolLocallyAndStreamsResult(t *testing.T) {
	r := newSampleRun(t, "Step03_FrontendTools")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	tools := r.baselineTools(1)
	res := r.runTurn(1, client.RunInput{Tools: tools})
	r.assertRequest(1, res)
	// The run stops on the frontend call, as a success leaving it pending.
	if !res.Succeeded() || len(res.PendingToolCallIDs) != 1 {
		t.Fatalf("status %s pending %v", res.Status, res.PendingToolCallIDs)
	}
	call := toolCallsOf(r.client.Store.Messages)[0]
	if call.ID != res.PendingToolCallIDs[0] || call.Function.Name != "GetUserLocation" {
		t.Fatalf("call %+v", call)
	}
	// The application runs the tool locally and answers the call (the
	// sample's GPS result, from the baseline's second request).
	turn2 := r.baselineMessages(2)
	answer := turn2[len(turn2)-1].(*agui.ToolMessage)
	r.client.AddMessage(&agui.ToolMessage{ID: answer.ID, ToolCallID: call.ID, Content: answer.Content})
	res = r.runTurn(2, client.RunInput{Tools: tools})
	r.assertRequest(2, res)
	if !res.Succeeded() || len(res.PendingToolCallIDs) != 0 || lastAssistantText(r.client.Store.Messages) == "" {
		t.Fatalf("turn 2: status %s pending %v", res.Status, res.PendingToolCallIDs)
	}
}

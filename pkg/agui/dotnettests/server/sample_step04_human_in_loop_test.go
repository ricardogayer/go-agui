package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step04_HumanInLoopTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step04_HumanInLoopTest.cs::Step04_HumanInLoopTest.PostRun_WithApprovalRequired_RoundTripsThroughBothWrappers
func TestCsStep04_HumanInLoopTest_PostRun_WithApprovalRequired_RoundTripsThroughBothWrappers(t *testing.T) {
	// The .NET sample routes the approval through a "request_approval"
	// frontend tool: turn 1 calls it, the user's decision is its result.
	r := newSampleRun(t, "Step04_HumanInLoop")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{})
	r.assertRequest(1, res)
	if !res.Succeeded() || len(res.PendingToolCallIDs) != 1 {
		t.Fatalf("status %s pending %v", res.Status, res.PendingToolCallIDs)
	}
	turn2 := r.baselineMessages(2)
	answer := turn2[len(turn2)-1].(*agui.ToolMessage)
	if answer.ToolCallID != res.PendingToolCallIDs[0] {
		t.Fatalf("the baseline answers %s, the run left %v pending", answer.ToolCallID, res.PendingToolCallIDs)
	}
	r.client.AddMessage(answer)
	res = r.runTurn(2, client.RunInput{})
	r.assertRequest(2, res)
	if !res.Succeeded() {
		t.Fatalf("turn 2: %s", res.Status)
	}
	if got := lastAssistantText(r.client.Store.Messages); got == "" {
		t.Fatal("no final assistant text")
	}
}

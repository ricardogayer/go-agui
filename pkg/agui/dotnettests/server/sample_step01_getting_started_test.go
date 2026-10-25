package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step01_GettingStartedTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step01_GettingStartedTest.cs::Step01_GettingStartedTest.PostRun_MultiTurn_SynthesizesAssistantMessages
func TestCsStep01_GettingStartedTest_PostRun_MultiTurn_SynthesizesAssistantMessages(t *testing.T) {
	r := newSampleRun(t, "Step01_GettingStarted")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("turn 1: %s", res.Status)
	}
	// The client synthesizes the assistant message from the stream...
	if got := lastAssistantText(r.client.Store.Messages); got != "Hello! How can I help you today?" {
		t.Fatalf("assistant message %q", got)
	}
	// ...and sends it back as history on the next turn, with the new user message.
	turn2 := r.baselineMessages(2)
	r.client.AddMessage(turn2[len(turn2)-1])
	res = r.runTurn(2, client.RunInput{})
	r.assertRequest(2, res)
}

package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step10_InterruptsUserInputTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step10_InterruptsUserInputTest.cs::Step10_InterruptsUserInputTest.PostRun_SetupRequest_EmitsInputInterruptThenResumes
func TestCsStep10_InterruptsUserInputTest_PostRun_SetupRequest_EmitsInputInterruptThenResumes(t *testing.T) {
	r := newSampleRun(t, "Step10_InterruptsUserInput")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{})
	r.assertRequest(1, res)
	if res.Status != client.StatusInterrupted || len(r.client.PendingInterrupts) != 1 {
		t.Fatalf("status %s interrupts %v", res.Status, r.client.PendingInterrupts)
	}
	i := r.client.PendingInterrupts[0]
	if i.Reason != "input_required" || i.ResponseSchema == nil {
		t.Fatalf("interrupt %+v", i)
	}
	// The user types a username; the resume entry carries it.
	resume := r.baselineResume(2)
	if len(resume) != 1 || resume[0].InterruptID != i.ID {
		t.Fatalf("resume %+v for interrupt %s", resume, i.ID)
	}
	res = r.runTurn(2, client.RunInput{Resume: resume})
	r.assertRequest(2, res)
	if !res.Succeeded() || lastAssistantText(r.client.Store.Messages) == "" {
		t.Fatalf("turn 2: %s", res.Status)
	}
}

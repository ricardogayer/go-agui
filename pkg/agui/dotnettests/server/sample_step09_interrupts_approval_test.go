package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step09_InterruptsApprovalTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// baselineResume decodes the resume entries of a turn's baseline request.
func (r *sampleRun) baselineResume(turn int) []agui.ResumeEntry {
	r.t.Helper()
	raw, _ := json.Marshal(r.turns[turn-1].Request["resume"])
	var out []agui.ResumeEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		r.t.Fatal(err)
	}
	return out
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step09_InterruptsApprovalTest.cs::Step09_InterruptsApprovalTest.PostRun_DeleteRequest_EmitsInterruptThenResumes
func TestCsStep09_InterruptsApprovalTest_PostRun_DeleteRequest_EmitsInterruptThenResumes(t *testing.T) {
	r := newSampleRun(t, "Step09_InterruptsApproval")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{})
	r.assertRequest(1, res)
	if res.Status != client.StatusInterrupted || len(r.client.PendingInterrupts) != 1 {
		t.Fatalf("status %s interrupts %v", res.Status, r.client.PendingInterrupts)
	}
	i := r.client.PendingInterrupts[0]
	if i.Reason != "tool_call" || agui.Deref(i.ToolCallID) == "" {
		t.Fatalf("interrupt %+v", i)
	}
	// The user approves: the resume entry answers the pending interrupt.
	resume := r.baselineResume(2)
	if len(resume) != 1 || resume[0].InterruptID != i.ID {
		t.Fatalf("resume %+v for interrupt %s", resume, i.ID)
	}
	res = r.runTurn(2, client.RunInput{Resume: resume})
	// Adapted: interrupt-resume.mdx — "the resuming run carries the same
	// threadId, the accumulated messages, and the state the interrupted run
	// left behind". The .NET sample client re-sends only its own chat
	// history (the user message); the Go client sends the accumulated
	// thread, which includes the approval-gated call of turn 1.
	r.assertRequestWith(2, res, func(want map[string]any) {
		call := toGeneric(t, toolCallsOf(res.Request.Messages)[0])
		want["messages"] = append(want["messages"].([]any), map[string]any{"role": "assistant", "toolCalls": []any{call}})
	})
	if !res.Succeeded() || len(r.client.PendingInterrupts) != 0 {
		t.Fatalf("turn 2: %s", res.Status)
	}
	if got := lastAssistantText(r.client.Store.Messages); got == "" {
		t.Fatal("no final assistant text")
	}
}

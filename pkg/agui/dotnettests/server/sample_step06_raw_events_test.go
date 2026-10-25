package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step06_RawEventsTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step06_RawEventsTest.cs::Step06_RawEventsTest.PostRun_ForwardsLlmUsageAsRawEvents
func TestCsStep06_RawEventsTest_PostRun_ForwardsLlmUsageAsRawEvents(t *testing.T) {
	r := newSampleRun(t, "Step06_RawEvents")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	raw := single[*agui.RawEvent](t, res.Events)
	usage, ok := raw.Event.(map[string]any)
	if !ok || usage["outputTokenCount"] == nil {
		t.Fatalf("raw event %+v", raw.Event)
	}
	// RAW is passthrough: it does not disturb the text message it arrived in.
	if lastAssistantText(r.client.Store.Messages) == "" {
		t.Fatal("no assistant text")
	}
}

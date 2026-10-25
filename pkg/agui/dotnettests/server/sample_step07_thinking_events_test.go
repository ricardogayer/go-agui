package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step07_ThinkingEventsTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step07_ThinkingEventsTest.cs::Step07_ThinkingEventsTest.PostRun_WithReasoningContent_EmitsReasoningAndTextEvents
func TestCsStep07_ThinkingEventsTest_PostRun_WithReasoningContent_EmitsReasoningAndTextEvents(t *testing.T) {
	r := newSampleRun(t, "Step07_ThinkingEvents")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	var reasoning *agui.ReasoningMessage
	for _, m := range r.client.Store.Messages {
		if rm, ok := m.(*agui.ReasoningMessage); ok {
			reasoning = rm
		}
	}
	if reasoning == nil || reasoning.Content == "" {
		t.Fatalf("no reasoning message in %+v", r.client.Store.Messages)
	}
	if reasoning.ID == single[*agui.TextMessageStartEvent](t, res.Events).MessageID {
		t.Fatal("reasoning and text share an id")
	}
	if lastAssistantText(r.client.Store.Messages) == "" {
		t.Fatal("no answer text")
	}
}

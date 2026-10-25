package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step08_MultimodalMessagesTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step08_MultimodalMessagesTest.cs::Step08_MultimodalMessagesTest.PostRun_WithImageUrl_DescribesImage
func TestCsStep08_MultimodalMessagesTest_PostRun_WithImageUrl_DescribesImage(t *testing.T) {
	r := newSampleRun(t, "Step08_MultimodalMessages")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	// The image travels as a data part next to the text.
	um := res.Request.Messages[0].(*agui.UserMessage)
	parts := um.Content.Parts()
	if len(parts) != 2 || parts[0].PartType() != agui.PartTypeText || parts[1].PartType() != agui.PartTypeImage {
		t.Fatalf("parts %+v", parts)
	}
	if _, ok := parts[1].(*agui.ImagePart).Source.(*agui.DataSource); !ok {
		t.Fatalf("image source %T", parts[1].(*agui.ImagePart).Source)
	}
}

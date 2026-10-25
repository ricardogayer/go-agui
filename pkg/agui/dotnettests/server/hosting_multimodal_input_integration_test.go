package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/MultimodalInputIntegrationTest.cs.

import (
	"net/http"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const multimodalInput = `{
  "threadId": "thread-1", "runId": "run-1",
  "messages": [{
    "id": "message-1", "role": "user",
    "content": [
      { "type": "text", "text": "Summarize this report." },
      { "type": "image", "source": { "type": "url", "value": "https://example.com/chart.png", "mimeType": "image/png" },
        "metadata": { "detail": "high" } },
      { "type": "document", "source": { "type": "data", "value": "JVBERg==", "mimeType": "application/pdf" },
        "metadata": { "filename": "report.pdf" } }
    ]
  }],
  "context": []
}`

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/MultimodalInputIntegrationTest.cs::MultimodalInputIntegrationTest.PostRun_CanonicalMultimodalContent_ReachesInnerChatClient
func TestCsMultimodalInputIntegrationTest_PostRun_CanonicalMultimodalContent_ReachesInnerChatClient(t *testing.T) {
	for _, format := range []string{formatJSON, formatProtobuf} {
		t.Run(format, func(t *testing.T) {
			var captured agui.Messages
			accept := sse.ContentType
			if format == formatProtobuf {
				accept = protobuf.ContentType
			}
			runner := server.RunnerFunc(func(ctx contextT, in *agui.RunAgentInput, s *stream.SSE) error {
				captured = in.Messages
				if err := s.RunStarted(in.ThreadID, in.RunID); err != nil {
					return err
				}
				return s.RunFinished(in.ThreadID, in.RunID)
			})
			if format == formatJSON {
				if w := post(t, runner, multimodalInput); w.Status != http.StatusOK {
					t.Fatalf("status %d", w.Status)
				}
			} else {
				resp, _ := postRaw(t, runner, multimodalInput, accept)
				if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != protobuf.ContentType {
					t.Fatalf("status %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
				}
			}
			if len(captured) != 1 {
				t.Fatalf("messages %+v", captured)
			}
			um := captured[0].(*agui.UserMessage)
			parts := um.Content.Parts()
			if um.ID != "message-1" || len(parts) != 3 {
				t.Fatalf("message %+v", um)
			}
			if p := parts[0].(*agui.TextPart); p.Text != "Summarize this report." {
				t.Fatalf("text %+v", p)
			}
			img := parts[1].(*agui.ImagePart)
			url := img.Source.(*agui.URLSource)
			if url.Value != "https://example.com/chart.png" || agui.Deref(url.MimeType) != "image/png" || img.Metadata.(map[string]any)["detail"] != "high" {
				t.Fatalf("image %+v %+v", img, url)
			}
			doc := parts[2].(*agui.DocumentPart)
			data := doc.Source.(*agui.DataSource)
			if data.Value != "JVBERg==" || data.MimeType != "application/pdf" || doc.Metadata.(map[string]any)["filename"] != "report.pdf" {
				t.Fatalf("document %+v %+v", doc, data)
			}
		})
	}
}

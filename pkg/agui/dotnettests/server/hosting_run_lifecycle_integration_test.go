package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunLifecycleIntegrationTest.cs.

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunLifecycleIntegrationTest.cs::RunLifecycleIntegrationTest.PostRun_EmptyStream_AutoGeneratesRunLifecycleEvents
func TestCsRunLifecycleIntegrationTest_PostRun_EmptyStream_AutoGeneratesRunLifecycleEvents(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := emptyRun(t, format, nil)
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{})
		assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeRunFinished)
		started, finished := res.Events[0].(*agui.RunStartedEvent), res.Events[1].(*agui.RunFinishedEvent)
		// The thread is the client's; the run id names this response.
		if started.ThreadID != c.ThreadID || started.RunID != res.Request.RunID || finished.RunID != started.RunID {
			t.Fatalf("identity: thread %s, started %s/%s, finished %s", c.ThreadID, started.ThreadID, started.RunID, finished.RunID)
		}
		if !res.Succeeded() {
			t.Fatalf("status %s", res.Status)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RunLifecycleIntegrationTest.cs::RunLifecycleIntegrationTest.PostRun_StreamThrowsAfterUpdate_EmitsSanitizedRunErrorWithoutRunFinished
func TestCsRunLifecycleIntegrationTest_PostRun_StreamThrowsAfterUpdate_EmitsSanitizedRunErrorWithoutRunFinished(t *testing.T) {
	for _, format := range []string{formatJSON, formatProtobuf} {
		for _, yield := range []bool{false, true} {
			for _, providerCancellation := range []bool{false, true} {
				name := fmt.Sprintf("%s_yieldThread=%v_providerCancellation=%v", format, yield, providerCancellation)
				t.Run(name, func(t *testing.T) {
					c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
						must(s.RunStarted(in.ThreadID, in.RunID))
						must(s.TextStart("msg-1"))
						must(s.TextContent("msg-1", "partial"))
						if yield {
							runtime.Gosched()
						}
						// The provider fails — with an error, or with its own
						// cancellation (the request itself is still live):
						// either way a run failure, reported sanitized with
						// the usage counted so far.
						providerErr := errors.New("sensitive provider failure details")
						if providerCancellation {
							providerErr = fmt.Errorf("provider timeout: %w", context.Canceled)
						}
						if providerErr != nil {
							must(s.Send(sanitizedRunError()))
						}
					}), format)
					c.say("u1", "Hi")
					res, err := c.Run(context.Background(), client.RunInput{}, nil)
					if err != nil {
						t.Fatal(err)
					}
					assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeTextMessageStart,
						agui.EventTypeTextMessageContent, agui.EventTypeRunError)
					if d := res.Events[2].(*agui.TextMessageContentEvent).Delta; d != "partial" {
						t.Fatalf("delta %q", d)
					}
					if res.Status != client.StatusFailed || res.Error == nil {
						t.Fatalf("status %s", res.Status)
					}
					e := res.Error
					if agui.Deref(e.Code) != "StreamingError" || e.Message != "An error occurred while streaming the agent response." {
						t.Fatalf("RUN_ERROR %+v", e)
					}
					if strings.Contains(e.Message, "sensitive") || strings.Contains(e.Message, "timeout") {
						t.Fatalf("provider details leaked: %q", e.Message)
					}
					if len(e.Usage) != 1 || agui.Deref(e.Usage[0].Model) != "test-model" || agui.Deref(e.Usage[0].InputTokens) != 7 {
						t.Fatalf("usage %+v", e.Usage)
					}
					if len(ofType[*agui.RunFinishedEvent](res.Events)) != 0 {
						t.Fatal("RUN_FINISHED after RUN_ERROR")
					}
					if format == formatProtobuf && c.contentTypes[0] != protoMediaType {
						t.Fatalf("Content-Type %q", c.contentTypes[0])
					}
				})
			}
		}
	}
}

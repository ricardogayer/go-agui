package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs.
//
// ChatOptions.RawRepresentationFactory lets the application hand
// AGUIChatClient the RunAgentInput to send (thread/run ids, messages), with
// ConversationId as the thread id fallback. In Go the application sets the
// same things directly: Agent.ThreadID, RunInput.RunID and the thread's
// messages (Agent.Store).

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

func emptyRun(t *testing.T, format string, seen *[]*agui.RunAgentInput) *testClient {
	return newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		if seen != nil {
			*seen = append(*seen, in)
		}
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs::RawRepresentationFactoryIntegrationTest.GetStreamingResponseAsync_WithRawRepresentationFactory_UsesProvidedRunAgentInput
func TestCsRawRepresentationFactoryIntegrationTest_GetStreamingResponseAsync_WithRawRepresentationFactory_UsesProvidedRunAgentInput(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := emptyRun(t, format, nil)
		c.ThreadID = "custom-thread"
		c.say("u1", "Hello")
		res := c.runOK(t, client.RunInput{RunID: "custom-run"})
		started := single[*agui.RunStartedEvent](t, res.Events)
		if started.ThreadID != "custom-thread" || started.RunID != "custom-run" {
			t.Fatalf("RUN_STARTED %s/%s", started.ThreadID, started.RunID)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs::RawRepresentationFactoryIntegrationTest.GetStreamingResponseAsync_WithRawRepresentationFactory_PreservesProvidedMessages
func TestCsRawRepresentationFactoryIntegrationTest_GetStreamingResponseAsync_WithRawRepresentationFactory_PreservesProvidedMessages(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var seen []*agui.RunAgentInput
		c := emptyRun(t, format, &seen)
		c.ThreadID = "t1"
		// The application supplies the thread's messages itself.
		c.Store.Messages = agui.Messages{&agui.UserMessage{ID: "m1", Content: agui.PartsContent(agui.NewTextPart("pre-set message"))}}
		c.runOK(t, client.RunInput{RunID: "r1"})
		if len(seen) != 1 || len(seen[0].Messages) != 1 || seen[0].Messages[0].MessageRole() != agui.RoleUser ||
			seen[0].Messages[0].(*agui.UserMessage).Content.String() != "pre-set message" {
			t.Fatalf("server received %+v", seen)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs::RawRepresentationFactoryIntegrationTest.GetStreamingResponseAsync_WithoutRawRepresentationFactory_FallsBackToConversationId
func TestCsRawRepresentationFactoryIntegrationTest_GetStreamingResponseAsync_WithoutRawRepresentationFactory_FallsBackToConversationId(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := emptyRun(t, format, nil)
		c.ThreadID = "conv-123"
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{})
		if th := single[*agui.RunStartedEvent](t, res.Events).ThreadID; th != "conv-123" {
			t.Fatalf("threadId %q", th)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs::RawRepresentationFactoryIntegrationTest.GetStreamingResponseAsync_WithRawRepresentationFactory_ConversationIdFallsBackForThreadId
func TestCsRawRepresentationFactoryIntegrationTest_GetStreamingResponseAsync_WithRawRepresentationFactory_ConversationIdFallsBackForThreadId(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := emptyRun(t, format, nil)
		c.ThreadID = "conv-456"
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{RunID: "explicit-run"})
		started := single[*agui.RunStartedEvent](t, res.Events)
		if len(res.Events) == 0 || started.ThreadID != "conv-456" || started.RunID != "explicit-run" {
			t.Fatalf("RUN_STARTED %s/%s", started.ThreadID, started.RunID)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/RawRepresentationFactoryIntegrationTest.cs::RawRepresentationFactoryIntegrationTest.GetStreamingResponseAsync_WithNonRunAgentInputFromFactory_FallsBackToDefault
func TestCsRawRepresentationFactoryIntegrationTest_GetStreamingResponseAsync_WithNonRunAgentInputFromFactory_FallsBackToDefault(t *testing.T) {
	for _, f := range []string{formatJSON, formatProtobuf} {
		t.Run(f, func(t *testing.T) {
			t.Skip("N/A in Go: pins MEAI's object-typed RawRepresentationFactory returning a non-RunAgentInput; the Go client's inputs are typed (Agent fields, client.RunInput)")
		})
	}
}

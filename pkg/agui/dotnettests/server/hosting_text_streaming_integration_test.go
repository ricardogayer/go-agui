package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// textAgent answers each run with the next of replies; it records the
// messages each run received.
func textAgent(t *testing.T, format string, seen *[]agui.Messages, replies ...string) *testClient {
	turn := 0
	return newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		if seen != nil {
			*seen = append(*seen, in.Messages)
		}
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage(replies[turn])
		turn++
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs::TextStreamingIntegrationTest.PostRun_TextContent_MapsToTextMessageEvents
func TestCsTextStreamingIntegrationTest_PostRun_TextContent_MapsToTextMessageEvents(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := textAgent(t, format, nil, "Hello world!")
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{})
		assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeTextMessageStart, agui.EventTypeTextMessageContent,
			agui.EventTypeTextMessageEnd, agui.EventTypeRunFinished)
		if r := agui.Deref(res.Events[1].(*agui.TextMessageStartEvent).Role); r != agui.TextMessageRoleAssistant {
			t.Fatalf("role %q", r)
		}
		if d := res.Events[2].(*agui.TextMessageContentEvent).Delta; d != "Hello world!" {
			t.Fatalf("delta %q", d)
		}
		if !res.Succeeded() {
			t.Fatalf("status %s", res.Status)
		}
		assertOneRun(t, res)
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs::TextStreamingIntegrationTest.PostRun_MultiTurn_MessagesConvertedFromChatToAGUI
func TestCsTextStreamingIntegrationTest_PostRun_MultiTurn_MessagesConvertedFromChatToAGUI(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var seen []agui.Messages
		c := textAgent(t, format, &seen, "Response to turn 1", "Response to turn 2")
		c.say("u1", "Hello")
		res1 := c.runOK(t, client.RunInput{})
		if len(seen[0]) != 1 || seen[0][0].MessageRole() != agui.RoleUser {
			t.Fatalf("turn 1 messages %+v", seen[0])
		}
		if got := assistantText(res1.Events); got != "Response to turn 1" {
			t.Fatalf("turn 1 text %q", got)
		}
		// The thread now holds the assistant's reply; the user follows up.
		c.say("u2", "Follow up")
		res2 := c.runOK(t, client.RunInput{})
		roles := []agui.Role{}
		for _, m := range seen[1] {
			roles = append(roles, m.MessageRole())
		}
		if len(roles) != 3 || roles[0] != agui.RoleUser || roles[1] != agui.RoleAssistant || roles[2] != agui.RoleUser {
			t.Fatalf("turn 2 roles %v", roles)
		}
		if got := assistantText(res2.Events); got != "Response to turn 2" {
			t.Fatalf("turn 2 text %q", got)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs::TextStreamingIntegrationTest.PostRun_Streaming_YieldsTextContent
func TestCsTextStreamingIntegrationTest_PostRun_Streaming_YieldsTextContent(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := textAgent(t, format, nil, "Hello via ChatClient!")
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{})
		contents := ofType[*agui.TextMessageContentEvent](res.Events)
		if len(contents) != 1 || contents[0].Delta != "Hello via ChatClient!" {
			t.Fatalf("contents %+v", contents)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs::TextStreamingIntegrationTest.PostRun_MultiTurn_ViaGetResponseAsync
func TestCsTextStreamingIntegrationTest_PostRun_MultiTurn_ViaGetResponseAsync(t *testing.T) {
	// GetResponseAsync aggregates a run into response messages; the Go
	// counterpart is the assistant message the Store assembles.
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := textAgent(t, format, nil, "First response", "Second response")
		c.say("u1", "Hello")
		c.runOK(t, client.RunInput{})
		if got := lastAssistantText(c.Store.Messages); got != "First response" {
			t.Fatalf("turn 1 %q", got)
		}
		c.say("u2", "Follow up")
		c.runOK(t, client.RunInput{})
		if got := lastAssistantText(c.Store.Messages); got != "Second response" {
			t.Fatalf("turn 2 %q", got)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TextStreamingIntegrationTest.cs::TextStreamingIntegrationTest.PostRun_AuthorName_RoundTrips
func TestCsTextStreamingIntegrationTest_PostRun_AuthorName_RoundTrips(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			start := agui.NewTextMessageStartEvent("m1")
			start.Name = agui.Ptr("TestAgent")
			must(s.Send(start))
			must(s.TextContent("m1", "Hello!"))
			must(s.TextEnd("m1"))
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), format)
		c.say("u1", "Hi")
		c.runOK(t, client.RunInput{})
		var am *agui.AssistantMessage
		for _, m := range c.Store.Messages {
			if v, ok := m.(*agui.AssistantMessage); ok {
				am = v
			}
		}
		if am == nil || agui.Deref(am.Name) != "TestAgent" {
			t.Fatalf("assistant message %+v", am)
		}
		return c
	})
}

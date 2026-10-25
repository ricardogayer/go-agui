package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// fullReasoningFlow is the C# EmitFullReasoningFlow: one span and one
// reasoning message, both named "reason-1".
func fullReasoningFlow(t *testing.T) *testClient {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("reason-1"))
		must(s.ReasoningMessageStart("reason-1"))
		must(s.ReasoningMessageContent("reason-1", "Thinking step 1"))
		must(s.ReasoningMessageEnd("reason-1"))
		must(s.ReasoningEnd("reason-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Think about this")
	return c
}

func reasoningMessages(c *testClient) []*agui.ReasoningMessage {
	var out []*agui.ReasoningMessage
	for _, m := range c.Store.Messages {
		if rm, ok := m.(*agui.ReasoningMessage); ok {
			out = append(out, rm)
		}
	}
	return out
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_FullReasoningFlow_MapsToUpdatesWithRawRepresentation
func TestCsReasoningIntegrationTest_PostRun_FullReasoningFlow_MapsToUpdatesWithRawRepresentation(t *testing.T) {
	c := fullReasoningFlow(t)
	res := c.runOK(t, client.RunInput{})
	assertTypes(t, res.Events, agui.EventTypeRunStarted, agui.EventTypeReasoningStart, agui.EventTypeReasoningMessageStart,
		agui.EventTypeReasoningMessageContent, agui.EventTypeReasoningMessageEnd, agui.EventTypeReasoningEnd, agui.EventTypeRunFinished)
	if rms := reasoningMessages(c); len(rms) != 1 || rms[0].Content != "Thinking step 1" {
		t.Fatalf("reasoning messages %+v", rms)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningStart_HasCorrectProperties
func TestCsReasoningIntegrationTest_PostRun_ReasoningStart_HasCorrectProperties(t *testing.T) {
	// Adapted: the C# stream opens span rs-1 and finishes the run with it
	// open; reasoning.mdx: a producer "MUST close every span it opens before
	// the run finishes", so the Go producer refuses that RUN_FINISHED.
	var unclosed error
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("rs-1"))
		unclosed = s.RunFinished(in.ThreadID, in.RunID)
		must(s.ReasoningEnd("rs-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Hi")
	res := c.runOK(t, client.RunInput{})
	if !refused(unclosed) {
		t.Fatalf("RUN_FINISHED with an open span was not refused: %v", unclosed)
	}
	if id := res.Events[1].(*agui.ReasoningStartEvent).MessageID; id != "rs-1" {
		t.Fatalf("messageId %q", id)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningMessageContent_HasTextReasoningContent
func TestCsReasoningIntegrationTest_PostRun_ReasoningMessageContent_HasTextReasoningContent(t *testing.T) {
	// Adapted: the C# stream sends REASONING_MESSAGE_CONTENT for a message
	// never started, which reasoning.mdx makes a violation ("nothing opened
	// has no referent"); the Go producer refuses it, so the content is sent
	// inside its message.
	var orphan error
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		orphan = s.ReasoningMessageContent("rmc-0", "orphan")
		must(s.ReasoningMessageStart("rmc-1"))
		must(s.ReasoningMessageContent("rmc-1", "Let me think..."))
		must(s.ReasoningMessageEnd("rmc-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Hi")
	res := c.runOK(t, client.RunInput{})
	if !refused(orphan) {
		t.Fatalf("orphan REASONING_MESSAGE_CONTENT was not refused: %v", orphan)
	}
	if d := single[*agui.ReasoningMessageContentEvent](t, res.Events).Delta; d != "Let me think..." {
		t.Fatalf("delta %q", d)
	}
	if rms := reasoningMessages(c); len(rms) != 1 || rms[0].Content != "Let me think..." {
		t.Fatalf("reasoning messages %+v", rms)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningEncryptedValue_HasTextReasoningContentWithProtectedData
func TestCsReasoningIntegrationTest_PostRun_ReasoningEncryptedValue_HasTextReasoningContentWithProtectedData(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningEncryptedValue(agui.EncryptedValueSubtypeToolCall, "entity-1", "enc-secret"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Hi")
	res := c.runOK(t, client.RunInput{})
	if len(res.Events) != 3 {
		t.Fatalf("events %v", types(res.Events))
	}
	e := res.Events[1].(*agui.ReasoningEncryptedValueEvent)
	if e.EncryptedValue != "enc-secret" || e.Subtype != agui.EncryptedValueSubtypeToolCall || e.EntityID != "entity-1" {
		t.Fatalf("event %+v", e)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningMessageChunk_MapsCorrectly
func TestCsReasoningIntegrationTest_PostRun_ReasoningMessageChunk_MapsCorrectly(t *testing.T) {
	agentFn := func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(agui.NewReasoningMessageChunkEvent("chunk-1", "compact")))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}
	// On the wire: the chunk itself.
	w := run(t, agentFn)
	if len(w.Events) != 3 {
		t.Fatalf("wire events %v", types(w.Events))
	}
	chunk := w.Events[1].(*agui.ReasoningMessageChunkEvent)
	if agui.Deref(chunk.MessageID) != "chunk-1" || agui.Deref(chunk.Delta) != "compact" {
		t.Fatalf("chunk %+v", chunk)
	}
	// The consumer expands it and builds the message.
	c := newClient(t, agent(t, agentFn))
	c.say("u1", "Hi")
	c.runOK(t, client.RunInput{})
	if rms := reasoningMessages(c); len(rms) != 1 || rms[0].ID != "chunk-1" || rms[0].Content != "compact" {
		t.Fatalf("reasoning messages %+v", rms)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningEvents_ShareResponseId
func TestCsReasoningIntegrationTest_PostRun_ReasoningEvents_ShareResponseId(t *testing.T) {
	assertOneRun(t, fullReasoningFlow(t).runOK(t, client.RunInput{}))
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ReasoningIntegrationTest.cs::ReasoningIntegrationTest.PostRun_ReasoningEvents_AllHaveAssistantRole
func TestCsReasoningIntegrationTest_PostRun_ReasoningEvents_AllHaveAssistantRole(t *testing.T) {
	// Adapted: MEAI surfaces reasoning on assistant-role updates; in 1.0 the
	// reasoning events build a message of role "reasoning".
	c := fullReasoningFlow(t)
	c.runOK(t, client.RunInput{})
	rms := reasoningMessages(c)
	if len(rms) != 1 || rms[0].MessageRole() != agui.RoleReasoning {
		t.Fatalf("reasoning messages %+v", rms)
	}
	if g := toGeneric(t, rms[0]).(map[string]any); g["role"] != "reasoning" {
		t.Fatalf("wire role %v", g["role"])
	}
}

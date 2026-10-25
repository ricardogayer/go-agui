package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs.
//
// The C# class pins AsAGUIEventStreamAsync, the converter from MEAI
// ChatResponseUpdate streams to AG-UI events. Go has no converter: an agent
// emits events itself. Each test is therefore a Go agent emitting the stream
// the C# converter is pinned to produce, served by pkg/server and checked on
// the wire; where the C# behaviour rests on a protocol rule (one RUN_STARTED,
// nothing after RUN_ERROR, a closed text message before RUN_FINISHED, ...) the
// Go test exercises that rule against the Go producer, which must refuse the
// violating event. Tests that pin only how MEAI content maps, or a mapper
// hook's dispatch, are N/A.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const naMapperHook = "N/A in Go: pins the dispatch of an AGUIStreamOptions mapper hook (MapInterrupt/MapContent) over MEAI AIContent types; a Go agent emits events directly and has no mapper pipeline"

var yieldRows = []struct {
	name  string
	yield bool
}{{"yieldThread=false", false}, {"yieldThread=true", true}}

// sanitizedRunError is what the C# converter emits when the provider stream
// throws: a fixed message and code, with the usage counted so far.
func sanitizedRunError() *agui.RunErrorEvent {
	e := agui.NewRunErrorEvent("An error occurred while streaming the agent response.")
	e.Code = agui.Ptr("StreamingError")
	e.Usage = []agui.TokenUsage{{Model: agui.Ptr("test-model"), InputTokens: agui.Ptr[int64](7)}}
	return e
}

// ---------------------------------------------------------------------------
// Run Lifecycle
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.EmptyStream_EmitsRunStartedAndRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_EmptyStream_EmitsRunStartedAndRunFinished(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeRunFinished)
	started := w.Events[0].(*agui.RunStartedEvent)
	if started.ThreadID != threadID || started.RunID != runID {
		t.Fatalf("RUN_STARTED identity %s/%s", started.ThreadID, started.RunID)
	}
	finished := w.Events[1].(*agui.RunFinishedEvent)
	if finished.ThreadID != threadID || finished.RunID != runID || !isSuccess(finished) {
		t.Fatalf("RUN_FINISHED %+v", finished)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InputStreamThrowsAfterUpdate_EmitsSanitizedRunErrorWithoutRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_InputStreamThrowsAfterUpdate_EmitsSanitizedRunErrorWithoutRunFinished(t *testing.T) {
	for _, row := range yieldRows {
		t.Run(row.name, func(t *testing.T) {
			var afterError error
			w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
				must(s.RunStarted(in.ThreadID, in.RunID))
				must(s.TextStart("msg-1"))
				must(s.TextContent("msg-1", "partial"))
				if row.yield {
					runtime.Gosched()
				}
				// The provider failed: the agent reports it without leaking
				// the provider's message, with the usage counted so far.
				must(s.Send(sanitizedRunError()))
				afterError = s.RunFinished(in.ThreadID, in.RunID)
			})
			assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeTextMessageStart,
				agui.EventTypeTextMessageContent, agui.EventTypeRunError)
			e := w.Events[3].(*agui.RunErrorEvent)
			if agui.Deref(e.Code) != "StreamingError" || e.Message != "An error occurred while streaming the agent response." {
				t.Fatalf("RUN_ERROR %+v", e)
			}
			if strings.Contains(strings.ToLower(e.Message), "sensitive") {
				t.Fatal("RUN_ERROR leaks provider details")
			}
			if len(e.Usage) != 1 || agui.Deref(e.Usage[0].Model) != "test-model" || agui.Deref(e.Usage[0].InputTokens) != 7 {
				t.Fatalf("usage %+v", e.Usage)
			}
			// lifecycle.mdx: "A producer MUST NOT emit anything after RUN_ERROR except RUN_STARTED."
			if !refused(afterError) {
				t.Fatalf("RUN_FINISHED after RUN_ERROR was not refused: %v", afterError)
			}
		})
	}
	// The handler's own guarantee: an agent that simply fails mid-message still
	// ends the stream with RUN_ERROR and never RUN_FINISHED. (The message is the
	// agent's error text: the specification does not require sanitising it, so
	// that part of the C# assertion is the agent's job, shown above.)
	t.Run("handler", func(t *testing.T) {
		w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			must(s.TextStart("msg-1"))
			must(s.TextContent("msg-1", "partial"))
			panic(abort{errors.New("sensitive provider failure details")})
		})
		assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeTextMessageStart,
			agui.EventTypeTextMessageContent, agui.EventTypeRunError)
	})
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ProviderOperationCanceledException_EmitsSanitizedRunError
func TestCsChatResponseUpdateAGUIExtensionsTest_ProviderOperationCanceledException_EmitsSanitizedRunError(t *testing.T) {
	for _, row := range yieldRows {
		t.Run(row.name, func(t *testing.T) {
			// A provider-side cancellation (its own timeout) is a failure of the
			// run, not a caller cancellation: the request is still live, so the
			// handler must report it.
			w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
				must(s.RunStarted(in.ThreadID, in.RunID))
				must(s.TextStart("msg-1"))
				must(s.TextContent("msg-1", "partial"))
				if row.yield {
					runtime.Gosched()
				}
				panic(abort{context.DeadlineExceeded})
			})
			if _, ok := last(t, w.Events).(*agui.RunErrorEvent); !ok {
				t.Fatalf("last event %s, want RUN_ERROR", last(t, w.Events).Type())
			}
			if len(ofType[*agui.RunFinishedEvent](w.Events)) != 0 {
				t.Fatal("RUN_FINISHED after a failed run")
			}
		})
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunStartedEvent_EmittedAutomatically_BeforeFirstContent
func TestCsChatResponseUpdateAGUIExtensionsTest_RunStartedEvent_EmittedAutomatically_BeforeFirstContent(t *testing.T) {
	// Go does not insert RUN_STARTED for the agent; the producer refuses
	// content ahead of it ("A stream MUST begin with RUN_STARTED or RUN_ERROR").
	var early error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		early = s.TextStart("msg-1")
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(early) {
		t.Fatalf("content before RUN_STARTED was not refused: %v", early)
	}
	if _, ok := w.Events[0].(*agui.RunStartedEvent); !ok {
		t.Fatalf("first event %s", w.Events[0].Type())
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunFinishedEvent_EmittedAutomatically_AfterLastContent
func TestCsChatResponseUpdateAGUIExtensionsTest_RunFinishedEvent_EmittedAutomatically_AfterLastContent(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if _, ok := last(t, w.Events).(*agui.RunFinishedEvent); !ok {
		t.Fatalf("last event %s", last(t, w.Events).Type())
	}
	// Go's handler does not synthesize success for an agent that forgets to
	// close its run (it cannot know the run succeeded): it closes the stream
	// with RUN_ERROR, so a terminal event is still guaranteed.
	w = run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
	})
	if e, ok := last(t, w.Events).(*agui.RunErrorEvent); !ok || agui.Deref(e.Code) != "AGENT_INCOMPLETE" {
		t.Fatalf("unclosed run must end with RUN_ERROR AGENT_INCOMPLETE, got %s", last(t, w.Events).Type())
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunFinishedEvent_NotDuplicated_WhenExplicitlyProvided
func TestCsChatResponseUpdateAGUIExtensionsTest_RunFinishedEvent_NotDuplicated_WhenExplicitlyProvided(t *testing.T) {
	var second error
	explicit := agui.NewRunFinishedEvent(threadID, runID)
	explicit.Metadata = agui.Metadata{"explicit": true}
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(explicit))
		second = s.RunFinished(in.ThreadID, in.RunID)
	})
	finished := single[*agui.RunFinishedEvent](t, w.Events)
	if finished.Metadata["explicit"] != true {
		t.Fatal("the explicit RUN_FINISHED is not the one on the wire")
	}
	if !refused(second) {
		t.Fatalf("a second RUN_FINISHED was not refused: %v", second)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunStartedEvent_NotDuplicated_WhenExplicitlyProvided
func TestCsChatResponseUpdateAGUIExtensionsTest_RunStartedEvent_NotDuplicated_WhenExplicitlyProvided(t *testing.T) {
	var second error
	explicit := agui.NewRunStartedEvent(threadID, runID)
	explicit.Metadata = agui.Metadata{"explicit": true}
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.Send(explicit))
		second = s.RunStarted(in.ThreadID, in.RunID)
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	started := single[*agui.RunStartedEvent](t, w.Events)
	if started.Metadata["explicit"] != true {
		t.Fatal("the explicit RUN_STARTED is not the one on the wire")
	}
	if !refused(second) {
		t.Fatalf("a second RUN_STARTED while the run is active was not refused: %v", second)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ExplicitRunStartedEvent_PassedThrough_Directly
func TestCsChatResponseUpdateAGUIExtensionsTest_ExplicitRunStartedEvent_PassedThrough_Directly(t *testing.T) {
	// Adapted: the C# test passes through a RUN_STARTED naming thread
	// "custom-thread" / run "custom-run" for an input naming thread-1 / run-1.
	// run-input.mdx: "Every run on the stream carries the input's threadId on
	// its boundary events" and "The requested run echoes this input's runId",
	// so the explicit event here carries the input's identity; what is pinned
	// is that the caller's own event (with parentRunId) reaches the wire as is.
	explicit := agui.NewRunStartedEvent(threadID, runID)
	explicit.ParentRunID = agui.Ptr("parent-run")
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.Send(explicit))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	started := w.Events[0].(*agui.RunStartedEvent)
	if started.ThreadID != threadID || started.RunID != runID || agui.Deref(started.ParentRunID) != "parent-run" {
		t.Fatalf("RUN_STARTED %+v", started)
	}

	// The C# ids themselves: a run on this stream naming a thread other than
	// the input's must not reach the wire (the producer should refuse it).
	foreign := agui.NewRunStartedEvent("custom-thread", "custom-run")
	foreign.ParentRunID = agui.Ptr("parent-run")
	w = run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		if s.Send(foreign) != nil {
			must(s.RunStarted(in.ThreadID, in.RunID))
			must(s.RunFinished(in.ThreadID, in.RunID))
			return
		}
		must(s.RunFinished("custom-thread", "custom-run"))
	})
	if got := w.Events[0].(*agui.RunStartedEvent).ThreadID; got != threadID {
		t.Errorf("RUN_STARTED carried threadId %q for an input naming %q (run-input.mdx: every run on the stream carries the input's threadId)", got, threadID)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.AutomaticRunStartedEvent_DeclaresProtocolVersion
func TestCsChatResponseUpdateAGUIExtensionsTest_AutomaticRunStartedEvent_DeclaresProtocolVersion(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if v := agui.Deref(w.Events[0].(*agui.RunStartedEvent).ProtocolVersion); v != agui.ProtocolVersion {
		t.Fatalf("protocolVersion %q, want %q", v, agui.ProtocolVersion)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.EmptyStream_RunStartedEvent_DeclaresProtocolVersion
func TestCsChatResponseUpdateAGUIExtensionsTest_EmptyStream_RunStartedEvent_DeclaresProtocolVersion(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if v := agui.Deref(w.Events[0].(*agui.RunStartedEvent).ProtocolVersion); v != agui.ProtocolVersion {
		t.Fatalf("protocolVersion %q", v)
	}
	if v, ok := w.Docs[0]["protocolVersion"]; !ok || v != "1.0" {
		t.Fatalf("wire RUN_STARTED: %v", w.Docs[0])
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ExplicitRunStartedEvent_ProtocolVersionLeftUntouched
func TestCsChatResponseUpdateAGUIExtensionsTest_ExplicitRunStartedEvent_ProtocolVersionLeftUntouched(t *testing.T) {
	// Adapted to the specification: the C# converter passes a caller's
	// RUN_STARTED through without a protocolVersion. versioning.mdx: "An
	// implementation of this version MUST send its declaration" on
	// RUN_STARTED.protocolVersion. So a caller-built RUN_STARTED without one
	// must not reach the wire undeclared: the producer either refuses it or
	// declares its version. What stays "untouched" is the caller's event value.
	explicit := &agui.RunStartedEvent{ThreadID: threadID, RunID: runID}
	var sendErr error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		sendErr = s.Send(explicit)
		if sendErr != nil {
			must(s.RunStarted(in.ThreadID, in.RunID))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if explicit.ProtocolVersion != nil {
		t.Fatal("the caller's event value was mutated")
	}
	if v, ok := w.Docs[0]["protocolVersion"]; !ok || v != agui.ProtocolVersion {
		t.Fatalf("RUN_STARTED reached the wire without the producer's protocolVersion declaration (Send error: %v): %v", sendErr, w.Docs[0])
	}
}

// ---------------------------------------------------------------------------
// Text Streaming
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.SingleTextUpdate_EmitsStart_Content_End
func TestCsChatResponseUpdateAGUIExtensionsTest_SingleTextUpdate_EmitsStart_Content_End(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "Hello"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(w.Events) != 5 {
		t.Fatalf("events %v", types(w.Events))
	}
	start := w.Events[1].(*agui.TextMessageStartEvent)
	if start.MessageID != "msg-1" || agui.Deref(start.Role) != agui.TextMessageRoleAssistant {
		t.Fatalf("start %+v", start)
	}
	content := w.Events[2].(*agui.TextMessageContentEvent)
	if content.MessageID != "msg-1" || content.Delta != "Hello" {
		t.Fatalf("content %+v", content)
	}
	if end := w.Events[3].(*agui.TextMessageEndEvent); end.MessageID != "msg-1" {
		t.Fatalf("end %+v", end)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.MultipleTextUpdates_SameMessageId_DoNotReStart
func TestCsChatResponseUpdateAGUIExtensionsTest_MultipleTextUpdates_SameMessageId_DoNotReStart(t *testing.T) {
	var restart error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "Hello "))
		// Restarting an open message is a protocol violation the producer refuses.
		restart = s.TextStart("msg-1")
		must(s.TextContent("msg-1", "World"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(w.Events) != 6 {
		t.Fatalf("events %v", types(w.Events))
	}
	if !refused(restart) {
		t.Fatalf("TEXT_MESSAGE_START for an open message was not refused: %v", restart)
	}
	single[*agui.TextMessageStartEvent](t, w.Events)
	contents := ofType[*agui.TextMessageContentEvent](w.Events)
	if len(contents) != 2 || contents[0].Delta != "Hello " || contents[1].Delta != "World" {
		t.Fatalf("contents %+v", contents)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.DifferentMessageIds_EmitsEndThenStartForNewMessage
func TestCsChatResponseUpdateAGUIExtensionsTest_DifferentMessageIds_EmitsEndThenStartForNewMessage(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		for _, m := range []struct{ id, text string }{{"msg-1", "First"}, {"msg-2", "Second"}} {
			must(s.TextStart(m.id))
			must(s.TextContent(m.id, m.text))
			must(s.TextEnd(m.id))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(w.Events) != 8 {
		t.Fatalf("events %v", types(w.Events))
	}
	if end := w.Events[3].(*agui.TextMessageEndEvent); end.MessageID != "msg-1" {
		t.Fatalf("events[3] %+v", end)
	}
	if start := w.Events[4].(*agui.TextMessageStartEvent); start.MessageID != "msg-2" {
		t.Fatalf("events[4] %+v", start)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.TextUpdate_WithNoMessageId_GeneratesMessageId
func TestCsChatResponseUpdateAGUIExtensionsTest_TextUpdate_WithNoMessageId_GeneratesMessageId(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	start := single[*agui.TextMessageStartEvent](t, w.Events)
	// Adapted: the "msg_" prefix is the .NET id generator's format; the
	// protocol only needs a non-empty id, which Go mints as a UUID.
	if start.MessageID == "" {
		t.Fatal("no message id was generated")
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.TextUpdate_EmptyTextContent_SkipsContentEvent
func TestCsChatResponseUpdateAGUIExtensionsTest_TextUpdate_EmptyTextContent_SkipsContentEvent(t *testing.T) {
	// The schema allows an empty delta; skipping it is the agent's choice. A
	// message opened and closed with no content is a valid stream.
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(ofType[*agui.TextMessageContentEvent](w.Events)) != 0 {
		t.Fatal("unexpected TEXT_MESSAGE_CONTENT")
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.AuthorName_IncludedInTextStart
func TestCsChatResponseUpdateAGUIExtensionsTest_AuthorName_IncludedInTextStart(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		start := agui.NewTextMessageStartEvent("msg-1")
		start.Name = agui.Ptr("agent-1")
		must(s.Send(start))
		must(s.TextContent("msg-1", "Hi"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if n := agui.Deref(single[*agui.TextMessageStartEvent](t, w.Events).Name); n != "agent-1" {
		t.Fatalf("name %q", n)
	}
}

// ---------------------------------------------------------------------------
// Reasoning
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ReasoningThenText_SharingProviderMessageId_EmitDistinctMessageIds
func TestCsChatResponseUpdateAGUIExtensionsTest_ReasoningThenText_SharingProviderMessageId_EmitDistinctMessageIds(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("msg-1-reasoning-span"))
		must(s.ReasoningMessageStart("msg-1-reasoning"))
		must(s.ReasoningMessageContent("msg-1-reasoning", "thinking"))
		must(s.ReasoningEncryptedValue(agui.EncryptedValueSubtypeMessage, "msg-1-reasoning", "signed-blob"))
		must(s.ReasoningMessageEnd("msg-1-reasoning"))
		must(s.ReasoningEnd("msg-1-reasoning-span"))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "visible answer"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	res := c.runOK(t, client.RunInput{})
	events := res.Events
	spanID := single[*agui.ReasoningStartEvent](t, events).MessageID
	reasoningID := single[*agui.ReasoningMessageStartEvent](t, events).MessageID
	textID := single[*agui.TextMessageStartEvent](t, events).MessageID
	if spanID == textID || reasoningID == textID {
		t.Fatal("reasoning and text share an id")
	}
	if single[*agui.ReasoningMessageContentEvent](t, events).MessageID != reasoningID ||
		single[*agui.ReasoningMessageEndEvent](t, events).MessageID != reasoningID ||
		single[*agui.ReasoningEncryptedValueEvent](t, events).EntityID != reasoningID {
		t.Fatal("reasoning events disagree on the reasoning message id")
	}
	// The consumer stores the artefact with the reasoning message it names.
	var found bool
	for _, m := range c.Store.Messages {
		if rm, ok := m.(*agui.ReasoningMessage); ok && rm.ID == reasoningID {
			found = agui.Deref(rm.EncryptedValue) == "signed-blob"
		}
	}
	if !found {
		t.Fatalf("encrypted value not stored on reasoning message %s: %+v", reasoningID, c.Store.Messages)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ReasoningDeltas_AcrossUpdates_ShareOneReasoningMessageId
func TestCsChatResponseUpdateAGUIExtensionsTest_ReasoningDeltas_AcrossUpdates_ShareOneReasoningMessageId(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("span-1"))
		must(s.ReasoningMessageStart("r-1"))
		must(s.ReasoningMessageContent("r-1", "first "))
		must(s.ReasoningMessageContent("r-1", "second"))
		must(s.ReasoningMessageEnd("r-1"))
		must(s.ReasoningEnd("span-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	id := single[*agui.ReasoningMessageStartEvent](t, w.Events).MessageID
	for _, e := range ofType[*agui.ReasoningMessageContentEvent](w.Events) {
		if e.MessageID != id {
			t.Fatalf("content for %s, want %s", e.MessageID, id)
		}
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ReasoningBlocks_SeparatedByText_GetDistinctReasoningMessageIds
func TestCsChatResponseUpdateAGUIExtensionsTest_ReasoningBlocks_SeparatedByText_GetDistinctReasoningMessageIds(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.Reasoning("first block")
		must(err)
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "interlude"))
		must(s.TextEnd("msg-1"))
		_, err = s.Reasoning("second block")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	ids := ofType[*agui.ReasoningMessageStartEvent](w.Events)
	if len(ids) != 2 || ids[0].MessageID == ids[1].MessageID {
		t.Fatalf("reasoning message starts %+v", ids)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ProtectedData_ArrivingWhileReasoningMessageOpen_BindsEntityIdToReasoningMessage
func TestCsChatResponseUpdateAGUIExtensionsTest_ProtectedData_ArrivingWhileReasoningMessageOpen_BindsEntityIdToReasoningMessage(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("span-1"))
		must(s.ReasoningMessageStart("r-1"))
		must(s.ReasoningMessageContent("r-1", "thinking"))
		must(s.ReasoningEncryptedValue(agui.EncryptedValueSubtypeMessage, "r-1", "signed-blob"))
		must(s.ReasoningMessageEnd("r-1"))
		must(s.ReasoningEnd("span-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	id := single[*agui.ReasoningMessageStartEvent](t, w.Events).MessageID
	if e := single[*agui.ReasoningEncryptedValueEvent](t, w.Events); e.EntityID != id {
		t.Fatalf("entityId %s, want %s", e.EntityID, id)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ProtectedData_ArrivingBeforeReasoningMessageOpens_BindsEntityIdToReasoningMessage
func TestCsChatResponseUpdateAGUIExtensionsTest_ProtectedData_ArrivingBeforeReasoningMessageOpens_BindsEntityIdToReasoningMessage(t *testing.T) {
	// The C# converter holds the artefact until the reasoning message opens;
	// a Go agent names the reasoning message it belongs to, and the consumer
	// stores it there — not on the text message that follows.
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ReasoningStart("span-1"))
		must(s.ReasoningMessageStart("r-1"))
		must(s.ReasoningEncryptedValue(agui.EncryptedValueSubtypeMessage, "r-1", "signed-blob"))
		must(s.ReasoningMessageContent("r-1", "thinking"))
		must(s.ReasoningMessageEnd("r-1"))
		must(s.ReasoningEnd("span-1"))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "hello"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	res := c.runOK(t, client.RunInput{})
	entity := single[*agui.ReasoningEncryptedValueEvent](t, res.Events).EntityID
	if entity != single[*agui.ReasoningMessageStartEvent](t, res.Events).MessageID {
		t.Fatal("entityId is not the reasoning message")
	}
	if entity == single[*agui.TextMessageStartEvent](t, res.Events).MessageID {
		t.Fatal("entityId is the text message")
	}
	for _, m := range c.Store.Messages {
		if am, ok := m.(*agui.AssistantMessage); ok && am.EncryptedValue != nil {
			t.Fatal("encrypted value landed on the assistant text message")
		}
	}
}

// ---------------------------------------------------------------------------
// Role Mapping
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.KnownRoles_MappedCorrectly
func TestCsChatResponseUpdateAGUIExtensionsTest_KnownRoles_MappedCorrectly(t *testing.T) {
	for _, role := range []string{"assistant", "user", "system", "tool"} {
		t.Run(role, func(t *testing.T) {
			var sendErr error
			w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
				must(s.RunStarted(in.ThreadID, in.RunID))
				start := agui.NewTextMessageStartEvent("msg-1")
				start.Role = agui.Ptr(agui.TextMessageRole(role))
				if sendErr = s.Send(start); sendErr == nil {
					must(s.TextContent("msg-1", "text"))
					must(s.TextEnd("msg-1"))
				}
				must(s.RunFinished(in.ThreadID, in.RunID))
			})
			if role == "tool" {
				// Adapted: the C# converter emits TEXT_MESSAGE_START with role
				// "tool". The 1.0 schema's TextMessageRole is
				// developer|system|assistant|user ("Excludes tool, which is
				// carried by TOOL_CALL_RESULT"), so a conformant producer must
				// refuse it — the C# row pins a schema violation.
				if !refused(sendErr) {
					t.Fatalf("TEXT_MESSAGE_START role tool was not refused: %v", sendErr)
				}
				return
			}
			if got := string(agui.Deref(single[*agui.TextMessageStartEvent](t, w.Events).Role)); got != role {
				t.Fatalf("role %q", got)
			}
		})
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.NullRole_DefaultsToAssistant
func TestCsChatResponseUpdateAGUIExtensionsTest_NullRole_DefaultsToAssistant(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "text"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if r := agui.Deref(single[*agui.TextMessageStartEvent](t, w.Events).Role); r != agui.TextMessageRoleAssistant {
		t.Fatalf("role %q", r)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.CustomRole_MappedToLowercase
func TestCsChatResponseUpdateAGUIExtensionsTest_CustomRole_MappedToLowercase(t *testing.T) {
	// Roles are case-sensitive enum values: "Developer" is not one, the
	// lowercase "developer" is.
	var upper error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		bad := agui.NewTextMessageStartEvent("msg-0")
		bad.Role = agui.Ptr(agui.TextMessageRole("Developer"))
		upper = s.Send(bad)
		start := agui.NewTextMessageStartEvent("msg-1")
		start.Role = agui.Ptr(agui.TextMessageRole(strings.ToLower("Developer")))
		must(s.Send(start))
		must(s.TextContent("msg-1", "text"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(upper) {
		t.Fatalf("role \"Developer\" was not refused: %v", upper)
	}
	if r := agui.Deref(single[*agui.TextMessageStartEvent](t, w.Events).Role); r != "developer" {
		t.Fatalf("role %q", r)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.DeveloperRole_MappedToAGUIRolesDeveloper
func TestCsChatResponseUpdateAGUIExtensionsTest_DeveloperRole_MappedToAGUIRolesDeveloper(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		start := agui.NewTextMessageStartEvent("msg-1")
		start.Role = agui.Ptr(agui.TextMessageRoleDeveloper)
		must(s.Send(start))
		must(s.TextContent("msg-1", "text"))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if r := agui.Deref(single[*agui.TextMessageStartEvent](t, w.Events).Role); r != agui.TextMessageRoleDeveloper {
		t.Fatalf("role %q", r)
	}
}

// ---------------------------------------------------------------------------
// Tool Calls
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionCallContent_EmitsToolCallStartArgsEnd
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionCallContent_EmitsToolCallStartArgsEnd(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "get_weather", `{"city":"Seattle"}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	start := single[*agui.ToolCallStartEvent](t, w.Events)
	if start.ToolCallID != "call-1" || start.ToolCallName != "get_weather" {
		t.Fatalf("start %+v", start)
	}
	args := single[*agui.ToolCallArgsEvent](t, w.Events)
	if args.ToolCallID != "call-1" || !strings.Contains(args.Delta, "Seattle") {
		t.Fatalf("args %+v", args)
	}
	if end := single[*agui.ToolCallEndEvent](t, w.Events); end.ToolCallID != "call-1" {
		t.Fatalf("end %+v", end)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionCallContent_SetsParentMessageId
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionCallContent_SetsParentMessageId(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallStartIn("parent-msg", "call-1", "my_tool"))
		must(s.ToolCallEnd("call-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	res := c.runOK(t, client.RunInput{})
	if p := agui.Deref(single[*agui.ToolCallStartEvent](t, res.Events).ParentMessageID); p != "parent-msg" {
		t.Fatalf("parentMessageId %q", p)
	}
	// The consumer attaches the call to the assistant message it names.
	var ok bool
	for _, m := range c.Store.Messages {
		if am, is := m.(*agui.AssistantMessage); is && am.ID == "parent-msg" && len(am.ToolCalls) == 1 && am.ToolCalls[0].ID == "call-1" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("call-1 not carried by assistant message parent-msg: %+v", c.Store.Messages)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.MultipleFunctionCalls_InSameUpdate_EmitsAllToolCallEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_MultipleFunctionCalls_InSameUpdate_EmitsAllToolCallEvents(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "tool_a", `{"x":1}`))
		must(s.ToolCall("call-2", "tool_b", `{"y":2}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	starts := ofType[*agui.ToolCallStartEvent](w.Events)
	if len(starts) != 2 || starts[0].ToolCallID != "call-1" || starts[1].ToolCallID != "call-2" {
		t.Fatalf("starts %+v", starts)
	}
}

// continuationInput is BuildContinuationContext: a frontend tool "confirm"
// whose call call-existing is already answered in the history.
const continuationInput = `{"threadId":"thread-1","runId":"run-1","protocolVersion":"1.0",
 "tools":[{"name":"confirm","description":"","parameters":{"type":"object"}}],
 "messages":[
  {"id":"a-1","role":"assistant","toolCalls":[{"id":"call-existing","type":"function","function":{"name":"confirm","arguments":"{}"}}]},
  {"id":"t-1","role":"tool","toolCallId":"call-existing","content":"{\"choice\":\"yes\"}"}]}`

// mixedContinuationInput is BuildMixedContinuationContext: call-server is a
// server tool call in the history whose result is not in the messages.
const mixedContinuationInput = `{"threadId":"thread-1","runId":"run-1","protocolVersion":"1.0",
 "tools":[{"name":"confirm","description":"","parameters":{"type":"object"}}],
 "messages":[
  {"id":"a-1","role":"assistant","toolCalls":[
    {"id":"call-existing","type":"function","function":{"name":"confirm","arguments":"{}"}},
    {"id":"call-server","type":"function","function":{"name":"server_tool","arguments":"{}"}}]},
  {"id":"t-1","role":"tool","toolCallId":"call-existing","content":"{\"choice\":\"yes\"}"}]}`

// answeredInHistory reports whether the input already holds the call and its answer.
func answeredInHistory(in *agui.RunAgentInput, callID string) bool {
	called, answered := false, false
	for _, m := range in.Messages {
		switch v := m.(type) {
		case *agui.AssistantMessage:
			for _, tc := range v.ToolCalls {
				called = called || tc.ID == callID
			}
		case *agui.ToolMessage:
			answered = answered || v.ToolCallID == callID
		}
	}
	return called && answered
}

// inHistory reports whether the input already holds the call.
func inHistory(in *agui.RunAgentInput, callID string) bool {
	for _, m := range in.Messages {
		if v, ok := m.(*agui.AssistantMessage); ok {
			for _, tc := range v.ToolCalls {
				if tc.ID == callID {
					return true
				}
			}
		}
	}
	return false
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_NewFunctionCall_EmitsToolCallEventsWithStableIdInOrder
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_NewFunctionCall_EmitsToolCallEventsWithStableIdInOrder(t *testing.T) {
	w := runWith(t, continuationInput, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		if !answeredInHistory(in, "call-existing") {
			t.Error("the continuation history did not reach the agent")
		}
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-new", "next_step", `{"value":42}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs,
		agui.EventTypeToolCallEnd, agui.EventTypeRunFinished)
	start := w.Events[1].(*agui.ToolCallStartEvent)
	if start.ToolCallID != "call-new" || start.ToolCallName != "next_step" {
		t.Fatalf("start %+v", start)
	}
	args := w.Events[2].(*agui.ToolCallArgsEvent)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args.Delta), &parsed); err != nil || parsed["value"] != float64(42) || args.ToolCallID != "call-new" {
		t.Fatalf("args %+v (%v)", args, err)
	}
	if w.Events[3].(*agui.ToolCallEndEvent).ToolCallID != "call-new" {
		t.Fatal("end id")
	}
}

// continuationLifecycleOnly is the shared body of the "replayed" continuation
// tests: the agent recognises what the input already holds and streams only
// the run boundaries, and the consumer is left with nothing pending.
func continuationLifecycleOnly(t *testing.T, input string, replayed ...string) {
	t.Helper()
	var calls int
	w := runWith(t, input, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		for _, id := range replayed {
			if inHistory(in, id) {
				continue // already in the history the consumer holds
			}
			calls++
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if calls != 0 {
		t.Fatalf("%d replayed calls were not found in the input history", calls)
	}
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeRunFinished)
	if f := w.Events[1].(*agui.RunFinishedEvent); !isSuccess(f) {
		t.Fatalf("outcome %+v", f.Outcome)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_ReplayedFunctionCallAndResult_EmitsOnlyLifecycleEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_ReplayedFunctionCallAndResult_EmitsOnlyLifecycleEvents(t *testing.T) {
	continuationLifecycleOnly(t, continuationInput, "call-existing")
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_ReplayedFunctionResultWithoutCall_EmitsOnlyLifecycleEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_ReplayedFunctionResultWithoutCall_EmitsOnlyLifecycleEvents(t *testing.T) {
	continuationLifecycleOnly(t, continuationInput)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_ReplayedMixedToolCallBatch_EmitsOnlyLifecycleEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_ReplayedMixedToolCallBatch_EmitsOnlyLifecycleEvents(t *testing.T) {
	continuationLifecycleOnly(t, mixedContinuationInput, "call-existing", "call-server")
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_EmptyResponse_EmitsOnlyLifecycleEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_EmptyResponse_EmitsOnlyLifecycleEvents(t *testing.T) {
	continuationLifecycleOnly(t, continuationInput)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.Continuation_MultipleNewFunctionCalls_EmitAllToolCallEventsInOrder
func TestCsChatResponseUpdateAGUIExtensionsTest_Continuation_MultipleNewFunctionCalls_EmitAllToolCallEventsInOrder(t *testing.T) {
	w := runWith(t, continuationInput, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-new-1", "next_step", `{}`))
		must(s.ToolCall("call-new-2", "final_step", `{}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	assertTypes(t, w.Events, agui.EventTypeRunStarted,
		agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs, agui.EventTypeToolCallEnd,
		agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs, agui.EventTypeToolCallEnd,
		agui.EventTypeRunFinished)
	want := []string{"call-new-1", "call-new-1", "call-new-1", "call-new-2", "call-new-2", "call-new-2"}
	for i, id := range want {
		var got string
		switch e := w.Events[i+1].(type) {
		case *agui.ToolCallStartEvent:
			got = e.ToolCallID
		case *agui.ToolCallArgsEvent:
			got = e.ToolCallID
		case *agui.ToolCallEndEvent:
			got = e.ToolCallID
		}
		if got != id {
			t.Fatalf("event %d is for %q, want %q", i+1, got, id)
		}
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionCallContent_ClosesOpenTextMessage
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionCallContent_ClosesOpenTextMessage(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "thinking..."))
		must(s.TextEnd("msg-1"))
		must(s.ToolCall("call-1", "my_tool", ""))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	endIdx, startIdx := indexOf(w.Events, agui.EventTypeTextMessageEnd), indexOf(w.Events, agui.EventTypeToolCallStart)
	if endIdx < 0 || endIdx > startIdx {
		t.Fatalf("TEXT_MESSAGE_END must precede TOOL_CALL_START: %v", types(w.Events))
	}
	if w.Events[endIdx].(*agui.TextMessageEndEvent).MessageID != "msg-1" {
		t.Fatal("wrong message closed")
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunFinished_HasSuccessOutcome_WhenToolCallsPresent
func TestCsChatResponseUpdateAGUIExtensionsTest_RunFinished_HasSuccessOutcome_WhenToolCallsPresent(t *testing.T) {
	// tool-calls.mdx: a run that stops on a frontend tool call "MUST use the
	// success outcome, or none, and MUST NOT report the run as interrupted".
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "my_tool", ""))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	res := c.runOK(t, client.RunInput{})
	if !isSuccess(single[*agui.RunFinishedEvent](t, res.Events)) || !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	if len(res.PendingToolCallIDs) != 1 || res.PendingToolCallIDs[0] != "call-1" {
		t.Fatalf("derived pending calls %v", res.PendingToolCallIDs)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RunFinished_HasSuccessOutcome_WhenNoToolCalls
func TestCsChatResponseUpdateAGUIExtensionsTest_RunFinished_HasSuccessOutcome_WhenNoToolCalls(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !isSuccess(single[*agui.RunFinishedEvent](t, w.Events)) {
		t.Fatal("not a success")
	}
}

// ---------------------------------------------------------------------------
// Tool Results
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionResultContent_EmitsToolCallResultEvent
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionResultContent_EmitsToolCallResultEvent(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallResult("call-1", "call-1", "result-data"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	r := single[*agui.ToolCallResultEvent](t, w.Events)
	if r.ToolCallID != "call-1" || r.Content.String() != "result-data" || r.Content.IsParts() {
		t.Fatalf("result %+v", r)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionResultContent_NullResult_EmitsEmptyString
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionResultContent_NullResult_EmitsEmptyString(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallResult("call-1", "call-1", ""))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	r := single[*agui.ToolCallResultEvent](t, w.Events)
	if r.Content.String() != "" {
		t.Fatalf("content %q", r.Content.String())
	}
	for _, d := range w.Docs {
		if d["type"] == "TOOL_CALL_RESULT" && d["content"] != "" {
			t.Fatalf("wire content %#v, want \"\"", d["content"])
		}
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionResultContent_JsonElementResult_EmitsRawText
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionResultContent_JsonElementResult_EmitsRawText(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallResult("call-1", "call-1", `{"status":"ok"}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	c := single[*agui.ToolCallResultEvent](t, w.Events).Content.String()
	if !strings.Contains(c, "status") || !strings.Contains(c, "ok") {
		t.Fatalf("content %q", c)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionResultContent_ObjectResult_SerializesToJson
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionResultContent_ObjectResult_SerializesToJson(t *testing.T) {
	result, _ := json.Marshal(map[string]any{"key": "value"})
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallResult("call-1", "call-1", string(result)))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	c := single[*agui.ToolCallResultEvent](t, w.Events).Content.String()
	if !strings.Contains(c, "key") || !strings.Contains(c, "value") {
		t.Fatalf("content %q", c)
	}
}

// ---------------------------------------------------------------------------
// Tool Approval (Interrupts)
// ---------------------------------------------------------------------------

// approvalInterrupt is what the C# converter builds for a ToolApprovalRequestContent.
func approvalInterrupt(requestID, callID, toolName string) agui.Interrupt {
	return agui.Interrupt{
		ID:         requestID,
		Reason:     "tool_call",
		ToolCallID: agui.Ptr(callID),
		Message:    agui.Ptr("Approval required for tool call: " + toolName),
		ResponseSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"approved": map[string]any{"type": "boolean"}},
			"required":   []any{"approved"},
		},
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolApprovalRequestContent_EmitsInterruptRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolApprovalRequestContent_EmitsInterruptRunFinished(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "delete_file", `{"path":"/tmp/file.txt"}`))
		must(s.RunInterrupted(in.ThreadID, in.RunID, approvalInterrupt("req-1", "call-1", "delete_file")))
	}))
	res := c.runOK(t, client.RunInput{})
	ev := res.Events
	start := single[*agui.ToolCallStartEvent](t, ev)
	args := single[*agui.ToolCallArgsEvent](t, ev)
	end := single[*agui.ToolCallEndEvent](t, ev)
	if start.ToolCallID != "call-1" || start.ToolCallName != "delete_file" || args.ToolCallID != "call-1" ||
		!strings.Contains(args.Delta, "path") || end.ToolCallID != "call-1" {
		t.Fatalf("tool call events %v", types(ev))
	}
	si, ai, ei, fi := indexOf(ev, agui.EventTypeToolCallStart), indexOf(ev, agui.EventTypeToolCallArgs),
		indexOf(ev, agui.EventTypeToolCallEnd), indexOf(ev, agui.EventTypeRunFinished)
	if !(si < ai && ai < ei && ei < fi) {
		t.Fatalf("order %v", types(ev))
	}
	out, ok := single[*agui.RunFinishedEvent](t, ev).Outcome.(*agui.InterruptOutcome)
	if !ok || len(out.Interrupts) != 1 {
		t.Fatalf("outcome %+v", single[*agui.RunFinishedEvent](t, ev).Outcome)
	}
	i := out.Interrupts[0]
	if i.ID != "req-1" || i.Reason != "tool_call" || agui.Deref(i.ToolCallID) != "call-1" ||
		agui.Deref(i.Message) != "Approval required for tool call: delete_file" {
		t.Fatalf("interrupt %+v", i)
	}
	if i.ResponseSchema["type"] != "object" {
		t.Fatalf("responseSchema %v", i.ResponseSchema)
	}
	if _, ok := i.ResponseSchema["properties"].(map[string]any)["approved"]; !ok {
		t.Fatalf("responseSchema %v", i.ResponseSchema)
	}
	if res.Status != "interrupted" || len(c.PendingInterrupts) != 1 {
		t.Fatalf("client status %s, pending %v", res.Status, c.PendingInterrupts)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolApprovalRequestContent_ClosesOpenTextMessage
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolApprovalRequestContent_ClosesOpenTextMessage(t *testing.T) {
	var early error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "I need to delete a file"))
		// RUN_FINISHED with the message still open is refused.
		early = s.RunInterrupted(in.ThreadID, in.RunID, approvalInterrupt("req-1", "call-1", "delete_file"))
		must(s.TextEnd("msg-1"))
		must(s.ToolCall("call-1", "delete_file", ""))
		must(s.RunInterrupted(in.ThreadID, in.RunID, approvalInterrupt("req-1", "call-1", "delete_file")))
	})
	if !refused(early) {
		t.Fatalf("RUN_FINISHED with an open text message was not refused: %v", early)
	}
	te, ts, f := indexOf(w.Events, agui.EventTypeTextMessageEnd), indexOf(w.Events, agui.EventTypeToolCallStart), indexOf(w.Events, agui.EventTypeRunFinished)
	if !(te < ts && ts < f) {
		t.Fatalf("order %v", types(w.Events))
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolApprovalRequestContent_PreventsAutoRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolApprovalRequestContent_PreventsAutoRunFinished(t *testing.T) {
	var second error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "delete_file", ""))
		must(s.RunInterrupted(in.ThreadID, in.RunID, approvalInterrupt("req-1", "call-1", "delete_file")))
		second = s.RunFinished(in.ThreadID, in.RunID)
	})
	f := single[*agui.RunFinishedEvent](t, w.Events)
	if _, ok := f.Outcome.(*agui.InterruptOutcome); !ok {
		t.Fatalf("outcome %+v", f.Outcome)
	}
	if !refused(second) {
		t.Fatalf("a second RUN_FINISHED was not refused: %v", second)
	}
}

// ---------------------------------------------------------------------------
// Raw Representation Pass-Through
// ---------------------------------------------------------------------------

// passThrough sends e between the run boundaries and returns the wire run.
func passThrough(t *testing.T, events ...agui.Event) *wireRun {
	t.Helper()
	return run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		for _, e := range events {
			must(s.Send(e))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
}

// sameJSON checks that the wire document is the event as the caller built it.
func sameJSON(t *testing.T, got map[string]any, want agui.Event) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(toGeneric(t, want))
	if string(g) != string(w) {
		t.Fatalf("wire document differs from the event sent\n got: %s\nwant: %s", g, w)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_BaseEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_BaseEvent_PassedThrough(t *testing.T) {
	custom := agui.NewCustomEvent("my.custom.event", map[string]any{"data": 42})
	w := passThrough(t, custom)
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeCustom, agui.EventTypeRunFinished)
	sameJSON(t, w.Docs[1], custom)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_RunStartedEvent_EmittedFirst_ThenOtherEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_RunStartedEvent_EmittedFirst_ThenOtherEvents(t *testing.T) {
	// The C# RUN_STARTED names thread "t" / run "r"; here it carries the
	// input's identity (run-input.mdx), which the boundary events must echo.
	started := agui.NewRunStartedEvent(threadID, runID)
	custom := agui.NewCustomEvent("x", nil)
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.Send(started))
		must(s.Send(custom))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	sameJSON(t, w.Docs[0], started)
	sameJSON(t, w.Docs[1], custom)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_NonRunStartedEvent_EmitsAutoRunStartedFirst
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_NonRunStartedEvent_EmitsAutoRunStartedFirst(t *testing.T) {
	// No automatic RUN_STARTED in Go: the producer refuses a CUSTOM first,
	// and the agent opens the run with the input's identity.
	var early error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		early = s.CustomEvent("x", nil)
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.CustomEvent("x", nil))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(early) {
		t.Fatalf("CUSTOM before RUN_STARTED was not refused: %v", early)
	}
	st, ok := w.Events[0].(*agui.RunStartedEvent)
	if !ok || st.ThreadID != threadID || st.RunID != runID {
		t.Fatalf("first event %+v", w.Events[0])
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_StepStartedEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_StepStartedEvent_PassedThrough(t *testing.T) {
	// Adapted: lifecycle.mdx "Every step a producer opens MUST be closed
	// before the run finishes" — the C# stream (STEP_STARTED then
	// RUN_FINISHED) is refused by a conformant producer, so the step is closed.
	step := agui.NewStepStartedEvent("step-1")
	var unclosed error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(step))
		unclosed = s.RunFinished(in.ThreadID, in.RunID)
		must(s.StepFinished("step-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(unclosed) {
		t.Fatalf("RUN_FINISHED with an open step was not refused: %v", unclosed)
	}
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeStepStarted, agui.EventTypeStepFinished, agui.EventTypeRunFinished)
	sameJSON(t, w.Docs[1], step)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_StepFinishedEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_StepFinishedEvent_PassedThrough(t *testing.T) {
	// Adapted: a STEP_FINISHED for a step never started is refused
	// (lifecycle.mdx, steps); the step is opened first.
	finished := agui.NewStepFinishedEvent("step-1")
	var orphan error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		orphan = s.Send(agui.NewStepFinishedEvent("never-started"))
		must(s.StepStarted("step-1"))
		must(s.Send(finished))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(orphan) {
		t.Fatalf("STEP_FINISHED without STEP_STARTED was not refused: %v", orphan)
	}
	sameJSON(t, w.Docs[2], finished)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_StateSnapshotEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_StateSnapshotEvent_PassedThrough(t *testing.T) {
	e := agui.NewStateSnapshotEvent(map[string]any{"counter": 1})
	w := passThrough(t, e)
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_StateDeltaEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_StateDeltaEvent_PassedThrough(t *testing.T) {
	e := agui.NewStateDeltaEvent(agui.Replace("/counter", 2))
	w := passThrough(t, e)
	sameJSON(t, w.Docs[1], e)
	assertJSONEqual(t, w.Docs[1]["delta"], generic(t, `[{"op":"replace","path":"/counter","value":2}]`))
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_RawEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_RawEvent_PassedThrough(t *testing.T) {
	e := agui.NewRawEvent(map[string]any{"data": 42})
	e.Source = agui.Ptr("test-source")
	w := passThrough(t, e)
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_MessagesSnapshotEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_MessagesSnapshotEvent_PassedThrough(t *testing.T) {
	e := agui.NewMessagesSnapshotEvent(&agui.UserMessage{ID: "msg1", Content: agui.PartsContent(agui.NewTextPart("Hello"))})
	w := passThrough(t, e)
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_ReasoningStartEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_ReasoningStartEvent_PassedThrough(t *testing.T) {
	// Adapted: the C# event has no messageId (required by the 1.0 schema) and
	// is never closed (reasoning.mdx: a producer "MUST close every span it
	// opens before the run finishes"); both are refused here.
	var unclosed error
	noID := agui.Validate("Event", []byte(`{"type":"REASONING_START"}`))
	e := agui.NewReasoningStartEvent("span-1")
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(e))
		unclosed = s.RunFinished(in.ThreadID, in.RunID)
		must(s.ReasoningEnd("span-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if noID == nil || !refused(unclosed) {
		t.Fatalf("refusals: no id %v, unclosed span %v", noID, unclosed)
	}
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_ActivitySnapshotEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_ActivitySnapshotEvent_PassedThrough(t *testing.T) {
	e := agui.NewActivitySnapshotEvent("act-1", "search", map[string]any{"title": "Searching"})
	w := passThrough(t, e)
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_RunErrorEvent_PassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_RunErrorEvent_PassedThrough(t *testing.T) {
	e := agui.NewRunErrorEvent("Something went wrong")
	e.Code = agui.Ptr("INTERNAL_ERROR")
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(e))
	})
	if len(w.Events) != 2 {
		t.Fatalf("events %v", types(w.Events))
	}
	got := w.Events[1].(*agui.RunErrorEvent)
	if got.Message != "Something went wrong" || agui.Deref(got.Code) != "INTERNAL_ERROR" {
		t.Fatalf("RUN_ERROR %+v", got)
	}
	sameJSON(t, w.Docs[1], e)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.RawRepresentation_MultipleEventTypes_AllPassedThrough
func TestCsChatResponseUpdateAGUIExtensionsTest_RawRepresentation_MultipleEventTypes_AllPassedThrough(t *testing.T) {
	// The C# REASONING_START/END carry no messageId; the schema requires one.
	evs := []agui.Event{
		agui.NewStepStartedEvent("step-1"),
		agui.NewStateSnapshotEvent(map[string]any{"count": 0}),
		agui.NewReasoningStartEvent("span-1"),
		agui.NewReasoningEndEvent("span-1"),
		agui.NewStepFinishedEvent("step-1"),
	}
	w := passThrough(t, evs...)
	if len(w.Events) != 7 {
		t.Fatalf("events %v", types(w.Events))
	}
	for i, e := range evs {
		sameJSON(t, w.Docs[i+1], e)
	}
	if _, ok := w.Events[6].(*agui.RunFinishedEvent); !ok {
		t.Fatal("last is not RUN_FINISHED")
	}
}

// ---------------------------------------------------------------------------
// Unmapped Update Handler
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UnmappedUpdateHandler_InvokedForUnknownContentTypes
func TestCsChatResponseUpdateAGUIExtensionsTest_UnmappedUpdateHandler_InvokedForUnknownContentTypes(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UnmappedUpdateHandler_NullReturn_SkipsContent
func TestCsChatResponseUpdateAGUIExtensionsTest_UnmappedUpdateHandler_NullReturn_SkipsContent(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UnmappedUpdateHandler_NotCalledForTextContent
func TestCsChatResponseUpdateAGUIExtensionsTest_UnmappedUpdateHandler_NotCalledForTextContent(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UnmappedUpdateHandler_RunFinishedEvent_FromHandler_PreventsAutoEmit
func TestCsChatResponseUpdateAGUIExtensionsTest_UnmappedUpdateHandler_RunFinishedEvent_FromHandler_PreventsAutoEmit(t *testing.T) {
	// The protocol half: an agent that closes its run itself gets exactly
	// that RUN_FINISHED — the handler adds none, and a second is refused.
	var second error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		e := agui.NewRunFinishedEvent(in.ThreadID, in.RunID)
		e.Outcome = &agui.SuccessOutcome{}
		must(s.Send(e))
		second = s.RunFinished(in.ThreadID, in.RunID)
	})
	f := single[*agui.RunFinishedEvent](t, w.Events)
	if _, ok := f.Outcome.(*agui.SuccessOutcome); !ok {
		t.Fatalf("outcome %+v", f.Outcome)
	}
	if !refused(second) {
		t.Fatal("second RUN_FINISHED not refused")
	}
}

// afterErrorStops is the shared body of the "RunErrorEvent stops the stream"
// tests: after RUN_ERROR nothing but RUN_STARTED may be sent (lifecycle.mdx),
// so the producer refuses the CUSTOM event that followed it in the mapper's
// output, and the handler adds no RUN_FINISHED.
func afterErrorStops(t *testing.T, before func(in *agui.RunAgentInput, s *stream.SSE, must func(error))) []agui.Event {
	t.Helper()
	var afterCustom, afterFinished error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		before(in, s, must)
		must(s.RunError("mapped error", nil))
		afterCustom = s.CustomEvent("after-error", true)
		afterFinished = s.RunFinished(in.ThreadID, in.RunID)
	})
	if !refused(afterCustom) || !refused(afterFinished) {
		t.Fatalf("events after RUN_ERROR were not refused: custom %v, finished %v", afterCustom, afterFinished)
	}
	if e, ok := last(t, w.Events).(*agui.RunErrorEvent); !ok || e.Message != "mapped error" {
		t.Fatalf("last event %+v", last(t, w.Events))
	}
	if len(ofType[*agui.RunFinishedEvent](w.Events)) != 0 || len(ofType[*agui.CustomEvent](w.Events)) != 0 {
		t.Fatalf("events %v", types(w.Events))
	}
	return w.Events
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ContentMapper_RunErrorEvent_StopsStream
func TestCsChatResponseUpdateAGUIExtensionsTest_ContentMapper_RunErrorEvent_StopsStream(t *testing.T) {
	var afterText error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunError("mapped error", nil))
		_ = s.CustomEvent("after-error", true)
		_, afterText = s.TextMessage("not emitted")
	})
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeRunError)
	if !refused(afterText) {
		t.Fatalf("text after RUN_ERROR was not refused: %v", afterText)
	}
	afterErrorStops(t, func(*agui.RunAgentInput, *stream.SSE, func(error)) {})
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.CallMapper_RunErrorEvent_StopsStream
func TestCsChatResponseUpdateAGUIExtensionsTest_CallMapper_RunErrorEvent_StopsStream(t *testing.T) {
	afterErrorStops(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.ToolCallStart("call-1", "mapped_tool"))
	})
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ResultMapper_RunErrorEvent_StopsStream
func TestCsChatResponseUpdateAGUIExtensionsTest_ResultMapper_RunErrorEvent_StopsStream(t *testing.T) {
	afterErrorStops(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.ToolCall("call-1", "mapped_tool", "{}"))
	})
}

// ---------------------------------------------------------------------------
// Interrupt Mapper
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_ReturnsInterrupt_EmitsRunFinishedWithInterrupt
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_ReturnsInterrupt_EmitsRunFinishedWithInterrupt(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-1", Reason: "user_input", Message: agui.Ptr("Enter email")}))
	})
	out, ok := single[*agui.RunFinishedEvent](t, w.Events).Outcome.(*agui.InterruptOutcome)
	if !ok || len(out.Interrupts) != 1 {
		t.Fatal("not a single interrupt")
	}
	i := out.Interrupts[0]
	if i.ID != "int-1" || i.Reason != "user_input" || agui.Deref(i.Message) != "Enter email" {
		t.Fatalf("interrupt %+v", i)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_ReturnsNull_FallsThroughToUnmappedHandler
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_ReturnsNull_FallsThroughToUnmappedHandler(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_ClosesOpenTextMessage_BeforeInterrupt
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_ClosesOpenTextMessage_BeforeInterrupt(t *testing.T) {
	var early error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "Processing..."))
		early = s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-1", Reason: "policy_hold"})
		must(s.TextEnd("msg-1"))
		must(s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-1", Reason: "policy_hold"}))
	})
	if !refused(early) {
		t.Fatalf("interrupt with an open text message was not refused: %v", early)
	}
	te, f := indexOf(w.Events, agui.EventTypeTextMessageEnd), indexOf(w.Events, agui.EventTypeRunFinished)
	if te < 0 || te > f || w.Events[te].(*agui.TextMessageEndEvent).MessageID != "msg-1" {
		t.Fatalf("order %v", types(w.Events))
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_PreventsAutoRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_PreventsAutoRunFinished(t *testing.T) {
	var second error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-1", Reason: "upload_required"}))
		second = s.RunFinished(in.ThreadID, in.RunID)
	})
	if _, ok := single[*agui.RunFinishedEvent](t, w.Events).Outcome.(*agui.InterruptOutcome); !ok {
		t.Fatal("not interrupted")
	}
	if !refused(second) {
		t.Fatal("second RUN_FINISHED not refused")
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_NotCalledForTextContent
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_NotCalledForTextContent(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_NotCalledForFunctionCallContent
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_NotCalledForFunctionCallContent(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_NotCalledForToolApprovalRequestContent
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_NotCalledForToolApprovalRequestContent(t *testing.T) {
	t.Skip(naMapperHook)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.InterruptMapper_CustomReason_PreservedInRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_InterruptMapper_CustomReason_PreservedInRunFinished(t *testing.T) {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{
			ID: "int-db-1", Reason: "database_modification", Message: agui.Ptr("DELETE on users affecting 42 rows"),
			Metadata: agui.Metadata{"action": "DELETE", "table": "users", "affectedRows": 42},
		}))
	}))
	res := c.runOK(t, client.RunInput{})
	if len(res.Interrupts) != 1 {
		t.Fatalf("interrupts %+v", res.Interrupts)
	}
	i := res.Interrupts[0]
	if i.ID != "int-db-1" || i.Reason != "database_modification" || i.Metadata["action"] != "DELETE" || i.Metadata["affectedRows"] != float64(42) {
		t.Fatalf("interrupt %+v", i)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.MultipleInterrupts_AreAccumulatedIntoSingleRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_MultipleInterrupts_AreAccumulatedIntoSingleRunFinished(t *testing.T) {
	var dup, second error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "delete_file", ""))
		// Interrupt ids must be unique within the run.
		dup = s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "x", Reason: "a"}, agui.Interrupt{ID: "x", Reason: "b"})
		must(s.RunInterrupted(in.ThreadID, in.RunID,
			approvalInterrupt("req-approval", "call-1", "delete_file"),
			agui.Interrupt{ID: "int-custom", Reason: "custom_reason"}))
		second = s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-3", Reason: "late"})
	})
	if !refused(dup) || !refused(second) {
		t.Fatalf("refusals: duplicate ids %v, second RUN_FINISHED %v", dup, second)
	}
	out, ok := single[*agui.RunFinishedEvent](t, w.Events).Outcome.(*agui.InterruptOutcome)
	if !ok || len(out.Interrupts) != 2 || out.Interrupts[0].ID != "req-approval" || out.Interrupts[1].ID != "int-custom" {
		t.Fatalf("outcome %+v", out)
	}
}

// ---------------------------------------------------------------------------
// RawEvent Attachment
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.NonRawRepresentation_Updates_HaveRawEventAttached
func TestCsChatResponseUpdateAGUIExtensionsTest_NonRawRepresentation_Updates_HaveRawEventAttached(t *testing.T) {
	// The provider-native update an event was translated from travels in
	// BaseEvent.rawEvent.
	native := map[string]any{"role": "assistant", "contents": []any{map[string]any{"$type": "text", "text": "Hello"}}}
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		start := agui.NewTextMessageStartEvent("msg-1")
		start.RawEvent = native
		content := agui.NewTextMessageContentEvent("msg-1", "Hello")
		content.RawEvent = native
		must(s.Send(start))
		must(s.Send(content))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if single[*agui.TextMessageStartEvent](t, w.Events).RawEvent == nil || single[*agui.TextMessageContentEvent](t, w.Events).RawEvent == nil {
		t.Fatal("rawEvent lost")
	}
	assertJSONEqual(t, w.Docs[1]["rawEvent"], toGeneric(t, native))
}

// ---------------------------------------------------------------------------
// Unicode and Special Characters
// ---------------------------------------------------------------------------

func textRun(t *testing.T, text string) *agui.TextMessageContentEvent {
	t.Helper()
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", text))
		must(s.TextEnd("msg-1"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	return single[*agui.TextMessageContentEvent](t, w.Events)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.TextContent_WithEmoji_PreservedInDelta
func TestCsChatResponseUpdateAGUIExtensionsTest_TextContent_WithEmoji_PreservedInDelta(t *testing.T) {
	if d := textRun(t, "Hello 🚀 World 😊").Delta; d != "Hello 🚀 World 😊" {
		t.Fatalf("delta %q", d)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.TextContent_WithUnicodeAndEscapeSequences_PreservedInDelta
func TestCsChatResponseUpdateAGUIExtensionsTest_TextContent_WithUnicodeAndEscapeSequences_PreservedInDelta(t *testing.T) {
	d := textRun(t, "Special: ñ € « » \n\t\"quoted\"").Delta
	for _, want := range []string{"ñ", "€", "\n", "\t", `"quoted"`} {
		if !strings.Contains(d, want) {
			t.Fatalf("delta %q lacks %q", d, want)
		}
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolCallArgs_WithUnicodeValues_SerializedCorrectly
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolCallArgs_WithUnicodeValues_SerializedCorrectly(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"city": "München", "emoji": "🌍"})
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "search", string(args)))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if d := single[*agui.ToolCallArgsEvent](t, w.Events).Delta; !strings.Contains(d, "München") {
		t.Fatalf("args %q", d)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.FunctionResultContent_WithUnicodeResult_PreservedInResult
func TestCsChatResponseUpdateAGUIExtensionsTest_FunctionResultContent_WithUnicodeResult_PreservedInResult(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallResult("call-1", "call-1", "Résultat: 42°C — succès ✓"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if c := single[*agui.ToolCallResultEvent](t, w.Events).Content.String(); c != "Résultat: 42°C — succès ✓" {
		t.Fatalf("content %q", c)
	}
}

// ---------------------------------------------------------------------------
// Mixed Content Scenarios
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.TextThenToolCall_ClosesTextBeforeToolEvents
func TestCsChatResponseUpdateAGUIExtensionsTest_TextThenToolCall_ClosesTextBeforeToolEvents(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.TextStart("msg-1"))
		must(s.TextContent("msg-1", "Let me check"))
		must(s.TextEnd("msg-1"))
		must(s.ToolCall("call-1", "lookup", ""))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	assertTypes(t, w.Events, agui.EventTypeRunStarted, agui.EventTypeTextMessageStart, agui.EventTypeTextMessageContent,
		agui.EventTypeTextMessageEnd, agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs, agui.EventTypeToolCallEnd,
		agui.EventTypeRunFinished)
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolCallArgsEvent_SerializesArgumentsAsJson
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolCallArgsEvent_SerializesArgumentsAsJson(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"name": "test", "count": 42})
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "my_tool", string(args)))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(single[*agui.ToolCallArgsEvent](t, w.Events).Delta), &parsed); err != nil || parsed["name"] != "test" {
		t.Fatalf("args %v (%v)", parsed, err)
	}
}

// ---------------------------------------------------------------------------
// Cancellation
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.CancellationToken_StopsEventStream
func TestCsChatResponseUpdateAGUIExtensionsTest_CancellationToken_StopsEventStream(t *testing.T) {
	// The consumer goes away after three events; the agent's context is
	// cancelled and its endless stream stops.
	stopped := make(chan error, 1)
	srv := httptest.NewServer(server.NewAgentHandler(server.RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		if err := s.RunStarted(in.ThreadID, in.RunID); err != nil {
			return err
		}
		if err := s.TextStart("msg-infinite"); err != nil {
			return err
		}
		for i := 0; ; i++ {
			if ctx.Err() != nil {
				stopped <- ctx.Err()
				return ctx.Err()
			}
			if err := s.TextContent("msg-infinite", "chunk"); err != nil {
				stopped <- err
				return err
			}
			time.Sleep(time.Millisecond)
		}
	}), server.Options{Logger: t.Logf}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(defaultInput))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r := sse.NewReader(resp.Body)
	var collected []agui.Event
	for len(collected) < 3 {
		data, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		e, err := agui.ParseEvent(data)
		if err != nil {
			t.Fatal(err)
		}
		collected = append(collected, e)
	}
	cancel()
	resp.Body.Close()
	if _, ok := collected[0].(*agui.RunStartedEvent); !ok {
		t.Fatalf("first event %s", collected[0].Type())
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent kept streaming after the consumer left")
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.OperationCanceledException_PropagatesWithoutRunError
func TestCsChatResponseUpdateAGUIExtensionsTest_OperationCanceledException_PropagatesWithoutRunError(t *testing.T) {
	for _, row := range yieldRows {
		t.Run(row.name, func(t *testing.T) {
			// A caller cancellation is not a run failure: once the request is
			// gone the handler reports nothing (no RUN_ERROR, no RUN_FINISHED).
			// The handler is driven directly so the cancelled request's
			// response can still be inspected.
			ctx, cancel := context.WithCancel(context.Background())
			h := server.NewAgentHandler(server.RunnerFunc(func(c context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
				if err := s.RunStarted(in.ThreadID, in.RunID); err != nil {
					return err
				}
				if err := s.TextStart("m"); err != nil {
					return err
				}
				if err := s.TextContent("m", "partial"); err != nil {
					return err
				}
				cancel() // the consumer cancels after the first update
				if row.yield {
					runtime.Gosched()
				}
				// The provider ignores cancellation and yields once more.
				_ = s.TextContent("m", "provider ignored cancellation")
				return c.Err()
			}), server.Options{Logger: t.Logf})
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(defaultInput)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			body := rec.Body.String()
			if !strings.Contains(body, `"RUN_STARTED"`) {
				t.Fatalf("no events before the cancellation: %s", body)
			}
			if strings.Contains(body, `"RUN_ERROR"`) || strings.Contains(body, `"RUN_FINISHED"`) {
				t.Fatalf("a cancelled request must not be reported as failed or finished:\n%s", body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tool Call Result
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.ToolCallResult_MessageId_EqualsToolCallId_ByDesign
func TestCsChatResponseUpdateAGUIExtensionsTest_ToolCallResult_MessageId_EqualsToolCallId_ByDesign(t *testing.T) {
	// A .NET design choice (the protocol only needs a message id); the Go
	// producer and consumer accept it, and the consumer keys the tool
	// message on it.
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCall("call-1", "lookup", "{}"))
		must(s.ToolCallResult("call-1", "call-1", "result-data"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	res := c.runOK(t, client.RunInput{})
	r := single[*agui.ToolCallResultEvent](t, res.Events)
	if r.ToolCallID != "call-1" || r.MessageID != r.ToolCallID {
		t.Fatalf("result %+v", r)
	}
	var tm *agui.ToolMessage
	for _, m := range c.Store.Messages {
		if v, ok := m.(*agui.ToolMessage); ok {
			tm = v
		}
	}
	if tm == nil || tm.ID != "call-1" || tm.ToolCallID != "call-1" {
		t.Fatalf("tool message %+v", tm)
	}
}

// ---------------------------------------------------------------------------
// Token Usage
// ---------------------------------------------------------------------------

func usageRun(t *testing.T, usage ...agui.TokenUsage) (*agui.RunFinishedEvent, map[string]any) {
	t.Helper()
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		_, err := s.TextMessage("Hello")
		must(err)
		f := agui.NewRunFinishedEvent(in.ThreadID, in.RunID)
		f.Usage = usage
		must(s.Send(f))
	})
	f, ok := last(t, w.Events).(*agui.RunFinishedEvent)
	if !ok {
		t.Fatal("last is not RUN_FINISHED")
	}
	return f, w.Docs[len(w.Docs)-1]
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UsageContent_SurfacedOnRunFinished
func TestCsChatResponseUpdateAGUIExtensionsTest_UsageContent_SurfacedOnRunFinished(t *testing.T) {
	f, doc := usageRun(t, agui.TokenUsage{
		Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](11), OutputTokens: agui.Ptr[int64](22),
		TotalTokens: agui.Ptr[int64](33), ReasoningTokens: agui.Ptr[int64](44), CachedInputTokens: agui.Ptr[int64](55),
		CacheWriteInputTokens: agui.Ptr[int64](66),
	})
	if len(f.Usage) != 1 {
		t.Fatalf("usage %+v", f.Usage)
	}
	assertJSONEqual(t, doc["usage"], generic(t, `[{"model":"gpt-4o","inputTokens":11,"outputTokens":22,"totalTokens":33,"reasoningTokens":44,"cachedInputTokens":55,"cacheWriteInputTokens":66}]`))
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.NoUsageContent_LeavesRunFinishedUsageNull
func TestCsChatResponseUpdateAGUIExtensionsTest_NoUsageContent_LeavesRunFinishedUsageNull(t *testing.T) {
	f, doc := usageRun(t)
	if f.Usage != nil {
		t.Fatalf("usage %+v", f.Usage)
	}
	if _, ok := doc["usage"]; ok {
		t.Fatalf("absent usage must be omitted, not null: %v", doc)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UsageContent_UnreportedCountsStayNull
func TestCsChatResponseUpdateAGUIExtensionsTest_UsageContent_UnreportedCountsStayNull(t *testing.T) {
	// lifecycle.mdx: "A producer MUST NOT emit a zero for a count it has no
	// data for" — unreported counts are absent on the wire.
	f, doc := usageRun(t, agui.TokenUsage{Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](7)})
	u := f.Usage[0]
	if agui.Deref(u.InputTokens) != 7 || u.OutputTokens != nil || u.TotalTokens != nil || u.ReasoningTokens != nil ||
		u.CachedInputTokens != nil || u.CacheWriteInputTokens != nil {
		t.Fatalf("usage %+v", u)
	}
	assertJSONEqual(t, doc["usage"], generic(t, `[{"model":"gpt-4o","inputTokens":7}]`))
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.MultipleUsageContents_AggregatedPerModel
func TestCsChatResponseUpdateAGUIExtensionsTest_MultipleUsageContents_AggregatedPerModel(t *testing.T) {
	// Aggregation is the agent's (here: summing two provider reports for one
	// model into one entry, as lifecycle.mdx's "one entry per provider and
	// model" requires).
	reports := []agui.TokenUsage{
		{Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](10), OutputTokens: agui.Ptr[int64](1)},
		{Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](5), OutputTokens: agui.Ptr[int64](2)},
	}
	sum := agui.TokenUsage{Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](0), OutputTokens: agui.Ptr[int64](0)}
	for _, r := range reports {
		*sum.InputTokens += *r.InputTokens
		*sum.OutputTokens += *r.OutputTokens
	}
	f, _ := usageRun(t, sum)
	if len(f.Usage) != 1 || agui.Deref(f.Usage[0].Model) != "gpt-4o" || agui.Deref(f.Usage[0].InputTokens) != 15 || agui.Deref(f.Usage[0].OutputTokens) != 3 {
		t.Fatalf("usage %+v", f.Usage)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.CallerSuppliedRunFinished_PassesThroughWithoutAccumulatedUsage
func TestCsChatResponseUpdateAGUIExtensionsTest_CallerSuppliedRunFinished_PassesThroughWithoutAccumulatedUsage(t *testing.T) {
	// The Go handler never rewrites an agent's events: a caller-supplied
	// RUN_FINISHED without usage reaches the wire without usage.
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.Send(agui.NewRunFinishedEvent(threadID, runID)))
	})
	if f := last(t, w.Events).(*agui.RunFinishedEvent); f.Usage != nil {
		t.Fatalf("usage %+v", f.Usage)
	}
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UsageContent_BlankModelId_OmitsModelLabel
func TestCsChatResponseUpdateAGUIExtensionsTest_UsageContent_BlankModelId_OmitsModelLabel(t *testing.T) {
	f, doc := usageRun(t, agui.TokenUsage{InputTokens: agui.Ptr[int64](7)})
	if f.Usage[0].Model != nil || agui.Deref(f.Usage[0].InputTokens) != 7 {
		t.Fatalf("usage %+v", f.Usage)
	}
	assertJSONEqual(t, doc["usage"], generic(t, `[{"inputTokens":7}]`))
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UsageProvider_LabelsUsageEntries
func TestCsChatResponseUpdateAGUIExtensionsTest_UsageProvider_LabelsUsageEntries(t *testing.T) {
	f, doc := usageRun(t, agui.TokenUsage{Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](10)})
	if agui.Deref(f.Usage[0].Provider) != "openai" || agui.Deref(f.Usage[0].Model) != "gpt-4o" {
		t.Fatalf("usage %+v", f.Usage)
	}
	assertJSONEqual(t, doc["usage"], generic(t, `[{"provider":"openai","model":"gpt-4o","inputTokens":10}]`))
}

// C#: tests/AGUI.Server.UnitTests/ChatResponseUpdateAGUIExtensionsTest.cs::ChatResponseUpdateAGUIExtensionsTest.UsageContents_FromDifferentModels_KeptSeparate
func TestCsChatResponseUpdateAGUIExtensionsTest_UsageContents_FromDifferentModels_KeptSeparate(t *testing.T) {
	f, _ := usageRun(t,
		agui.TokenUsage{Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr[int64](10)},
		agui.TokenUsage{Model: agui.Ptr("gpt-4o-mini"), InputTokens: agui.Ptr[int64](5)})
	if len(f.Usage) != 2 || agui.Deref(f.Usage[0].Model) != "gpt-4o" || agui.Deref(f.Usage[0].InputTokens) != 10 ||
		agui.Deref(f.Usage[1].Model) != "gpt-4o-mini" || agui.Deref(f.Usage[1].InputTokens) != 5 {
		t.Fatalf("usage %+v", f.Usage)
	}
}

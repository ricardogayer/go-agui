package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs.
//
// The C# tap turns provider argument fragments into incremental
// TOOL_CALL_ARGS. The protocol's own spelling for exactly that is
// TOOL_CALL_CHUNK: a Go agent forwards fragments as chunks and the chunk
// expansion (basic/patterns/streaming.mdx) opens, continues and closes the
// call. The tests check the chunks on the wire and their expanded triads.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// argsOf concatenates the expanded TOOL_CALL_ARGS deltas of one call.
func argsOf(events []agui.Event, callID string) string {
	var b strings.Builder
	for _, a := range ofType[*agui.ToolCallArgsEvent](events) {
		if a.ToolCallID == callID {
			b.WriteString(a.Delta)
		}
	}
	return b.String()
}

func countFor[T agui.Event](events []agui.Event, callID string) int {
	n := 0
	for _, e := range ofType[T](events) {
		switch v := any(e).(type) {
		case *agui.ToolCallStartEvent:
			if v.ToolCallID == callID {
				n++
			}
		case *agui.ToolCallEndEvent:
			if v.ToolCallID == callID {
				n++
			}
		}
	}
	return n
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.Continuation_FragmentsSuppressReplayedCallAndStreamNewCallAsync
func TestCsStreamingToolCallArgsTest_Continuation_FragmentsSuppressReplayedCallAndStreamNewCallAsync(t *testing.T) {
	type fragment struct{ id, name, delta string }
	fragments := []fragment{
		{"call_existing", "confirm", `{"choice":`}, {"", "", `"yes"}`},
		{"call_new", "next_step", `{"value":`}, {"", "", `42}`},
	}
	input := strings.ReplaceAll(continuationInput, "call-existing", "call_existing")
	w := runWith(t, input, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		replaying := false
		for _, f := range fragments {
			if f.id != "" {
				replaying = inHistory(in, f.id)
			}
			if replaying {
				continue // the consumer already holds this call
			}
			must(s.ToolCallChunk(f.id, f.name, f.delta))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	assertTypes(t, w.Expanded, agui.EventTypeRunStarted, agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs,
		agui.EventTypeToolCallArgs, agui.EventTypeToolCallEnd, agui.EventTypeRunFinished)
	if w.Expanded[1].(*agui.ToolCallStartEvent).ToolCallID != "call_new" ||
		w.Expanded[2].(*agui.ToolCallArgsEvent).Delta != `{"value":` || w.Expanded[3].(*agui.ToolCallArgsEvent).Delta != `42}` ||
		w.Expanded[4].(*agui.ToolCallEndEvent).ToolCallID != "call_new" {
		t.Fatalf("expanded %v", types(w.Expanded))
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.Fragments_StreamIncrementally_ThenCoalescedCallClosesAsync
func TestCsStreamingToolCallArgsTest_Fragments_StreamIncrementally_ThenCoalescedCallClosesAsync(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallChunk("call_a", "get_weather", `{"ci`))
		must(s.ToolCallChunk("", "", `ty":"Pa`))
		must(s.ToolCallChunk("", "", `ris"}`))
		// The coalesced call arrives: the call is complete. In chunk form the
		// END is synthesized by whatever ends the chunk stream (streaming.mdx).
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	start := single[*agui.ToolCallStartEvent](t, w.Expanded)
	if start.ToolCallID != "call_a" || start.ToolCallName != "get_weather" {
		t.Fatalf("start %+v", start)
	}
	if n := len(ofType[*agui.ToolCallArgsEvent](w.Expanded)); n != 3 {
		t.Fatalf("%d ARGS, want 3 incremental ones", n)
	}
	if got := argsOf(w.Expanded, "call_a"); got != `{"city":"Paris"}` {
		t.Fatalf("args %q", got)
	}
	if countFor[*agui.ToolCallEndEvent](w.Expanded, "call_a") != 1 {
		t.Fatal("not closed exactly once")
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.CallMapper_RunErrorEvent_StopsStreamAsync
func TestCsStreamingToolCallArgsTest_CallMapper_RunErrorEvent_StopsStreamAsync(t *testing.T) {
	var afterCustom error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallChunk("call_a", "get_weather", "{}"))
		must(s.RunError("mapped error", nil))
		afterCustom = s.CustomEvent("after-error", true)
	})
	if !refused(afterCustom) {
		t.Fatalf("CUSTOM after RUN_ERROR not refused: %v", afterCustom)
	}
	if e, ok := last(t, w.Events).(*agui.RunErrorEvent); !ok || e.Message != "mapped error" {
		t.Fatalf("last %s", last(t, w.Events).Type())
	}
	if len(ofType[*agui.RunFinishedEvent](w.Events)) != 0 || len(ofType[*agui.CustomEvent](w.Events)) != 0 {
		t.Fatalf("events %v", types(w.Events))
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.ParallelCalls_TrackedByIndex_EachBalancedAsync
func TestCsStreamingToolCallArgsTest_ParallelCalls_TrackedByIndex_EachBalancedAsync(t *testing.T) {
	// Interleaved calls cannot share one chunk stream (a chunk naming a new id
	// closes the open one), so concurrent calls use the explicit triad, which
	// allows several open calls at once.
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallStart("call_a", "a"))
		must(s.ToolCallArgs("call_a", `{"x":1}`))
		must(s.ToolCallStart("call_b", "b"))
		must(s.ToolCallArgs("call_b", `{"y":2}`))
		must(s.ToolCallEnd("call_a"))
		must(s.ToolCallEnd("call_b"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(ofType[*agui.ToolCallStartEvent](w.Expanded)) != 2 {
		t.Fatal("starts")
	}
	if argsOf(w.Expanded, "call_a") != `{"x":1}` || argsOf(w.Expanded, "call_b") != `{"y":2}` {
		t.Fatal("args misattributed")
	}
	if countFor[*agui.ToolCallEndEvent](w.Expanded, "call_a") != 1 || countFor[*agui.ToolCallEndEvent](w.Expanded, "call_b") != 1 {
		t.Fatal("ends")
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.IndexReusedByNewCall_AfterUncoalescedCall_ClosesStaleAndDoesNotMisattributeAsync
func TestCsStreamingToolCallArgsTest_IndexReusedByNewCall_AfterUncoalescedCall_ClosesStaleAndDoesNotMisattributeAsync(t *testing.T) {
	// streaming.mdx: "A chunk that names a different identifier opens a new
	// item, and the previous one closes first."
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallChunk("call_a", "render", `{"partial":`))
		must(s.ToolCallChunk("call_b", "render", `{"good":1}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if countFor[*agui.ToolCallStartEvent](w.Expanded, "call_a") != 1 || countFor[*agui.ToolCallEndEvent](w.Expanded, "call_a") != 1 ||
		countFor[*agui.ToolCallStartEvent](w.Expanded, "call_b") != 1 {
		t.Fatalf("expanded %v", types(w.Expanded))
	}
	if indexOf(w.Expanded, agui.EventTypeToolCallEnd) > 3 {
		t.Fatalf("call_a was not closed before call_b opened: %v", types(w.Expanded))
	}
	if got := argsOf(w.Expanded, "call_b"); got != `{"good":1}` || strings.Contains(got, "partial") {
		t.Fatalf("call_b args %q", got)
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.IndexReusedBySameId_AfterUncoalescedCall_ReopensCleanlyAsync
func TestCsStreamingToolCallArgsTest_IndexReusedBySameId_AfterUncoalescedCall_ReopensCleanlyAsync(t *testing.T) {
	// Adapted: in chunk form a chunk carrying the open call's id continues it
	// (streaming.mdx), so a retried call is closed and reopened with the
	// explicit triad. The producer refuses a second START while the call is
	// open; START/END alternate and each segment carries its own arguments.
	// (tool-calls.mdx: a reopened call's "further arguments append" on the
	// consumer — segmentation is visible only on the wire.)
	var doubleStart error
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallStart("call_a", "render"))
		must(s.ToolCallArgs("call_a", `{"partial":`))
		doubleStart = s.ToolCallStart("call_a", "render")
		must(s.ToolCallEnd("call_a"))
		must(s.ToolCallStart("call_a", "render"))
		must(s.ToolCallArgs("call_a", `{"good":1}`))
		must(s.ToolCallEnd("call_a"))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if !refused(doubleStart) {
		t.Fatalf("TOOL_CALL_START for an open call was not refused: %v", doubleStart)
	}
	open := false
	var segments []string
	for _, e := range w.Expanded {
		switch v := e.(type) {
		case *agui.ToolCallStartEvent:
			if open {
				t.Fatal("START while a call was open")
			}
			open = true
			segments = append(segments, "")
		case *agui.ToolCallArgsEvent:
			segments[len(segments)-1] += v.Delta
		case *agui.ToolCallEndEvent:
			open = false
		}
	}
	if open || len(segments) != 2 || segments[0] != `{"partial":` || segments[1] != `{"good":1}` {
		t.Fatalf("open=%v segments=%q", open, segments)
	}
}

// C#: tests/AGUI.Server.UnitTests/StreamingToolCallArgsTest.cs::StreamingToolCallArgsTest.UncoalescedCall_ClosedByEndOfStreamSweepAsync
func TestCsStreamingToolCallArgsTest_UncoalescedCall_ClosedByEndOfStreamSweepAsync(t *testing.T) {
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.ToolCallChunk("call_a", "render", `{"a":`))
		must(s.ToolCallChunk("", "", `1}`))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if countFor[*agui.ToolCallStartEvent](w.Expanded, "call_a") != 1 || countFor[*agui.ToolCallEndEvent](w.Expanded, "call_a") != 1 {
		t.Fatalf("expanded %v", types(w.Expanded))
	}
	// The END is synthesized before RUN_FINISHED.
	if i := indexOf(w.Expanded, agui.EventTypeToolCallEnd); i != len(w.Expanded)-2 {
		t.Fatalf("expanded %v", types(w.Expanded))
	}
}

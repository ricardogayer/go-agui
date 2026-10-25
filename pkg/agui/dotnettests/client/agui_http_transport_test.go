package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs.
//
// AGUIHttpTransport posts the input and yields the decoded events. The Go
// analogue is client.Agent.Run against an httptest agent; the events it
// delivers are in RunResult.Events.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

func transportAgent(url string) *client.Agent {
	a, _ := newAgent(url)
	a.ThreadID = "t1"
	return a
}

func textEvents(deltas ...string) []agui.Event {
	out := []agui.Event{
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)},
	}
	for _, d := range deltas {
		out = append(out, &agui.TextMessageContentEvent{MessageID: "m1", Delta: d})
	}
	return append(out,
		&agui.TextMessageEndEvent{MessageID: "m1"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"})
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_SuccessfulResponse_ParsesSSEEvents
func TestCsAGUIHttpTransportTest_SendAsync_SuccessfulResponse_ParsesSSEEvents(t *testing.T) {
	stub := newStub(t, static(textEvents("Hello")...))
	res, err := transportAgent(stub.URL).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"}
	if got := types(res.Events); !eqStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func rejectingServer(t *testing.T, status int) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", sse.ContentType)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_NonSuccessStatusCode_ThrowsHttpRequestException
//
// http-sse.mdx: input rejected before the run starts is an HTTP error status
// with no event stream; the run never started.
func TestCsAGUIHttpTransportTest_SendAsync_NonSuccessStatusCode_ThrowsHttpRequestException(t *testing.T) {
	_, err := transportAgent(rejectingServer(t, http.StatusInternalServerError)).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	var rej *client.RejectedError
	if !errors.As(err, &rej) || rej.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want a RejectedError with status 500, got %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_NotFound_ThrowsHttpRequestException
func TestCsAGUIHttpTransportTest_SendAsync_NotFound_ThrowsHttpRequestException(t *testing.T) {
	_, err := transportAgent(rejectingServer(t, http.StatusNotFound)).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	var rej *client.RejectedError
	if !errors.As(err, &rej) || rej.StatusCode != http.StatusNotFound {
		t.Fatalf("want a RejectedError with status 404, got %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_EmptyStream_CompletesSuccessfully
//
// The transport completes without error and yields nothing. The run itself,
// though, never reached a terminal event: transports/index.mdx says a
// truncated run has no outcome and a consumer MUST NOT report it as having
// succeeded — so the Go result is StatusTruncated, not a success.
func TestCsAGUIHttpTransportTest_SendAsync_EmptyStream_CompletesSuccessfully(t *testing.T) {
	stub := newStub(t, static())
	res, err := transportAgent(stub.URL).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatalf("an empty stream must not be a transport error: %v", err)
	}
	if len(res.Events) != 0 {
		t.Fatalf("delivered %v", types(res.Events))
	}
	if res.Status != client.StatusTruncated || res.Succeeded() {
		t.Fatalf("status %s: an empty stream is a truncated run, not a success", res.Status)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_CancellationRequested_ThrowsOperationCanceled
func TestCsAGUIHttpTransportTest_SendAsync_CancellationRequested_ThrowsOperationCanceled(t *testing.T) {
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := transportAgent(srv.URL).Run(ctx, client.RunInput{RunID: "r1"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if reached {
		t.Fatal("a cancelled run must not reach the agent")
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_CancellationDuringEnumeration_StopsConsumer
//
// The consumer stops after the content event. In Go the consumer cancels the
// run's context from its event callback; the agent holds the rest of the
// stream back until the request is gone, so nothing after the cancellation
// can arrive.
func TestCsAGUIHttpTransportTest_SendAsync_CancellationDuringEnumeration_StopsConsumer(t *testing.T) {
	events := textEvents("Hello")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse.SetHeaders(w.Header())
		sw := sse.NewWriter(w)
		for _, e := range events[:3] {
			b, _ := jsonMarshal(e)
			_ = sw.WriteData(b)
		}
		<-r.Context().Done()
		for _, e := range events[3:] {
			b, _ := jsonMarshal(e)
			_ = sw.WriteData(b)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var results []agui.Event
	_, err := transportAgent(srv.URL).Run(ctx, client.RunInput{RunID: "r1"}, func(e agui.Event) {
		results = append(results, e)
		if _, ok := e.(*agui.TextMessageContentEvent); ok {
			cancel() // the consumer decides to stop
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the run to end with context.Canceled, got %v", err)
	}
	want := []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT"}
	if got := types(results); !eqStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_MultipleEventTypes_DeserializesCorrectly
func TestCsAGUIHttpTransportTest_SendAsync_MultipleEventTypes_DeserializesCorrectly(t *testing.T) {
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.ToolCallStartEvent{ToolCallID: "tc1", ToolCallName: "get_weather"},
		&agui.ToolCallArgsEvent{ToolCallID: "tc1", Delta: `{"loc":"NYC"}`},
		&agui.ToolCallEndEvent{ToolCallID: "tc1"},
		&agui.RunErrorEvent{Message: "Something failed", Code: agui.Ptr("ERR01")},
	))
	res, err := transportAgent(stub.URL).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 5 {
		t.Fatalf("delivered %v", types(res.Events))
	}
	if s, ok := res.Events[0].(*agui.RunStartedEvent); !ok || s.ThreadID != "t1" {
		t.Errorf("events[0] = %#v", res.Events[0])
	}
	if s, ok := res.Events[1].(*agui.ToolCallStartEvent); !ok || s.ToolCallName != "get_weather" {
		t.Errorf("events[1] = %#v", res.Events[1])
	}
	if a, ok := res.Events[2].(*agui.ToolCallArgsEvent); !ok || a.Delta != `{"loc":"NYC"}` {
		t.Errorf("events[2] = %#v", res.Events[2])
	}
	if _, ok := res.Events[3].(*agui.ToolCallEndEvent); !ok {
		t.Errorf("events[3] = %#v", res.Events[3])
	}
	if e, ok := res.Events[4].(*agui.RunErrorEvent); !ok || e.Message != "Something failed" {
		t.Errorf("events[4] = %#v", res.Events[4])
	}
	// A RUN_ERROR is the run reporting its own failure, not a transport error.
	if res.Status != client.StatusFailed || res.Error == nil || agui.Deref(res.Error.Code) != "ERR01" {
		t.Errorf("status %s error %+v", res.Status, res.Error)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_MultipleEventsInSingleChunk_ParsesAll
func TestCsAGUIHttpTransportTest_SendAsync_MultipleEventsInSingleChunk_ParsesAll(t *testing.T) {
	body := buildSse(t, textEvents("A", "B", "C"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse.SetHeaders(w.Header())
		_, _ = w.Write([]byte(body)) // one write, one chunk
	}))
	defer srv.Close()
	res, err := transportAgent(srv.URL).Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 7 {
		t.Fatalf("delivered %v", types(res.Events))
	}
	for i, want := range []string{"A", "B", "C"} {
		c, ok := res.Events[2+i].(*agui.TextMessageContentEvent)
		if !ok || c.Delta != want {
			t.Errorf("events[%d] = %#v, want delta %q", 2+i, res.Events[2+i], want)
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIHttpTransportTest.cs::AGUIHttpTransportTest.SendAsync_StateAndActivityEvents_DeserializeCorrectly
func TestCsAGUIHttpTransportTest_SendAsync_StateAndActivityEvents_DeserializeCorrectly(t *testing.T) {
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.StateSnapshotEvent{Snapshot: map[string]any{"count": 42}},
		&agui.StateDeltaEvent{Delta: []agui.PatchOperation{agui.Replace("/count", 43)}},
		&agui.ActivitySnapshotEvent{MessageID: "a1", ActivityType: "PLAN", Content: map[string]any{"text": "Processing request"}},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"},
	))
	agent := transportAgent(stub.URL)
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 5 {
		t.Fatalf("delivered %v", types(res.Events))
	}
	snap, ok := res.Events[1].(*agui.StateSnapshotEvent)
	if !ok {
		t.Fatalf("events[1] is %T", res.Events[1])
	}
	g, _ := agui.ToGeneric(snap.Snapshot)
	if got := mustAt(t, g, "count"); jsonString(got) != "42" {
		t.Errorf("snapshot.count = %v", got)
	}
	delta, ok := res.Events[2].(*agui.StateDeltaEvent)
	if !ok {
		t.Fatalf("events[2] is %T", res.Events[2])
	}
	dg, _ := agui.ToGeneric(delta.Delta)
	if got := mustAt(t, dg, "0.op"); got != "replace" {
		t.Errorf("delta[0].op = %v", got)
	}
	act, ok := res.Events[3].(*agui.ActivitySnapshotEvent)
	if !ok || act.ActivityType != "PLAN" {
		t.Errorf("events[3] = %#v", res.Events[3])
	}
	// Beyond the .NET test (which has no state store): the delta applied.
	if got := mustAt(t, agent.Store.State, "count"); jsonString(got) != "43" {
		t.Errorf("state.count after the delta = %v", got)
	}
}

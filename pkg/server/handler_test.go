package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// collect posts body to h and returns the status and the events of the SSE
// response, each of which must validate against the 1.0 schema and together
// pass the stream verifier.
func collect(t *testing.T, h http.Handler, body string) (int, []agui.Event) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/agent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	if ct := rec.Header().Get("Content-Type"); ct != sse.ContentType {
		t.Fatalf("Content-Type = %q", ct)
	}
	if strings.Contains(rec.Body.String(), "\r") {
		t.Fatal("the SSE binding requires LF line endings")
	}
	var events []agui.Event
	var checker agui.StreamChecker
	r := sse.NewReader(rec.Body)
	for {
		data, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		e, err := agui.ParseEvent(data)
		if err != nil {
			t.Fatalf("emitted event is not valid AG-UI 1.0: %v\n%s", err, data)
		}
		if _, err := checker.Check(e); err != nil {
			t.Fatalf("emitted stream breaks the protocol: %v", err)
		}
		events = append(events, e)
	}
	if !checker.Verifier.RunClosed() {
		t.Fatal("stream ended without a terminal event")
	}
	return rec.Code, events
}

func types(events []agui.Event) string {
	var out []string
	for _, e := range events {
		out = append(out, string(e.Type()))
	}
	return strings.Join(out, ",")
}

const minimalInput = `{"threadId":"t1","runId":"r1","protocolVersion":"1.0","messages":[{"id":"u1","role":"user","content":"hi"}]}`

func TestEchoRunner(t *testing.T) {
	code, events := collect(t, NewAgentHandler(&EchoRunner{}), minimalInput)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if got := types(events); got != "RUN_STARTED,TEXT_MESSAGE_START,TEXT_MESSAGE_CONTENT,TEXT_MESSAGE_END,RUN_FINISHED" {
		t.Fatalf("events: %s", got)
	}
	rs := events[0].(*agui.RunStartedEvent)
	if agui.Deref(rs.ProtocolVersion) != agui.ProtocolVersion || rs.ThreadID != "t1" || rs.RunID != "r1" {
		t.Fatalf("RUN_STARTED must declare %s and echo identity: %+v", agui.ProtocolVersion, rs)
	}
	if c := events[2].(*agui.TextMessageContentEvent).Delta; c != "Echo: hi" {
		t.Fatalf("content %q", c)
	}
}

func TestRejectsMalformedInputBeforeTheStream(t *testing.T) {
	h := NewAgentHandler(&EchoRunner{}, Options{Logger: t.Logf})
	for _, body := range []string{
		`not json`,
		`{"threadId":"t","messages":[]}`,
		`{"threadId":"t","runId":"r","messages":[{"id":"1","role":"user","content":7}]}`,
		`{"threadId":"t","runId":"r","messages":[],"tools":[{"name":"x"}]}`,
	} {
		if code, _ := collect(t, h, body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/agent", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status %d", rec.Code)
	}
}

func TestToleratesNewerPeers(t *testing.T) {
	var logged []string
	h := NewAgentHandler(&EchoRunner{}, Options{Logger: func(f string, a ...any) { logged = append(logged, f) }})
	body := `{"threadId":"t","runId":"r","protocolVersion":"1.7","futureField":1,"messages":[{"id":"u","role":"user","content":"x","futureMessageField":true}]}`
	code, events := collect(t, h, body)
	if code != 200 || len(events) != 5 {
		t.Fatalf("a newer minor version must be served: %d %s", code, types(events))
	}
	if len(logged) < 3 {
		t.Fatalf("expected warnings for the version and both stripped fields, got %d", len(logged))
	}
}

func TestRunErrorOnAgentFailure(t *testing.T) {
	h := NewAgentHandler(RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		if err := s.RunStarted(in.ThreadID, in.RunID); err != nil {
			return err
		}
		if err := s.TextStart("m"); err != nil {
			return err
		}
		return errors.New("model unavailable")
	}), Options{Logger: t.Logf})
	_, events := collect(t, h, minimalInput)
	last := events[len(events)-1].(*agui.RunErrorEvent)
	if last.Message != "model unavailable" {
		t.Fatalf("RUN_ERROR message %q", last.Message)
	}
}

func TestRunErrorBeforeStart(t *testing.T) {
	h := NewAgentHandler(RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		return errors.New("cannot reach agent")
	}), Options{Logger: t.Logf})
	_, events := collect(t, h, minimalInput)
	if types(events) != "RUN_ERROR" {
		t.Fatalf("events: %s", types(events))
	}
}

func TestUnfinishedRunIsClosed(t *testing.T) {
	h := NewAgentHandler(RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		return s.RunStarted(in.ThreadID, in.RunID)
	}), Options{Logger: t.Logf})
	_, events := collect(t, h, minimalInput)
	if types(events) != "RUN_STARTED,RUN_ERROR" {
		t.Fatalf("events: %s", types(events))
	}
}

func TestPanicBecomesRunError(t *testing.T) {
	h := NewAgentHandler(RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		_ = s.RunStarted(in.ThreadID, in.RunID)
		panic("boom")
	}), Options{Logger: t.Logf})
	_, events := collect(t, h, minimalInput)
	if types(events) != "RUN_STARTED,RUN_ERROR" {
		t.Fatalf("events: %s", types(events))
	}
}

func TestStreamRefusesNonConformantEvents(t *testing.T) {
	var refusals []error
	h := NewAgentHandler(RunnerFunc(func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
		refusals = append(refusals, s.TextStart("early")) // before RUN_STARTED
		if err := s.RunStarted(in.ThreadID, in.RunID); err != nil {
			return err
		}
		refusals = append(refusals,
			s.TextContent("nope", "x"),                             // not open
			s.Send(&agui.CustomEvent{Value: 1}),                    // fine shape-wise? name "" is valid
			s.RunInterrupted(in.ThreadID, in.RunID),                // interrupt outcome needs interrupts
			s.ToolCallEnd("missing"),                               // not open
			s.Send(&agui.ToolCallChunkEvent{Delta: agui.Ptr("x")}), // first chunk without id
		)
		if err := s.StepStarted("s"); err != nil {
			return err
		}
		refusals = append(refusals, s.RunFinished(in.ThreadID, in.RunID)) // step still open
		if err := s.StepFinished("s"); err != nil {
			return err
		}
		return s.RunFinished(in.ThreadID, in.RunID)
	}), Options{Logger: t.Logf})
	_, events := collect(t, h, minimalInput)
	if got := types(events); got != "RUN_STARTED,CUSTOM,STEP_STARTED,STEP_FINISHED,RUN_FINISHED" {
		t.Fatalf("events: %s", got)
	}
	want := []bool{true, true, false, true, true, true, true}
	for i, err := range refusals {
		if (err != nil) != want[i] {
			t.Errorf("refusal %d: got %v, want refused=%v", i, err, want[i])
		}
	}
}

func TestCapabilitiesHandler(t *testing.T) {
	h := CapabilitiesHandler(agui.AgentCapabilities{Reasoning: &agui.ReasoningCapabilities{Supported: agui.Ptr(true)}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"reasoning":{"supported":true}}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

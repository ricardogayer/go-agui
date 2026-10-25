package client_test

// Shared plumbing for the ports of the .NET SDK's AGUI.Client and
// AGUI.Formatting unit tests. The .NET tests drive an IAGUITransport stub or
// a stubbed HttpMessageHandler; the Go analogue is a real HTTP agent
// (httptest) speaking the SSE binding, consumed by client.Agent, or — for the
// TextMessageBuilder/ToolCallBuilder tests — the client.Pipeline feeding a
// client.Store directly.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// stubAgent is an HTTP + SSE agent that records every request and answers
// with the events respond returns.
type stubAgent struct {
	*httptest.Server
	mu       sync.Mutex
	bodies   [][]byte
	requests []*http.Request
}

func newStub(t *testing.T, respond func(in *agui.RunAgentInput) []agui.Event) *stubAgent {
	t.Helper()
	s := &stubAgent{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, buf.Bytes())
		s.requests = append(s.requests, r)
		s.mu.Unlock()
		var in agui.RunAgentInput
		if err := json.Unmarshal(buf.Bytes(), &in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sse.SetHeaders(w.Header())
		sw := sse.NewWriter(w)
		for _, e := range respond(&in) {
			b, err := json.Marshal(e)
			if err != nil {
				t.Errorf("marshal %s: %v", e.Type(), err)
				return
			}
			_ = sw.WriteData(b)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// echo answers like a stateless AG-UI server: RUN_STARTED with the request's
// ids, the middle events, RUN_FINISHED.
func echo(middle ...agui.Event) func(*agui.RunAgentInput) []agui.Event {
	return func(in *agui.RunAgentInput) []agui.Event {
		out := []agui.Event{&agui.RunStartedEvent{ThreadID: in.ThreadID, RunID: in.RunID}}
		out = append(out, middle...)
		return append(out, &agui.RunFinishedEvent{ThreadID: in.ThreadID, RunID: in.RunID})
	}
}

// static answers with exactly these events, whatever the request.
func static(events ...agui.Event) func(*agui.RunAgentInput) []agui.Event {
	return func(*agui.RunAgentInput) []agui.Event { return events }
}

func (s *stubAgent) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

// lastBody returns the last request body in generic JSON form.
func (s *stubAgent) lastBody(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.bodies) == 0 {
		t.Fatal("the agent received no request")
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(s.bodies[len(s.bodies)-1]))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("request body is not a JSON object: %v", err)
	}
	return m
}

// lastInput returns the last request body decoded as a RunAgentInput.
func (s *stubAgent) lastInput(t *testing.T) *agui.RunAgentInput {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.bodies) == 0 {
		t.Fatal("the agent received no request")
	}
	var in agui.RunAgentInput
	if err := json.Unmarshal(s.bodies[len(s.bodies)-1], &in); err != nil {
		t.Fatal(err)
	}
	return &in
}

func (s *stubAgent) lastRequest(t *testing.T) *http.Request {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("the agent received no request")
	}
	return s.requests[len(s.requests)-1]
}

// newAgent returns a client on url collecting its warnings.
func newAgent(url string) (*client.Agent, *[]string) {
	a := client.New(url)
	var warnings []string
	a.Warn = func(w agui.Warning) { warnings = append(warnings, w.String()) }
	return a, &warnings
}

func user(id, text string) *agui.UserMessage {
	return &agui.UserMessage{ID: id, Content: agui.TextContent(text)}
}

// feed is the consumer pipeline on its own, the analogue of the .NET
// TextMessageBuilder/ToolCallBuilder pair: verified events go in, the Store
// assembles messages and tool calls.
type feed struct {
	p         *client.Pipeline
	store     *client.Store
	warnings  []string
	delivered []agui.Event
}

func newFeed() *feed {
	f := &feed{store: client.NewStore(nil, nil)}
	f.p = &client.Pipeline{Store: f.store, Warn: func(w agui.Warning) { f.warnings = append(f.warnings, w.String()) }}
	return f
}

// push sends one event through the pipeline.
func (f *feed) push(e agui.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return f.pushRaw(b)
}

func (f *feed) pushRaw(b []byte) error {
	out, err := f.p.Push(b)
	f.delivered = append(f.delivered, out...)
	return err
}

func (f *feed) mustPush(t *testing.T, events ...agui.Event) {
	t.Helper()
	for _, e := range events {
		if err := f.push(e); err != nil {
			t.Fatalf("%s rejected: %v", e.Type(), err)
		}
	}
}

// started returns a feed with a run open on thread/run.
func started(t *testing.T, thread, run string) *feed {
	t.Helper()
	f := newFeed()
	f.mustPush(t, &agui.RunStartedEvent{ThreadID: thread, RunID: run})
	return f
}

func (f *feed) message(t *testing.T, id string) agui.Message {
	t.Helper()
	for _, m := range f.store.Messages {
		if m.MessageID() == id {
			return m
		}
	}
	t.Fatalf("no message %q in the store", id)
	return nil
}

// toolCall finds a tool call in the store, with the message carrying it.
func (f *feed) toolCall(t *testing.T, id string) (*agui.AssistantMessage, agui.ToolCall) {
	t.Helper()
	for _, m := range f.store.Messages {
		if a, ok := m.(*agui.AssistantMessage); ok {
			for _, tc := range a.ToolCalls {
				if tc.ID == id {
					return a, tc
				}
			}
		}
	}
	t.Fatalf("no tool call %q in the store", id)
	return nil, agui.ToolCall{}
}

// toolCalls returns every tool call in the store, in store order.
func (f *feed) toolCalls() []agui.ToolCall {
	var out []agui.ToolCall
	for _, m := range f.store.Messages {
		if a, ok := m.(*agui.AssistantMessage); ok {
			out = append(out, a.ToolCalls...)
		}
	}
	return out
}

func roleOf(m agui.Message) agui.Role { return m.MessageRole() }

// textOf returns the text of a text-bearing message.
func textOf(m agui.Message) string {
	switch t := m.(type) {
	case *agui.AssistantMessage:
		return agui.Deref(t.Content)
	case *agui.UserMessage:
		return t.Content.String()
	case *agui.SystemMessage:
		return t.Content
	case *agui.DeveloperMessage:
		return t.Content
	case *agui.ReasoningMessage:
		return t.Content
	case *agui.ToolMessage:
		return t.Content.String()
	}
	return ""
}

func nameOf(m agui.Message) *string {
	switch t := m.(type) {
	case *agui.AssistantMessage:
		return t.Name
	case *agui.UserMessage:
		return t.Name
	case *agui.SystemMessage:
		return t.Name
	case *agui.DeveloperMessage:
		return t.Name
	}
	return nil
}

// types lists event types.
func types(events []agui.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, string(e.Type()))
	}
	return out
}

func eqStrings(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// at reads a dotted path ("messages.0.content.0.metadata.detail") from a
// generic JSON value.
func at(v any, path string) (any, bool) {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch t := cur.(type) {
		case map[string]any:
			x, ok := t[seg]
			if !ok {
				return nil, false
			}
			cur = x
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			cur = t[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func mustAt(t *testing.T, v any, path string) any {
	t.Helper()
	x, ok := at(v, path)
	if !ok {
		b, _ := json.Marshal(v)
		t.Fatalf("%s is absent from %s", path, b)
	}
	return x
}

// parseArgs decodes a tool call's argument text as a JSON object.
func parseArgs(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("arguments %q are not a JSON object: %v", s, err)
	}
	return m
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// jsonString renders a generic JSON value compactly ("42", "\"x\"", ...).
func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

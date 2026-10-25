package server_test

// Shared harness for the port of the .NET SDK's server tests
// (sdks/dotnet/tests/AGUI.Server.UnitTests and
// AGUI.Hosting.AspNetCore.IntegrationTests).
//
// The C# server turns Microsoft.Extensions.AI ChatResponseUpdate streams into
// AG-UI events. Go has no MEAI: an agent emits events itself through
// pkg/stream (which validates and verifies every event) behind pkg/server (the
// HTTP handler). So each C# scenario becomes a Go agent emitting the stream
// the C# converter is pinned to produce, served over real HTTP by
// server.NewAgentHandler, and checked on the wire (every event validated
// against the 1.0 schema and the whole stream run through the verifier) and,
// where the C# test drives AGUIChatClient, through pkg/client.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const (
	threadID = "thread-1"
	runID    = "run-1"
)

// defaultInput is the C# tests' `new RunAgentInput { ThreadId = ThreadId, RunId = RunId }`.
const defaultInput = `{"threadId":"thread-1","runId":"run-1","protocolVersion":"1.0","messages":[]}`

// abort carries an agent step's error out of the agent body.
type abort struct{ err error }

// agentFn is an agent body: must reports an unexpected refusal of an event as
// a test error and stops the agent.
type agentFn func(in *agui.RunAgentInput, s *stream.SSE, must func(error))

// agent adapts an agent body to a server.RunnerFunc.
func agent(t *testing.T, fn agentFn) server.RunnerFunc {
	return func(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) (err error) {
		defer func() {
			if p := recover(); p != nil {
				if a, ok := p.(abort); ok {
					err = a.err
					return
				}
				panic(p)
			}
		}()
		fn(in, s, func(e error) {
			if e != nil {
				t.Errorf("agent: event refused: %v", e)
				panic(abort{e})
			}
		})
		return nil
	}
}

// wireRun is what one POST produced.
type wireRun struct {
	Status   int
	Header   http.Header
	Body     string
	Docs     []map[string]any // each SSE data payload, as generic JSON
	Events   []agui.Event     // the events as sent (chunks unexpanded)
	Expanded []agui.Event     // chunk events expanded into their triads
	Logs     []string         // handler warnings
}

type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) log(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// post serves runner and POSTs body to it over real HTTP. A 200 response
// must be an SSE stream of schema-valid events that passes the stream
// verifier and ends with a terminal event.
func post(t *testing.T, runner server.AgentRunner, body string, header ...http.Header) *wireRun {
	t.Helper()
	logs := &logSink{}
	srv := httptest.NewServer(server.NewAgentHandler(runner, server.Options{Logger: logs.log}))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", sse.ContentType)
	for _, h := range header {
		for k, v := range h {
			req.Header[k] = v
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	w := &wireRun{Status: resp.StatusCode, Header: resp.Header, Body: string(raw)}
	logs.mu.Lock()
	w.Logs = append(w.Logs, logs.lines...)
	logs.mu.Unlock()
	if resp.StatusCode != http.StatusOK {
		return w
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, sse.ContentType) {
		t.Fatalf("Content-Type = %q, want %s", ct, sse.ContentType)
	}
	if strings.Contains(w.Body, "\r") {
		t.Fatal("the SSE binding requires LF line endings")
	}
	var checker agui.StreamChecker
	r := sse.NewReader(strings.NewReader(w.Body))
	for {
		data, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("SSE data is not one JSON object: %v\n%s", err, data)
		}
		e, err := agui.ParseEvent(data)
		if err != nil {
			t.Fatalf("emitted event is not valid AG-UI 1.0: %v\n%s", err, data)
		}
		expanded, err := checker.Check(e)
		if err != nil {
			t.Fatalf("emitted stream breaks the protocol: %v\n%s", err, data)
		}
		w.Docs = append(w.Docs, doc)
		w.Events = append(w.Events, e)
		w.Expanded = append(w.Expanded, expanded...)
	}
	if !checker.Verifier.RunClosed() {
		t.Fatal("stream ended without a terminal event")
	}
	return w
}

// run serves an agent body and POSTs the default input to it.
func run(t *testing.T, fn agentFn) *wireRun {
	t.Helper()
	return post(t, agent(t, fn), defaultInput)
}

// runWith serves an agent body and POSTs body to it.
func runWith(t *testing.T, body string, fn agentFn) *wireRun {
	t.Helper()
	return post(t, agent(t, fn), body)
}

// types lists the event types of events.
func types(events []agui.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = string(e.Type())
	}
	return out
}

// assertTypes checks the exact event type sequence.
func assertTypes(t *testing.T, events []agui.Event, want ...agui.EventType) {
	t.Helper()
	got := types(events)
	ws := make([]string, len(want))
	for i, w := range want {
		ws[i] = string(w)
	}
	if !reflect.DeepEqual(got, ws) {
		t.Fatalf("event types\n got: %v\nwant: %v", got, ws)
	}
}

// ofType returns the events of type T, in order.
func ofType[T agui.Event](events []agui.Event) []T {
	var out []T
	for _, e := range events {
		if v, ok := e.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

// single returns the only event of type T.
func single[T agui.Event](t *testing.T, events []agui.Event) T {
	t.Helper()
	got := ofType[T](events)
	if len(got) != 1 {
		var zero T
		t.Fatalf("want exactly one %T, got %d in %v", zero, len(got), types(events))
	}
	return got[0]
}

// indexOf returns the index of the first event of type et, or -1.
func indexOf(events []agui.Event, et agui.EventType) int {
	for i, e := range events {
		if e.Type() == et {
			return i
		}
	}
	return -1
}

// last returns the final event.
func last(t *testing.T, events []agui.Event) agui.Event {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	return events[len(events)-1]
}

// isSuccess reports whether RUN_FINISHED reports success. The C# converter
// always writes {"type":"success"}; in 1.0 an absent outcome also means
// success, and that is what sse.RunFinished sends.
func isSuccess(f *agui.RunFinishedEvent) bool {
	if f.Outcome == nil {
		return true
	}
	_, ok := f.Outcome.(*agui.SuccessOutcome)
	return ok
}

// generic decodes a JSON literal.
func generic(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

// toGeneric marshals v and decodes it back to generic JSON.
func toGeneric(t *testing.T, v any) any {
	t.Helper()
	g, err := agui.ToGeneric(v)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", g, w)
	}
}

// refused reports whether err is the producer refusing an event (a schema
// or a sequencing violation).
func refused(err error) bool {
	var ve *agui.ValidationError
	var pe *agui.ProtocolError
	return err != nil && (errors.As(err, &ve) || errors.As(err, &pe) || strings.Contains(err.Error(), "refusing to emit"))
}

// ---------------------------------------------------------------------------
// Client side (the Go counterpart of AGUIChatClient over the test server).
// ---------------------------------------------------------------------------

type testClient struct {
	*client.Agent
	srv          *httptest.Server
	warnings     []string
	contentTypes []string // the Content-Type of every response
}

// Transport formats, the C# TransportFormat rows.
const (
	formatJSON     = "Json"
	formatProtobuf = "Protobuf"
)

// newClient serves runner and returns a pkg/client agent pointed at it,
// speaking the SSE binding (the C# tests' default TransportFormat.Json).
func newClient(t *testing.T, runner server.AgentRunner) *testClient {
	t.Helper()
	return newClientFormat(t, runner, formatJSON)
}

// newClientFormat is newClient for a C# TransportFormat row: Json asks for
// SSE only; Protobuf advertises both bindings, and the server answers
// protobuf.
func newClientFormat(t *testing.T, runner server.AgentRunner, format string) *testClient {
	t.Helper()
	srv := httptest.NewServer(server.NewAgentHandler(runner, server.Options{Logger: t.Logf}))
	t.Cleanup(srv.Close)
	c := &testClient{Agent: client.New(srv.URL), srv: srv}
	c.DisableProtobuf = format != formatProtobuf
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil {
			c.contentTypes = append(c.contentTypes, resp.Header.Get("Content-Type"))
		}
		return resp, err
	})}
	c.Warn = func(w agui.Warning) { c.warnings = append(c.warnings, w.String()) }
	return c
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// forFormats runs body once per C# TransportFormat row and checks the
// responses used that row's binding.
func forFormats(t *testing.T, body func(t *testing.T, format string) *testClient) {
	t.Helper()
	for _, f := range []string{formatJSON, formatProtobuf} {
		t.Run(f, func(t *testing.T) {
			c := body(t, f)
			want := sse.ContentType
			if f == formatProtobuf {
				want = protobuf.ContentType
			}
			if c == nil {
				return
			}
			for _, ct := range c.contentTypes {
				if !strings.HasPrefix(ct, want) {
					t.Fatalf("%s row answered with Content-Type %q", f, ct)
				}
			}
		})
	}
}

// runOK runs one turn and fails the test on a transport or protocol error.
func (c *testClient) runOK(t *testing.T, in client.RunInput) *client.RunResult {
	t.Helper()
	res, err := c.Run(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("client run: %v", err)
	}
	return res
}

// say appends a user message to the client's thread.
func (c *testClient) say(id, text string) {
	c.AddMessage(&agui.UserMessage{ID: id, Content: agui.TextContent(text)})
}

// assistantText concatenates the deltas of the text messages a run streamed.
func assistantText(events []agui.Event) string {
	var b strings.Builder
	for _, e := range events {
		switch v := e.(type) {
		case *agui.TextMessageContentEvent:
			b.WriteString(v.Delta)
		case *agui.TextMessageChunkEvent:
			b.WriteString(agui.Deref(v.Delta))
		}
	}
	return b.String()
}

// clientTool is a tool declaration as the C# CreateToolDeclaration builds it.
func clientTool(name, description string) agui.Tool {
	return agui.Tool{Name: name, Description: description, Parameters: map[string]any{"type": "object"}}
}

// lastUserText is the text of the last user message of the input.
func lastUserText(in *agui.RunAgentInput) string { return server.LastUserText(in) }

// ---------------------------------------------------------------------------
// Sample replay (Samples/GettingStarted and the mixed-invocation integration
// test). The C# tests replay a recorded LLM run through the .NET server and
// pin every capture point with Verify baselines. The Go port replays the
// baseline's wire events (testdata/samples, see its README.txt) from a Go
// agent — so every event the .NET server put on the wire must pass the Go
// producer's schema validation and stream verification — drives the Go
// client through the sample's turns, and compares the Go wire documents
// (request and events) structurally with the baselines, ids and timestamps
// ignored the way the C# scrubbers ignore them.
// ---------------------------------------------------------------------------

type sampleTurn struct {
	Request map[string]any   // turnNN.request.json
	Events  []map[string]any // turnNN.events.json
}

func loadSample(t *testing.T, name string) []sampleTurn {
	t.Helper()
	var turns []sampleTurn
	for i := 1; ; i++ {
		base := fmt.Sprintf("testdata/samples/%s/turn%02d", name, i)
		req, err := os.ReadFile(base + ".request.json")
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var turn sampleTurn
		if err := json.Unmarshal(req, &turn.Request); err != nil {
			t.Fatal(err)
		}
		ev, err := os.ReadFile(base + ".events.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(ev, &turn.Events); err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}
	if len(turns) == 0 {
		t.Fatalf("no baselines for %s", name)
	}
	return turns
}

// withIdentity returns the baseline event with the boundary events' identity
// replaced by the run's (the baseline's scrubbed thread/run ids stand for
// whatever the client minted).
func withIdentity(doc map[string]any, in *agui.RunAgentInput) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		out[k] = v
	}
	switch out["type"] {
	case "RUN_STARTED", "RUN_FINISHED":
		out["threadId"], out["runId"] = in.ThreadID, in.RunID
		if _, ok := out["parentRunId"]; ok && in.ParentRunID != nil {
			out["parentRunId"] = *in.ParentRunID
		}
	}
	return out
}

// replayServer is a Go agent replaying one baseline turn per call.
type replayServer struct {
	t      *testing.T
	turns  []sampleTurn
	mu     sync.Mutex
	inputs []*agui.RunAgentInput
}

func (r *replayServer) Run(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
	r.mu.Lock()
	k := len(r.inputs)
	r.inputs = append(r.inputs, in)
	r.mu.Unlock()
	if k >= len(r.turns) {
		return fmt.Errorf("unexpected call %d", k+1)
	}
	for i, doc := range r.turns[k].Events {
		data, _ := json.Marshal(withIdentity(doc, in))
		e, err := agui.ParseEvent(data)
		if err != nil {
			r.t.Errorf("turn %d event %d: the .NET wire event is not valid AG-UI 1.0: %v\n%s", k+1, i, err, data)
			return err
		}
		if err := s.Send(e); err != nil {
			r.t.Errorf("turn %d event %d: the Go producer refused the .NET wire event: %v\n%s", k+1, i, err, data)
			return err
		}
	}
	return nil
}

// sampleRun is one replayed sample: the server, and a Go client on it.
type sampleRun struct {
	t      *testing.T
	turns  []sampleTurn
	server *replayServer
	client *testClient
}

func newSampleRun(t *testing.T, name string) *sampleRun {
	t.Helper()
	turns := loadSample(t, name)
	srv := &replayServer{t: t, turns: turns}
	return &sampleRun{t: t, turns: turns, server: srv, client: newClient(t, srv)}
}

// baselineMessages decodes the messages of a turn's baseline request.
func (r *sampleRun) baselineMessages(turn int) agui.Messages {
	r.t.Helper()
	raw, _ := json.Marshal(r.turns[turn-1].Request["messages"])
	var msgs agui.Messages
	if err := json.Unmarshal(raw, &msgs); err != nil {
		r.t.Fatalf("baseline turn %d messages: %v", turn, err)
	}
	return msgs
}

// baselineTools decodes the tools of a turn's baseline request.
func (r *sampleRun) baselineTools(turn int) []agui.Tool {
	raw, _ := json.Marshal(r.turns[turn-1].Request["tools"])
	var tools []agui.Tool
	_ = json.Unmarshal(raw, &tools)
	return tools
}

// runTurn runs the client for turn and checks the events it received
// against the baseline's wire events.
func (r *sampleRun) runTurn(turn int, in client.RunInput) *client.RunResult {
	r.t.Helper()
	res, err := r.client.Run(context.Background(), in, nil)
	if err != nil {
		r.t.Fatalf("turn %d: the Go client rejected the replayed .NET stream: %v", turn, err)
	}
	want := r.turns[turn-1].Events
	if len(res.Events) != len(want) {
		r.t.Fatalf("turn %d: client received %d events, baseline has %d: %v", turn, len(res.Events), len(want), types(res.Events))
	}
	for i, e := range res.Events {
		got := toGeneric(r.t, e).(map[string]any)
		delete(got, "timestamp")
		exp := withIdentity(want[i], res.Request)
		if canon(got) != canon(exp) {
			r.t.Fatalf("turn %d event %d differs from the baseline\n got: %s\nwant: %s", turn, i, canon(got), canon(exp))
		}
	}
	return res
}

// normalizedRequest drops what the C# scrubber replaces (thread/run ids,
// message ids) so two requests compare structurally.
func normalizedRequest(t *testing.T, doc any) map[string]any {
	t.Helper()
	g := generic(t, canon(doc)).(map[string]any)
	delete(g, "threadId")
	delete(g, "runId")
	delete(g, "parentRunId")
	if msgs, ok := g["messages"].([]any); ok {
		for _, m := range msgs {
			mm := m.(map[string]any)
			delete(mm, "id")
			// Arguments are text the protocol carries verbatim; the .NET
			// client re-serializes them (escaping ' as \u0027), so they
			// compare as the JSON documents they hold.
			calls, _ := mm["toolCalls"].([]any)
			for _, c := range calls {
				fn := c.(map[string]any)["function"].(map[string]any)
				var parsed any
				if s, ok := fn["arguments"].(string); ok && json.Unmarshal([]byte(s), &parsed) == nil {
					fn["arguments"] = canon(parsed)
				}
			}
		}
	}
	return g
}

// assertRequest compares the Go client's request for turn with the baseline.
func (r *sampleRun) assertRequest(turn int, res *client.RunResult) {
	r.t.Helper()
	got := normalizedRequest(r.t, toGeneric(r.t, res.Request))
	want := normalizedRequest(r.t, r.turns[turn-1].Request)
	if canon(got) != canon(want) {
		r.t.Fatalf("turn %d request differs from the baseline\n got: %s\nwant: %s", turn, canon(got), canon(want))
	}
	// What the client sent is what the server received.
	if canon(toGeneric(r.t, r.server.inputs[turn-1])) != canon(toGeneric(r.t, res.Request)) {
		r.t.Fatalf("turn %d: the server received a different input than the client sent", turn)
	}
}

// canon renders a JSON value canonically (sorted keys).
func canon(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<%v>", err)
	}
	var g any
	_ = json.Unmarshal(b, &g)
	b, _ = json.Marshal(g)
	return string(b)
}

// lastAssistantText is the content of the thread's last assistant message.
func lastAssistantText(msgs agui.Messages) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if am, ok := msgs[i].(*agui.AssistantMessage); ok && am.Content != nil {
			return *am.Content
		}
	}
	return ""
}

// assertRequestWith compares the Go client's request for turn with the
// baseline after adjust has applied a documented difference to the
// (normalized) baseline.
func (r *sampleRun) assertRequestWith(turn int, res *client.RunResult, adjust func(want map[string]any)) {
	r.t.Helper()
	got := normalizedRequest(r.t, toGeneric(r.t, res.Request))
	want := normalizedRequest(r.t, r.turns[turn-1].Request)
	adjust(want)
	if canon(got) != canon(want) {
		r.t.Fatalf("turn %d request differs from the (adjusted) baseline\n got: %s\nwant: %s", turn, canon(got), canon(want))
	}
}

// toolCallsOf lists every tool call carried by the thread's assistant
// messages, in order.
func toolCallsOf(msgs agui.Messages) []agui.ToolCall {
	var out []agui.ToolCall
	for _, m := range msgs {
		if am, ok := m.(*agui.AssistantMessage); ok {
			out = append(out, am.ToolCalls...)
		}
	}
	return out
}

// toolMessagesOf lists the thread's tool messages.
func toolMessagesOf(msgs agui.Messages) []*agui.ToolMessage {
	var out []*agui.ToolMessage
	for _, m := range msgs {
		if tm, ok := m.(*agui.ToolMessage); ok {
			out = append(out, tm)
		}
	}
	return out
}

type contextT = context.Context

// postRaw POSTs body to runner with the given Accept and returns the
// response and its body, without interpreting the binding.
func postRaw(t *testing.T, runner server.AgentRunner, body, accept string) (*http.Response, []byte) {
	t.Helper()
	srv := httptest.NewServer(server.NewAgentHandler(runner, server.Options{Logger: t.Logf}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

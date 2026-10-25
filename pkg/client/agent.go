// Package client is the consumer side of AG-UI 1.0 over HTTP + SSE: it sends
// a RunAgentInput, reads the event stream, runs it through the processing
// pipeline the specification prescribes, and keeps the thread's messages and
// state.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// Agent is a remote AG-UI agent reached over HTTP + SSE, together with the
// thread the application holds with it.
type Agent struct {
	// URL is the agent endpoint.
	URL string
	// HTTPClient sends requests (default http.DefaultClient).
	HTTPClient *http.Client
	// Header is added to every request (authentication, ...).
	Header http.Header
	// ThreadID names the conversation.
	ThreadID string
	// Store holds the thread's messages and state.
	Store *Store
	// Middleware runs on every event before enforcement.
	Middleware []Middleware
	// Warn receives warnings; nil discards them.
	Warn func(agui.Warning)
	// PeerVersion is the agent's protocol version when it is known to be
	// older than this client ("0.0.57"); it selects downgrades. Leave it
	// empty for a current or unknown peer.
	PeerVersion string
	// PendingInterrupts are the interrupts the last run ended with. The next
	// run must answer or abandon every one of them in RunInput.Resume.
	PendingInterrupts []agui.Interrupt
	// DisableProtobuf makes the client ask for the SSE binding only. By
	// default it accepts both bindings and reads whichever the agent answers.
	DisableProtobuf bool
}

// New returns an agent on url with a fresh thread.
func New(url string) *Agent {
	return &Agent{URL: url, ThreadID: uuid.NewString(), Store: NewStore(nil, nil)}
}

// AddMessage appends a message to the thread (typically the user's turn).
func (a *Agent) AddMessage(m agui.Message) {
	a.Store.Messages = append(a.Store.Messages, m)
}

// RunInput holds the per-run parts of a RunAgentInput; the thread id,
// messages and state come from the Agent.
type RunInput struct {
	// RunID names the run (minted when empty).
	RunID          string
	ParentRunID    *string
	Tools          []agui.Tool
	Context        []agui.Context
	ForwardedProps any
	// Resume answers PendingInterrupts.
	Resume []agui.ResumeEntry
}

// RunStatus is how a run ended, from the consumer's point of view.
type RunStatus string

// Run statuses.
const (
	// StatusSucceeded: RUN_FINISHED with no outcome or the success outcome.
	StatusSucceeded RunStatus = "succeeded"
	// StatusInterrupted: RUN_FINISHED with the interrupt outcome.
	StatusInterrupted RunStatus = "interrupted"
	// StatusCancelled: RUN_FINISHED with the cancelled outcome — neither a
	// success nor a failure.
	StatusCancelled RunStatus = "cancelled"
	// StatusFailed: the producer reported RUN_ERROR (possibly after
	// RUN_FINISHED). Not a rejection: the stream was well formed.
	StatusFailed RunStatus = "failed"
	// StatusTruncated: the stream ended without a terminal event. A
	// truncated run has no outcome and did not succeed.
	StatusTruncated RunStatus = "truncated"
)

// RunResult reports a run.
type RunResult struct {
	Status RunStatus
	// Request is the input that was sent.
	Request *agui.RunAgentInput
	// Events are the events delivered to the application, in order.
	Events []agui.Event
	// Finished is the last RUN_FINISHED, if any.
	Finished *agui.RunFinishedEvent
	// Error is the RUN_ERROR that failed the last run on the stream, if any.
	Error *agui.RunErrorEvent
	// Errors are every RUN_ERROR the stream delivered, including those of
	// earlier runs a producer replayed ahead of the requested one.
	Errors []*agui.RunErrorEvent
	// Interrupts are what an interrupted run waits for.
	Interrupts []agui.Interrupt
	// PendingToolCallIDs are the frontend tool calls the run left for the
	// application to answer: named by the success outcome or, when it names
	// none, derived from the stream.
	PendingToolCallIDs []string
}

// Succeeded reports whether the run completed successfully.
func (r *RunResult) Succeeded() bool { return r.Status == StatusSucceeded }

// BuildInput assembles the run input as it will be sent, applying the
// outgoing rules: activity messages are stripped, retired content shapes are
// upgraded, unrecognised material is removed (with a warning) and a malformed
// known field is an error — all before anything is sent — and the resume list
// must cover every pending interrupt.
func (a *Agent) BuildInput(in RunInput) (*agui.RunAgentInput, []byte, error) {
	if in.RunID == "" {
		in.RunID = uuid.NewString()
	}
	if a.ThreadID == "" {
		a.ThreadID = uuid.NewString()
	}
	if a.Store == nil {
		a.Store = NewStore(nil, nil)
	}
	if err := checkResume(a.PendingInterrupts, in.Resume, time.Now()); err != nil {
		return nil, nil, err
	}
	var msgs agui.Messages
	for _, m := range a.Store.Messages {
		if m.MessageRole() != agui.RoleActivity {
			msgs = append(msgs, m)
		}
	}
	input := agui.RunAgentInput{
		ThreadID: a.ThreadID, RunID: in.RunID, ParentRunID: in.ParentRunID,
		State: a.Store.State, Messages: msgs, Tools: in.Tools, Context: in.Context,
		ForwardedProps: in.ForwardedProps, Resume: in.Resume,
	}
	if !peerPredatesVersioning(a.PeerVersion) {
		input.ProtocolVersion = agui.Ptr(agui.ProtocolVersion)
	}
	doc, err := agui.ToGeneric(input)
	if err != nil {
		return nil, nil, err
	}
	obj := doc.(map[string]any)
	for _, w := range agui.UpgradeLegacyContent(obj["messages"], "/messages", false) {
		a.warn(w)
	}
	if eraAtMost(a.PeerVersion, 0, 0, 39) {
		a.flattenContent(obj["messages"])
	}
	if eraAtMost(a.PeerVersion, 0, 0, 57) {
		if list, ok := obj["messages"].([]any); ok {
			for _, m := range list {
				if mm, ok := m.(map[string]any); ok {
					delete(mm, "subagentRunId")
				}
			}
		}
	}
	res, err := agui.SanitizeValue("RunAgentInput", obj, agui.SanitizeOptions{})
	if res != nil {
		for _, w := range res.Warnings {
			a.warn(agui.Warning{Path: w.Path, Message: "outgoing input: " + w.Message})
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("ag-ui: refusing to send a malformed run input: %w", err)
	}
	body, err := json.Marshal(res.Value)
	if err != nil {
		return nil, nil, err
	}
	var sent agui.RunAgentInput
	if !eraAtMost(a.PeerVersion, 0, 0, 39) {
		if err := json.Unmarshal(body, &sent); err != nil {
			return nil, nil, err
		}
	} else {
		sent = input // flattened content is no longer the 1.0 shape
	}
	return &sent, body, nil
}

// flattenContent is the 0.0.39 era shim: a peer that predates content parts
// receives string content — the text parts concatenated — and every part that
// cannot be carried is reported.
func (a *Agent) flattenContent(messages any) {
	list, _ := messages.([]any)
	for i, m := range list {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := mm["content"].([]any)
		if !ok {
			continue
		}
		var text strings.Builder
		for j, p := range parts {
			part, _ := p.(map[string]any)
			if part["type"] == "text" {
				s, _ := part["text"].(string)
				text.WriteString(s)
				continue
			}
			a.warn(agui.Warning{Path: fmt.Sprintf("/messages/%d/content/%d", i, j), Message: fmt.Sprintf("flattening content for a %s peer DROPS non-text parts: a %v part was not sent", a.PeerVersion, part["type"])})
		}
		mm["content"] = text.String()
	}
}

// checkResume enforces the consumer's coverage rule: every pending interrupt
// is answered or abandoned, no entry names an unknown interrupt, and an
// interrupt judged expired can only be abandoned.
func checkResume(pending []agui.Interrupt, resume []agui.ResumeEntry, now time.Time) error {
	byID := map[string]agui.Interrupt{}
	for _, i := range pending {
		byID[i.ID] = i
	}
	covered := map[string]bool{}
	for _, r := range resume {
		in, ok := byID[r.InterruptID]
		if !ok {
			return fmt.Errorf("ag-ui: resume entry names interrupt %q, which the last run did not raise", r.InterruptID)
		}
		if covered[r.InterruptID] {
			return fmt.Errorf("ag-ui: interrupt %q is answered twice", r.InterruptID)
		}
		covered[r.InterruptID] = true
		if r.Status == agui.ResumeStatusResolved && in.ExpiresAt != nil {
			if t, err := time.Parse(time.RFC3339, *in.ExpiresAt); err == nil && !t.After(now) {
				return fmt.Errorf("ag-ui: interrupt %q expired at %s; it can only be abandoned (status %q)", r.InterruptID, *in.ExpiresAt, agui.ResumeStatusCancelled)
			}
		}
	}
	for _, i := range pending {
		if !covered[i.ID] {
			return fmt.Errorf("ag-ui: interrupt %q is not answered: every pending interrupt needs a resume entry (resolved, or %q to abandon it)", i.ID, agui.ResumeStatusCancelled)
		}
	}
	return nil
}

func (a *Agent) warn(w agui.Warning) {
	if a.Warn != nil {
		a.Warn(w)
	}
}

// RejectedError is an HTTP rejection of the run input (the run never started).
type RejectedError struct {
	StatusCode int
	Body       string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("ag-ui: agent rejected the run input: HTTP %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

// Run sends the run input and consumes the stream. onEvent, when not nil,
// receives every event delivered to the application, in order, after it has
// been applied to the Store.
//
// The error is non-nil when the request was rejected, the transport failed,
// or the stream broke the protocol and was rejected. A run that reports its
// own failure with RUN_ERROR is not an error: it comes back with
// StatusFailed.
func (a *Agent) Run(ctx context.Context, in RunInput, onEvent func(agui.Event)) (*RunResult, error) {
	input, body, err := a.BuildInput(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vs := range a.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	accept := sse.ContentType
	if !a.DisableProtobuf {
		accept += ", " + protobuf.ContentType
	}
	req.Header.Set("Accept", accept)
	hc := a.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, &RejectedError{StatusCode: resp.StatusCode, Body: string(b)}
	}
	var result *RunResult
	switch ct := strings.ToLower(resp.Header.Get("Content-Type")); {
	case strings.HasPrefix(ct, sse.ContentType):
		result, err = a.Consume(resp.Body, onEvent)
	case !a.DisableProtobuf && strings.HasPrefix(ct, protobuf.ContentType):
		result, err = a.ConsumeProtobuf(resp.Body, onEvent)
	default:
		return nil, fmt.Errorf("ag-ui: unsupported response Content-Type %q", resp.Header.Get("Content-Type"))
	}
	if result != nil {
		result.Request = input
	}
	return result, err
}

// Consume runs an SSE event stream through the pipeline, applying it to the
// Store. Run uses it; it is exposed for custom transports and recorded
// streams.
func (a *Agent) Consume(body io.Reader, onEvent func(agui.Event)) (*RunResult, error) {
	if a.Store == nil {
		a.Store = NewStore(nil, nil)
	}
	r := sse.NewReader(body)
	return a.consume(func() ([]byte, error) {
		data, err := r.Next()
		if errors.Is(err, sse.ErrTruncated) {
			return nil, errTruncated
		}
		return data, err
	}, onEvent)
}

// ConsumeProtobuf is Consume for a stream of the HTTP + Protobuf binding.
// A frame carrying an event of a type this build does not know is dropped
// with a warning, the answer enforcement gives the same event over SSE.
func (a *Agent) ConsumeProtobuf(body io.Reader, onEvent func(agui.Event)) (*RunResult, error) {
	r := protobuf.NewFrameReader(body)
	opts := protobuf.Options{Warn: func(s string) { a.warn(agui.Warning{Message: s}) }}
	return a.consume(func() ([]byte, error) {
		for {
			frame, err := r.Next()
			if errors.Is(err, protobuf.ErrTruncated) {
				return nil, errTruncated
			}
			if err != nil {
				return nil, err
			}
			data, err := protobuf.DecodeJSON(frame, opts)
			if errors.Is(err, protobuf.ErrUnknownEventType) {
				a.warn(agui.Warning{Message: "dropped a protobuf frame carrying an event of a type this build does not know"})
				continue
			}
			return data, err
		}
	}, onEvent)
}

var errTruncated = errors.New("truncated")

// consume runs the events next yields through the pipeline. next returns
// io.EOF at a clean end of stream and errTruncated mid-event.
func (a *Agent) consume(next func() ([]byte, error), onEvent func(agui.Event)) (*RunResult, error) {
	if a.Store == nil {
		a.Store = NewStore(nil, nil)
	}
	p := &Pipeline{Store: a.Store, Middleware: a.Middleware, Warn: a.Warn, PeerVersion: a.PeerVersion}
	result := &RunResult{Status: StatusTruncated}
	for {
		data, err := next()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, errTruncated) {
			result.Status = StatusTruncated
			return result, nil
		}
		if err != nil {
			return result, err
		}
		events, err := p.Push(data)
		for _, e := range events {
			result.observe(e, a)
			if onEvent != nil {
				onEvent(e)
			}
		}
		if err != nil {
			return result, err
		}
	}
	if !p.RunClosed() {
		result.Status = StatusTruncated
	}
	return result, nil
}

func (r *RunResult) observe(e agui.Event, a *Agent) {
	r.Events = append(r.Events, e)
	switch t := e.(type) {
	case *agui.RunStartedEvent:
		r.Status, r.Finished, r.Error, r.Interrupts, r.PendingToolCallIDs = StatusTruncated, nil, nil, nil, nil
		a.PendingInterrupts = nil
	case *agui.RunFinishedEvent:
		r.Finished = t
		r.Status = StatusSucceeded
		switch o := t.Outcome.(type) {
		case *agui.InterruptOutcome:
			r.Status, r.Interrupts = StatusInterrupted, o.Interrupts
			a.PendingInterrupts = o.Interrupts
		case *agui.CancelledOutcome:
			r.Status = StatusCancelled
		case *agui.SuccessOutcome:
			r.PendingToolCallIDs = o.PendingToolCallIDs
		}
		if r.Status == StatusSucceeded && len(r.PendingToolCallIDs) == 0 {
			r.PendingToolCallIDs = a.Store.PendingToolCallIDs()
		}
	case *agui.RunErrorEvent:
		r.Status, r.Error = StatusFailed, t
		r.Errors = append(r.Errors, t)
	}
}

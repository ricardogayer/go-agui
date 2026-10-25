// Package stream is the producer side of the AG-UI HTTP + SSE binding: it
// writes protocol events to an HTTP response as Server-Sent Events.
//
// Every event is held to the protocol before it is written. It must marshal
// to a document the AG-UI 1.0 schema accepts, and it must respect the
// stream's ordering rules (the run lifecycle, open/close discipline, chunk
// rules, subagent ownership). An event that breaks either is not written and
// Send returns the error, so a producer built on this package cannot emit a
// non-conformant stream.
package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// SSE writes AG-UI events to an HTTP response as Server-Sent Events. It is
// safe for concurrent use: events from parallel goroutines (subagents, say)
// are serialised in the order Send is called.
//
// The "current message" shortcut (TextStart with an empty id, TextContent and
// TextEnd with "") tracks one message per stream; goroutines streaming
// messages concurrently must pass explicit message ids.
type SSE struct {
	mu        sync.Mutex
	w         *sse.Writer
	frames    *protobuf.FrameWriter // set when the stream speaks the protobuf binding
	checker   agui.StreamChecker
	validate  bool
	messageID string
	threadID  string
	runID     string
	err       error
}

// Option configures an SSE stream.
type Option func(*SSE)

// WithoutSchemaValidation skips validating each event against the 1.0 schema
// before writing it. Ordering rules are still verified. Validation costs a
// marshal and a schema walk per event; skip it only for a producer whose
// event construction is already proven.
func WithoutSchemaValidation() Option { return func(s *SSE) { s.validate = false } }

// New prepares w for the SSE binding (headers, 200 status on first write) and
// returns a stream writing to it.
func New(w http.ResponseWriter, opts ...Option) *SSE {
	sse.SetHeaders(w.Header())
	return NewWriter(w, opts...)
}

// ForThread binds the stream to the input's thread: every RUN_STARTED it
// emits must carry this threadId. pkg/server sets it for every request.
func ForThread(threadID string) Option {
	return func(s *SSE) { s.checker.Verifier.ThreadID = threadID }
}

// NewProtobuf prepares w for the HTTP + Protobuf binding and returns a stream
// writing length-prefixed protobuf frames to it.
func NewProtobuf(w http.ResponseWriter, opts ...Option) *SSE {
	w.Header().Set("Content-Type", protobuf.ContentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	return NewProtobufWriter(w, opts...)
}

// NewProtobufWriter returns a stream writing protobuf frames to any writer.
func NewProtobufWriter(w io.Writer, opts ...Option) *SSE {
	s := NewWriter(w, opts...)
	s.frames = protobuf.NewFrameWriter(w)
	return s
}

// Negotiate returns a stream for the binding the request asks for: protobuf
// when its Accept header admits application/vnd.ag-ui.event+proto with a
// positive quality, SSE otherwise.
func Negotiate(w http.ResponseWriter, r *http.Request, opts ...Option) *SSE {
	if protobuf.AdmitsProtobuf(r.Header.Get("Accept")) {
		return NewProtobuf(w, opts...)
	}
	return New(w, opts...)
}

// Protobuf reports whether the stream speaks the protobuf binding.
func (s *SSE) Protobuf() bool { return s.frames != nil }

// NewWriter returns a stream writing SSE frames to any writer — a custom
// transport, a test buffer. Flushing happens when w is an http.Flusher.
func NewWriter(w io.Writer, opts ...Option) *SSE {
	s := &SSE{w: sse.NewWriter(w), validate: true}
	s.checker.Verifier.Producer = true
	for _, o := range opts {
		o(s)
	}
	return s
}

// ErrStreamFailed is wrapped by Send once a write to the connection has
// failed: the stream is broken and nothing more can be sent.
var ErrStreamFailed = errors.New("ag-ui: stream failed")

// Send writes one event. Events created by this package's constructors are
// stamped with the current time; Send does not change the event.
//
// It returns a *agui.ValidationError for an event the schema rejects and a
// *agui.ProtocolError for one that breaks the stream's ordering rules; in
// both cases nothing is written and the stream stays usable.
func (s *SSE) Send(e agui.Event) error {
	if e == nil {
		return errors.New("ag-ui: nil event")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("ag-ui: cannot marshal %s: %w", e.Type(), err)
	}
	if s.validate {
		if err := agui.Validate("Event", data); err != nil {
			return fmt.Errorf("ag-ui: refusing to emit %s: %w", e.Type(), err)
		}
	}
	// A rejected event leaves the checker's state untouched.
	if _, err := s.checker.CheckAtomic(e); err != nil {
		return fmt.Errorf("ag-ui: refusing to emit %s: %w", e.Type(), err)
	}
	write := s.w.WriteData
	if s.frames != nil {
		frame, err := protobuf.EncodeJSON(data, protobuf.Options{SkipValidation: s.validate})
		if err != nil {
			return fmt.Errorf("ag-ui: cannot encode %s for the protobuf binding: %w", e.Type(), err)
		}
		data, write = frame, s.frames.WriteFrame
	}
	if err := write(data); err != nil {
		s.err = fmt.Errorf("%w: %w", ErrStreamFailed, err)
		return s.err
	}
	return nil
}

// Comment writes an SSE comment line (": text"), which consumers ignore —
// useful as a keep-alive on idle connections. On the protobuf binding, which
// has no comments, it does nothing.
func (s *SSE) Comment(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if s.frames != nil {
		return nil // the binary binding has no comments
	}
	if err := s.w.WriteComment(text); err != nil {
		s.err = fmt.Errorf("%w: %w", ErrStreamFailed, err)
		return s.err
	}
	return nil
}

// RunActive reports whether a run is open on the stream.
func (s *SSE) RunActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checker.Verifier.RunActive()
}

// Started reports whether any event has been written.
func (s *SSE) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checker.Verifier.Started()
}

// Failed returns the write error that broke the stream, if any.
func (s *SSE) Failed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// =============================================================================
// Runs and steps
// =============================================================================

// RunStarted opens a run, declaring the protocol version this producer
// speaks.
func (s *SSE) RunStarted(threadID, runID string) error {
	if err := s.Send(agui.NewRunStartedEvent(threadID, runID)); err != nil {
		return err
	}
	s.mu.Lock()
	s.threadID, s.runID = threadID, runID
	s.mu.Unlock()
	return nil
}

// RunFinished closes the run successfully (no outcome).
func (s *SSE) RunFinished(threadID, runID string) error {
	return s.Send(agui.NewRunFinishedEvent(threadID, runID))
}

// RunFinishedWithResult closes the run successfully with a return value.
func (s *SSE) RunFinishedWithResult(threadID, runID string, result any) error {
	e := agui.NewRunFinishedEvent(threadID, runID)
	e.Result = result
	return s.Send(e)
}

// RunFinishedPending closes a run that stopped on frontend tool calls: a
// success, naming the calls left for the application to answer.
func (s *SSE) RunFinishedPending(threadID, runID string, pendingToolCallIDs ...string) error {
	e := agui.NewRunFinishedEvent(threadID, runID)
	e.Outcome = &agui.SuccessOutcome{PendingToolCallIDs: pendingToolCallIDs}
	return s.Send(e)
}

// RunInterrupted closes a run that stopped to ask for outside input. The next
// run resumes with answers to these interrupts in RunAgentInput.Resume.
func (s *SSE) RunInterrupted(threadID, runID string, interrupts ...agui.Interrupt) error {
	e := agui.NewRunFinishedEvent(threadID, runID)
	e.Outcome = &agui.InterruptOutcome{Interrupts: interrupts}
	return s.Send(e)
}

// RunCancelled closes a run stopped on purpose before it completed. It is not
// a failure and must not be reported as success.
func (s *SSE) RunCancelled(threadID, runID string) error {
	e := agui.NewRunFinishedEvent(threadID, runID)
	e.Outcome = &agui.CancelledOutcome{}
	return s.Send(e)
}

// RunError ends the run as failed. code is optional.
func (s *SSE) RunError(message string, code *string) error {
	e := agui.NewRunErrorEvent(message)
	e.Code = code
	return s.Send(e)
}

// StepStarted opens a step.
func (s *SSE) StepStarted(stepName string) error {
	return s.Send(agui.NewStepStartedEvent(stepName))
}

// StepFinished closes a step.
func (s *SSE) StepFinished(stepName string) error {
	return s.Send(agui.NewStepFinishedEvent(stepName))
}

// =============================================================================
// Text messages
// =============================================================================

// TextStart opens an assistant text message. An empty messageID mints one;
// it becomes the current message for TextContent and TextEnd.
func (s *SSE) TextStart(messageID string) error {
	if messageID == "" {
		messageID = uuid.NewString()
	}
	if err := s.Send(agui.NewTextMessageStartEvent(messageID)); err != nil {
		return err
	}
	s.mu.Lock()
	s.messageID = messageID
	s.mu.Unlock()
	return nil
}

// TextContent extends a text message; an empty messageID means the current
// one.
func (s *SSE) TextContent(messageID, delta string) error {
	return s.Send(agui.NewTextMessageContentEvent(s.current(messageID), delta))
}

// TextEnd closes a text message; an empty messageID means the current one.
func (s *SSE) TextEnd(messageID string) error {
	return s.Send(agui.NewTextMessageEndEvent(s.current(messageID)))
}

// TextMessage sends a complete assistant message (start, content, end) and
// returns its id.
func (s *SSE) TextMessage(content string) (string, error) {
	id := uuid.NewString()
	if err := s.TextStart(id); err != nil {
		return id, err
	}
	if err := s.TextContent(id, content); err != nil {
		return id, err
	}
	return id, s.TextEnd(id)
}

// TextChunk sends a TEXT_MESSAGE_CHUNK. Pass the message id on the first
// chunk of a message and "" on its continuations; the chunk stream closes
// itself when another item or the run ends.
func (s *SSE) TextChunk(messageID, delta string) error {
	return s.Send(agui.NewTextMessageChunkEvent(messageID, delta))
}

func (s *SSE) current(id string) string {
	if id != "" {
		return id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageID
}

// =============================================================================
// Tool calls
// =============================================================================

// ToolCallStart opens a tool call.
func (s *SSE) ToolCallStart(toolCallID, toolCallName string) error {
	return s.Send(agui.NewToolCallStartEvent(toolCallID, toolCallName))
}

// ToolCallStartIn opens a tool call carried by the assistant message
// parentMessageID.
func (s *SSE) ToolCallStartIn(parentMessageID, toolCallID, toolCallName string) error {
	e := agui.NewToolCallStartEvent(toolCallID, toolCallName)
	e.ParentMessageID = agui.Ptr(parentMessageID)
	return s.Send(e)
}

// ToolCallArgs extends a tool call's argument text.
func (s *SSE) ToolCallArgs(toolCallID, delta string) error {
	return s.Send(agui.NewToolCallArgsEvent(toolCallID, delta))
}

// ToolCallEnd closes a tool call.
func (s *SSE) ToolCallEnd(toolCallID string) error {
	return s.Send(agui.NewToolCallEndEvent(toolCallID))
}

// ToolCall sends a complete tool call (start, args, end).
func (s *SSE) ToolCall(toolCallID, toolCallName, arguments string) error {
	if err := s.ToolCallStart(toolCallID, toolCallName); err != nil {
		return err
	}
	if err := s.ToolCallArgs(toolCallID, arguments); err != nil {
		return err
	}
	return s.ToolCallEnd(toolCallID)
}

// ToolCallChunk sends a TOOL_CALL_CHUNK. Pass id and name on the first chunk
// of a call and "" on its continuations.
func (s *SSE) ToolCallChunk(toolCallID, toolCallName, delta string) error {
	return s.Send(agui.NewToolCallChunkEvent(toolCallID, toolCallName, delta))
}

// ToolCallResult answers an agent-executed tool call with string content, as
// the tool message messageID. Frontend tools (RunAgentInput.Tools) must not
// be answered by the producer.
func (s *SSE) ToolCallResult(messageID, toolCallID, content string) error {
	return s.Send(agui.NewToolCallResultEvent(messageID, toolCallID, content))
}

// ToolCallResultParts answers a tool call with content parts (a document, an
// image, ...).
func (s *SSE) ToolCallResultParts(messageID, toolCallID string, parts ...agui.ContentPart) error {
	e := agui.NewToolCallResultEvent(messageID, toolCallID, "")
	e.Content = agui.PartsContent(parts...)
	return s.Send(e)
}

// =============================================================================
// Reasoning
// =============================================================================

// ReasoningStart opens a reasoning span.
func (s *SSE) ReasoningStart(spanID string) error {
	return s.Send(agui.NewReasoningStartEvent(spanID))
}

// ReasoningEnd closes a reasoning span.
func (s *SSE) ReasoningEnd(spanID string) error {
	return s.Send(agui.NewReasoningEndEvent(spanID))
}

// ReasoningMessageStart opens a reasoning message.
func (s *SSE) ReasoningMessageStart(messageID string) error {
	return s.Send(agui.NewReasoningMessageStartEvent(messageID))
}

// ReasoningMessageContent extends a reasoning message.
func (s *SSE) ReasoningMessageContent(messageID, delta string) error {
	return s.Send(agui.NewReasoningMessageContentEvent(messageID, delta))
}

// ReasoningMessageEnd closes a reasoning message.
func (s *SSE) ReasoningMessageEnd(messageID string) error {
	return s.Send(agui.NewReasoningMessageEndEvent(messageID))
}

// Reasoning sends a complete reasoning span holding one reasoning message,
// and returns the message id.
func (s *SSE) Reasoning(text string) (string, error) {
	span, msg := uuid.NewString(), uuid.NewString()
	for _, step := range []func() error{
		func() error { return s.ReasoningStart(span) },
		func() error { return s.ReasoningMessageStart(msg) },
		func() error { return s.ReasoningMessageContent(msg, text) },
		func() error { return s.ReasoningMessageEnd(msg) },
		func() error { return s.ReasoningEnd(span) },
	} {
		if err := step(); err != nil {
			return msg, err
		}
	}
	return msg, nil
}

// ReasoningEncryptedValue attaches a provider's opaque reasoning artefact to
// a message or tool call.
func (s *SSE) ReasoningEncryptedValue(subtype agui.ReasoningEncryptedValueSubtype, entityID, encryptedValue string) error {
	return s.Send(agui.NewReasoningEncryptedValueEvent(subtype, entityID, encryptedValue))
}

// =============================================================================
// State, messages and activity
// =============================================================================

// StateSnapshot replaces the state wholesale.
func (s *SSE) StateSnapshot(snapshot any) error {
	return s.Send(agui.NewStateSnapshotEvent(snapshot))
}

// StateDelta amends the state with JSON Patch operations, computed against
// the input's state until the run's own first snapshot replaces it.
func (s *SSE) StateDelta(ops ...agui.PatchOperation) error {
	return s.Send(agui.NewStateDeltaEvent(ops...))
}

// MessagesSnapshot restates the complete set of messages the producer owns.
func (s *SSE) MessagesSnapshot(messages ...agui.Message) error {
	return s.Send(agui.NewMessagesSnapshotEvent(messages...))
}

// ActivitySnapshot creates or replaces an activity message.
func (s *SSE) ActivitySnapshot(messageID, activityType string, content map[string]any) error {
	return s.Send(agui.NewActivitySnapshotEvent(messageID, activityType, content))
}

// ActivityDelta amends an activity message created by ActivitySnapshot.
func (s *SSE) ActivityDelta(messageID, activityType string, ops ...agui.PatchOperation) error {
	return s.Send(agui.NewActivityDeltaEvent(messageID, activityType, ops...))
}

// =============================================================================
// Subagents
// =============================================================================

// SubagentStarted announces a subagent invocation. Events it produces carry
// its id in SubagentRunID.
func (s *SSE) SubagentStarted(subagentRunID, name string) error {
	return s.Send(agui.NewSubagentStartedEvent(subagentRunID, name))
}

// SubagentFinished closes a subagent invocation successfully.
func (s *SSE) SubagentFinished(subagentRunID string) error {
	return s.Send(agui.NewSubagentFinishedEvent(subagentRunID))
}

// SubagentError reports that a subagent invocation failed (the run goes on).
func (s *SSE) SubagentError(subagentRunID, message string) error {
	return s.Send(agui.NewSubagentErrorEvent(subagentRunID, message))
}

// =============================================================================
// Passthrough
// =============================================================================

// CustomEvent sends an application event. Prefix names with a vendor or
// application identifier ("acme.cart-updated").
func (s *SSE) CustomEvent(name string, value any) error {
	return s.Send(agui.NewCustomEvent(name, value))
}

// RawEvent passes a provider-native event through, alongside — never
// instead of — the standard events. source is optional.
func (s *SSE) RawEvent(event any, source *string) error {
	e := agui.NewRawEvent(event)
	e.Source = source
	return s.Send(e)
}

// =============================================================================
// Streaming helpers
// =============================================================================

// StreamFunc receives chunks of content to stream.
type StreamFunc func(chunk string) error

// StreamingCallback returns a callback that streams each chunk as
// TEXT_MESSAGE_CONTENT of messageID ("" for the current message).
func (s *SSE) StreamingCallback(messageID string) StreamFunc {
	id := s.current(messageID)
	return func(chunk string) error {
		return s.TextContent(id, chunk)
	}
}

// Stream copies a reader into TEXT_MESSAGE_CONTENT events of messageID ("" for
// the current message), bufSize bytes at a time (default 1024). Multi-byte
// UTF-8 sequences are never split across events.
func (s *SSE) Stream(messageID string, reader io.Reader, bufSize int) error {
	if bufSize <= 0 {
		bufSize = 1024
	}
	id := s.current(messageID)
	buf := make([]byte, bufSize)
	var carry []byte
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := append(carry, buf[:n]...)
			cut := validUTF8Prefix(chunk)
			carry = append([]byte(nil), chunk[cut:]...)
			if cut > 0 {
				if sendErr := s.TextContent(id, string(chunk[:cut])); sendErr != nil {
					return sendErr
				}
			}
		}
		if err == io.EOF {
			if len(carry) > 0 {
				return s.TextContent(id, string(carry))
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// validUTF8Prefix returns the length of b without a trailing incomplete UTF-8
// sequence.
func validUTF8Prefix(b []byte) int {
	for i := len(b) - 1; i >= 0 && i >= len(b)-3; i-- {
		c := b[i]
		if c < 0x80 {
			return len(b)
		}
		if c >= 0xC0 { // a lead byte: is its sequence complete?
			need := 2
			switch {
			case c >= 0xF0:
				need = 4
			case c >= 0xE0:
				need = 3
			}
			if len(b)-i < need {
				return i
			}
			return len(b)
		}
	}
	return len(b)
}

// MessageID returns the current text message id.
func (s *SSE) MessageID() string { return s.current("") }

// SetMessageID sets the current text message id.
func (s *SSE) SetMessageID(id string) {
	s.mu.Lock()
	s.messageID = id
	s.mu.Unlock()
}

// NewMessageID mints and sets a new current text message id.
func (s *SSE) NewMessageID() string {
	id := uuid.NewString()
	s.SetMessageID(id)
	return id
}

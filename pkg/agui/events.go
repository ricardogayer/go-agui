package agui

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// =============================================================================
// Event types
// =============================================================================

// EventType is the discriminator of an event.
type EventType string

// The 31 event types of AG-UI 1.0, in eight families.
const (
	// Runs and steps.
	EventTypeRunStarted   EventType = "RUN_STARTED"
	EventTypeRunFinished  EventType = "RUN_FINISHED"
	EventTypeRunError     EventType = "RUN_ERROR"
	EventTypeStepStarted  EventType = "STEP_STARTED"
	EventTypeStepFinished EventType = "STEP_FINISHED"

	// Text messages.
	EventTypeTextMessageStart   EventType = "TEXT_MESSAGE_START"
	EventTypeTextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	EventTypeTextMessageEnd     EventType = "TEXT_MESSAGE_END"
	EventTypeTextMessageChunk   EventType = "TEXT_MESSAGE_CHUNK"

	// Tool calls.
	EventTypeToolCallStart  EventType = "TOOL_CALL_START"
	EventTypeToolCallArgs   EventType = "TOOL_CALL_ARGS"
	EventTypeToolCallEnd    EventType = "TOOL_CALL_END"
	EventTypeToolCallChunk  EventType = "TOOL_CALL_CHUNK"
	EventTypeToolCallResult EventType = "TOOL_CALL_RESULT"

	// Reasoning.
	EventTypeReasoningStart          EventType = "REASONING_START"
	EventTypeReasoningMessageStart   EventType = "REASONING_MESSAGE_START"
	EventTypeReasoningMessageContent EventType = "REASONING_MESSAGE_CONTENT"
	EventTypeReasoningMessageEnd     EventType = "REASONING_MESSAGE_END"
	EventTypeReasoningMessageChunk   EventType = "REASONING_MESSAGE_CHUNK"
	EventTypeReasoningEnd            EventType = "REASONING_END"
	EventTypeReasoningEncryptedValue EventType = "REASONING_ENCRYPTED_VALUE"

	// State.
	EventTypeStateSnapshot    EventType = "STATE_SNAPSHOT"
	EventTypeStateDelta       EventType = "STATE_DELTA"
	EventTypeMessagesSnapshot EventType = "MESSAGES_SNAPSHOT"

	// Activity.
	EventTypeActivitySnapshot EventType = "ACTIVITY_SNAPSHOT"
	EventTypeActivityDelta    EventType = "ACTIVITY_DELTA"

	// Subagents.
	EventTypeSubagentStarted  EventType = "SUBAGENT_STARTED"
	EventTypeSubagentFinished EventType = "SUBAGENT_FINISHED"
	EventTypeSubagentError    EventType = "SUBAGENT_ERROR"

	// Passthrough.
	EventTypeRaw    EventType = "RAW"
	EventTypeCustom EventType = "CUSTOM"
)

// EventTypes lists every event type of this protocol version.
var EventTypes = []EventType{
	EventTypeRunStarted, EventTypeRunFinished, EventTypeRunError, EventTypeStepStarted, EventTypeStepFinished,
	EventTypeTextMessageStart, EventTypeTextMessageContent, EventTypeTextMessageEnd, EventTypeTextMessageChunk,
	EventTypeToolCallStart, EventTypeToolCallArgs, EventTypeToolCallEnd, EventTypeToolCallChunk, EventTypeToolCallResult,
	EventTypeReasoningStart, EventTypeReasoningMessageStart, EventTypeReasoningMessageContent, EventTypeReasoningMessageEnd,
	EventTypeReasoningMessageChunk, EventTypeReasoningEnd, EventTypeReasoningEncryptedValue,
	EventTypeStateSnapshot, EventTypeStateDelta, EventTypeMessagesSnapshot,
	EventTypeActivitySnapshot, EventTypeActivityDelta,
	EventTypeSubagentStarted, EventTypeSubagentFinished, EventTypeSubagentError,
	EventTypeRaw, EventTypeCustom,
}

// =============================================================================
// Envelope
// =============================================================================

// Event is a protocol event. Every concrete event type in this package
// implements it through a pointer (*RunStartedEvent, ...).
type Event interface {
	// Type returns the event's discriminator.
	Type() EventType
	// Base returns the envelope fields every event carries.
	Base() *BaseEvent
}

// BaseEvent holds the envelope fields every event carries.
type BaseEvent struct {
	// Timestamp is when the event was created, by convention in milliseconds
	// since the Unix epoch. Informational only: arrival order is the
	// protocol's order.
	Timestamp *int64 `json:"timestamp,omitempty"`
	// RawEvent is the provider-native event this one was translated from,
	// carried verbatim. Nil means absent.
	RawEvent any `json:"rawEvent,omitempty"`
	// Metadata is the event's open channel; it merges into the item the event
	// builds (see the metadata rules).
	Metadata Metadata `json:"metadata,omitzero"`
}

// Base returns the envelope itself.
func (b *BaseEvent) Base() *BaseEvent { return b }

// Stamp sets Timestamp to the current time (milliseconds) when it is unset.
func (b *BaseEvent) Stamp() {
	if b.Timestamp == nil {
		ts := time.Now().UnixMilli()
		b.Timestamp = &ts
	}
}

// Attribution is embedded by the events that can belong to a subagent's work.
type Attribution struct {
	// SubagentRunID attributes the event to one subagent invocation; nil
	// means the parent agent.
	SubagentRunID *string `json:"subagentRunId,omitempty"`
}

// AttributedTo returns the subagent invocation the event is attributed to,
// or "" for the parent agent.
func (a *Attribution) AttributedTo() string { return Deref(a.SubagentRunID) }

// Attributable is implemented by events that may carry subagentRunId.
type Attributable interface {
	AttributedTo() string
}

// SubagentOf returns the subagentRunId an event is attributed to ("" for the
// parent agent, or for events that cannot carry attribution). The subagent
// lifecycle events return the invocation they concern.
func SubagentOf(e Event) string {
	switch t := e.(type) {
	case Attributable:
		return t.AttributedTo()
	case *SubagentStartedEvent:
		return t.SubagentRunID
	case *SubagentFinishedEvent:
		return t.SubagentRunID
	case *SubagentErrorEvent:
		return t.SubagentRunID
	}
	return ""
}

func stamped() BaseEvent {
	ts := time.Now().UnixMilli()
	return BaseEvent{Timestamp: &ts}
}

// =============================================================================
// Runs and steps
// =============================================================================

// RunStartedEvent opens a run.
type RunStartedEvent struct {
	BaseEvent
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
	// ProtocolVersion is the version the producer speaks — its own, never an
	// echo of the input's.
	ProtocolVersion *string `json:"protocolVersion,omitempty"`
	ParentRunID     *string `json:"parentRunId,omitempty"`
	// Input optionally echoes the input the run was started from.
	Input *RunAgentInput `json:"input,omitempty"`
}

// RunFinishedEvent closes a run that did not fail and reports how it ended.
type RunFinishedEvent struct {
	BaseEvent
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
	// Result is the run's return value; nil means absent.
	Result any `json:"result,omitempty"`
	// Outcome is nil (success), *SuccessOutcome, *InterruptOutcome or
	// *CancelledOutcome.
	Outcome RunFinishedOutcome `json:"outcome,omitempty"`
	Usage   []TokenUsage       `json:"usage,omitzero"`
}

// RunErrorEvent ends a run that failed.
type RunErrorEvent struct {
	BaseEvent
	Message string       `json:"message"`
	Code    *string      `json:"code,omitempty"`
	Usage   []TokenUsage `json:"usage,omitzero"`
}

// StepStartedEvent opens a step.
type StepStartedEvent struct {
	BaseEvent
	Attribution
	StepName string `json:"stepName"`
}

// StepFinishedEvent closes a step.
type StepFinishedEvent struct {
	BaseEvent
	Attribution
	StepName string `json:"stepName"`
}

// =============================================================================
// Text messages
// =============================================================================

// TextMessageStartEvent opens a text message.
type TextMessageStartEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
	// Role is optional; absent means assistant.
	Role *TextMessageRole `json:"role,omitempty"`
	Name *string          `json:"name,omitempty"`
}

// TextMessageContentEvent extends an open text message.
type TextMessageContentEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

// TextMessageEndEvent closes a text message.
type TextMessageEndEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
}

// TextMessageChunkEvent is the compact spelling of the text message triad.
// The first chunk of a message must carry MessageID.
type TextMessageChunkEvent struct {
	BaseEvent
	Attribution
	MessageID *string          `json:"messageId,omitempty"`
	Role      *TextMessageRole `json:"role,omitempty"`
	Delta     *string          `json:"delta,omitempty"`
	Name      *string          `json:"name,omitempty"`
}

// =============================================================================
// Tool calls
// =============================================================================

// ToolCallStartEvent opens a tool call.
type ToolCallStartEvent struct {
	BaseEvent
	Attribution
	ToolCallID      string  `json:"toolCallId"`
	ToolCallName    string  `json:"toolCallName"`
	ParentMessageID *string `json:"parentMessageId,omitempty"`
}

// ToolCallArgsEvent extends an open tool call's argument text.
type ToolCallArgsEvent struct {
	BaseEvent
	Attribution
	ToolCallID string `json:"toolCallId"`
	Delta      string `json:"delta"`
}

// ToolCallEndEvent closes a tool call: its argument text is complete.
type ToolCallEndEvent struct {
	BaseEvent
	Attribution
	ToolCallID string `json:"toolCallId"`
}

// ToolCallChunkEvent is the compact spelling of the tool call triad. The
// first chunk of a call must carry ToolCallID and ToolCallName.
type ToolCallChunkEvent struct {
	BaseEvent
	Attribution
	ToolCallID      *string `json:"toolCallId,omitempty"`
	ToolCallName    *string `json:"toolCallName,omitempty"`
	ParentMessageID *string `json:"parentMessageId,omitempty"`
	Delta           *string `json:"delta,omitempty"`
}

// ToolCallResultEvent carries the result of a call as a tool message of its
// own.
type ToolCallResultEvent struct {
	BaseEvent
	Attribution
	MessageID  string  `json:"messageId"`
	ToolCallID string  `json:"toolCallId"`
	Content    Content `json:"content"`
	// Role is optional and, when present, "tool".
	Role *Role `json:"role,omitempty"`
}

// =============================================================================
// Reasoning
// =============================================================================

// ReasoningStartEvent opens a reasoning span.
type ReasoningStartEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
}

// ReasoningEndEvent closes a reasoning span.
type ReasoningEndEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
}

// ReasoningMessageStartEvent opens a reasoning message (role "reasoning").
type ReasoningMessageStartEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
}

// ReasoningMessageContentEvent extends an open reasoning message.
type ReasoningMessageContentEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

// ReasoningMessageEndEvent closes a reasoning message.
type ReasoningMessageEndEvent struct {
	BaseEvent
	Attribution
	MessageID string `json:"messageId"`
}

// ReasoningMessageChunkEvent is the compact spelling of the reasoning message
// triad. The first chunk of a message must carry MessageID.
type ReasoningMessageChunkEvent struct {
	BaseEvent
	Attribution
	MessageID *string `json:"messageId,omitempty"`
	Delta     *string `json:"delta,omitempty"`
}

// ReasoningEncryptedValueSubtype says what kind of entity an encrypted value
// belongs to.
type ReasoningEncryptedValueSubtype string

// Encrypted value subtypes.
const (
	EncryptedValueSubtypeToolCall ReasoningEncryptedValueSubtype = "tool-call"
	EncryptedValueSubtypeMessage  ReasoningEncryptedValueSubtype = "message"
)

// ReasoningEncryptedValueEvent carries a provider's opaque reasoning artefact
// for the message or tool call EntityID names. A consumer stores it with that
// entity and returns it unmodified in later input.
type ReasoningEncryptedValueEvent struct {
	BaseEvent
	Attribution
	Subtype        ReasoningEncryptedValueSubtype `json:"subtype"`
	EntityID       string                         `json:"entityId"`
	EncryptedValue string                         `json:"encryptedValue"`
}

// =============================================================================
// State
// =============================================================================

// StateSnapshotEvent replaces the state wholesale. Snapshot may be any JSON
// value, including null.
type StateSnapshotEvent struct {
	BaseEvent
	Attribution
	Snapshot any `json:"snapshot"`
}

// StateDeltaEvent amends the state with an RFC 6902 patch.
type StateDeltaEvent struct {
	BaseEvent
	Attribution
	Delta JSONPatch `json:"delta"`
}

// MessagesSnapshotEvent restates the complete set of messages the producer
// owns; the consumer reconciles it by id.
type MessagesSnapshotEvent struct {
	BaseEvent
	Messages Messages `json:"messages"`
}

// =============================================================================
// Activity
// =============================================================================

// ActivitySnapshotEvent creates or replaces an activity message.
type ActivitySnapshotEvent struct {
	BaseEvent
	Attribution
	MessageID    string         `json:"messageId"`
	ActivityType string         `json:"activityType"`
	Content      map[string]any `json:"content"`
	// Replace is optional; absent means true. An explicit false leaves an
	// existing activity as it stands.
	Replace *bool `json:"replace,omitempty"`
}

// ActivityDeltaEvent amends an activity message's content with a patch.
type ActivityDeltaEvent struct {
	BaseEvent
	Attribution
	MessageID    string    `json:"messageId"`
	ActivityType string    `json:"activityType"`
	Patch        JSONPatch `json:"patch"`
}

// =============================================================================
// Subagents
// =============================================================================

// SubagentStartedEvent announces a subagent invocation.
type SubagentStartedEvent struct {
	BaseEvent
	SubagentRunID       string  `json:"subagentRunId"`
	Name                string  `json:"name"`
	Description         *string `json:"description,omitempty"`
	ParentSubagentRunID *string `json:"parentSubagentRunId,omitempty"`
	ParentToolCallID    *string `json:"parentToolCallId,omitempty"`
	ParentMessageID     *string `json:"parentMessageId,omitempty"`
}

// SubagentFinishedEvent closes a subagent invocation.
type SubagentFinishedEvent struct {
	BaseEvent
	SubagentRunID string `json:"subagentRunId"`
	// Result is the invocation's return value; nil means absent.
	Result any `json:"result,omitempty"`
	// Outcome is nil (success), *SubagentSuccessOutcome or
	// *SubagentSuspendedOutcome.
	Outcome SubagentFinishedOutcome `json:"outcome,omitempty"`
}

// SubagentErrorEvent reports that a subagent invocation failed. It ends the
// invocation, not the run.
type SubagentErrorEvent struct {
	BaseEvent
	SubagentRunID string  `json:"subagentRunId"`
	Message       string  `json:"message"`
	Code          *string `json:"code,omitempty"`
}

// =============================================================================
// Passthrough
// =============================================================================

// RawEvent passes a provider-native event through untranslated.
type RawEvent struct {
	BaseEvent
	Attribution
	Event  any     `json:"event"`
	Source *string `json:"source,omitempty"`
}

// CustomEvent is an application's own event. Names should be prefixed with a
// vendor or application identifier.
type CustomEvent struct {
	BaseEvent
	Attribution
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// =============================================================================
// Type(), MarshalJSON and UnmarshalJSON
// =============================================================================

func (*RunStartedEvent) Type() EventType              { return EventTypeRunStarted }
func (*RunFinishedEvent) Type() EventType             { return EventTypeRunFinished }
func (*RunErrorEvent) Type() EventType                { return EventTypeRunError }
func (*StepStartedEvent) Type() EventType             { return EventTypeStepStarted }
func (*StepFinishedEvent) Type() EventType            { return EventTypeStepFinished }
func (*TextMessageStartEvent) Type() EventType        { return EventTypeTextMessageStart }
func (*TextMessageContentEvent) Type() EventType      { return EventTypeTextMessageContent }
func (*TextMessageEndEvent) Type() EventType          { return EventTypeTextMessageEnd }
func (*TextMessageChunkEvent) Type() EventType        { return EventTypeTextMessageChunk }
func (*ToolCallStartEvent) Type() EventType           { return EventTypeToolCallStart }
func (*ToolCallArgsEvent) Type() EventType            { return EventTypeToolCallArgs }
func (*ToolCallEndEvent) Type() EventType             { return EventTypeToolCallEnd }
func (*ToolCallChunkEvent) Type() EventType           { return EventTypeToolCallChunk }
func (*ToolCallResultEvent) Type() EventType          { return EventTypeToolCallResult }
func (*ReasoningStartEvent) Type() EventType          { return EventTypeReasoningStart }
func (*ReasoningEndEvent) Type() EventType            { return EventTypeReasoningEnd }
func (*ReasoningMessageStartEvent) Type() EventType   { return EventTypeReasoningMessageStart }
func (*ReasoningMessageContentEvent) Type() EventType { return EventTypeReasoningMessageContent }
func (*ReasoningMessageEndEvent) Type() EventType     { return EventTypeReasoningMessageEnd }
func (*ReasoningMessageChunkEvent) Type() EventType   { return EventTypeReasoningMessageChunk }
func (*ReasoningEncryptedValueEvent) Type() EventType { return EventTypeReasoningEncryptedValue }
func (*StateSnapshotEvent) Type() EventType           { return EventTypeStateSnapshot }
func (*StateDeltaEvent) Type() EventType              { return EventTypeStateDelta }
func (*MessagesSnapshotEvent) Type() EventType        { return EventTypeMessagesSnapshot }
func (*ActivitySnapshotEvent) Type() EventType        { return EventTypeActivitySnapshot }
func (*ActivityDeltaEvent) Type() EventType           { return EventTypeActivityDelta }
func (*SubagentStartedEvent) Type() EventType         { return EventTypeSubagentStarted }
func (*SubagentFinishedEvent) Type() EventType        { return EventTypeSubagentFinished }
func (*SubagentErrorEvent) Type() EventType           { return EventTypeSubagentError }
func (*RawEvent) Type() EventType                     { return EventTypeRaw }
func (*CustomEvent) Type() EventType                  { return EventTypeCustom }

func tagEvent(t EventType, b []byte, err error) ([]byte, error) {
	return withTag("type", string(t), b, err)
}

func (e RunStartedEvent) MarshalJSON() ([]byte, error) {
	type plain RunStartedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeRunStarted, b, err)
}

func (e RunFinishedEvent) MarshalJSON() ([]byte, error) {
	type plain RunFinishedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeRunFinished, b, err)
}

func (e RunErrorEvent) MarshalJSON() ([]byte, error) {
	type plain RunErrorEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeRunError, b, err)
}

func (e StepStartedEvent) MarshalJSON() ([]byte, error) {
	type plain StepStartedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeStepStarted, b, err)
}

func (e StepFinishedEvent) MarshalJSON() ([]byte, error) {
	type plain StepFinishedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeStepFinished, b, err)
}

func (e TextMessageStartEvent) MarshalJSON() ([]byte, error) {
	type plain TextMessageStartEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeTextMessageStart, b, err)
}

func (e TextMessageContentEvent) MarshalJSON() ([]byte, error) {
	type plain TextMessageContentEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeTextMessageContent, b, err)
}

func (e TextMessageEndEvent) MarshalJSON() ([]byte, error) {
	type plain TextMessageEndEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeTextMessageEnd, b, err)
}

func (e TextMessageChunkEvent) MarshalJSON() ([]byte, error) {
	type plain TextMessageChunkEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeTextMessageChunk, b, err)
}

func (e ToolCallStartEvent) MarshalJSON() ([]byte, error) {
	type plain ToolCallStartEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeToolCallStart, b, err)
}

func (e ToolCallArgsEvent) MarshalJSON() ([]byte, error) {
	type plain ToolCallArgsEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeToolCallArgs, b, err)
}

func (e ToolCallEndEvent) MarshalJSON() ([]byte, error) {
	type plain ToolCallEndEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeToolCallEnd, b, err)
}

func (e ToolCallChunkEvent) MarshalJSON() ([]byte, error) {
	type plain ToolCallChunkEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeToolCallChunk, b, err)
}

func (e ToolCallResultEvent) MarshalJSON() ([]byte, error) {
	type plain ToolCallResultEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeToolCallResult, b, err)
}

func (e ReasoningStartEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningStartEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningStart, b, err)
}

func (e ReasoningEndEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningEndEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningEnd, b, err)
}

func (e ReasoningMessageStartEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningMessageStartEvent
	b, err := json.Marshal(struct {
		plain
		Role Role `json:"role"`
	}{plain(e), RoleReasoning})
	return tagEvent(EventTypeReasoningMessageStart, b, err)
}

func (e ReasoningMessageContentEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningMessageContentEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningMessageContent, b, err)
}

func (e ReasoningMessageEndEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningMessageEndEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningMessageEnd, b, err)
}

func (e ReasoningMessageChunkEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningMessageChunkEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningMessageChunk, b, err)
}

func (e ReasoningEncryptedValueEvent) MarshalJSON() ([]byte, error) {
	type plain ReasoningEncryptedValueEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeReasoningEncryptedValue, b, err)
}

func (e StateSnapshotEvent) MarshalJSON() ([]byte, error) {
	type plain StateSnapshotEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeStateSnapshot, b, err)
}

func (e StateDeltaEvent) MarshalJSON() ([]byte, error) {
	type plain StateDeltaEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeStateDelta, b, err)
}

func (e MessagesSnapshotEvent) MarshalJSON() ([]byte, error) {
	type plain MessagesSnapshotEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeMessagesSnapshot, b, err)
}

func (e ActivitySnapshotEvent) MarshalJSON() ([]byte, error) {
	type plain ActivitySnapshotEvent
	if e.Content == nil {
		e.Content = map[string]any{}
	}
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeActivitySnapshot, b, err)
}

func (e ActivityDeltaEvent) MarshalJSON() ([]byte, error) {
	type plain ActivityDeltaEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeActivityDelta, b, err)
}

func (e SubagentStartedEvent) MarshalJSON() ([]byte, error) {
	type plain SubagentStartedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeSubagentStarted, b, err)
}

func (e SubagentFinishedEvent) MarshalJSON() ([]byte, error) {
	type plain SubagentFinishedEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeSubagentFinished, b, err)
}

func (e SubagentErrorEvent) MarshalJSON() ([]byte, error) {
	type plain SubagentErrorEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeSubagentError, b, err)
}

func (e RawEvent) MarshalJSON() ([]byte, error) {
	type plain RawEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeRaw, b, err)
}

func (e CustomEvent) MarshalJSON() ([]byte, error) {
	type plain CustomEvent
	b, err := json.Marshal(plain(e))
	return tagEvent(EventTypeCustom, b, err)
}

// UnmarshalJSON decodes the union-typed outcome.
func (e *RunFinishedEvent) UnmarshalJSON(data []byte) error {
	type plain RunFinishedEvent
	data = filterExact(data, reflect.TypeOf(plain{}))
	aux := struct {
		*plain
		Outcome json.RawMessage `json:"outcome"`
	}{plain: (*plain)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.Outcome = nil
	if !isNull(aux.Outcome) {
		o, err := UnmarshalRunFinishedOutcome(aux.Outcome)
		if err != nil {
			return err
		}
		e.Outcome = o
	}
	return nil
}

// UnmarshalJSON decodes the union-typed outcome.
func (e *SubagentFinishedEvent) UnmarshalJSON(data []byte) error {
	type plain SubagentFinishedEvent
	data = filterExact(data, reflect.TypeOf(plain{}))
	aux := struct {
		*plain
		Outcome json.RawMessage `json:"outcome"`
	}{plain: (*plain)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.Outcome = nil
	if !isNull(aux.Outcome) {
		o, err := UnmarshalSubagentFinishedOutcome(aux.Outcome)
		if err != nil {
			return err
		}
		e.Outcome = o
	}
	return nil
}

// ErrUnknownEventType is returned by UnmarshalEvent for an event type this
// version of the protocol does not describe.
var ErrUnknownEventType = errors.New("ag-ui: unknown event type")

// NewEvent returns a zero event of the given type.
func NewEvent(t EventType) (Event, error) {
	switch t {
	case EventTypeRunStarted:
		return &RunStartedEvent{}, nil
	case EventTypeRunFinished:
		return &RunFinishedEvent{}, nil
	case EventTypeRunError:
		return &RunErrorEvent{}, nil
	case EventTypeStepStarted:
		return &StepStartedEvent{}, nil
	case EventTypeStepFinished:
		return &StepFinishedEvent{}, nil
	case EventTypeTextMessageStart:
		return &TextMessageStartEvent{}, nil
	case EventTypeTextMessageContent:
		return &TextMessageContentEvent{}, nil
	case EventTypeTextMessageEnd:
		return &TextMessageEndEvent{}, nil
	case EventTypeTextMessageChunk:
		return &TextMessageChunkEvent{}, nil
	case EventTypeToolCallStart:
		return &ToolCallStartEvent{}, nil
	case EventTypeToolCallArgs:
		return &ToolCallArgsEvent{}, nil
	case EventTypeToolCallEnd:
		return &ToolCallEndEvent{}, nil
	case EventTypeToolCallChunk:
		return &ToolCallChunkEvent{}, nil
	case EventTypeToolCallResult:
		return &ToolCallResultEvent{}, nil
	case EventTypeReasoningStart:
		return &ReasoningStartEvent{}, nil
	case EventTypeReasoningEnd:
		return &ReasoningEndEvent{}, nil
	case EventTypeReasoningMessageStart:
		return &ReasoningMessageStartEvent{}, nil
	case EventTypeReasoningMessageContent:
		return &ReasoningMessageContentEvent{}, nil
	case EventTypeReasoningMessageEnd:
		return &ReasoningMessageEndEvent{}, nil
	case EventTypeReasoningMessageChunk:
		return &ReasoningMessageChunkEvent{}, nil
	case EventTypeReasoningEncryptedValue:
		return &ReasoningEncryptedValueEvent{}, nil
	case EventTypeStateSnapshot:
		return &StateSnapshotEvent{}, nil
	case EventTypeStateDelta:
		return &StateDeltaEvent{}, nil
	case EventTypeMessagesSnapshot:
		return &MessagesSnapshotEvent{}, nil
	case EventTypeActivitySnapshot:
		return &ActivitySnapshotEvent{}, nil
	case EventTypeActivityDelta:
		return &ActivityDeltaEvent{}, nil
	case EventTypeSubagentStarted:
		return &SubagentStartedEvent{}, nil
	case EventTypeSubagentFinished:
		return &SubagentFinishedEvent{}, nil
	case EventTypeSubagentError:
		return &SubagentErrorEvent{}, nil
	case EventTypeRaw:
		return &RawEvent{}, nil
	case EventTypeCustom:
		return &CustomEvent{}, nil
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownEventType, t)
}

// UnmarshalEvent decodes an event into its Go type by "type". It does not
// validate: use ParseEvent for a schema-checked decode, or the client
// pipeline for material from a peer.
func UnmarshalEvent(data []byte) (Event, error) {
	t, err := peekString(data, "type")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: event: %w", err)
	}
	e, err := NewEvent(EventType(t))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, e); err != nil {
		return nil, err
	}
	return e, nil
}

// ParseEvent validates an event against the 1.0 schema (exactly, as Validate
// does) and decodes it.
func ParseEvent(data []byte) (Event, error) {
	v, err := decodeGeneric(data)
	if err != nil {
		return nil, err
	}
	checked, err := validated(agSchema().root, v)
	if err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(checked)
	if err != nil {
		return nil, err
	}
	return UnmarshalEvent(canonical)
}

// ValidateEvent checks that a Go event marshals to a document the 1.0 schema
// accepts. Producers use it to make sure they emit only what the protocol
// defines.
func ValidateEvent(e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return Validate("Event", b)
}

// =============================================================================
// Constructors
// =============================================================================
//
// Constructors fill the required fields and stamp the event with the current
// time; optional fields are set on the returned value.

// NewRunStartedEvent returns a RUN_STARTED declaring ProtocolVersion.
func NewRunStartedEvent(threadID, runID string) *RunStartedEvent {
	return &RunStartedEvent{BaseEvent: stamped(), ThreadID: threadID, RunID: runID, ProtocolVersion: Ptr(ProtocolVersion)}
}

// NewRunFinishedEvent returns a RUN_FINISHED (success, no outcome).
func NewRunFinishedEvent(threadID, runID string) *RunFinishedEvent {
	return &RunFinishedEvent{BaseEvent: stamped(), ThreadID: threadID, RunID: runID}
}

// NewRunErrorEvent returns a RUN_ERROR.
func NewRunErrorEvent(message string) *RunErrorEvent {
	return &RunErrorEvent{BaseEvent: stamped(), Message: message}
}

// NewStepStartedEvent returns a STEP_STARTED.
func NewStepStartedEvent(stepName string) *StepStartedEvent {
	return &StepStartedEvent{BaseEvent: stamped(), StepName: stepName}
}

// NewStepFinishedEvent returns a STEP_FINISHED.
func NewStepFinishedEvent(stepName string) *StepFinishedEvent {
	return &StepFinishedEvent{BaseEvent: stamped(), StepName: stepName}
}

// NewTextMessageStartEvent returns a TEXT_MESSAGE_START with role assistant.
func NewTextMessageStartEvent(messageID string) *TextMessageStartEvent {
	return &TextMessageStartEvent{BaseEvent: stamped(), MessageID: messageID, Role: Ptr(TextMessageRoleAssistant)}
}

// NewTextMessageContentEvent returns a TEXT_MESSAGE_CONTENT.
func NewTextMessageContentEvent(messageID, delta string) *TextMessageContentEvent {
	return &TextMessageContentEvent{BaseEvent: stamped(), MessageID: messageID, Delta: delta}
}

// NewTextMessageEndEvent returns a TEXT_MESSAGE_END.
func NewTextMessageEndEvent(messageID string) *TextMessageEndEvent {
	return &TextMessageEndEvent{BaseEvent: stamped(), MessageID: messageID}
}

// NewTextMessageChunkEvent returns a TEXT_MESSAGE_CHUNK. Pass the message id on
// the first chunk of a message and "" on continuations.
func NewTextMessageChunkEvent(messageID, delta string) *TextMessageChunkEvent {
	e := &TextMessageChunkEvent{BaseEvent: stamped(), Delta: Ptr(delta)}
	if messageID != "" {
		e.MessageID = Ptr(messageID)
	}
	return e
}

// NewToolCallStartEvent returns a TOOL_CALL_START.
func NewToolCallStartEvent(toolCallID, toolCallName string) *ToolCallStartEvent {
	return &ToolCallStartEvent{BaseEvent: stamped(), ToolCallID: toolCallID, ToolCallName: toolCallName}
}

// NewToolCallArgsEvent returns a TOOL_CALL_ARGS.
func NewToolCallArgsEvent(toolCallID, delta string) *ToolCallArgsEvent {
	return &ToolCallArgsEvent{BaseEvent: stamped(), ToolCallID: toolCallID, Delta: delta}
}

// NewToolCallEndEvent returns a TOOL_CALL_END.
func NewToolCallEndEvent(toolCallID string) *ToolCallEndEvent {
	return &ToolCallEndEvent{BaseEvent: stamped(), ToolCallID: toolCallID}
}

// NewToolCallChunkEvent returns a TOOL_CALL_CHUNK. Pass the id and name on the
// first chunk of a call and "" on continuations.
func NewToolCallChunkEvent(toolCallID, toolCallName, delta string) *ToolCallChunkEvent {
	e := &ToolCallChunkEvent{BaseEvent: stamped(), Delta: Ptr(delta)}
	if toolCallID != "" {
		e.ToolCallID = Ptr(toolCallID)
	}
	if toolCallName != "" {
		e.ToolCallName = Ptr(toolCallName)
	}
	return e
}

// NewToolCallResultEvent returns a TOOL_CALL_RESULT with string content.
func NewToolCallResultEvent(messageID, toolCallID, content string) *ToolCallResultEvent {
	return &ToolCallResultEvent{BaseEvent: stamped(), MessageID: messageID, ToolCallID: toolCallID, Content: TextContent(content), Role: Ptr(RoleTool)}
}

// NewReasoningStartEvent returns a REASONING_START.
func NewReasoningStartEvent(messageID string) *ReasoningStartEvent {
	return &ReasoningStartEvent{BaseEvent: stamped(), MessageID: messageID}
}

// NewReasoningEndEvent returns a REASONING_END.
func NewReasoningEndEvent(messageID string) *ReasoningEndEvent {
	return &ReasoningEndEvent{BaseEvent: stamped(), MessageID: messageID}
}

// NewReasoningMessageStartEvent returns a REASONING_MESSAGE_START.
func NewReasoningMessageStartEvent(messageID string) *ReasoningMessageStartEvent {
	return &ReasoningMessageStartEvent{BaseEvent: stamped(), MessageID: messageID}
}

// NewReasoningMessageContentEvent returns a REASONING_MESSAGE_CONTENT.
func NewReasoningMessageContentEvent(messageID, delta string) *ReasoningMessageContentEvent {
	return &ReasoningMessageContentEvent{BaseEvent: stamped(), MessageID: messageID, Delta: delta}
}

// NewReasoningMessageEndEvent returns a REASONING_MESSAGE_END.
func NewReasoningMessageEndEvent(messageID string) *ReasoningMessageEndEvent {
	return &ReasoningMessageEndEvent{BaseEvent: stamped(), MessageID: messageID}
}

// NewReasoningMessageChunkEvent returns a REASONING_MESSAGE_CHUNK. Pass the
// message id on the first chunk and "" on continuations.
func NewReasoningMessageChunkEvent(messageID, delta string) *ReasoningMessageChunkEvent {
	e := &ReasoningMessageChunkEvent{BaseEvent: stamped(), Delta: Ptr(delta)}
	if messageID != "" {
		e.MessageID = Ptr(messageID)
	}
	return e
}

// NewReasoningEncryptedValueEvent returns a REASONING_ENCRYPTED_VALUE.
func NewReasoningEncryptedValueEvent(subtype ReasoningEncryptedValueSubtype, entityID, encryptedValue string) *ReasoningEncryptedValueEvent {
	return &ReasoningEncryptedValueEvent{BaseEvent: stamped(), Subtype: subtype, EntityID: entityID, EncryptedValue: encryptedValue}
}

// NewStateSnapshotEvent returns a STATE_SNAPSHOT.
func NewStateSnapshotEvent(snapshot any) *StateSnapshotEvent {
	return &StateSnapshotEvent{BaseEvent: stamped(), Snapshot: snapshot}
}

// NewStateDeltaEvent returns a STATE_DELTA.
func NewStateDeltaEvent(delta ...PatchOperation) *StateDeltaEvent {
	return &StateDeltaEvent{BaseEvent: stamped(), Delta: delta}
}

// NewMessagesSnapshotEvent returns a MESSAGES_SNAPSHOT.
func NewMessagesSnapshotEvent(messages ...Message) *MessagesSnapshotEvent {
	return &MessagesSnapshotEvent{BaseEvent: stamped(), Messages: messages}
}

// NewActivitySnapshotEvent returns an ACTIVITY_SNAPSHOT (replace absent,
// meaning true).
func NewActivitySnapshotEvent(messageID, activityType string, content map[string]any) *ActivitySnapshotEvent {
	return &ActivitySnapshotEvent{BaseEvent: stamped(), MessageID: messageID, ActivityType: activityType, Content: content}
}

// NewActivityDeltaEvent returns an ACTIVITY_DELTA.
func NewActivityDeltaEvent(messageID, activityType string, patch ...PatchOperation) *ActivityDeltaEvent {
	return &ActivityDeltaEvent{BaseEvent: stamped(), MessageID: messageID, ActivityType: activityType, Patch: patch}
}

// NewSubagentStartedEvent returns a SUBAGENT_STARTED.
func NewSubagentStartedEvent(subagentRunID, name string) *SubagentStartedEvent {
	return &SubagentStartedEvent{BaseEvent: stamped(), SubagentRunID: subagentRunID, Name: name}
}

// NewSubagentFinishedEvent returns a SUBAGENT_FINISHED (success).
func NewSubagentFinishedEvent(subagentRunID string) *SubagentFinishedEvent {
	return &SubagentFinishedEvent{BaseEvent: stamped(), SubagentRunID: subagentRunID}
}

// NewSubagentErrorEvent returns a SUBAGENT_ERROR.
func NewSubagentErrorEvent(subagentRunID, message string) *SubagentErrorEvent {
	return &SubagentErrorEvent{BaseEvent: stamped(), SubagentRunID: subagentRunID, Message: message}
}

// NewRawEvent returns a RAW event.
func NewRawEvent(event any) *RawEvent {
	return &RawEvent{BaseEvent: stamped(), Event: event}
}

// NewCustomEvent returns a CUSTOM event.
func NewCustomEvent(name string, value any) *CustomEvent {
	return &CustomEvent{BaseEvent: stamped(), Name: name, Value: value}
}

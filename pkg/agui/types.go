// Package agui is a Go implementation of the AG-UI protocol, version 1.0
// (https://docs.ag-ui.com/spec/1.0).
//
// The package carries the protocol's data model — events, messages, content
// parts, the run input, outcomes, capabilities — as Go types that serialise to
// exactly the wire format the 1.0 schema defines: optional fields with no value
// are omitted (never null), discriminators are always set, and required
// fields are always present.
//
// It also carries the protocol's rules: Validate checks a document against the
// official schema (embedded verbatim), Sanitize applies the processing model to
// material from a peer (strip unrecognised material with a warning, fail on
// malformed known values), and Verifier enforces the ordering rules of a
// stream. The producer helpers in package stream and server, and the consumer
// in package client, are built on these.
package agui

import (
	"encoding/json"
	"fmt"
)

// =============================================================================
// Roles
// =============================================================================

// Role is a message role.
type Role string

// Message roles.
const (
	RoleDeveloper Role = "developer"
	RoleSystem    Role = "system"
	RoleAssistant Role = "assistant"
	RoleUser      Role = "user"
	RoleTool      Role = "tool"
	RoleActivity  Role = "activity"
	RoleReasoning Role = "reasoning"
)

// TextMessageRole is the role a text message stream may carry.
type TextMessageRole string

// Text message roles. An absent role means assistant.
const (
	TextMessageRoleDeveloper TextMessageRole = "developer"
	TextMessageRoleSystem    TextMessageRole = "system"
	TextMessageRoleAssistant TextMessageRole = "assistant"
	TextMessageRoleUser      TextMessageRole = "user"
)

// =============================================================================
// Messages
// =============================================================================

// Message is one message of the conversation: *DeveloperMessage,
// *SystemMessage, *AssistantMessage, *UserMessage, *ToolMessage,
// *ActivityMessage or *ReasoningMessage.
type Message interface {
	// MessageID returns the message's id.
	MessageID() string
	// MessageRole returns the message's role.
	MessageRole() Role
	isMessage()
}

// DeveloperMessage is a message from the developer.
type DeveloperMessage struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Name           *string  `json:"name,omitempty"`
	EncryptedValue *string  `json:"encryptedValue,omitempty"`
	Metadata       Metadata `json:"metadata,omitzero"`
	SubagentRunID  *string  `json:"subagentRunId,omitempty"`
}

// SystemMessage is a system prompt.
type SystemMessage struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Name           *string  `json:"name,omitempty"`
	EncryptedValue *string  `json:"encryptedValue,omitempty"`
	Metadata       Metadata `json:"metadata,omitzero"`
	SubagentRunID  *string  `json:"subagentRunId,omitempty"`
}

// AssistantMessage is a message from the agent. Content is optional: an
// assistant message may carry only tool calls.
type AssistantMessage struct {
	ID             string     `json:"id"`
	Content        *string    `json:"content,omitempty"`
	ToolCalls      []ToolCall `json:"toolCalls,omitzero"`
	Name           *string    `json:"name,omitempty"`
	EncryptedValue *string    `json:"encryptedValue,omitempty"`
	Metadata       Metadata   `json:"metadata,omitzero"`
	SubagentRunID  *string    `json:"subagentRunId,omitempty"`
}

// UserMessage is a message from the user: a string, or multimodal parts.
type UserMessage struct {
	ID             string   `json:"id"`
	Content        Content  `json:"content"`
	Name           *string  `json:"name,omitempty"`
	EncryptedValue *string  `json:"encryptedValue,omitempty"`
	Metadata       Metadata `json:"metadata,omitzero"`
	SubagentRunID  *string  `json:"subagentRunId,omitempty"`
}

// ToolMessage answers a tool call. Error set means the call failed; it is
// still an answer.
type ToolMessage struct {
	ID             string   `json:"id"`
	Content        Content  `json:"content"`
	ToolCallID     string   `json:"toolCallId"`
	Error          *string  `json:"error,omitempty"`
	EncryptedValue *string  `json:"encryptedValue,omitempty"`
	Metadata       Metadata `json:"metadata,omitzero"`
	SubagentRunID  *string  `json:"subagentRunId,omitempty"`
}

// ActivityMessage is structured progress rendered by the consumer. Activity
// messages never travel back to the producer: a consumer strips them from
// the run input.
type ActivityMessage struct {
	ID            string         `json:"id"`
	ActivityType  string         `json:"activityType"`
	Content       map[string]any `json:"content"`
	Metadata      Metadata       `json:"metadata,omitzero"`
	SubagentRunID *string        `json:"subagentRunId,omitempty"`
}

// ReasoningMessage is the model's visible reasoning.
type ReasoningMessage struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	EncryptedValue *string  `json:"encryptedValue,omitempty"`
	Metadata       Metadata `json:"metadata,omitzero"`
	SubagentRunID  *string  `json:"subagentRunId,omitempty"`
}

func (m *DeveloperMessage) MessageID() string { return m.ID }
func (m *SystemMessage) MessageID() string    { return m.ID }
func (m *AssistantMessage) MessageID() string { return m.ID }
func (m *UserMessage) MessageID() string      { return m.ID }
func (m *ToolMessage) MessageID() string      { return m.ID }
func (m *ActivityMessage) MessageID() string  { return m.ID }
func (m *ReasoningMessage) MessageID() string { return m.ID }

func (*DeveloperMessage) MessageRole() Role { return RoleDeveloper }
func (*SystemMessage) MessageRole() Role    { return RoleSystem }
func (*AssistantMessage) MessageRole() Role { return RoleAssistant }
func (*UserMessage) MessageRole() Role      { return RoleUser }
func (*ToolMessage) MessageRole() Role      { return RoleTool }
func (*ActivityMessage) MessageRole() Role  { return RoleActivity }
func (*ReasoningMessage) MessageRole() Role { return RoleReasoning }

func (*DeveloperMessage) isMessage() {}
func (*SystemMessage) isMessage()    {}
func (*AssistantMessage) isMessage() {}
func (*UserMessage) isMessage()      {}
func (*ToolMessage) isMessage()      {}
func (*ActivityMessage) isMessage()  {}
func (*ReasoningMessage) isMessage() {}

func (m DeveloperMessage) MarshalJSON() ([]byte, error) {
	type plain DeveloperMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleDeveloper), b, err)
}

func (m SystemMessage) MarshalJSON() ([]byte, error) {
	type plain SystemMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleSystem), b, err)
}

func (m AssistantMessage) MarshalJSON() ([]byte, error) {
	type plain AssistantMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleAssistant), b, err)
}

func (m UserMessage) MarshalJSON() ([]byte, error) {
	type plain UserMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleUser), b, err)
}

func (m ToolMessage) MarshalJSON() ([]byte, error) {
	type plain ToolMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleTool), b, err)
}

func (m ActivityMessage) MarshalJSON() ([]byte, error) {
	type plain ActivityMessage
	if m.Content == nil {
		m.Content = map[string]any{}
	}
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleActivity), b, err)
}

func (m ReasoningMessage) MarshalJSON() ([]byte, error) {
	type plain ReasoningMessage
	b, err := json.Marshal(plain(m))
	return withTag("role", string(RoleReasoning), b, err)
}

// UnmarshalMessage decodes one message by its "role".
func UnmarshalMessage(data []byte) (Message, error) {
	role, err := peekString(data, "role")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: message: %w", err)
	}
	var m Message
	switch Role(role) {
	case RoleDeveloper:
		m = &DeveloperMessage{}
	case RoleSystem:
		m = &SystemMessage{}
	case RoleAssistant:
		m = &AssistantMessage{}
	case RoleUser:
		m = &UserMessage{}
	case RoleTool:
		m = &ToolMessage{}
	case RoleActivity:
		m = &ActivityMessage{}
	case RoleReasoning:
		m = &ReasoningMessage{}
	default:
		return nil, fmt.Errorf("ag-ui: unrecognised message role %q", role)
	}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Messages is an ordered list of messages. It marshals as [] when empty.
type Messages []Message

// MarshalJSON implements json.Marshaler.
func (ms Messages) MarshalJSON() ([]byte, error) {
	return json.Marshal(emptyIfNil([]Message(ms)))
}

// UnmarshalJSON implements json.Unmarshaler.
func (ms *Messages) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("ag-ui: messages must be an array")
	}
	out := make(Messages, 0, len(raws))
	for _, r := range raws {
		m, err := UnmarshalMessage(r)
		if err != nil {
			return err
		}
		out = append(out, m)
	}
	*ms = out
	return nil
}

// =============================================================================
// Tool calls, tools and context
// =============================================================================

// FunctionCall is the name and argument text of a call. Arguments is
// conventionally a JSON document, carried as text and never validated by the
// protocol.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall is a call carried on an assistant message.
type ToolCall struct {
	ID             string       `json:"id"`
	Function       FunctionCall `json:"function"`
	EncryptedValue *string      `json:"encryptedValue,omitempty"`
	Metadata       Metadata     `json:"metadata,omitzero"`
}

// NewToolCall returns a function tool call.
func NewToolCall(id, name, arguments string) ToolCall {
	return ToolCall{ID: id, Function: FunctionCall{Name: name, Arguments: arguments}}
}

func (t ToolCall) MarshalJSON() ([]byte, error) {
	type plain ToolCall
	b, err := json.Marshal(plain(t))
	return withTag("type", "function", b, err)
}

// Tool is a tool the agent may call: in RunAgentInput, a frontend tool the
// application executes; in capabilities, one the agent provides.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Parameters is a JSON Schema for the arguments, carried opaquely. Nil
	// means absent (no arguments).
	Parameters any      `json:"parameters,omitempty"`
	Metadata   Metadata `json:"metadata,omitzero"`
}

// Context is ambient information for the run, to be made available to the
// model.
type Context struct {
	Description string `json:"description"`
	Value       string `json:"value"`
}

// =============================================================================
// Run input
// =============================================================================

// RunAgentInput is the request that opens an exchange (the HTTP POST body).
type RunAgentInput struct {
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
	// ProtocolVersion is the version the consumer speaks ("1.0"). Absent
	// identifies a peer from before the protocol carried a version.
	ProtocolVersion *string `json:"protocolVersion,omitempty"`
	ParentRunID     *string `json:"parentRunId,omitempty"`
	// State is the state the run starts from; nil (absent) means {}.
	State          any       `json:"state,omitempty"`
	Messages       Messages  `json:"messages"`
	Tools          []Tool    `json:"tools,omitzero"`
	Context        []Context `json:"context,omitzero"`
	ForwardedProps any       `json:"forwardedProps,omitempty"`
	// Resume answers the interrupts of the run being continued.
	Resume []ResumeEntry `json:"resume,omitzero"`
}

// StartingState returns the state the run starts from: State, or the empty
// object when the input carried none.
func (in *RunAgentInput) StartingState() any {
	if in.State == nil {
		return map[string]any{}
	}
	return in.State
}

// =============================================================================
// Interrupts and resume
// =============================================================================

// Interrupt is something a run is waiting for: an approval, a value, a choice.
type Interrupt struct {
	// ID is unique within the run; a resume entry answers it by this id.
	ID string `json:"id"`
	// Reason is an open string ("tool_approval", ...).
	Reason string `json:"reason"`
	// Message is a human-readable prompt for whoever answers.
	Message *string `json:"message,omitempty"`
	// ToolCallID names the tool call an approval concerns.
	ToolCallID *string `json:"toolCallId,omitempty"`
	// ResponseSchema is a JSON Schema for the expected answer.
	ResponseSchema map[string]any `json:"responseSchema,omitzero"`
	// ExpiresAt is conventionally an ISO 8601 timestamp.
	ExpiresAt     *string  `json:"expiresAt,omitempty"`
	Metadata      Metadata `json:"metadata,omitzero"`
	SubagentRunID *string  `json:"subagentRunId,omitempty"`
}

// ResumeStatus says whether an interrupt was answered or abandoned.
type ResumeStatus string

// Resume statuses.
const (
	ResumeStatusResolved  ResumeStatus = "resolved"
	ResumeStatusCancelled ResumeStatus = "cancelled"
)

// ResumeEntry answers one interrupt of the run being continued.
type ResumeEntry struct {
	InterruptID string       `json:"interruptId"`
	Status      ResumeStatus `json:"status"`
	// Payload is the answer, any JSON value; nil means absent.
	Payload  any      `json:"payload,omitempty"`
	Metadata Metadata `json:"metadata,omitzero"`
}

// =============================================================================
// Token usage
// =============================================================================

// TokenUsage reports one provider/model's token counts for a run. Counts are
// pointers because an absent count (not reported) differs from zero.
type TokenUsage struct {
	Provider *string `json:"provider,omitempty"`
	Model    *string `json:"model,omitempty"`
	// InputTokens counts every prompt token charged, cached or not.
	InputTokens *int64 `json:"inputTokens,omitempty"`
	// OutputTokens counts every generated token, reasoning included.
	OutputTokens *int64 `json:"outputTokens,omitempty"`
	// TotalTokens is InputTokens + OutputTokens.
	TotalTokens *int64 `json:"totalTokens,omitempty"`
	// ReasoningTokens is the part of OutputTokens spent reasoning.
	ReasoningTokens *int64 `json:"reasoningTokens,omitempty"`
	// CachedInputTokens is the part of InputTokens read from a cache.
	CachedInputTokens *int64 `json:"cachedInputTokens,omitempty"`
	// CacheWriteInputTokens is the part of InputTokens written to a cache.
	CacheWriteInputTokens *int64 `json:"cacheWriteInputTokens,omitempty"`
}

// =============================================================================
// Outcomes
// =============================================================================

// RunFinishedOutcome reports how a run ended: *SuccessOutcome,
// *InterruptOutcome or *CancelledOutcome. A nil outcome means success.
type RunFinishedOutcome interface {
	OutcomeType() string
	isRunFinishedOutcome()
}

// Outcome discriminators.
const (
	OutcomeSuccess   = "success"
	OutcomeInterrupt = "interrupt"
	OutcomeCancelled = "cancelled"
	OutcomeSuspended = "suspended"
)

// SuccessOutcome is a run that completed. PendingToolCallIDs names the
// frontend tool calls the run left unanswered, in call order.
type SuccessOutcome struct {
	PendingToolCallIDs []string `json:"pendingToolCallIds,omitzero"`
}

// InterruptOutcome is a run that stopped to ask for outside input. It
// carries at least one interrupt.
type InterruptOutcome struct {
	Interrupts []Interrupt `json:"interrupts"`
}

// CancelledOutcome is a run stopped on purpose before it completed.
type CancelledOutcome struct{}

func (*SuccessOutcome) OutcomeType() string     { return OutcomeSuccess }
func (*InterruptOutcome) OutcomeType() string   { return OutcomeInterrupt }
func (*CancelledOutcome) OutcomeType() string   { return OutcomeCancelled }
func (*SuccessOutcome) isRunFinishedOutcome()   {}
func (*InterruptOutcome) isRunFinishedOutcome() {}
func (*CancelledOutcome) isRunFinishedOutcome() {}

func (o SuccessOutcome) MarshalJSON() ([]byte, error) {
	type plain SuccessOutcome
	b, err := json.Marshal(plain(o))
	return withTag("type", OutcomeSuccess, b, err)
}

func (o InterruptOutcome) MarshalJSON() ([]byte, error) {
	if len(o.Interrupts) == 0 {
		return nil, fmt.Errorf("ag-ui: an interrupt outcome carries at least one interrupt")
	}
	type plain InterruptOutcome
	b, err := json.Marshal(plain(o))
	return withTag("type", OutcomeInterrupt, b, err)
}

func (o CancelledOutcome) MarshalJSON() ([]byte, error) {
	return []byte(`{"type":"cancelled"}`), nil
}

// UnmarshalRunFinishedOutcome decodes a RUN_FINISHED outcome by its "type".
func UnmarshalRunFinishedOutcome(data []byte) (RunFinishedOutcome, error) {
	kind, err := peekString(data, "type")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: outcome: %w", err)
	}
	var o RunFinishedOutcome
	switch kind {
	case OutcomeSuccess:
		o = &SuccessOutcome{}
	case OutcomeInterrupt:
		o = &InterruptOutcome{}
	case OutcomeCancelled:
		o = &CancelledOutcome{}
	default:
		return nil, fmt.Errorf("ag-ui: unrecognised outcome type %q", kind)
	}
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

// SubagentFinishedOutcome reports why a subagent invocation ended:
// *SubagentSuccessOutcome or *SubagentSuspendedOutcome. Nil means success.
type SubagentFinishedOutcome interface {
	OutcomeType() string
	isSubagentFinishedOutcome()
}

// SubagentSuccessOutcome is an invocation that completed.
type SubagentSuccessOutcome struct{}

// SubagentSuspendedOutcome is an invocation paused awaiting outside input,
// which a later run may continue under the same subagentRunId.
type SubagentSuspendedOutcome struct {
	InterruptIDs []string `json:"interruptIds,omitzero"`
}

func (*SubagentSuccessOutcome) OutcomeType() string          { return OutcomeSuccess }
func (*SubagentSuspendedOutcome) OutcomeType() string        { return OutcomeSuspended }
func (*SubagentSuccessOutcome) isSubagentFinishedOutcome()   {}
func (*SubagentSuspendedOutcome) isSubagentFinishedOutcome() {}

func (o SubagentSuccessOutcome) MarshalJSON() ([]byte, error) {
	return []byte(`{"type":"success"}`), nil
}

func (o SubagentSuspendedOutcome) MarshalJSON() ([]byte, error) {
	type plain SubagentSuspendedOutcome
	b, err := json.Marshal(plain(o))
	return withTag("type", OutcomeSuspended, b, err)
}

// UnmarshalSubagentFinishedOutcome decodes a SUBAGENT_FINISHED outcome.
func UnmarshalSubagentFinishedOutcome(data []byte) (SubagentFinishedOutcome, error) {
	kind, err := peekString(data, "type")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: outcome: %w", err)
	}
	var o SubagentFinishedOutcome
	switch kind {
	case OutcomeSuccess:
		o = &SubagentSuccessOutcome{}
	case OutcomeSuspended:
		o = &SubagentSuspendedOutcome{}
	default:
		return nil, fmt.Errorf("ag-ui: unrecognised subagent outcome type %q", kind)
	}
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

// Code generated for case-exact JSON decoding; see exactjson.go. DO NOT EDIT.

package agui

// UnmarshalJSON decodes with case-exact member names.
func (v *AgentCapabilities) UnmarshalJSON(data []byte) error {
	type plain AgentCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *IdentityCapabilities) UnmarshalJSON(data []byte) error {
	type plain IdentityCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TransportCapabilities) UnmarshalJSON(data []byte) error {
	type plain TransportCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolsCapabilities) UnmarshalJSON(data []byte) error {
	type plain ToolsCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *OutputCapabilities) UnmarshalJSON(data []byte) error {
	type plain OutputCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *StateCapabilities) UnmarshalJSON(data []byte) error {
	type plain StateCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SubagentInfo) UnmarshalJSON(data []byte) error {
	type plain SubagentInfo
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *MultiAgentCapabilities) UnmarshalJSON(data []byte) error {
	type plain MultiAgentCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningCapabilities) UnmarshalJSON(data []byte) error {
	type plain ReasoningCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *MultimodalInputCapabilities) UnmarshalJSON(data []byte) error {
	type plain MultimodalInputCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *MultimodalOutputCapabilities) UnmarshalJSON(data []byte) error {
	type plain MultimodalOutputCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *MultimodalCapabilities) UnmarshalJSON(data []byte) error {
	type plain MultimodalCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ExecutionCapabilities) UnmarshalJSON(data []byte) error {
	type plain ExecutionCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *HumanInTheLoopCapabilities) UnmarshalJSON(data []byte) error {
	type plain HumanInTheLoopCapabilities
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TextPart) UnmarshalJSON(data []byte) error {
	type plain TextPart
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *DataSource) UnmarshalJSON(data []byte) error {
	type plain DataSource
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *URLSource) UnmarshalJSON(data []byte) error {
	type plain URLSource
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *FileSource) UnmarshalJSON(data []byte) error {
	type plain FileSource
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *RunStartedEvent) UnmarshalJSON(data []byte) error {
	type plain RunStartedEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *RunErrorEvent) UnmarshalJSON(data []byte) error {
	type plain RunErrorEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *StepStartedEvent) UnmarshalJSON(data []byte) error {
	type plain StepStartedEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *StepFinishedEvent) UnmarshalJSON(data []byte) error {
	type plain StepFinishedEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TextMessageStartEvent) UnmarshalJSON(data []byte) error {
	type plain TextMessageStartEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TextMessageContentEvent) UnmarshalJSON(data []byte) error {
	type plain TextMessageContentEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TextMessageEndEvent) UnmarshalJSON(data []byte) error {
	type plain TextMessageEndEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TextMessageChunkEvent) UnmarshalJSON(data []byte) error {
	type plain TextMessageChunkEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCallStartEvent) UnmarshalJSON(data []byte) error {
	type plain ToolCallStartEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCallArgsEvent) UnmarshalJSON(data []byte) error {
	type plain ToolCallArgsEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCallEndEvent) UnmarshalJSON(data []byte) error {
	type plain ToolCallEndEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCallChunkEvent) UnmarshalJSON(data []byte) error {
	type plain ToolCallChunkEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCallResultEvent) UnmarshalJSON(data []byte) error {
	type plain ToolCallResultEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningStartEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningStartEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningEndEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningEndEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningMessageStartEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningMessageStartEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningMessageContentEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningMessageContentEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningMessageEndEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningMessageEndEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningMessageChunkEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningMessageChunkEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningEncryptedValueEvent) UnmarshalJSON(data []byte) error {
	type plain ReasoningEncryptedValueEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *StateSnapshotEvent) UnmarshalJSON(data []byte) error {
	type plain StateSnapshotEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *StateDeltaEvent) UnmarshalJSON(data []byte) error {
	type plain StateDeltaEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *MessagesSnapshotEvent) UnmarshalJSON(data []byte) error {
	type plain MessagesSnapshotEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ActivitySnapshotEvent) UnmarshalJSON(data []byte) error {
	type plain ActivitySnapshotEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ActivityDeltaEvent) UnmarshalJSON(data []byte) error {
	type plain ActivityDeltaEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SubagentStartedEvent) UnmarshalJSON(data []byte) error {
	type plain SubagentStartedEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SubagentErrorEvent) UnmarshalJSON(data []byte) error {
	type plain SubagentErrorEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *RawEvent) UnmarshalJSON(data []byte) error {
	type plain RawEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *CustomEvent) UnmarshalJSON(data []byte) error {
	type plain CustomEvent
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *DeveloperMessage) UnmarshalJSON(data []byte) error {
	type plain DeveloperMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SystemMessage) UnmarshalJSON(data []byte) error {
	type plain SystemMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *AssistantMessage) UnmarshalJSON(data []byte) error {
	type plain AssistantMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *UserMessage) UnmarshalJSON(data []byte) error {
	type plain UserMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolMessage) UnmarshalJSON(data []byte) error {
	type plain ToolMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ActivityMessage) UnmarshalJSON(data []byte) error {
	type plain ActivityMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ReasoningMessage) UnmarshalJSON(data []byte) error {
	type plain ReasoningMessage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *FunctionCall) UnmarshalJSON(data []byte) error {
	type plain FunctionCall
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ToolCall) UnmarshalJSON(data []byte) error {
	type plain ToolCall
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *Tool) UnmarshalJSON(data []byte) error {
	type plain Tool
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *Context) UnmarshalJSON(data []byte) error {
	type plain Context
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *RunAgentInput) UnmarshalJSON(data []byte) error {
	type plain RunAgentInput
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *Interrupt) UnmarshalJSON(data []byte) error {
	type plain Interrupt
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *ResumeEntry) UnmarshalJSON(data []byte) error {
	type plain ResumeEntry
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *TokenUsage) UnmarshalJSON(data []byte) error {
	type plain TokenUsage
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SuccessOutcome) UnmarshalJSON(data []byte) error {
	type plain SuccessOutcome
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *InterruptOutcome) UnmarshalJSON(data []byte) error {
	type plain InterruptOutcome
	return decodeExact(data, (*plain)(v))
}

// UnmarshalJSON decodes with case-exact member names.
func (v *SubagentSuspendedOutcome) UnmarshalJSON(data []byte) error {
	type plain SubagentSuspendedOutcome
	return decodeExact(data, (*plain)(v))
}

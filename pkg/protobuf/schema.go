package protobuf

import (
	"sort"

	"google.golang.org/protobuf/encoding/protowire"
)

// The wire schema of the AG-UI 1.0 HTTP + Protobuf binding, transcribed from
// the generated proto files (proto/events.proto, types.proto, patch.proto in
// this package, identical to the reference SDKs' copies). The schema test
// checks this table against those files.

type kind int

const (
	kString kind = iota
	kInt64
	kBool
	kEnum
	kDouble
	kMessage
)

type field struct {
	num      protowire.Number
	name     string // the camelCase field name
	kind     kind
	msg      string // message type, for kMessage
	repeated bool
	optional bool
	oneof    bool // a member of the message's (single) oneof
}

type message struct {
	name   string
	fields []*field
	byNum  map[protowire.Number]*field
	byName map[string]*field
	hasOne bool
}

const (
	valueMsg  = "google.protobuf.Value"
	structMsg = "google.protobuf.Struct"
	listMsg   = "google.protobuf.ListValue"
	entryMsg  = "google.protobuf.Struct.FieldsEntry"
)

func str(num int, name string) *field {
	return &field{num: protowire.Number(num), name: name, kind: kString}
}
func ostr(num int, name string) *field { f := str(num, name); f.optional = true; return f }
func rstr(num int, name string) *field { f := str(num, name); f.repeated = true; return f }
func m(num int, name, msg string) *field {
	return &field{num: protowire.Number(num), name: name, kind: kMessage, msg: msg}
}
func om(num int, name, msg string) *field { f := m(num, name, msg); f.optional = true; return f }
func rm(num int, name, msg string) *field { f := m(num, name, msg); f.repeated = true; return f }
func arm(f *field) *field                 { f.oneof = true; return f }
func val(num int, name string) *field     { return m(num, name, valueMsg) }
func oval(num int, name string) *field    { return om(num, name, valueMsg) }
func ostruct(num int, name string) *field { return om(num, name, structMsg) }
func oi64(num int, name string) *field {
	return &field{num: protowire.Number(num), name: name, kind: kInt64, optional: true}
}

var schema = map[string]*message{}

func def(name string, fields ...*field) {
	msg := &message{name: name, fields: fields, byNum: map[protowire.Number]*field{}, byName: map[string]*field{}}
	sort.Slice(msg.fields, func(i, j int) bool { return msg.fields[i].num < msg.fields[j].num })
	for _, f := range fields {
		msg.byNum[f.num] = f
		msg.byName[f.name] = f
		if f.oneof {
			msg.hasOne = true
		}
	}
	schema[name] = msg
}

// eventArms maps the Event envelope's oneof field numbers to the AG-UI event
// type each carries, and the wire message name of its payload.
var eventArms = []struct {
	num       protowire.Number
	name      string // camelCase arm name
	eventType string
	msg       string
}{
	{1, "textMessageStart", "TEXT_MESSAGE_START", "TextMessageStartEvent"},
	{2, "textMessageContent", "TEXT_MESSAGE_CONTENT", "TextMessageContentEvent"},
	{3, "textMessageEnd", "TEXT_MESSAGE_END", "TextMessageEndEvent"},
	{4, "toolCallStart", "TOOL_CALL_START", "ToolCallStartEvent"},
	{5, "toolCallArgs", "TOOL_CALL_ARGS", "ToolCallArgsEvent"},
	{6, "toolCallEnd", "TOOL_CALL_END", "ToolCallEndEvent"},
	{7, "stateSnapshot", "STATE_SNAPSHOT", "StateSnapshotEvent"},
	{8, "stateDelta", "STATE_DELTA", "StateDeltaEvent"},
	{9, "messagesSnapshot", "MESSAGES_SNAPSHOT", "MessagesSnapshotEvent"},
	{10, "raw", "RAW", "RawEvent"},
	{11, "custom", "CUSTOM", "CustomEvent"},
	{12, "runStarted", "RUN_STARTED", "RunStartedEvent"},
	{13, "runFinished", "RUN_FINISHED", "RunFinishedEvent"},
	{14, "runError", "RUN_ERROR", "RunErrorEvent"},
	{15, "stepStarted", "STEP_STARTED", "StepStartedEvent"},
	{16, "stepFinished", "STEP_FINISHED", "StepFinishedEvent"},
	{17, "textMessageChunk", "TEXT_MESSAGE_CHUNK", "TextMessageChunkEvent"},
	{18, "toolCallChunk", "TOOL_CALL_CHUNK", "ToolCallChunkEvent"},
	{19, "subagentStarted", "SUBAGENT_STARTED", "SubagentStartedEvent"},
	{20, "subagentFinished", "SUBAGENT_FINISHED", "SubagentFinishedEvent"},
	{21, "subagentError", "SUBAGENT_ERROR", "SubagentErrorEvent"},
	{22, "toolCallResult", "TOOL_CALL_RESULT", "ToolCallResultEvent"},
	{23, "activitySnapshot", "ACTIVITY_SNAPSHOT", "ActivitySnapshotEvent"},
	{24, "activityDelta", "ACTIVITY_DELTA", "ActivityDeltaEvent"},
	{25, "reasoningStart", "REASONING_START", "ReasoningStartEvent"},
	{26, "reasoningMessageStart", "REASONING_MESSAGE_START", "ReasoningMessageStartEvent"},
	{27, "reasoningMessageContent", "REASONING_MESSAGE_CONTENT", "ReasoningMessageContentEvent"},
	{28, "reasoningMessageEnd", "REASONING_MESSAGE_END", "ReasoningMessageEndEvent"},
	{29, "reasoningMessageChunk", "REASONING_MESSAGE_CHUNK", "ReasoningMessageChunkEvent"},
	{30, "reasoningEnd", "REASONING_END", "ReasoningEndEvent"},
	{31, "reasoningEncryptedValue", "REASONING_ENCRYPTED_VALUE", "ReasoningEncryptedValueEvent"},
}

// eventTypeNumbers is the EventType enum (events.proto). It numbers types in
// a different order from the envelope arms.
var eventTypeNumbers = map[string]int32{
	"TEXT_MESSAGE_START": 0, "TEXT_MESSAGE_CONTENT": 1, "TEXT_MESSAGE_END": 2,
	"TOOL_CALL_START": 3, "TOOL_CALL_ARGS": 4, "TOOL_CALL_END": 5,
	"STATE_SNAPSHOT": 6, "STATE_DELTA": 7, "MESSAGES_SNAPSHOT": 8,
	"RAW": 9, "CUSTOM": 10, "RUN_STARTED": 11, "RUN_FINISHED": 12, "RUN_ERROR": 13,
	"STEP_STARTED": 14, "STEP_FINISHED": 15, "SUBAGENT_STARTED": 16, "SUBAGENT_FINISHED": 17,
	"SUBAGENT_ERROR": 18, "TEXT_MESSAGE_CHUNK": 19, "TOOL_CALL_CHUNK": 20, "TOOL_CALL_RESULT": 21,
	"ACTIVITY_SNAPSHOT": 22, "ACTIVITY_DELTA": 23, "REASONING_START": 24,
	"REASONING_MESSAGE_START": 25, "REASONING_MESSAGE_CONTENT": 26, "REASONING_MESSAGE_END": 27,
	"REASONING_MESSAGE_CHUNK": 28, "REASONING_END": 29, "REASONING_ENCRYPTED_VALUE": 30,
}

// patchOps is the JsonPatchOperationType enum (patch.proto).
var patchOps = []string{"add", "remove", "replace", "move", "copy", "test"}

func init() {
	// google/protobuf/struct.proto
	def(valueMsg,
		arm(&field{num: 1, name: "nullValue", kind: kEnum}),
		arm(&field{num: 2, name: "numberValue", kind: kDouble}),
		arm(str(3, "stringValue")),
		arm(&field{num: 4, name: "boolValue", kind: kBool}),
		arm(m(5, "structValue", structMsg)),
		arm(m(6, "listValue", listMsg)),
	)
	def(structMsg, rm(1, "fields", entryMsg))
	def(entryMsg, str(1, "key"), m(2, "value", valueMsg))
	def(listMsg, rm(1, "values", valueMsg))

	// patch.proto
	def("JsonPatchOperation",
		&field{num: 1, name: "op", kind: kEnum},
		str(2, "path"),
		ostr(3, "from"),
		oval(4, "value"),
	)

	// types.proto
	def("TextInputPart", str(1, "text"), ostr(2, "id"), oval(3, "metadata"))
	for _, name := range []string{"ImageInputPart", "AudioInputPart", "VideoInputPart", "DocumentInputPart"} {
		def(name, m(1, "source", "InputContentSource"), oval(2, "metadata"), ostr(3, "id"))
	}
	def("InputContentDataSource", str(1, "value"), str(2, "mimeType"))
	def("InputContentUrlSource", str(1, "value"), ostr(2, "mimeType"))
	def("InputContentFileSource", str(1, "value"), ostr(2, "provider"), ostr(3, "mimeType"))
	def("InputContentSource",
		arm(m(1, "data", "InputContentDataSource")),
		arm(m(2, "url", "InputContentUrlSource")),
		arm(m(3, "file", "InputContentFileSource")),
	)
	def("InputContent",
		arm(m(1, "text", "TextInputPart")),
		arm(m(2, "image", "ImageInputPart")),
		arm(m(3, "audio", "AudioInputPart")),
		arm(m(4, "video", "VideoInputPart")),
		arm(m(5, "document", "DocumentInputPart")),
	)
	def("ToolCall.Function", str(1, "name"), str(2, "arguments"))
	def("ToolCall", str(1, "id"), str(2, "type"), m(3, "function", "ToolCall.Function"), ostruct(4, "metadata"), ostr(5, "encryptedValue"))
	def("Message",
		str(1, "id"), str(2, "role"), ostr(3, "content"), ostr(4, "name"),
		rm(5, "toolCalls", "ToolCall"), ostr(6, "toolCallId"), ostr(7, "error"),
		rm(8, "contentParts", "InputContent"), ostruct(9, "metadata"),
		ostr(10, "subagentRunId"), ostr(11, "encryptedValue"), ostr(12, "activityType"),
		ostruct(13, "activityContent"),
	)
	def("Tool", str(1, "name"), str(2, "description"), oval(3, "parameters"), ostruct(4, "metadata"))
	def("Context", str(1, "description"), str(2, "value"))
	def("ResumeEntry", str(1, "interruptId"), str(2, "status"), oval(3, "payload"), ostruct(4, "metadata"))
	def("RunAgentInput",
		str(1, "threadId"), str(2, "runId"), ostr(3, "parentRunId"), oval(4, "state"),
		rm(5, "messages", "Message"), rm(6, "tools", "Tool"), rm(7, "context", "Context"),
		oval(8, "forwardedProps"), rm(9, "resume", "ResumeEntry"), ostr(10, "protocolVersion"),
	)
	def("Interrupt",
		str(1, "id"), str(2, "reason"), ostr(3, "message"), ostr(4, "toolCallId"),
		oval(5, "responseSchema"), ostr(6, "expiresAt"), oval(7, "metadata"), ostr(8, "subagentRunId"),
	)

	// events.proto
	def("BaseEvent",
		&field{num: 1, name: "type", kind: kEnum},
		&field{num: 2, name: "timestamp", kind: kInt64, optional: true},
		oval(3, "rawEvent"),
		ostruct(4, "metadata"),
	)
	base := func() *field { return m(1, "baseEvent", "BaseEvent") }
	def("TextMessageStartEvent", base(), str(2, "messageId"), ostr(3, "role"), ostr(4, "name"), ostr(5, "subagentRunId"))
	def("TextMessageContentEvent", base(), str(2, "messageId"), str(3, "delta"), ostr(4, "subagentRunId"))
	def("TextMessageEndEvent", base(), str(2, "messageId"), ostr(3, "subagentRunId"))
	def("TextMessageChunkEvent", base(), ostr(2, "messageId"), ostr(3, "role"), ostr(4, "delta"), ostr(5, "name"), ostr(6, "subagentRunId"))
	def("ToolCallStartEvent", base(), str(2, "toolCallId"), str(3, "toolCallName"), ostr(4, "parentMessageId"), ostr(5, "subagentRunId"))
	def("ToolCallArgsEvent", base(), str(2, "toolCallId"), str(3, "delta"), ostr(4, "subagentRunId"))
	def("ToolCallEndEvent", base(), str(2, "toolCallId"), ostr(3, "subagentRunId"))
	def("ToolCallChunkEvent", base(), ostr(2, "toolCallId"), ostr(3, "toolCallName"), ostr(4, "parentMessageId"), ostr(5, "delta"), ostr(6, "subagentRunId"))
	def("ToolCallResultEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"), str(4, "toolCallId"), ostr(5, "content"), ostr(6, "role"), rm(7, "contentParts", "InputContent"))
	def("StateSnapshotEvent", base(), val(2, "snapshot"), ostr(3, "subagentRunId"))
	def("StateDeltaEvent", base(), rm(2, "delta", "JsonPatchOperation"), ostr(3, "subagentRunId"))
	def("MessagesSnapshotEvent", base(), rm(2, "messages", "Message"))
	def("ActivitySnapshotEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"), str(4, "activityType"), m(5, "content", structMsg), &field{num: 6, name: "replace", kind: kBool, optional: true})
	def("ActivityDeltaEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"), str(4, "activityType"), rm(5, "patch", "JsonPatchOperation"))
	def("RawEvent", base(), val(2, "event"), ostr(3, "source"), ostr(4, "subagentRunId"))
	def("CustomEvent", base(), str(2, "name"), val(3, "value"), ostr(4, "subagentRunId"))
	def("RunStartedEvent", base(), str(2, "threadId"), str(3, "runId"), ostr(4, "parentRunId"), om(5, "input", "RunAgentInput"), ostr(6, "protocolVersion"))
	def("RunFinishedEvent", base(), str(2, "threadId"), str(3, "runId"), oval(4, "result"), str(5, "outcome"), rm(6, "interrupts", "Interrupt"), rm(7, "usage", "Usage"), rstr(8, "pendingToolCallIds"))
	def("RunErrorEvent", base(), ostr(2, "code"), str(3, "message"), rm(4, "usage", "Usage"))
	def("StepStartedEvent", base(), str(2, "stepName"), ostr(3, "subagentRunId"))
	def("StepFinishedEvent", base(), str(2, "stepName"), ostr(3, "subagentRunId"))
	def("ReasoningStartEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"))
	def("ReasoningMessageStartEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"), str(4, "role"))
	def("ReasoningMessageContentEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"), str(4, "delta"))
	def("ReasoningMessageEndEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"))
	def("ReasoningMessageChunkEvent", base(), ostr(2, "subagentRunId"), ostr(3, "messageId"), ostr(4, "delta"))
	def("ReasoningEndEvent", base(), ostr(2, "subagentRunId"), str(3, "messageId"))
	def("ReasoningEncryptedValueEvent", base(), ostr(2, "subagentRunId"), str(3, "subtype"), str(4, "entityId"), str(5, "encryptedValue"))
	def("SubagentStartedEvent", base(), str(2, "subagentRunId"), str(3, "name"), ostr(4, "description"), ostr(5, "parentSubagentRunId"), ostr(6, "parentToolCallId"), ostr(7, "parentMessageId"))
	def("SubagentFinishedEvent", base(), str(2, "subagentRunId"), oval(3, "result"), str(4, "outcome"), rstr(5, "interruptIds"))
	def("SubagentErrorEvent", base(), str(2, "subagentRunId"), str(3, "message"), ostr(4, "code"))
	def("Usage", ostr(1, "provider"), ostr(2, "model"), oi64(3, "inputTokens"), oi64(4, "outputTokens"), oi64(5, "totalTokens"), oi64(6, "reasoningTokens"), oi64(7, "cachedInputTokens"), oi64(8, "cacheWriteInputTokens"))

	envelope := make([]*field, 0, len(eventArms))
	for _, a := range eventArms {
		envelope = append(envelope, arm(m(int(a.num), a.name, a.msg)))
	}
	def("Event", envelope...)
}

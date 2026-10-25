package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs.
//
// The .NET theories run each case through three System.Text.Json
// configurations ("ordinary", "context", "resolver"). Go has exactly one
// serialization path (encoding/json driven by the types' own tags and
// MarshalJSON methods), so every row exercises json.Marshal; the rows are kept
// so the mapping to the C# cases stays one-to-one.

import (
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

var serializerRows = []string{"ordinary", "context", "resolver"}

// absentValues are the Go spellings of .NET's (JsonElement?)null, a JsonElement
// holding JSON null, and default(JsonElement): in Go all three are a nil any
// (decoding the JSON text `null` into an any yields nil).
func absentValues(t *testing.T) []any {
	return []any{nil, decode(t, []byte("null"))}
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.RunAgentInput_UnsetOptionalPropertiesAreOmitted
func TestCsDefaultSerializationOmissionTest_RunAgentInput_UnsetOptionalPropertiesAreOmitted(t *testing.T) {
	o := obj(t, &agui.RunAgentInput{ThreadID: "thread_1", RunID: "run_1"})
	assertEq(t, o["threadId"], "thread_1", "threadId")
	assertEq(t, o["runId"], "run_1", "runId")
	if _, ok := o["messages"].([]any); !ok {
		t.Errorf("messages must be an array, got %#v", o["messages"])
	}
	assertOmitted(t, o, "protocolVersion", "parentRunId", "state", "tools", "context", "forwardedProps", "resume")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.Tool_UnsetParametersAreOmittedWhenNestedInARun
func TestCsDefaultSerializationOmissionTest_Tool_UnsetParametersAreOmittedWhenNestedInARun(t *testing.T) {
	o := obj(t, &agui.RunAgentInput{Tools: []agui.Tool{{Name: "search", Description: "Search"}}})
	tool := objOf(t, arrOf(t, o["tools"])[0])
	assertEq(t, tool["name"], "search", "tools[0].name")
	assertOmitted(t, tool, "parameters")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.TextMessageStart_UnsetRoleIsOmitted
func TestCsDefaultSerializationOmissionTest_TextMessageStart_UnsetRoleIsOmitted(t *testing.T) {
	o := obj(t, &agui.TextMessageStartEvent{MessageID: "message_1"})
	assertEq(t, o["type"], "TEXT_MESSAGE_START", "type")
	assertOmitted(t, o, "role")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.RunStarted_UnsetProtocolVersionIsOmitted
func TestCsDefaultSerializationOmissionTest_RunStarted_UnsetProtocolVersionIsOmitted(t *testing.T) {
	o := obj(t, &agui.RunStartedEvent{ThreadID: "thread_1", RunID: "run_1"})
	assertEq(t, o["type"], "RUN_STARTED", "type")
	assertOmitted(t, o, "protocolVersion")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.TextMessageChunk_UnsetPropertiesAreOmitted
func TestCsDefaultSerializationOmissionTest_TextMessageChunk_UnsetPropertiesAreOmitted(t *testing.T) {
	o := obj(t, &agui.TextMessageChunkEvent{})
	assertEq(t, o["type"], "TEXT_MESSAGE_CHUNK", "type")
	assertOmitted(t, o, "subagentRunId", "messageId", "role", "delta", "name", "timestamp", "rawEvent", "metadata")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.ToolCallChunk_UnsetPropertiesAreOmitted
func TestCsDefaultSerializationOmissionTest_ToolCallChunk_UnsetPropertiesAreOmitted(t *testing.T) {
	o := obj(t, &agui.ToolCallChunkEvent{})
	assertEq(t, o["type"], "TOOL_CALL_CHUNK", "type")
	assertOmitted(t, o, "subagentRunId", "toolCallId", "toolCallName", "parentMessageId", "delta", "timestamp", "rawEvent", "metadata")
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.OptionalOpaqueJson_WholeNullAndUndefinedAreOmitted
func TestCsDefaultSerializationOmissionTest_OptionalOpaqueJson_WholeNullAndUndefinedAreOmitted(t *testing.T) {
	for _, row := range serializerRows {
		t.Run(row, func(t *testing.T) {
			for _, absent := range absentValues(t) {
				input := obj(t, &agui.RunAgentInput{
					State:          absent,
					ForwardedProps: absent,
					Tools:          []agui.Tool{{Name: "search", Parameters: absent}},
				})
				// Metadata is a typed object (agui.Metadata); its "no value" is nil.
				chunk := obj(t, &agui.TextMessageChunkEvent{BaseEvent: agui.BaseEvent{RawEvent: absent, Metadata: nil}})
				assertOmitted(t, input, "state", "forwardedProps")
				assertOmitted(t, objOf(t, arrOf(t, input["tools"])[0]), "parameters")
				assertOmitted(t, chunk, "rawEvent", "metadata")
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.OptionalOpaqueJson_AssignedNullIsAbsentAcrossModelFamilies
func TestCsDefaultSerializationOmissionTest_OptionalOpaqueJson_AssignedNullIsAbsentAcrossModelFamilies(t *testing.T) {
	for _, row := range []string{"context", "resolver"} {
		t.Run(row, func(t *testing.T) {
			// Message metadata and responseSchema are typed objects in Go; a
			// JSON null assigned to them is the nil map. Part metadata is
			// opaque (any), where a decoded JSON null is nil.
			jsonNull := decode(t, []byte("null"))
			message := obj(t, &agui.AssistantMessage{Metadata: nil})
			img := agui.NewImagePart(&agui.URLSource{Value: "https://example.com/image.png"})
			img.Metadata = jsonNull
			content := obj(t, img)
			interrupt := obj(t, &agui.Interrupt{ResponseSchema: nil})

			assertOmitted(t, message, "metadata")
			assertOmitted(t, content, "metadata")
			assertOmitted(t, interrupt, "responseSchema")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.OptionalOpaqueJson_NestedNullAndNonNullValuesArePreserved
func TestCsDefaultSerializationOmissionTest_OptionalOpaqueJson_NestedNullAndNonNullValuesArePreserved(t *testing.T) {
	for _, row := range serializerRows {
		t.Run(row, func(t *testing.T) {
			for _, payload := range []string{`{"selectedId":null,"items":[null,1]}`, `[null,1]`, `false`, `0`, `""`} {
				value := decode(t, []byte(payload))
				input := obj(t, &agui.RunAgentInput{
					State:          value,
					ForwardedProps: value,
					Tools:          []agui.Tool{{Name: "search", Parameters: value}},
				})
				if !reflect.DeepEqual(value, input["state"]) {
					t.Errorf("%s: state = %#v", payload, input["state"])
				}
				if !reflect.DeepEqual(value, input["forwardedProps"]) {
					t.Errorf("%s: forwardedProps = %#v", payload, input["forwardedProps"])
				}
				if !reflect.DeepEqual(value, objOf(t, arrOf(t, input["tools"])[0])["parameters"]) {
					t.Errorf("%s: tools[0].parameters = %#v", payload, objOf(t, arrOf(t, input["tools"])[0])["parameters"])
				}
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.RequiredOpaqueJson_WholeNullIsPreserved
func TestCsDefaultSerializationOmissionTest_RequiredOpaqueJson_WholeNullIsPreserved(t *testing.T) {
	for _, row := range serializerRows {
		t.Run(row, func(t *testing.T) {
			snapshot := obj(t, &agui.StateSnapshotEvent{Snapshot: decode(t, []byte("null"))})
			custom := obj(t, &agui.CustomEvent{Name: "empty", Value: nil})
			if v, ok := snapshot["snapshot"]; !ok || v != nil {
				t.Errorf("snapshot must be written as null, got %v", snapshot)
			}
			if v, ok := custom["value"]; !ok || v != nil {
				t.Errorf("value must be written as null, got %v", custom)
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/DefaultSerializationOmissionTest.cs::DefaultSerializationOmissionTest.Chunk_DefaultValuedDataIsPreserved
func TestCsDefaultSerializationOmissionTest_Chunk_DefaultValuedDataIsPreserved(t *testing.T) {
	text := obj(t, &agui.TextMessageChunkEvent{BaseEvent: agui.BaseEvent{Timestamp: agui.Ptr[int64](0)}, Delta: agui.Ptr("")})
	tool := obj(t, &agui.ToolCallChunkEvent{BaseEvent: agui.BaseEvent{Timestamp: agui.Ptr[int64](0)}, Delta: agui.Ptr("")})
	assertEq(t, text["delta"], "", "text.delta")
	assertEq(t, text["timestamp"], float64(0), "text.timestamp")
	assertEq(t, tool["delta"], "", "tool.delta")
	assertEq(t, tool["timestamp"], float64(0), "tool.timestamp")
}

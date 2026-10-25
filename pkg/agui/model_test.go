package agui

import (
	"encoding/json"
	"strings"
	"testing"
)

// typedDecoders maps schema definitions to a decode into their Go type.
var typedDecoders = map[string]func([]byte) (any, error){
	"RunAgentInput":          decodeInto[RunAgentInput],
	"AgentCapabilities":      decodeInto[AgentCapabilities],
	"ExecutionCapabilities":  decodeInto[ExecutionCapabilities],
	"MultiAgentCapabilities": decodeInto[MultiAgentCapabilities],
	"SubagentInfo":           decodeInto[SubagentInfo],
	"Tool":                   decodeInto[Tool],
	"Interrupt":              decodeInto[Interrupt],
	"ResumeEntry":            decodeInto[ResumeEntry],
	"FileSource": func(b []byte) (any, error) {
		return UnmarshalPartSource(b)
	},
	"UserMessage": func(b []byte) (any, error) { return UnmarshalMessage(b) },
	"ToolMessage": func(b []byte) (any, error) { return UnmarshalMessage(b) },
}

func decodeInto[T any](b []byte) (any, error) {
	v := new(T)
	err := json.Unmarshal(b, v)
	return v, err
}

func decoderFor(def string) func([]byte) (any, error) {
	if d, ok := typedDecoders[def]; ok {
		return d
	}
	if strings.HasSuffix(def, "Event") {
		return func(b []byte) (any, error) { return UnmarshalEvent(b) }
	}
	return nil
}

// TestTypedRoundTrip decodes every valid fixture into its Go type and
// re-encodes it: the result must be the same JSON document, and valid. This
// proves the Go model carries every field of the protocol and emits nothing
// it does not define — optional fields absent, never null.
func TestTypedRoundTrip(t *testing.T) {
	covered := 0
	for _, f := range loadSchemaFixtures(t) {
		if !f.valid {
			continue
		}
		dec := decoderFor(f.definition)
		if dec == nil {
			t.Errorf("no Go type for definition %s", f.definition)
			continue
		}
		covered++
		t.Run(f.definition+"/"+f.name, func(t *testing.T) {
			v, err := dec(f.data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			out, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			want, _ := decodeGeneric(f.data)
			got, _ := decodeGeneric(out)
			if !jsonEqual(want, got) {
				t.Fatalf("round trip changed the document\n want %s\n got  %s", f.data, out)
			}
			if err := Validate(f.definition, out); err != nil {
				t.Fatalf("re-encoded document invalid: %v", err)
			}
		})
	}
	if covered == 0 {
		t.Fatal("no fixtures covered")
	}
}

// TestConstructorsEmitValidEvents checks that every constructor, with only
// its required arguments, produces an event the schema accepts.
func TestConstructorsEmitValidEvents(t *testing.T) {
	events := []Event{
		NewRunStartedEvent("t", "r"),
		NewRunFinishedEvent("t", "r"),
		NewRunErrorEvent("boom"),
		NewStepStartedEvent("s"),
		NewStepFinishedEvent("s"),
		NewTextMessageStartEvent("m"),
		NewTextMessageContentEvent("m", "hi"),
		NewTextMessageEndEvent("m"),
		NewTextMessageChunkEvent("m", "hi"),
		NewTextMessageChunkEvent("", "hi"),
		NewToolCallStartEvent("c", "search"),
		NewToolCallArgsEvent("c", "{}"),
		NewToolCallEndEvent("c"),
		NewToolCallChunkEvent("c", "search", "{"),
		NewToolCallResultEvent("m2", "c", "ok"),
		NewReasoningStartEvent("r"),
		NewReasoningEndEvent("r"),
		NewReasoningMessageStartEvent("rm"),
		NewReasoningMessageContentEvent("rm", "hmm"),
		NewReasoningMessageEndEvent("rm"),
		NewReasoningMessageChunkEvent("rm", "hmm"),
		NewReasoningEncryptedValueEvent(EncryptedValueSubtypeMessage, "rm", "b3BhcXVl"),
		NewStateSnapshotEvent(nil),
		NewStateSnapshotEvent(map[string]any{"a": 1}),
		NewStateDeltaEvent(),
		NewStateDeltaEvent(Add("/a", 1), Remove("/b"), Replace("/c", nil), Move("/d", "/e"), Copy("/f", "/g"), Test("/h", "x")),
		NewMessagesSnapshotEvent(),
		NewMessagesSnapshotEvent(
			&UserMessage{ID: "1", Content: TextContent("hi")},
			&UserMessage{ID: "1b", Content: PartsContent()},
			&AssistantMessage{ID: "2", ToolCalls: []ToolCall{NewToolCall("c", "search", "{}")}},
			&ToolMessage{ID: "3", ToolCallID: "c", Content: PartsContent(NewTextPart("x"), NewDocumentPart(&FileSource{Value: "file-1"}))},
			&ActivityMessage{ID: "4", ActivityType: "search"},
			&ReasoningMessage{ID: "5", Content: "r"},
			&DeveloperMessage{ID: "6", Content: "d"},
			&SystemMessage{ID: "7", Content: "s"},
		),
		NewActivitySnapshotEvent("a", "search", nil),
		NewActivityDeltaEvent("a", "search", Replace("/found", 3)),
		NewSubagentStartedEvent("s1", "researcher"),
		NewSubagentFinishedEvent("s1"),
		NewSubagentErrorEvent("s1", "failed"),
		NewRawEvent(nil),
		NewCustomEvent("acme.thing", nil),
		&RunFinishedEvent{ThreadID: "t", RunID: "r", Outcome: &InterruptOutcome{Interrupts: []Interrupt{{ID: "i", Reason: "approval"}}}},
		&RunFinishedEvent{ThreadID: "t", RunID: "r", Outcome: &SuccessOutcome{PendingToolCallIDs: []string{"c"}}},
		&RunFinishedEvent{ThreadID: "t", RunID: "r", Outcome: &CancelledOutcome{}, Usage: []TokenUsage{{InputTokens: Ptr[int64](3)}}},
		&SubagentFinishedEvent{SubagentRunID: "s", Outcome: &SubagentSuspendedOutcome{}},
		&SubagentFinishedEvent{SubagentRunID: "s", Outcome: &SubagentSuccessOutcome{}},
	}
	seen := map[EventType]bool{}
	for _, e := range events {
		seen[e.Type()] = true
		if err := ValidateEvent(e); err != nil {
			b, _ := json.Marshal(e)
			t.Errorf("%s: %v\n%s", e.Type(), err, b)
		}
	}
	for _, et := range EventTypes {
		if !seen[et] {
			t.Errorf("no constructor exercised for %s", et)
		}
	}
	if len(EventTypes) != 31 {
		t.Errorf("expected 31 event types, have %d", len(EventTypes))
	}
}

// TestEventTypesMatchSchema keeps EventTypes in lockstep with the schema's
// EventType enum.
func TestEventTypesMatchSchema(t *testing.T) {
	n := agSchema().defs["EventType"]
	if len(n.enum) != len(EventTypes) {
		t.Fatalf("schema has %d event types, Go has %d", len(n.enum), len(EventTypes))
	}
	for _, et := range EventTypes {
		if !inEnum(n.enum, string(et)) {
			t.Errorf("%s is not in the schema", et)
		}
		if _, err := NewEvent(et); err != nil {
			t.Errorf("NewEvent(%s): %v", et, err)
		}
	}
}

func TestVersion(t *testing.T) {
	cases := map[string]PeerVersion{"1.0": PeerSame, "1.1": PeerNewerMinor, "1.10": PeerNewerMinor, "2.0": PeerOtherMajor, "0.0.57": PeerUninterpretable, "x": PeerUninterpretable}
	for in, want := range cases {
		got, _ := ClassifyPeerVersion(&in)
		if got != want {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
	if got, w := ClassifyPeerVersion(nil); got != PeerUndeclared || w != nil {
		t.Errorf("nil: got %v %v", got, w)
	}
	a, _ := ParseVersion("1.10")
	b, _ := ParseVersion("1.9")
	if a.Compare(b) != 1 {
		t.Error("1.10 must be newer than 1.9")
	}
}

// TestDecodingIsCaseExact: member names are exact. encoding/json alone
// would read "subAgents" (retired in 1.0) into Subagents, or "MESSAGEID"
// into MessageID; neither is the protocol's key.
func TestDecodingIsCaseExact(t *testing.T) {
	var m MultiAgentCapabilities
	if err := json.Unmarshal([]byte(`{"subAgents":[{"name":"planner"}]}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Subagents != nil {
		t.Fatalf("the retired subAgents key was read: %v", m.Subagents)
	}
	cases := map[string]string{
		"event":   `{"type":"TEXT_MESSAGE_CONTENT","MESSAGEID":"x","messageId":"m","Delta":"y","delta":"d"}`,
		"outcome": `{"type":"RUN_FINISHED","threadId":"t","runId":"r","Outcome":{"type":"cancelled"}}`,
		"part":    `{"type":"TOOL_CALL_RESULT","messageId":"m","toolCallId":"c","content":[{"type":"image","Source":{"type":"url","value":"u"},"source":{"type":"url","value":"v","MimeType":"x"}}]}`,
	}
	for name, doc := range cases {
		e, err := UnmarshalEvent([]byte(doc))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		switch ev := e.(type) {
		case *TextMessageContentEvent:
			if ev.MessageID != "m" || ev.Delta != "d" {
				t.Errorf("%s: %+v", name, ev)
			}
		case *RunFinishedEvent:
			if ev.Outcome != nil {
				t.Errorf("%s: Outcome read from a wrongly cased key", name)
			}
		case *ToolCallResultEvent:
			src := ev.Content.Parts()[0].(*ImagePart).Source.(*URLSource)
			if src.Value != "v" || src.MimeType != nil {
				t.Errorf("%s: %+v", name, src)
			}
		}
	}
}

// TestIntegralFloatsAreIntegers: JSON Schema counts 12.0 as an integer, so a
// document the schema accepts must decode, while 1.5 is still rejected and an
// open object keeps its numbers as written.
func TestIntegralFloatsAreIntegers(t *testing.T) {
	e, err := ParseEvent([]byte(`{"type":"RUN_FINISHED","threadId":"t","runId":"r","timestamp":1.7e12,"usage":[{"inputTokens":12.0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	rf := e.(*RunFinishedEvent)
	if *rf.Timestamp != 1700000000000 || *rf.Usage[0].InputTokens != 12 {
		t.Fatalf("%+v", rf)
	}
	if _, err := ParseEvent([]byte(`{"type":"TEXT_MESSAGE_END","messageId":"m","timestamp":1.5}`)); err == nil {
		t.Fatal("1.5 is not an integer")
	}
	res, err := Sanitize("RunAgentInput", []byte(`{"threadId":"t","runId":"r","messages":[],"tools":[{"name":"x","description":"y","parameters":{"maxItems":2.0}}]}`), SanitizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.Value)
	if !strings.Contains(string(b), `"maxItems":2.0`) {
		t.Fatalf("an open object's number must stay as written: %s", b)
	}
}

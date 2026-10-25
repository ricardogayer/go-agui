package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs.

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.PolymorphicDeserialization_RunStartedEvent
func TestCsBaseEventJsonConverterTest_PolymorphicDeserialization_RunStartedEvent(t *testing.T) {
	var original agui.Event = &agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "type", toObject(t, b)["type"], "RUN_STARTED")
	typed := asType[*agui.RunStartedEvent](t, viaBaseEvent(t, string(b)))
	eq(t, "ThreadID", typed.ThreadID, "t1")
	eq(t, "RunID", typed.RunID, "r1")
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.PolymorphicDeserialization_RunFinishedEvent
func TestCsBaseEventJsonConverterTest_PolymorphicDeserialization_RunFinishedEvent(t *testing.T) {
	var original agui.Event = &agui.RunFinishedEvent{ThreadID: "t1", Outcome: &agui.SuccessOutcome{}}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	typed := asType[*agui.RunFinishedEvent](t, viaBaseEvent(t, string(b)))
	eq(t, "ThreadID", typed.ThreadID, "t1")
	asType[*agui.SuccessOutcome](t, typed.Outcome)
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.PolymorphicDeserialization_RunErrorEvent
func TestCsBaseEventJsonConverterTest_PolymorphicDeserialization_RunErrorEvent(t *testing.T) {
	var original agui.Event = &agui.RunErrorEvent{Message: "fail", Code: agui.Ptr("E1")}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	typed := asType[*agui.RunErrorEvent](t, viaBaseEvent(t, string(b)))
	eq(t, "Message", typed.Message, "fail")
	eq(t, "Code", agui.Deref(typed.Code), "E1")
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.Deserialization_ThrowsForUnknownType
//
// Go's typed decoder reports ErrUnknownEventType naming the type; the
// processing-model layer (Sanitize) marks the event as unrecognised so a
// stream reader drops it, which is what the .NET exception type is for.
func TestCsBaseEventJsonConverterTest_Deserialization_ThrowsForUnknownType(t *testing.T) {
	doc := []byte(`{"type":"UNKNOWN_TYPE"}`)
	_, err := agui.UnmarshalEvent(doc)
	if !errors.Is(err, agui.ErrUnknownEventType) {
		t.Fatalf("err = %v, want ErrUnknownEventType", err)
	}
	if !strings.Contains(err.Error(), "UNKNOWN_TYPE") {
		t.Errorf("error %q does not name the event type", err)
	}
	res, err := agui.Sanitize("Event", doc, agui.SanitizeOptions{})
	if err != nil || !res.Dropped {
		t.Errorf("Sanitize: res=%+v err=%v, want dropped as unrecognised", res, err)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.Serialization_KeepsAnExplicitNullRequiredValue
func TestCsBaseEventJsonConverterTest_Serialization_KeepsAnExplicitNullRequiredValue(t *testing.T) {
	const doc = `{"type":"CUSTOM","name":"app.thing","value":null}`
	custom := asType[*agui.CustomEvent](t, viaBaseEvent(t, doc))
	if custom.Value != nil {
		t.Errorf("Value = %#v, want nil", custom.Value)
	}
	b, err := json.Marshal(agui.Event(custom))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "re-serialised", string(b), doc)
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.Serialization_OmitsAnUnsetOptionalRole
func TestCsBaseEventJsonConverterTest_Serialization_OmitsAnUnsetOptionalRole(t *testing.T) {
	b, err := json.Marshal(agui.Event(&agui.TextMessageStartEvent{MessageID: "m1"}))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "json", string(b), `{"type":"TEXT_MESSAGE_START","messageId":"m1"}`)
}

// assertMalformedNotUnknown checks a document with a malformed discriminator
// is an error at every layer and never takes the unknown-type (skip) path.
func assertMalformedNotUnknown(t *testing.T, doc []byte) {
	t.Helper()
	_, err := agui.UnmarshalEvent(doc)
	if err == nil {
		t.Errorf("UnmarshalEvent accepted %s", doc)
	} else if errors.Is(err, agui.ErrUnknownEventType) {
		t.Errorf("UnmarshalEvent took the unknown-type path for %s: %v", doc, err)
	}
	if _, err := agui.ParseEvent(doc); err == nil {
		t.Errorf("ParseEvent accepted %s", doc)
	}
	res, err := agui.Sanitize("Event", doc, agui.SanitizeOptions{})
	if err == nil {
		t.Errorf("Sanitize did not fail on %s (dropped=%v, warnings=%v)", doc, res.Dropped, res.Warnings)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.Deserialization_ThrowsForNonStringType
func TestCsBaseEventJsonConverterTest_Deserialization_ThrowsForNonStringType(t *testing.T) {
	assertMalformedNotUnknown(t, []byte(`{"type":null}`))
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.Deserialization_ThrowsForMissingType
func TestCsBaseEventJsonConverterTest_Deserialization_ThrowsForMissingType(t *testing.T) {
	assertMalformedNotUnknown(t, []byte(`{"threadId":"t1"}`))
}

// schemaDefs returns the $defs of the embedded 1.0 schema.
func schemaDefs(t *testing.T) map[string]map[string]any {
	t.Helper()
	var s struct {
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(agui.SchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	return s.Defs
}

// unionMembers returns, for a oneOf union definition, each member's
// definition name keyed by its discriminator constant.
func unionMembers(t *testing.T, union, discriminator string) map[string]string {
	t.Helper()
	defs := schemaDefs(t)
	u, ok := defs[union]
	if !ok {
		t.Fatalf("schema has no %s", union)
	}
	out := map[string]string{}
	for _, ref := range u["oneOf"].([]any) {
		name := strings.TrimPrefix(ref.(map[string]any)["$ref"].(string), "#/$defs/")
		prop, _ := defs[name]["properties"].(map[string]any)[discriminator].(map[string]any)
		c, ok := prop["const"].(string)
		if !ok {
			t.Fatalf("%s.%s has no const discriminator: %v", name, discriminator, prop)
		}
		out[c] = name
	}
	return out
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.EveryUnionMember_IsCarriedByItsConverter
//
// Go has no assembly reflection over subclasses; the closure is checked
// against the schema instead: every member of each discriminated union the
// 1.0 schema declares must decode, through the Go union decoder, to a value
// carrying its own discriminator, and re-serialise with it.
func TestCsBaseEventJsonConverterTest_EveryUnionMember_IsCarriedByItsConverter(t *testing.T) {
	type decoder func([]byte) (string, any, error)
	rows := []struct {
		name          string // C# base type
		union         string // schema definition
		discriminator string
		decode        decoder
		minimal       map[string]string // discriminator value -> minimal valid document
	}{
		{"AGUIMessage", "Message", "role",
			func(b []byte) (string, any, error) {
				m, err := agui.UnmarshalMessage(b)
				if err != nil {
					return "", nil, err
				}
				return string(m.MessageRole()), m, nil
			},
			map[string]string{
				"developer": `{"id":"m","role":"developer","content":"c"}`,
				"system":    `{"id":"m","role":"system","content":"c"}`,
				"assistant": `{"id":"m","role":"assistant"}`,
				"user":      `{"id":"m","role":"user","content":"c"}`,
				"tool":      `{"id":"m","role":"tool","content":"c","toolCallId":"t"}`,
				"activity":  `{"id":"m","role":"activity","activityType":"a","content":{}}`,
				"reasoning": `{"id":"m","role":"reasoning","content":"c"}`,
			}},
		{"AGUIInputContent", "ContentPart", "type",
			func(b []byte) (string, any, error) {
				p, err := agui.UnmarshalContentPart(b)
				if err != nil {
					return "", nil, err
				}
				if _, unknown := p.(*agui.UnknownPart); unknown {
					return "", nil, errors.New("decoded as UnknownPart")
				}
				return p.PartType(), p, nil
			},
			map[string]string{
				"text":     `{"type":"text","text":"x"}`,
				"image":    `{"type":"image","source":{"type":"url","value":"https://example.com/a.png"}}`,
				"audio":    `{"type":"audio","source":{"type":"url","value":"https://example.com/a.mp3"}}`,
				"video":    `{"type":"video","source":{"type":"url","value":"https://example.com/a.mp4"}}`,
				"document": `{"type":"document","source":{"type":"url","value":"https://example.com/a.pdf"}}`,
			}},
		{"AGUIInputContentSource", "PartSource", "type",
			func(b []byte) (string, any, error) {
				s, err := agui.UnmarshalPartSource(b)
				if err != nil {
					return "", nil, err
				}
				return s.SourceType(), s, nil
			},
			map[string]string{
				"data": `{"type":"data","value":"AA==","mimeType":"image/png"}`,
				"url":  `{"type":"url","value":"https://example.com/a.png"}`,
				"file": `{"type":"file","value":"file-123"}`,
			}},
		{"RunFinishedOutcome", "RunFinishedOutcome", "type",
			func(b []byte) (string, any, error) {
				o, err := agui.UnmarshalRunFinishedOutcome(b)
				if err != nil {
					return "", nil, err
				}
				return o.OutcomeType(), o, nil
			},
			map[string]string{
				"success":   `{"type":"success"}`,
				"interrupt": `{"type":"interrupt","interrupts":[{"id":"i1","reason":"tool_approval"}]}`,
				"cancelled": `{"type":"cancelled"}`,
			}},
		{"SubagentFinishedOutcome", "SubagentFinishedOutcome", "type",
			func(b []byte) (string, any, error) {
				o, err := agui.UnmarshalSubagentFinishedOutcome(b)
				if err != nil {
					return "", nil, err
				}
				return o.OutcomeType(), o, nil
			},
			map[string]string{
				"success":   `{"type":"success"}`,
				"suspended": `{"type":"suspended"}`,
			}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			members := unionMembers(t, row.union, row.discriminator)
			if len(members) == 0 {
				t.Fatal("no members")
			}
			for value, def := range members {
				doc, ok := row.minimal[value]
				if !ok {
					t.Errorf("schema member %s (%s=%q) has no Go decode case in this test", def, row.discriminator, value)
					continue
				}
				if err := agui.Validate(def, []byte(doc)); err != nil {
					t.Fatalf("test document for %s is not valid: %v", def, err)
				}
				got, v, err := row.decode([]byte(doc))
				if err != nil {
					t.Errorf("%s: decode failed: %v", def, err)
					continue
				}
				eq(t, def+" discriminator", got, value)
				b, err := json.Marshal(v)
				if err != nil {
					t.Errorf("%s: no write case: %v", def, err)
					continue
				}
				eq(t, def+" written discriminator", toObject(t, b)[row.discriminator], value)
			}
			if len(row.minimal) != len(members) {
				t.Errorf("Go table has %d members, schema %d", len(row.minimal), len(members))
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/BaseEventJsonConverterTest.cs::BaseEventJsonConverterTest.EveryDeclaredEventType_IsCarriedByBothConverterDirections
func TestCsBaseEventJsonConverterTest_EveryDeclaredEventType_IsCarriedByBothConverterDirections(t *testing.T) {
	if len(agui.EventTypes) == 0 {
		t.Fatal("no event types declared")
	}
	carried := map[string]bool{}
	for _, et := range agui.EventTypes {
		model, err := agui.NewEvent(et)
		if err != nil {
			t.Errorf("%s: NewEvent: %v", et, err)
			continue
		}
		decoded, err := agui.UnmarshalEvent([]byte(`{"type":"` + string(et) + `"}`))
		if err != nil {
			t.Errorf("%s: no read case: %v", et, err)
			continue
		}
		if decoded.Type() != et || model.Type() != et {
			t.Errorf("%s: decoded %T (%s), model %T", et, decoded, decoded.Type(), model)
		}
		b, err := json.Marshal(model)
		if err != nil {
			t.Errorf("%s: no write case: %v", et, err)
			continue
		}
		eq(t, string(et)+" written type", toObject(t, b)["type"], string(et))
		carried[string(model.Type())] = true
	}
	declared := map[string]bool{}
	for v := range unionMembers(t, "Event", "type") {
		declared[v] = true
	}
	keys := func(m map[string]bool) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	eq(t, "carried event types", keys(carried), keys(declared))
}

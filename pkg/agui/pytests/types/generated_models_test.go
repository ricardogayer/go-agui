package types_test

// Port of sdks/python/tests/test_generated_models.py.
//
// Python's generated Pydantic models are the SDK's TOLERANT layer: they parse
// the fixture corpus leniently and a recorded set of invalid fixtures is
// expected to parse. The Go analogue of that layer is typed decoding —
// json.Unmarshal into the anchor's Go type, with agui.UnmarshalEvent /
// agui.UnmarshalMessage / agui.UnmarshalPartSource for the discriminated
// unions (so the discriminator is still checked). The strict contract is
// agui.Validate, and the strip-and-warn stage is agui.Sanitize.
//
// Go's tolerances differ from Pydantic's, so goToleratedInvalid below is Go's
// own recorded list, not a copy of Python's.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const specFixtures = "../../testdata/spec-1.0/fixtures"

type fixture struct {
	name   string // "<Anchor>/<kind>/<file>"
	anchor string
	data   []byte
	doc    any
}

// collect returns every fixture of one kind ("valid" or "invalid"), sorted
// as the Python helper sorts them.
func collect(t *testing.T, kind string) []fixture {
	t.Helper()
	anchors, err := os.ReadDir(specFixtures)
	if err != nil {
		t.Fatal(err)
	}
	var out []fixture
	for _, a := range anchors {
		if !a.IsDir() {
			continue
		}
		dir := filepath.Join(specFixtures, a.Name(), kind)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") || strings.HasSuffix(f.Name(), ".expect.json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var doc any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("%s: %v", f.Name(), err)
			}
			out = append(out, fixture{
				name:   a.Name() + "/" + kind + "/" + f.Name(),
				anchor: a.Name(),
				data:   data,
				doc:    doc,
			})
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s fixtures found under %s", kind, specFixtures)
	}
	return out
}

// eventTypeByName maps a Go event type name ("TextMessageStartEvent") to its
// discriminator.
func eventTypeByName() map[string]agui.EventType {
	m := map[string]agui.EventType{}
	for _, et := range agui.EventTypes {
		e, err := agui.NewEvent(et)
		if err != nil {
			panic(err)
		}
		m[reflect.TypeOf(e).Elem().Name()] = et
	}
	return m
}

// plainTypes are the non-union anchors decoded with plain json.Unmarshal.
var plainTypes = map[string]func() any{
	"AgentCapabilities":      func() any { return &agui.AgentCapabilities{} },
	"ExecutionCapabilities":  func() any { return &agui.ExecutionCapabilities{} },
	"Interrupt":              func() any { return &agui.Interrupt{} },
	"MultiAgentCapabilities": func() any { return &agui.MultiAgentCapabilities{} },
	"ResumeEntry":            func() any { return &agui.ResumeEntry{} },
	"RunAgentInput":          func() any { return &agui.RunAgentInput{} },
	"SubagentInfo":           func() any { return &agui.SubagentInfo{} },
	"Tool":                   func() any { return &agui.Tool{} },
}

// decodeAs is adapter_for(anchor).validate_python(document): the tolerant,
// typed decode of one document as the anchor's Go type.
func decodeAs(anchor string, data []byte) (any, error) {
	if et, ok := eventTypeByName()[anchor]; ok {
		e, err := agui.UnmarshalEvent(data)
		if err != nil {
			return nil, err
		}
		if e.Type() != et {
			return nil, fmt.Errorf("decoded %s, want %s", e.Type(), et)
		}
		return e, nil
	}
	if strings.HasSuffix(anchor, "Message") {
		m, err := agui.UnmarshalMessage(data)
		if err != nil {
			return nil, err
		}
		if got := reflect.TypeOf(m).Elem().Name(); got != anchor {
			return nil, fmt.Errorf("decoded %s, want %s", got, anchor)
		}
		return m, nil
	}
	if anchor == "FileSource" {
		s, err := agui.UnmarshalPartSource(data)
		if err != nil {
			return nil, err
		}
		if _, ok := s.(*agui.FileSource); !ok {
			return nil, fmt.Errorf("decoded %T, want *agui.FileSource", s)
		}
		return s, nil
	}
	newT, ok := plainTypes[anchor]
	if !ok {
		return nil, fmt.Errorf("no Go type registered for anchor %s", anchor)
	}
	v := newT()
	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

// goToleratedInvalid is Go's recorded list of invalid fixtures the typed
// decode accepts. The classes:
//
//	unknown-keys — encoding/json ignores members the struct does not declare
//	  (it drops them; see TestPyGeneratedModelsAgainstFixtures_test_unknown_fields_survive_the_parse
//	  for where Go's strip-and-warn stage sees them instead).
//	null-means-absent — a JSON null decodes to the Go zero value (nil pointer,
//	  nil map, nil interface), i.e. reads as absent.
//	required-not-enforced — encoding/json has no notion of a required member;
//	  a missing one decodes to the zero value (Python's "const-fills-in" is the
//	  one case of this Pydantic shares).
//	constraint-not-enforced — a value of the right JSON type that breaks a
//	  schema constraint the Go type cannot express (minItems, maximum,
//	  minimum, pattern).
//	closed-set-not-enforced — a closed set of string values (an enum or a
//	  const) is an open Go string type, so an unknown value decodes.
//	unknown-part-kept — see the entries below.
//
// Python's "lax-coercion" class has no Go member: encoding/json does not
// coerce "yes" to a bool, so AgentCapabilities/invalid/streaming-not-boolean
// is rejected here. Python's one "const-fills-in" entry (role-missing) is a
// required-not-enforced entry in Go: the field is simply never decoded, and
// marshaling writes the const back.
var goToleratedInvalid = map[string]string{
	// Pydantic shares these with Go (Python's list records them too).
	"AgentCapabilities/invalid/old-subagents-key.json":                               "unknown-keys",
	"AgentCapabilities/invalid/custom-null.json":                                     "null-means-absent",
	"AgentCapabilities/invalid/metadata-null.json":                                   "null-means-absent",
	"MultiAgentCapabilities/invalid/old-subagents-key.json":                          "unknown-keys",
	"FileSource/invalid/reference-record.json":                                       "unknown-keys",
	"MessagesSnapshotEvent/invalid/message-metadata-null.json":                       "null-means-absent",
	"ReasoningMessageStartEvent/invalid/role-missing.json":                           "required-not-enforced",
	"RunFinishedEvent/invalid/outcome-null.json":                                     "null-means-absent",
	"RunFinishedEvent/invalid/outcome-success-carrying-interrupts.json":              "unknown-keys",
	"RunFinishedEvent/invalid/outcome-cancelled-carrying-interrupts.json":            "unknown-keys",
	"RunFinishedEvent/invalid/outcome-interrupt-carrying-pending-tool-call-ids.json": "unknown-keys",
	"SubagentErrorEvent/invalid/code-null.json":                                      "null-means-absent",
	"SubagentFinishedEvent/invalid/outcome-null.json":                                "null-means-absent",
	"SubagentFinishedEvent/invalid/outcome-success-carrying-interrupt-ids.json":      "unknown-keys",
	"SubagentStartedEvent/invalid/description-null.json":                             "null-means-absent",
	"TextMessageContentEvent/invalid/metadata-null.json":                             "null-means-absent",
	"TextMessageContentEvent/invalid/subagent-run-id-null.json":                      "null-means-absent",
	"TextMessageEndEvent/invalid/raw-event-null.json":                                "null-means-absent",
	"TextMessageEndEvent/invalid/unknown-property.json":                              "unknown-keys",
	"ToolCallChunkEvent/invalid/parent-message-id-null.json":                         "null-means-absent",
	"ToolCallStartEvent/invalid/parent-message-id-null.json":                         "null-means-absent",

	// Go-only tolerances: encoding/json does not enforce required members.
	"CustomEvent/invalid/name-missing.json":                    "required-not-enforced",
	"CustomEvent/invalid/value-missing.json":                   "required-not-enforced",
	"FileSource/invalid/value-missing.json":                    "required-not-enforced",
	"Interrupt/invalid/reason-missing.json":                    "required-not-enforced",
	"RawEvent/invalid/event-missing.json":                      "required-not-enforced",
	"RunAgentInput/invalid/messages-missing.json":              "required-not-enforced",
	"StateDeltaEvent/invalid/patch-add-without-value.json":     "required-not-enforced",
	"StateDeltaEvent/invalid/patch-copy-without-from.json":     "required-not-enforced",
	"StateDeltaEvent/invalid/patch-move-without-from.json":     "required-not-enforced",
	"StateDeltaEvent/invalid/patch-replace-without-value.json": "required-not-enforced",
	"StateDeltaEvent/invalid/patch-test-without-value.json":    "required-not-enforced",
	"StateSnapshotEvent/invalid/snapshot-missing.json":         "required-not-enforced",
	"SubagentInfo/invalid/name-missing.json":                   "required-not-enforced",
	"SubagentStartedEvent/invalid/name-missing.json":           "required-not-enforced",
	"TextMessageContentEvent/invalid/delta-missing.json":       "required-not-enforced",
	"Tool/invalid/name-missing.json":                           "required-not-enforced",
	"ToolMessage/invalid/tool-call-id-missing.json":            "required-not-enforced",
	"UserMessage/invalid/content-missing.json":                 "required-not-enforced",
	"UserMessage/invalid/file-source-value-missing.json":       "required-not-enforced",

	// Go-only: numeric bounds, minItems and JSON Pointer patterns are not
	// expressible in the Go types.
	"ExecutionCapabilities/invalid/max-iterations-over-maximum.json":     "constraint-not-enforced",
	"ExecutionCapabilities/invalid/negative-max-execution-time.json":     "constraint-not-enforced",
	"RunFinishedEvent/invalid/outcome-interrupt-with-no-interrupts.json": "constraint-not-enforced",
	"RunFinishedEvent/invalid/usage-negative-cache-write-count.json":     "constraint-not-enforced",
	"RunFinishedEvent/invalid/usage-negative-token-count.json":           "constraint-not-enforced",
	"StateDeltaEvent/invalid/patch-pointer-bad-tilde-escape.json":        "constraint-not-enforced",
	"StateDeltaEvent/invalid/patch-pointer-without-leading-slash.json":   "constraint-not-enforced",
	"TextMessageEndEvent/invalid/timestamp-above-safe-integer.json":      "constraint-not-enforced",
	"TextMessageEndEvent/invalid/timestamp-below-safe-integer.json":      "constraint-not-enforced",

	// Go-only: closed sets of strings are open Go string types.
	"ReasoningEncryptedValueEvent/invalid/unknown-subtype.json": "closed-set-not-enforced",
	"ResumeEntry/invalid/unknown-status.json":                   "closed-set-not-enforced",
	"StateDeltaEvent/invalid/patch-unknown-op.json":             "closed-set-not-enforced",
	"ToolCallResultEvent/invalid/role-not-tool.json":            "closed-set-not-enforced",

	// Go-only, by design: a content part of an unrecognised type (a newer
	// version's part, or the retired binary part) decodes to *agui.UnknownPart
	// so compatibility shims can still see it; Validate rejects it and
	// Sanitize strips it.
	"ToolCallResultEvent/invalid/content-part-unknown-type.json":        "unknown-part-kept",
	"ToolMessage/invalid/content-part-unknown-type.json":                "unknown-part-kept",
	"UserMessage/invalid/legacy-binary-part.json":                       "unknown-part-kept",
	"UserMessage/invalid/part-shaped-like-media-with-unknown-type.json": "unknown-part-kept",
	"UserMessage/invalid/part-shaped-like-text-with-unknown-type.json":  "unknown-part-kept",
}

func TestPyGeneratedModelsAgainstFixtures_test_valid_fixtures_parse(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_valid_fixtures_parse
	for _, f := range collect(t, "valid") {
		t.Run(f.name, func(t *testing.T) {
			if _, err := decodeAs(f.anchor, f.data); err != nil {
				t.Fatalf("typed decode rejected a valid fixture: %v", err)
			}
		})
	}
}

func TestPyGeneratedModelsAgainstFixtures_test_invalid_fixtures_fail_except_the_recorded_tolerances(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_invalid_fixtures_fail_except_the_recorded_tolerances
	// Exactly the recorded set parses: an entry that stops parsing is a
	// tolerance silently lost, an unlisted one that parses is a tolerance
	// silently gained. Every invalid fixture must still be rejected by the
	// strict layer, agui.Validate.
	seen := map[string]bool{}
	for _, f := range collect(t, "invalid") {
		seen[f.name] = true
		t.Run(f.name, func(t *testing.T) {
			if err := agui.Validate(f.anchor, f.data); err == nil {
				t.Errorf("agui.Validate(%q) accepted an invalid fixture", f.anchor)
			}
			_, err := decodeAs(f.anchor, f.data)
			_, tolerated := goToleratedInvalid[f.name]
			if err != nil && tolerated {
				t.Errorf("recorded tolerance lost: typed decode now rejects it: %v", err)
			}
			if err == nil && !tolerated {
				t.Errorf("unrecorded tolerance gained: typed decode accepts it")
			}
		})
	}
	var stale []string
	for name := range goToleratedInvalid {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("recorded tolerances name fixtures that do not exist: %v", stale)
	}
}

func TestPyGeneratedModelsAgainstFixtures_test_valid_event_fixtures_parse_through_the_union(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_valid_event_fixtures_parse_through_the_union
	eventTypes := map[string]bool{}
	for _, et := range agui.EventTypes {
		eventTypes[string(et)] = true
	}
	n := 0
	for _, f := range collect(t, "valid") {
		obj, ok := f.doc.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := obj["type"].(string)
		if !eventTypes[typ] {
			continue
		}
		n++
		t.Run(f.name, func(t *testing.T) {
			e, err := agui.UnmarshalEvent(f.data)
			if err != nil {
				t.Fatalf("event union rejected a valid fixture: %v", err)
			}
			if string(e.Type()) != typ {
				t.Fatalf("decoded %s, want %s", e.Type(), typ)
			}
		})
	}
	if n == 0 {
		t.Fatal("no event fixtures found")
	}
}

func TestPyGeneratedModelsAgainstFixtures_test_message_fixtures_parse_through_the_union(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_message_fixtures_parse_through_the_union
	n := 0
	for _, f := range collect(t, "valid") {
		if !strings.HasSuffix(f.anchor, "Message") {
			continue
		}
		n++
		t.Run(f.name, func(t *testing.T) {
			m, err := agui.UnmarshalMessage(f.data)
			if err != nil {
				t.Fatalf("message union rejected a valid fixture: %v", err)
			}
			if got := reflect.TypeOf(m).Elem().Name(); got != f.anchor {
				t.Fatalf("decoded %s, want %s", got, f.anchor)
			}
		})
	}
	if n == 0 {
		t.Fatal("no message fixtures found")
	}
}

func TestPyGeneratedModelsAgainstFixtures_test_unknown_fields_survive_the_parse(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_unknown_fields_survive_the_parse
	//
	// Adapted (Python-specific design, not a spec requirement). Python's
	// models keep unknown keys so a later strip-and-warn stage can see them;
	// the spec only binds the pipeline ("This binds the pipeline, not the
	// parser", basic/processing.mdx). In Go the strip-and-warn stage,
	// agui.Sanitize, runs on the generic JSON *before* typed decoding, so the
	// promise to port is: the tolerant decode still accepts the document, the
	// enforcement stage sees the unknown key and strips it with a warning
	// naming its path, and the strict layer rejects it.
	for _, f := range collect(t, "valid") {
		obj, ok := f.doc.(map[string]any)
		if !ok {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			probed := map[string]any{}
			for k, v := range obj {
				probed[k] = v
			}
			probed["xPassthroughProbe"] = 1
			data, err := json.Marshal(probed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeAs(f.anchor, data); err != nil {
				t.Fatalf("tolerant decode rejected an unknown key: %v", err)
			}
			res, err := agui.Sanitize(f.anchor, data, agui.SanitizeOptions{})
			if err != nil {
				t.Fatalf("Sanitize failed: %v", err)
			}
			if res.Dropped {
				t.Fatal("Sanitize dropped the whole document")
			}
			warned := false
			for _, w := range res.Warnings {
				if w.Path == "/xPassthroughProbe" {
					warned = true
				}
			}
			if !warned {
				t.Errorf("Sanitize did not warn about /xPassthroughProbe: %v", res.Warnings)
			}
			if m, ok := res.Value.(map[string]any); !ok {
				t.Errorf("sanitized value is %T", res.Value)
			} else if _, ok := m["xPassthroughProbe"]; ok {
				t.Error("unknown key survived Sanitize")
			}
			if err := agui.Validate(f.anchor, data); err == nil {
				t.Error("Validate accepted an unknown key")
			}
		})
	}
}

func TestPyGeneratedModelsAgainstFixtures_test_wire_dump_omits_absent_fields_and_preserves_required_nulls(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedModelsAgainstFixtures::test_wire_dump_omits_absent_fields_and_preserves_required_nulls
	//
	// Go has no field metadata to walk as Pydantic does, so the check is the
	// stronger one it implies: decode, marshal, and the wire document is the
	// fixture again — no absent field reaches the wire (as null or a
	// default), and a required member whose value is null stays, as null.
	for _, f := range collect(t, "valid") {
		t.Run(f.name, func(t *testing.T) {
			v, err := decodeAs(f.anchor, f.data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, f.doc) {
				t.Errorf("wire dump differs from the fixture\n got: %s\nwant: %s", out, f.data)
			}
			if err := agui.Validate(f.anchor, out); err != nil {
				t.Errorf("wire dump fails the schema: %v", err)
			}
		})
	}
}

func TestPyPublicErgonomics_test_snake_case_and_alias_both_populate(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_snake_case_and_alias_both_populate
	// Go: a struct literal (the "by name" constructor) and a decode of the
	// camelCase wire form populate the same field.
	byName := agui.TextMessageStartEvent{MessageID: "m1"}
	e, err := agui.UnmarshalEvent([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"m1"}`))
	if err != nil {
		t.Fatal(err)
	}
	byAlias := e.(*agui.TextMessageStartEvent)
	if byName.MessageID != byAlias.MessageID {
		t.Errorf("by name %q != by alias %q", byName.MessageID, byAlias.MessageID)
	}
}

func TestPyPublicErgonomics_test_discriminators_default_so_constructors_never_spell_them(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_discriminators_default_so_constructors_never_spell_them
	event := &agui.RunFinishedEvent{ThreadID: "t", RunID: "r"}
	if event.Type() != agui.EventTypeRunFinished {
		t.Errorf("Type() = %s", event.Type())
	}
	if got := marshalObj(t, event)["type"]; got != "RUN_FINISHED" {
		t.Errorf("wire type = %v", got)
	}
	call := agui.ToolCall{ID: "c1", Function: agui.FunctionCall{Name: "f", Arguments: "{}"}}
	if got := marshalObj(t, call)["type"]; got != "function" {
		t.Errorf("tool call wire type = %v", got)
	}
}

func TestPyPublicErgonomics_test_schema_defaults_stay_documentation(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_schema_defaults_stay_documentation
	// An absent role MEANS assistant and an absent replace MEANS replace —
	// normative prose, never materialised by the type or the decoder.
	start := agui.TextMessageStartEvent{MessageID: "m"}
	if start.Role != nil {
		t.Errorf("Role = %v, want nil", *start.Role)
	}
	if _, ok := marshalObj(t, start)["role"]; ok {
		t.Error("role materialised on the wire")
	}
	e, err := agui.UnmarshalEvent([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := e.(*agui.TextMessageStartEvent).Role; r != nil {
		t.Errorf("decoded Role = %v, want nil", *r)
	}
	snap := agui.ActivitySnapshotEvent{MessageID: "m", ActivityType: "a", Content: map[string]any{}}
	if snap.Replace != nil {
		t.Error("Replace materialised")
	}
	if _, ok := marshalObj(t, snap)["replace"]; ok {
		t.Error("replace materialised on the wire")
	}
}

func TestPyPublicErgonomics_test_explicit_none_means_absent(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_explicit_none_means_absent
	event := agui.TextMessageStartEvent{MessageID: "m", Name: nil}
	if _, ok := marshalObj(t, event)["name"]; ok {
		t.Error("nil Name reached the wire")
	}
}

func TestPyPublicErgonomics_test_lax_coercion_is_the_public_layer(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_lax_coercion_is_the_public_layer
	//
	// Adapted: Pydantic's lax coercion ("42" -> 42) is a Python layer choice,
	// not protocol behaviour; the schema types timestamp as an integer, so a
	// string there is a malformed known value. Go's typed layer does not
	// coerce, and the processing model must treat it as fatal (not strip it).
	doc := []byte(`{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":"d","timestamp":"42"}`)
	if _, err := agui.UnmarshalEvent(doc); err == nil {
		t.Error("typed decode coerced a string timestamp")
	}
	if err := agui.Validate("Event", doc); err == nil {
		t.Error("Validate accepted a string timestamp")
	}
	if _, err := agui.Sanitize("Event", doc, agui.SanitizeOptions{}); err == nil {
		t.Error("Sanitize accepted a malformed known value")
	}
	// The well-typed spelling decodes to the same value Python coerces to.
	e, err := agui.UnmarshalEvent([]byte(`{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":"d","timestamp":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if ts := e.Base().Timestamp; ts == nil || *ts != 42 {
		t.Errorf("timestamp = %v, want 42", ts)
	}
}

func TestPyPublicErgonomics_test_the_hierarchy_supports_isinstance(t *testing.T) {
	// Python: tests/test_generated_models.py::PublicErgonomics::test_the_hierarchy_supports_isinstance
	// Go: the hierarchy is the Event and Message interfaces plus embedded
	// BaseEvent; BaseMessage's distinguishing member is `name`, which tool
	// messages do not compose (the schema says so).
	var event agui.Event = &agui.StepStartedEvent{StepName: "s"}
	if event.Base() == nil {
		t.Error("StepStartedEvent has no BaseEvent envelope")
	}
	if _, ok := reflect.TypeOf(agui.StepStartedEvent{}).FieldByName("BaseEvent"); !ok {
		t.Error("StepStartedEvent does not embed BaseEvent")
	}
	var user agui.Message = &agui.UserMessage{ID: "1", Content: agui.TextContent("hi")}
	if _, ok := reflect.TypeOf(*user.(*agui.UserMessage)).FieldByName("Name"); !ok {
		t.Error("UserMessage lacks BaseMessage's name")
	}
	var tool agui.Message = &agui.ToolMessage{ID: "1", Content: agui.TextContent("c"), ToolCallID: "tc"}
	if _, ok := reflect.TypeOf(*tool.(*agui.ToolMessage)).FieldByName("Name"); ok {
		t.Error("ToolMessage composes BaseMessage's name")
	}
	if err := agui.Validate("ToolMessage", []byte(`{"id":"1","role":"tool","content":"c","toolCallId":"tc","name":"n"}`)); err == nil {
		t.Error("schema accepted name on a tool message")
	}
}

func TestPyGeneratedPackageShape_test_version_constant(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedPackageShape::test_version_constant
	if agui.ProtocolVersion != "1.0" {
		t.Errorf("ProtocolVersion = %q", agui.ProtocolVersion)
	}
}

func TestPyGeneratedPackageShape_test_all_31_events_and_every_message_type_exist(t *testing.T) {
	// Python: tests/test_generated_models.py::GeneratedPackageShape::test_all_31_events_and_every_message_type_exist
	if len(agui.EventTypes) != 31 {
		t.Errorf("%d event types, want 31", len(agui.EventTypes))
	}
	for _, et := range agui.EventTypes {
		if _, err := agui.NewEvent(et); err != nil {
			t.Errorf("NewEvent(%s): %v", et, err)
		}
	}
	for _, role := range []string{"developer", "system", "assistant", "user", "tool", "activity", "reasoning"} {
		m, err := agui.UnmarshalMessage([]byte(`{"role":"` + role + `"}`))
		if err != nil {
			t.Errorf("no message type for role %s: %v", role, err)
			continue
		}
		want := strings.ToUpper(role[:1]) + role[1:] + "Message"
		if got := reflect.TypeOf(m).Elem().Name(); got != want {
			t.Errorf("role %s decoded to %s, want %s", role, got, want)
		}
	}
}

// marshalObj marshals v and decodes it as a JSON object.
func marshalObj(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return m
}

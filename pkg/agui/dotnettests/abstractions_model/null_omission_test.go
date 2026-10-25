package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs.
//
// .NET discovers its wire types by assembly reflection; Go cannot enumerate a
// package's types, so the non-event types are listed and the event types come
// from agui.EventTypes / agui.NewEvent (a newly registered event is covered
// automatically).

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// requiredNullsThatMustBeWritten: CUSTOM.value is required and any JSON value,
// including null, is legal for it; a probe leaves it unset and it must still
// be written, as null.
var requiredNullsThatMustBeWritten = map[string]bool{"CustomEvent/value": true}

// findNullPaths is NullOmissionProbe.FindNullPaths.
func findNullPaths(v any, path string) []string {
	switch x := v.(type) {
	case nil:
		if path == "" {
			return []string{"/"}
		}
		return []string{path}
	case map[string]any:
		var out []string
		for k, val := range x {
			out = append(out, findNullPaths(val, path+"/"+k)...)
		}
		sort.Strings(out)
		return out
	case []any:
		var out []string
		for i, val := range x {
			out = append(out, findNullPaths(val, path+"/"+strconv.Itoa(i))...)
		}
		return out
	}
	return nil
}

// fillRequiredOpaque mirrors NullOmissionProbe.Create's SampleFor(JsonElement):
// a required (non-nullable in .NET) opaque JSON member is set to {} so the
// probe is "every required property set". CustomEvent.Value is nullable in
// .NET (object?) and is left unset, as there.
func fillRequiredOpaque(name string, probe any) {
	rv := reflect.ValueOf(probe)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return
	}
	s := rv.Elem()
	for i := 0; i < s.NumField(); i++ {
		f := s.Type().Field(i)
		if f.Anonymous {
			continue
		}
		tagName, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tagName == "" || tagName == "-" || strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero") {
			continue
		}
		if f.Type.Kind() == reflect.Interface && f.Type.NumMethod() == 0 && name != "CustomEvent" {
			s.Field(i).Set(reflect.ValueOf(map[string]any{}))
		}
	}
}

func goTypeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func eventProbes(t *testing.T) map[string]agui.Event {
	t.Helper()
	out := map[string]agui.Event{}
	for _, et := range agui.EventTypes {
		e, err := agui.NewEvent(et)
		if err != nil {
			t.Fatalf("NewEvent(%s): %v", et, err)
		}
		name := goTypeName(e)
		fillRequiredOpaque(name, e)
		out[name] = e
	}
	return out
}

func wireProbes(t *testing.T) map[string]any {
	t.Helper()
	src := &agui.DataSource{Value: "eA==", MimeType: "text/plain"}
	list := []any{
		&agui.DeveloperMessage{}, &agui.SystemMessage{}, &agui.AssistantMessage{}, &agui.UserMessage{},
		&agui.ToolMessage{}, &agui.ActivityMessage{}, &agui.ReasoningMessage{},
		&agui.ToolCall{}, &agui.FunctionCall{}, &agui.Tool{}, &agui.Context{}, &agui.RunAgentInput{},
		&agui.Interrupt{}, &agui.ResumeEntry{}, &agui.TokenUsage{},
		&agui.SuccessOutcome{}, &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{ID: "x", Reason: "x"}}},
		&agui.CancelledOutcome{}, &agui.SubagentSuccessOutcome{}, &agui.SubagentSuspendedOutcome{},
		&agui.TextPart{}, agui.NewImagePart(src), agui.NewAudioPart(src), agui.NewVideoPart(src), agui.NewDocumentPart(src),
		&agui.DataSource{}, &agui.URLSource{}, &agui.FileSource{},
		&agui.PatchOperation{Op: agui.PatchRemove},
		&agui.AgentCapabilities{}, &agui.IdentityCapabilities{}, &agui.TransportCapabilities{}, &agui.ToolsCapabilities{},
		&agui.OutputCapabilities{}, &agui.StateCapabilities{}, &agui.SubagentInfo{}, &agui.MultiAgentCapabilities{},
		&agui.ReasoningCapabilities{}, &agui.MultimodalInputCapabilities{}, &agui.MultimodalOutputCapabilities{},
		&agui.MultimodalCapabilities{}, &agui.ExecutionCapabilities{}, &agui.HumanInTheLoopCapabilities{},
	}
	out := map[string]any{}
	for _, p := range list {
		name := goTypeName(p)
		fillRequiredOpaque(name, p)
		out[name] = p
	}
	for name, e := range eventProbes(t) {
		out[name] = e
	}
	return out
}

func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.EveryWireTypeOmitsPropertiesWithoutAValue
//
// Also carries the control of the "host options" sweep: the detector does see
// a null when one is written (CUSTOM.value).
func TestCsNullOmissionTest_EveryWireTypeOmitsPropertiesWithoutAValue(t *testing.T) {
	probes := wireProbes(t)
	if len(probes) <= 30 {
		t.Fatalf("only %d wire types", len(probes))
	}
	var offenders []string
	seen := map[string]bool{}
	for _, name := range sortedNames(probes) {
		for _, p := range findNullPaths(decode(t, mustJSON(t, probes[name])), "") {
			off := name + p
			if requiredNullsThatMustBeWritten[off] {
				seen[off] = true
			} else {
				offenders = append(offenders, off)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("nulls written for properties without a value: %v", offenders)
	}
	if !reflect.DeepEqual(seen, requiredNullsThatMustBeWritten) {
		t.Errorf("control: required nulls seen %v, want %v", seen, requiredNullsThatMustBeWritten)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.EveryEventOmitsPropertiesWithoutAValueWhenWrittenAsBaseEvent
//
// The Go counterpart of writing through the polymorphic BaseEvent type info is
// marshaling through the agui.Event interface, as the producer does.
func TestCsNullOmissionTest_EveryEventOmitsPropertiesWithoutAValueWhenWrittenAsBaseEvent(t *testing.T) {
	probes := eventProbes(t)
	if len(probes) <= 20 {
		t.Fatalf("only %d event types", len(probes))
	}
	var offenders []string
	for _, name := range sortedNames(probes) {
		var e agui.Event = probes[name]
		obj := objOf(t, decode(t, mustJSON(t, e)))
		if obj["type"] == nil {
			t.Errorf("%s: discriminator missing", name)
		}
		for _, p := range findNullPaths(obj, "") {
			if off := name + p; !requiredNullsThatMustBeWritten[off] {
				offenders = append(offenders, off)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("nulls written for properties without a value: %v", offenders)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.EveryWireTypeOmitsPropertiesWithoutAValueThroughCallerOwnedOptions
func TestCsNullOmissionTest_EveryWireTypeOmitsPropertiesWithoutAValueThroughCallerOwnedOptions(t *testing.T) {
	t.Skip("N/A in Go: re-runs the sweep through caller-owned JsonSerializerOptions with AGUIJsonUtilities.DefaultTypeInfoResolver inserted; Go has no serializer options, omission lives in the types (covered by EveryWireTypeOmitsPropertiesWithoutAValue)")
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.EveryWireTypeOmitsPropertiesWithoutAValueThroughHostOptionsWithTheBareContext
func TestCsNullOmissionTest_EveryWireTypeOmitsPropertiesWithoutAValueThroughHostOptionsWithTheBareContext(t *testing.T) {
	t.Skip("N/A in Go: re-runs the sweep through host JsonSerializerOptions holding only the bare source-generated context (no DefaultIgnoreCondition); Go has no such configuration path. Its control assertion (CUSTOM.value is still written as null) is ported into EveryWireTypeOmitsPropertiesWithoutAValue")
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.EveryEventOmitsPropertiesWithoutAValueWhenWrittenAsBaseEventThroughHostOptionsWithTheBareContext
func TestCsNullOmissionTest_EveryEventOmitsPropertiesWithoutAValueWhenWrittenAsBaseEventThroughHostOptionsWithTheBareContext(t *testing.T) {
	t.Skip("N/A in Go: BaseEvent written through host JsonSerializerOptions holding the bare context; Go has one marshal path, covered by EveryEventOmitsPropertiesWithoutAValueWhenWrittenAsBaseEvent")
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.OmissionSurvivesCallerOwnedSerializerOptions
func TestCsNullOmissionTest_OmissionSurvivesCallerOwnedSerializerOptions(t *testing.T) {
	t.Skip("N/A in Go: checks that omission survives JsonSerializerOptions{DefaultIgnoreCondition = Never}; encoding/json has no global ignore condition to override (ToolCallStartOmitsParentMessageIdWhenItHasNoValue covers the wire result)")
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.HostOptionsWithTheBareContextKeepNullsThatAreValues
//
// The host-options aspect has no Go counterpart; the assertions about which
// nulls are values hold for Go's single marshal path. Documents holding Go
// maps are compared as JSON values (Go writes map keys sorted).
func TestCsNullOmissionTest_HostOptionsWithTheBareContextKeepNullsThatAreValues(t *testing.T) {
	parse := func(s string) any { return decode(t, []byte(s)) }
	check := func(e agui.Event, want string) {
		t.Helper()
		got := mustJSON(t, e)
		if !jsonEqual(t, got, []byte(want)) {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	check(&agui.StateSnapshotEvent{Snapshot: parse(`{"selectedId":null,"items":[null,1]}`)},
		`{"type":"STATE_SNAPSHOT","snapshot":{"selectedId":null,"items":[null,1]}}`)
	check(&agui.StateSnapshotEvent{Snapshot: parse("null")}, `{"type":"STATE_SNAPSHOT","snapshot":null}`)
	var delta agui.JSONPatch
	if err := json.Unmarshal([]byte(`[{"op":"add","path":"/selectedId","value":null}]`), &delta); err != nil {
		t.Fatal(err)
	}
	check(&agui.StateDeltaEvent{Delta: delta}, `{"type":"STATE_DELTA","delta":[{"op":"add","path":"/selectedId","value":null}]}`)
	check(&agui.RawEvent{Event: parse("null")}, `{"type":"RAW","event":null}`)

	check(&agui.CustomEvent{Name: "ping", Value: parse("null")}, `{"type":"CUSTOM","name":"ping","value":null}`)
	check(&agui.CustomEvent{Name: "ping"}, `{"type":"CUSTOM","name":"ping","value":null}`)
	check(&agui.CustomEvent{Name: "ping", Value: parse(`{"latencyMs":null}`)}, `{"type":"CUSTOM","name":"ping","value":{"latencyMs":null}}`)

	finished := decode(t, mustJSON(t, &agui.RunFinishedEvent{
		BaseEvent: agui.BaseEvent{Metadata: agui.Metadata{"retained": nil}}, ThreadID: "t1", RunID: "r1",
	}))
	if v, ok := objOf(t, objOf(t, finished)["metadata"])["retained"]; !ok || v != nil {
		t.Errorf("metadata.retained must be written as null: %v", finished)
	}
	assertEq(t, findNullPaths(finished, ""), []string{"/metadata/retained"}, "null paths")

	input := string(mustJSON(t, &agui.RunAgentInput{ThreadID: "t1", RunID: "r1", State: parse(`{"cursor":null}`)}))
	if !strings.Contains(input, `"state":{"cursor":null}`) {
		t.Errorf("state lost its null value: %s", input)
	}
	if strings.Contains(input, "parentRunId") {
		t.Errorf("parentRunId written: %s", input)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.CallerOwnedOptionsKeepNullsThatAreValues
//
// Caller-owned options are a .NET concept; the value-null assertion is ported
// on Go's single marshal path.
func TestCsNullOmissionTest_CallerOwnedOptionsKeepNullsThatAreValues(t *testing.T) {
	var e agui.Event = &agui.StateSnapshotEvent{Snapshot: decode(t, []byte(`{"selectedId":null}`))}
	if js := string(mustJSON(t, e)); !strings.Contains(js, `"selectedId":null`) {
		t.Fatalf("nested null lost: %s", js)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.ToolCallStartOmitsParentMessageIdWhenItHasNoValue
func TestCsNullOmissionTest_ToolCallStartOmitsParentMessageIdWhenItHasNoValue(t *testing.T) {
	var e agui.Event = &agui.ToolCallStartEvent{ToolCallID: "tc_1", ToolCallName: "search"}
	if js := string(mustJSON(t, e)); strings.Contains(js, "parentMessageId") {
		t.Fatalf("parentMessageId written: %s", js)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.RunFinishedOmitsOutcomeWhenItHasNoValue
func TestCsNullOmissionTest_RunFinishedOmitsOutcomeWhenItHasNoValue(t *testing.T) {
	var e agui.Event = &agui.RunFinishedEvent{ThreadID: "thread_1", RunID: "run_1"}
	if js := string(mustJSON(t, e)); strings.Contains(js, "outcome") {
		t.Fatalf("outcome written: %s", js)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionTest.cs::NullOmissionTest.NullsInsideAnOpaquePayloadAreValuesAndSurvive
func TestCsNullOmissionTest_NullsInsideAnOpaquePayloadAreValuesAndSurvive(t *testing.T) {
	var e agui.Event = &agui.StateSnapshotEvent{Snapshot: decode(t, []byte(`{"selectedId":null,"items":[null,1]}`))}
	js := string(mustJSON(t, e))
	if !strings.Contains(js, `"selectedId":null`) {
		t.Errorf("selectedId null lost: %s", js)
	}
	if !strings.Contains(js, "[null,1]") {
		t.Errorf("array nulls lost: %s", js)
	}
}

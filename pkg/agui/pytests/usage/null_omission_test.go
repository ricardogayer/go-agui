package usage_test

// Port of sdks/python/tests/test_null_omission.py.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

const nullOmissionFixture = "../../testdata/sdk-fixtures/null-omission.json"

// findNulls returns the JSON-Pointer-ish paths of every JSON null in v.
func findNulls(v any, path string) []string {
	switch t := v.(type) {
	case nil:
		if path == "" {
			return []string{"/"}
		}
		return []string{path}
	case map[string]any:
		var out []string
		for k, x := range t {
			out = append(out, findNulls(x, path+"/"+k)...)
		}
		return out
	case []any:
		var out []string
		for i, x := range t {
			out = append(out, findNulls(x, path+"/"+strconv.Itoa(i))...)
		}
		return out
	}
	return nil
}

// permittedNullPaths is _permitted_null_paths: the top-level members of a
// probe that are required (no omitempty/omitzero), typed to accept any JSON
// value, and hold nil — STATE_SNAPSHOT.snapshot, RAW.event, CUSTOM.value. The
// contract says those keys are still written, as null.
func permittedNullPaths(v any) []string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	var out []string
	var walk func(reflect.Value)
	walk = func(s reflect.Value) {
		for i := 0; i < s.NumField(); i++ {
			f := s.Type().Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(s.Field(i))
				continue
			}
			tag := f.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" || name == "-" || strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero") {
				continue
			}
			if f.Type.Kind() == reflect.Interface && f.Type.NumMethod() == 0 && s.Field(i).IsNil() {
				out = append(out, "/"+name)
			}
		}
	}
	if rv.Kind() == reflect.Struct {
		walk(rv)
	}
	return out
}

func offendingNulls(payload any, probe any) []string {
	permitted := map[string]bool{}
	for _, p := range permittedNullPaths(probe) {
		permitted[p] = true
	}
	var off []string
	for _, p := range findNulls(payload, "") {
		if !permitted[p] {
			off = append(off, p)
		}
	}
	sort.Strings(off)
	return off
}

// wireProbes are the Go library's wire types with only their required fields
// set (the zero value), the counterpart of _wire_model_classes + _build_probe.
// Go has no module reflection, so the non-event types are listed; every event
// type comes from agui.EventTypes / agui.NewEvent, so a new event is covered
// the moment it is registered. As in Python, a type with a cross-field
// invariant (a media part needs a source, an interrupt outcome needs an
// interrupt) gets a hand-written constructor.
func wireProbes(t *testing.T) map[string]any {
	t.Helper()
	src := &agui.DataSource{Value: "eA==", MimeType: "text/plain"}
	probes := map[string]any{
		// messages
		"DeveloperMessage": &agui.DeveloperMessage{}, "SystemMessage": &agui.SystemMessage{},
		"AssistantMessage": &agui.AssistantMessage{}, "UserMessage": &agui.UserMessage{},
		"ToolMessage": &agui.ToolMessage{}, "ActivityMessage": &agui.ActivityMessage{},
		"ReasoningMessage": &agui.ReasoningMessage{},
		// tool calls, tools, context, input
		"ToolCall": &agui.ToolCall{}, "FunctionCall": &agui.FunctionCall{}, "Tool": &agui.Tool{},
		"Context": &agui.Context{}, "RunAgentInput": &agui.RunAgentInput{},
		// interrupts, resume, usage, outcomes
		"Interrupt": &agui.Interrupt{}, "ResumeEntry": &agui.ResumeEntry{}, "TokenUsage": &agui.TokenUsage{},
		"SuccessOutcome":           &agui.SuccessOutcome{},
		"InterruptOutcome":         &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{ID: "int_1", Reason: "input_required"}}},
		"CancelledOutcome":         &agui.CancelledOutcome{},
		"SubagentSuccessOutcome":   &agui.SubagentSuccessOutcome{},
		"SubagentSuspendedOutcome": &agui.SubagentSuspendedOutcome{},
		// content
		"TextPart": &agui.TextPart{}, "ImagePart": agui.NewImagePart(src), "AudioPart": agui.NewAudioPart(src),
		"VideoPart": agui.NewVideoPart(src), "DocumentPart": agui.NewDocumentPart(src),
		"DataSource": &agui.DataSource{}, "URLSource": &agui.URLSource{}, "FileSource": &agui.FileSource{},
		"PatchOperation": &agui.PatchOperation{Op: agui.PatchRemove},
		// capabilities
		"AgentCapabilities": &agui.AgentCapabilities{}, "IdentityCapabilities": &agui.IdentityCapabilities{},
		"TransportCapabilities": &agui.TransportCapabilities{}, "ToolsCapabilities": &agui.ToolsCapabilities{},
		"OutputCapabilities": &agui.OutputCapabilities{}, "StateCapabilities": &agui.StateCapabilities{},
		"SubagentInfo": &agui.SubagentInfo{}, "MultiAgentCapabilities": &agui.MultiAgentCapabilities{},
		"ReasoningCapabilities": &agui.ReasoningCapabilities{}, "MultimodalInputCapabilities": &agui.MultimodalInputCapabilities{},
		"MultimodalOutputCapabilities": &agui.MultimodalOutputCapabilities{}, "MultimodalCapabilities": &agui.MultimodalCapabilities{},
		"ExecutionCapabilities": &agui.ExecutionCapabilities{}, "HumanInTheLoopCapabilities": &agui.HumanInTheLoopCapabilities{},
	}
	for name, e := range eventProbes(t) {
		probes[name] = e
	}
	return probes
}

func eventProbes(t *testing.T) map[string]agui.Event {
	t.Helper()
	out := map[string]agui.Event{}
	for _, et := range agui.EventTypes {
		e, err := agui.NewEvent(et)
		if err != nil {
			t.Fatalf("NewEvent(%s): %v", et, err)
		}
		out[string(et)] = e
	}
	return out
}

// encodeFrame is the producer's framing: the event marshaled by its own
// MarshalJSON and written as one SSE data frame by sse.Writer — what
// stream.SSE.Send writes. Send itself also enforces the run lifecycle and the
// schema, which a required-fields-only probe (or a fixture case read out of
// its stream) cannot satisfy in isolation, so the frame is produced directly.
// It returns the frame text and its decoded payload.
func encodeFrame(t *testing.T, e agui.Event) (string, any) {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal %T: %v", e, err)
	}
	var buf bytes.Buffer
	if err := sse.NewWriter(&buf).WriteData(data); err != nil {
		t.Fatalf("WriteData: %v", err)
	}
	frame := buf.String()
	payload, err := sse.NewReader(strings.NewReader(frame)).Next()
	if err != nil {
		t.Fatalf("read frame %q: %v", frame, err)
	}
	return frame, parse(t, string(payload))
}

// ---------------------------------------------------------------------------
// TestNullOmissionSweep
// ---------------------------------------------------------------------------

// Python: tests/test_null_omission.py::TestNullOmissionSweep::test_every_wire_type_omits_fields_without_a_value
//
// Python sweeps both by_alias=True and False; Go has a single JSON spelling.
func TestPyTestNullOmissionSweep_EveryWireTypeOmitsFieldsWithoutAValue(t *testing.T) {
	probes := wireProbes(t)
	if len(probes) <= 30 {
		t.Fatalf("only %d wire types found", len(probes))
	}
	for name, probe := range probes {
		t.Run(name, func(t *testing.T) {
			if off := offendingNulls(generic(t, probe), probe); len(off) > 0 {
				t.Fatalf("%s wrote null at %v for fields that have no value", name, off)
			}
		})
	}
}

// Python: tests/test_null_omission.py::TestNullOmissionSweep::test_encoded_event_stream_contains_no_omittable_nulls
func TestPyTestNullOmissionSweep_EncodedEventStreamContainsNoOmittableNulls(t *testing.T) {
	probes := eventProbes(t)
	if len(probes) <= 25 {
		t.Fatalf("only %d event types found", len(probes))
	}
	for name, probe := range probes {
		t.Run(name, func(t *testing.T) {
			_, payload := encodeFrame(t, probe)
			if off := offendingNulls(payload, probe); len(off) > 0 {
				t.Fatalf("%s encoded null at %v", name, off)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNullOmissionIsTheBaseModelsDoing
// ---------------------------------------------------------------------------

// toolCallStartWithoutOmission mirrors agui.ToolCallStartEvent's wire fields
// with the omission (omitempty) removed: the Go counterpart of overriding the
// base model's wrap serializer with a passthrough.
type toolCallStartWithoutOmission struct {
	Type            string  `json:"type"`
	ToolCallID      string  `json:"toolCallId"`
	ToolCallName    string  `json:"toolCallName"`
	ParentMessageID *string `json:"parentMessageId"`
}

func revertedToolCallStart(e agui.ToolCallStartEvent) toolCallStartWithoutOmission {
	return toolCallStartWithoutOmission{
		Type: string(e.Type()), ToolCallID: e.ToolCallID, ToolCallName: e.ToolCallName, ParentMessageID: e.ParentMessageID,
	}
}

// Python: tests/test_null_omission.py::TestNullOmissionIsTheBaseModelsDoing::test_reverting_the_setting_reintroduces_the_null
func TestPyTestNullOmissionIsTheBaseModelsDoing_RevertingTheSettingReintroducesTheNull(t *testing.T) {
	e := agui.ToolCallStartEvent{ToolCallID: "tc_1", ToolCallName: "search"}
	serialized := object(t, revertedToolCallStart(e))
	v, ok := serialized["parentMessageId"]
	if !ok || v != nil {
		t.Fatalf("reverting the omission should bring the null back; got %v", serialized)
	}
	// And the sweep's detector sees it, so a passing sweep proves something.
	if off := offendingNulls(serialized, &e); !reflect.DeepEqual(off, []string{"/parentMessageId"}) {
		t.Fatalf("detector found %v, want [/parentMessageId]", off)
	}
}

// Python: tests/test_null_omission.py::TestNullOmissionIsTheBaseModelsDoing::test_reverting_the_setting_reintroduces_the_null_in_the_encoder
func TestPyTestNullOmissionIsTheBaseModelsDoing_RevertingTheSettingReintroducesTheNullInTheEncoder(t *testing.T) {
	e := agui.ToolCallStartEvent{ToolCallID: "tc_1", ToolCallName: "search"}
	data, err := json.Marshal(revertedToolCallStart(e))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := sse.NewWriter(&buf).WriteData(data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"parentMessageId":null`) {
		t.Fatalf("frame %q lacks the reintroduced null", buf.String())
	}
	// The real event through the same framing carries no such null.
	frame, _ := encodeFrame(t, &e)
	if strings.Contains(frame, "parentMessageId") {
		t.Fatalf("unmodified event framed parentMessageId: %q", frame)
	}
}

// Python: tests/test_null_omission.py::TestNullOmissionIsTheBaseModelsDoing::test_omission_holds_for_the_unmodified_class
func TestPyTestNullOmissionIsTheBaseModelsDoing_OmissionHoldsForTheUnmodifiedClass(t *testing.T) {
	e := agui.ToolCallStartEvent{ToolCallID: "tc_1", ToolCallName: "search"}
	if _, ok := object(t, e)["parentMessageId"]; ok {
		t.Fatalf("parentMessageId serialized for an event without it")
	}
}

// ---------------------------------------------------------------------------
// TestLegitimateNullsRoundTrip
// ---------------------------------------------------------------------------

// Python: tests/test_null_omission.py::TestLegitimateNullsRoundTrip::test_null_inside_a_metadata_dict_survives
func TestPyTestLegitimateNullsRoundTrip_NullInsideAMetadataDictSurvives(t *testing.T) {
	in := agui.Interrupt{ID: "int_1", Reason: "input_required", Metadata: agui.Metadata{"traceId": nil}}
	if got := object(t, in)["metadata"]; !reflect.DeepEqual(got, map[string]any{"traceId": nil}) {
		t.Fatalf("metadata = %v", got)
	}
}

// Python: tests/test_null_omission.py::TestLegitimateNullsRoundTrip::test_null_extra_field_survives
//
// Adapted (b): Pydantic models are extra="allow", so Python carries an
// unknown "vendorHint": null through. In the 1.0 schema STEP_STARTED is
// closed (unevaluatedProperties: false), so an unknown member is unrecognised
// material: the strict check rejects it and the processing model
// (basic/processing.mdx) strips it with a warning. The Go type has no place
// for it. What survives is the spec-mandated behaviour, not the extra.
func TestPyTestLegitimateNullsRoundTrip_NullExtraFieldSurvives(t *testing.T) {
	doc := `{"type": "STEP_STARTED", "stepName": "plan", "vendorHint": null}`
	mustReject(t, "Event", doc)
	res, err := agui.Sanitize("Event", []byte(doc), agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if res.Dropped {
		t.Fatalf("the whole event was dropped")
	}
	if _, ok := res.Value.(map[string]any)["vendorHint"]; ok {
		t.Fatalf("unrecognised member survived sanitizing: %v", res.Value)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("stripping vendorHint produced no warning")
	}
}

// Python: tests/test_null_omission.py::TestLegitimateNullsRoundTrip::test_required_field_holding_null_survives
func TestPyTestLegitimateNullsRoundTrip_RequiredFieldHoldingNullSurvives(t *testing.T) {
	e := agui.StateSnapshotEvent{Snapshot: nil}
	d := object(t, e)
	v, ok := d["snapshot"]
	if !ok || v != nil {
		t.Fatalf("snapshot = %v (present=%v), want present null", v, ok)
	}
}

// Python: tests/test_null_omission.py::TestLegitimateNullsRoundTrip::test_run_agent_input_with_no_state_omits_the_key
func TestPyTestLegitimateNullsRoundTrip_RunAgentInputWithNoStateOmitsTheKey(t *testing.T) {
	original := agui.RunAgentInput{
		ThreadID: "thread_1", RunID: "run_1",
		State: nil, Messages: agui.Messages{}, Tools: []agui.Tool{}, Context: []agui.Context{},
		ForwardedProps: nil,
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"state"`)) {
		t.Fatalf("state written: %s", encoded)
	}
	if bytes.Contains(encoded, []byte(`"forwardedProps"`)) {
		t.Fatalf("forwardedProps written: %s", encoded)
	}
	var restored agui.RunAgentInput
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("decode %s: %v", encoded, err)
	}
	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("round trip changed the input\n got: %#v\nwant: %#v", restored, original)
	}
	mustValidate(t, "RunAgentInput", string(encoded))
}

// Python: tests/test_null_omission.py::TestLegitimateNullsRoundTrip::test_run_agent_input_accepts_absent_state
func TestPyTestLegitimateNullsRoundTrip_RunAgentInputAcceptsAbsentState(t *testing.T) {
	doc := `{"threadId": "t", "runId": "r", "messages": [], "tools": [], "context": [], "forwardedProps": null}`
	var parsed agui.RunAgentInput
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed.State != nil {
		t.Fatalf("state = %v, want absent", parsed.State)
	}
	// The consumer-side processing model reads the legacy forwardedProps
	// null as absent too.
	res, err := agui.Sanitize("RunAgentInput", []byte(doc), agui.SanitizeOptions{TolerateLegacyNulls: true})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	m := res.Value.(map[string]any)
	if _, ok := m["state"]; ok {
		t.Fatalf("state appeared: %v", m)
	}
	if _, ok := m["forwardedProps"]; ok {
		t.Fatalf("forwardedProps null kept: %v", m)
	}
}

// ---------------------------------------------------------------------------
// TestNullOmissionCrossLanguageFixture
// ---------------------------------------------------------------------------

type nullOmissionCase struct {
	Name       string          `json:"name"`
	ProducedBy []string        `json:"producedBy"`
	Input      json.RawMessage `json:"input"`
	Expected   json.RawMessage `json:"expected"`
}

// fixtureCases returns every case of the shared fixture. The fixture names
// typescript/python/dotnet in producedBy; Go is held to all of them.
func fixtureCases(t *testing.T) []nullOmissionCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(nullOmissionFixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f struct {
		Stream []nullOmissionCase `json:"stream"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return f.Stream
}

// Python: tests/test_null_omission.py::TestNullOmissionCrossLanguageFixture::test_fixture_covers_this_sdk
func TestPyTestNullOmissionCrossLanguageFixture_FixtureCoversThisSdk(t *testing.T) {
	if n := len(fixtureCases(t)); n <= 15 {
		t.Fatalf("fixture lost most of its cases: %d", n)
	}
}

// Python: tests/test_null_omission.py::TestNullOmissionCrossLanguageFixture::test_every_case_reserializes_to_its_expected_json
func TestPyTestNullOmissionCrossLanguageFixture_EveryCaseReserializesToItsExpectedJson(t *testing.T) {
	for _, c := range fixtureCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			// Lenient typed parse, as TypeAdapter(Event).validate_python is:
			// an input may carry legacy whole-field nulls.
			ev, err := agui.UnmarshalEvent(c.Input)
			if err != nil {
				t.Fatalf("UnmarshalEvent(%s): %v", c.Input, err)
			}
			_, got := encodeFrame(t, ev)
			want := parse(t, string(c.Expected))
			if !reflect.DeepEqual(got, want) {
				gb, _ := json.Marshal(got)
				t.Fatalf("re-serialized\n got: %s\nwant: %s", gb, c.Expected)
			}
			// What the producer emits must also be a conformant event.
			b, _ := json.Marshal(got)
			if err := agui.Validate("Event", b); err != nil {
				t.Fatalf("emitted event is not schema-valid: %v", err)
			}
		})
	}
}

package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/ConformanceStreamTest.cs.
//
// The .NET conformance lane replays every fixture of the shared corpus
// (spec/1.0/conformance/streams) as raw SSE bytes into the real consumer and
// asserts the fixture's expectation with that lane's overrides
// ("expectOverrides.dotnet") applied. Those overrides record .NET gaps and
// .NET-surface limitations (no state store, builder-only events invisible to
// the application, ...), so this Go lane deliberately does NOT apply them: it
// resolves each fixture to its base "expect" — the specification's outcome —
// plus the one documented Go-lane override (see goLaneOverride), and asserts
// every expectation key, state included.
//
// pkg/client/conformance_test.go runs the same corpus from inside package
// client; its helpers are not importable from here, so this is a compact
// re-statement of the lane logic against the public client.Agent API.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

const conformanceStreamsDir = "../../testdata/spec-1.0/conformance/streams"

type conformanceFixture struct {
	Name   string `json:"name"`
	Client struct {
		MaxProtocolVersion string `json:"maxProtocolVersion"`
	} `json:"client"`
	Input struct {
		Messages       json.RawMessage `json:"messages"`
		Tools          []agui.Tool     `json:"tools"`
		Context        []agui.Context  `json:"context"`
		ForwardedProps json.RawMessage `json:"forwardedProps"`
	} `json:"input"`
	Stream          []json.RawMessage         `json:"stream"`
	Expect          map[string]any            `json:"-"`
	ExpectOverrides map[string]map[string]any `json:"-"`
}

func decodeGeneric(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

func loadConformanceFixture(t *testing.T, name string) conformanceFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(conformanceStreamsDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx conformanceFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	g, err := decodeGeneric(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := g.(map[string]any)
	fx.Expect, _ = doc["expect"].(map[string]any)
	if fx.Expect == nil {
		fx.Expect = map[string]any{}
	}
	if ov, ok := doc["expectOverrides"].(map[string]any); ok {
		fx.ExpectOverrides = map[string]map[string]any{}
		for lane, v := range ov {
			fx.ExpectOverrides[lane], _ = v.(map[string]any)
		}
	}
	return fx
}

func conformanceFixtureNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(conformanceStreamsDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	sort.Strings(names)
	return names
}

// goLaneOverride is the Go lane's own documented divergence from a base
// expectation (the corpus's mechanism for a lane that legitimately differs).
// unknown-enum-value-role-fatal pins an ADMITTED GAP of the reference client
// (its description says so): processing.mdx requires an unrecognised member
// of a closed string set to be stripped with a warning, not to fail the run.
var goLaneOverride = map[string]map[string]any{
	"unknown-enum-value-role-fatal": {"outcome": "completed", "errorContains": nil, "warnings": []any{"/role"}},
}

// resolveGoExpectation is the base expectation plus the Go lane's override.
// Unlike the .NET lane, expectOverrides.dotnet is never applied.
func resolveGoExpectation(fx conformanceFixture) map[string]any {
	out := map[string]any{}
	for k, v := range fx.Expect {
		out[k] = v
	}
	for k, v := range goLaneOverride[fx.Name] {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

type conformanceResult struct {
	outcome   string
	err       error
	events    []any
	types     []string
	warnings  []string
	messages  any
	state     any
	request   any
	runErrors []string
}

func replayConformance(t *testing.T, fx conformanceFixture) conformanceResult {
	t.Helper()
	var requestBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		requestBody = buf.Bytes()
		sse.SetHeaders(w.Header())
		sw := sse.NewWriter(w)
		for _, ev := range fx.Stream { // verbatim: never round-tripped through the typed models
			var compact bytes.Buffer
			_ = json.Compact(&compact, ev)
			_ = sw.WriteData(compact.Bytes())
		}
	}))
	defer srv.Close()

	agent, warnings := newAgent(srv.URL)
	agent.PeerVersion = fx.Client.MaxProtocolVersion
	in := client.RunInput{Tools: fx.Input.Tools, Context: fx.Input.Context}
	if len(fx.Stream) > 0 {
		var opener struct{ ThreadID, RunID string }
		_ = json.Unmarshal(fx.Stream[0], &opener)
		if opener.ThreadID != "" {
			agent.ThreadID = opener.ThreadID
		}
		in.RunID = opener.RunID
	}
	if len(fx.Input.Messages) > 0 {
		var msgs agui.Messages
		if err := json.Unmarshal(fx.Input.Messages, &msgs); err != nil {
			t.Fatalf("input messages: %v", err)
		}
		agent.Store.Messages = msgs
	}
	if len(fx.Input.ForwardedProps) > 0 {
		if string(bytes.TrimSpace(fx.Input.ForwardedProps)) == "null" {
			t.Skip("an explicit null forwardedProps is not representable in client.RunInput")
		}
		in.ForwardedProps, _ = decodeGeneric(fx.Input.ForwardedProps)
	}

	res, err := agent.Run(t.Context(), in, nil)
	out := conformanceResult{outcome: "completed", err: err, warnings: *warnings}
	if err != nil {
		out.outcome = "failed"
	}
	if res != nil {
		for _, e := range res.Events {
			out.types = append(out.types, string(e.Type()))
			g, _ := agui.ToGeneric(e)
			out.events = append(out.events, g)
		}
		for _, e := range res.Errors {
			out.runErrors = append(out.runErrors, e.Message)
		}
	}
	out.messages, _ = agui.ToGeneric(agent.Store.Messages)
	if out.messages == nil {
		out.messages = []any{}
	}
	out.state, _ = agui.ToGeneric(agent.Store.State)
	if requestBody != nil {
		out.request, _ = decodeGeneric(requestBody)
	}
	return out
}

// subsetMatch: every key the expectation names must be present and match;
// arrays match by length and element; null matches only an explicit null.
func subsetMatch(got, want any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, present := g[k]
			if !present || !subsetMatch(gv, wv) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !subsetMatch(g[i], w[i]) {
				return false
			}
		}
		return true
	case nil:
		return got == nil
	case json.Number:
		g, ok := got.(json.Number)
		if !ok {
			return false
		}
		wf, _ := w.Float64()
		gf, _ := g.Float64()
		return wf == gf
	default:
		return fmt.Sprint(got) == fmt.Sprint(want) && got != nil
	}
}

func assertConformance(t *testing.T, r conformanceResult, exp map[string]any) {
	t.Helper()
	defer func() {
		if t.Failed() {
			t.Logf("error: %v\nwarnings: %q\nevents: %v", r.err, r.warnings, r.types)
		}
	}()
	if o, ok := exp["outcome"].(string); ok && o != r.outcome {
		t.Errorf("expected the run to be %s, it was %s (%v)", o, r.outcome, r.err)
	}
	if want, ok := exp["eventTypes"].([]any); ok {
		var ws []string
		for _, x := range want {
			ws = append(ws, fmt.Sprint(x))
		}
		if !eqStrings(ws, r.types) {
			t.Errorf("eventTypes:\n got  %v\n want %v", r.types, ws)
		}
	}
	if absent, ok := exp["eventTypesAbsent"].([]any); ok {
		for _, a := range absent {
			for _, ty := range r.types {
				if ty == fmt.Sprint(a) {
					t.Errorf("%s must not reach application code", ty)
				}
			}
		}
	}
	if paths, ok := exp["eventPaths"].(map[string]any); ok {
		for p, want := range paths {
			got, present := at(r.events, p)
			if !present || !subsetMatch(got, want) {
				t.Errorf("eventPaths[%s]: got %s (present %v), want %s", p, jsonString(got), present, jsonString(want))
			}
		}
	}
	if paths, ok := exp["eventAbsentPaths"].([]any); ok {
		for _, p := range paths {
			if got, present := at(r.events, fmt.Sprint(p)); present {
				t.Errorf("eventAbsentPaths[%s]: present with %s", p, jsonString(got))
			}
		}
	}
	if s, ok := exp["errorContains"].(string); ok {
		if r.err == nil || !strings.Contains(strings.ReplaceAll(r.err.Error(), `\`, ""), strings.ReplaceAll(s, `\`, "")) {
			t.Errorf("rejection %v does not contain %q", r.err, s)
		}
	}
	switch want := exp["runError"].(type) {
	case bool:
		if want != (len(r.runErrors) > 0) {
			t.Errorf("runError %v; saw %q", want, r.runErrors)
		}
	case string:
		if !anyContains(r.runErrors, want) {
			t.Errorf("no reported run failure contains %q; saw %q", want, r.runErrors)
		}
	}
	if ws, ok := exp["warnings"].([]any); ok {
		for _, w := range ws {
			if !anyContains(r.warnings, fmt.Sprint(w)) {
				t.Errorf("no warning contains %q", w)
			}
		}
	}
	if nw, _ := exp["noWarnings"].(bool); nw && len(r.warnings) > 0 {
		t.Errorf("a conformant stream must not make the client complain; saw %q", r.warnings)
	}
	if n, ok := exp["messageCount"].(json.Number); ok {
		want, _ := n.Int64()
		if got := len(r.messages.([]any)); int64(got) != want {
			t.Errorf("messageCount: got %d want %d: %s", got, want, jsonString(r.messages))
		}
	}
	if want, ok := exp["messages"]; ok && !subsetMatch(r.messages, want) {
		t.Errorf("messages:\n got  %s\n want %s", jsonString(r.messages), jsonString(want))
	}
	// Asserted here although the .NET lane cannot (it has no state store).
	if want, ok := exp["state"]; ok && !subsetMatch(r.state, want) {
		t.Errorf("state: got %s want %s", jsonString(r.state), jsonString(want))
	}
	if want, ok := exp["request"]; ok && !subsetMatch(r.request, want) {
		t.Errorf("request:\n got  %s\n want %s", jsonString(r.request), jsonString(want))
	}
	if paths, ok := exp["requestAbsentPaths"].([]any); ok {
		for _, p := range paths {
			if got, present := at(r.request, fmt.Sprint(p)); present {
				t.Errorf("requestAbsentPaths[%s]: present with %s", p, jsonString(got))
			}
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/ConformanceStreamTest.cs::ConformanceStreamTest.HasFixturesToRun
//
// Also checks the invariant the .NET ResolveExpectation asserts while
// applying an override: every lane override states why it is intentional.
func TestCsConformanceStreamTest_HasFixturesToRun(t *testing.T) {
	names := conformanceFixtureNames(t)
	if len(names) == 0 {
		t.Fatalf("no fixtures under %s", conformanceStreamsDir)
	}
	for _, n := range names {
		fx := loadConformanceFixture(t, n)
		if fx.Name != n {
			t.Errorf("%s: name %q does not match the file", n, fx.Name)
		}
		for lane, o := range fx.ExpectOverrides {
			if s, _ := o["intentional"].(string); strings.TrimSpace(s) == "" {
				t.Errorf("%s: the %s override does not state why the divergence is deliberate", n, lane)
			}
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/ConformanceStreamTest.cs::ConformanceStreamTest.Fixture
//
// One subtest per fixture (the [MemberData] rows). The base expectation is
// asserted; a fixture's expectOverrides.dotnet is reported in the log and
// ignored — the .NET gaps it records are not rules.
func TestCsConformanceStreamTest_Fixture(t *testing.T) {
	for _, name := range conformanceFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			fx := loadConformanceFixture(t, name)
			if fx.Name != name {
				t.Fatalf("name %q does not match the file", fx.Name)
			}
			if _, ok := fx.ExpectOverrides["dotnet"]; ok {
				t.Logf("expectOverrides.dotnet ignored: the Go lane asserts the base expectation")
			}
			exp := resolveGoExpectation(fx)
			if _, ok := goLaneOverride[name]; ok {
				t.Logf("Go lane override applied (processing.mdx: unknown closed-set value is stripped with a warning)")
			}
			assertConformance(t, replayConformance(t, fx), exp)
		})
	}
}

// Guards the lane logic itself: resolving a fixture that carries a dotnet
// override must yield its base expectation, untouched by the override.
// (Supplementary to the port; no C# counterpart.)
func TestGoConformanceLane_IgnoresDotnetOverrides(t *testing.T) {
	checked := 0
	for _, name := range conformanceFixtureNames(t) {
		fx := loadConformanceFixture(t, name)
		dn, ok := fx.ExpectOverrides["dotnet"]
		if !ok {
			continue
		}
		if _, own := goLaneOverride[name]; own {
			continue
		}
		exp := resolveGoExpectation(fx)
		for k := range dn {
			if k == "intentional" {
				continue
			}
			if jsonString(exp[k]) != jsonString(fx.Expect[k]) {
				t.Errorf("%s: key %s resolved to %s, base is %s", name, k, jsonString(exp[k]), jsonString(fx.Expect[k]))
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no fixture carries a dotnet override; the corpus changed shape")
	}
}

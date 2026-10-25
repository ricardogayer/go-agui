package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// The official AG-UI 1.0 consumer conformance corpus
// (spec/1.0/conformance/streams in the AG-UI repository).
const streamsDir = "../agui/testdata/spec-1.0/conformance/streams"

type fixture struct {
	Name   string `json:"name"`
	Client struct {
		MaxProtocolVersion string `json:"maxProtocolVersion"`
	} `json:"client"`
	Input struct {
		Messages json.RawMessage `json:"messages"`
		Tools    []agui.Tool     `json:"tools"`
	} `json:"input"`
	Stream []json.RawMessage `json:"stream"`
	Expect expectation       `json:"expect"`
}

type expectation struct {
	Outcome            string         `json:"outcome"`
	ErrorContains      *string        `json:"errorContains"`
	RunError           any            `json:"runError"`
	EventTypes         []string       `json:"eventTypes"`
	EventTypesAbsent   []string       `json:"eventTypesAbsent"`
	EventPaths         map[string]any `json:"eventPaths"`
	EventAbsentPaths   []string       `json:"eventAbsentPaths"`
	Warnings           []string       `json:"warnings"`
	NoWarnings         bool           `json:"noWarnings"`
	MessageCount       *int           `json:"messageCount"`
	Messages           any            `json:"messages"`
	State              any            `json:"state"`
	HasState           bool           `json:"-"`
	Request            any            `json:"request"`
	RequestAbsentPaths []string       `json:"requestAbsentPaths"`
}

// goLaneOverrides records where this implementation deliberately differs
// from the corpus, each with its reason — the corpus's own mechanism for a
// lane that legitimately differs (its README).
var goLaneOverrides = map[string]struct {
	reason string
	apply  func(*expectation)
}{
	"unknown-enum-value-role-fatal": {
		reason: "The corpus pins an ADMITTED GAP of the reference client (its description says so): an unrecognised member of a closed string set is fatal there, where processing.mdx requires it to be stripped. This implementation follows the specification: /role is stripped with a warning, the absent role means assistant, and the run completes.",
		apply: func(e *expectation) {
			e.Outcome, e.ErrorContains, e.Warnings = "completed", nil, []string{"/role"}
		},
	},
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(streamsDir, "*.json"))
	if err != nil || len(files) < 60 {
		t.Fatalf("conformance corpus not found (%d files): %v", len(files), err)
	}
	var out []fixture
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var probe struct {
			Expect map[string]json.RawMessage `json:"expect"`
		}
		_ = json.Unmarshal(raw, &probe)
		if s, ok := probe.Expect["state"]; ok {
			fx.Expect.HasState = true
			fx.Expect.State, _ = generic(s)
		}
		if strings.TrimSuffix(filepath.Base(f), ".json") != fx.Name {
			t.Fatalf("%s: name %q does not match the file", f, fx.Name)
		}
		out = append(out, fx)
	}
	return out
}

func generic(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

func toGeneric(t *testing.T, v any) any {
	g, err := agui.ToGeneric(v)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestConformanceStreams replays every stream of the official corpus to the
// client over real HTTP + SSE and checks the outcome the corpus requires.
func TestConformanceStreams(t *testing.T) {
	for _, fx := range loadFixtures(t) {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			exp := fx.Expect
			if o, ok := goLaneOverrides[fx.Name]; ok {
				t.Logf("Go lane override: %s", o.reason)
				o.apply(&exp)
			}

			var requestBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestBody, _ = io.ReadAll(r.Body)
				sse.SetHeaders(w.Header())
				sw := sse.NewWriter(w)
				for _, ev := range fx.Stream {
					var compact bytes.Buffer
					_ = json.Compact(&compact, ev)
					_ = sw.WriteData(compact.Bytes())
				}
			}))
			defer srv.Close()

			agent := New(srv.URL)
			agent.PeerVersion = fx.Client.MaxProtocolVersion
			var warnings []string
			agent.Warn = func(w agui.Warning) { warnings = append(warnings, w.String()) }
			if len(fx.Input.Messages) > 0 {
				var msgs agui.Messages
				if err := json.Unmarshal(fx.Input.Messages, &msgs); err != nil {
					t.Fatalf("input messages: %v", err)
				}
				agent.Store.Messages = msgs
			}
			for _, ev := range fx.Stream {
				var rs struct{ Type, ThreadID, RunID string }
				_ = json.Unmarshal(ev, &rs)
				if rs.Type == "RUN_STARTED" {
					agent.ThreadID = rs.ThreadID
					break
				}
			}

			result, err := agent.Run(context.Background(), RunInput{Tools: fx.Input.Tools}, nil)
			defer func() {
				if t.Failed() {
					t.Logf("error: %v", err)
					t.Logf("warnings:\n  %s", strings.Join(warnings, "\n  "))
				}
			}()

			switch exp.Outcome {
			case "completed":
				if err != nil {
					t.Fatalf("stream rejected: %v", err)
				}
			case "failed":
				if err == nil {
					t.Fatalf("stream accepted, expected a rejection containing %q", deref(exp.ErrorContains))
				}
			}
			if exp.ErrorContains != nil && err != nil {
				msg := strings.ReplaceAll(err.Error(), `\`, "")
				if !strings.Contains(msg, strings.ReplaceAll(*exp.ErrorContains, `\`, "")) {
					t.Errorf("rejection %q does not contain %q", err, *exp.ErrorContains)
				}
			}
			if result == nil {
				return
			}
			switch want := exp.RunError.(type) {
			case bool:
				if want != (len(result.Errors) > 0) {
					t.Errorf("runError: status %s, errors %v", result.Status, result.Errors)
				}
			case string:
				found := false
				for _, e := range result.Errors {
					found = found || strings.Contains(e.Message, want)
				}
				if !found {
					t.Errorf("runError: want %q, got %+v", want, result.Errors)
				}
			}
			if exp.Outcome == "completed" && exp.RunError == nil && result.Status == StatusFailed {
				t.Errorf("run unexpectedly failed: %+v", result.Error)
			}

			var types []string
			var delivered []any
			for _, e := range result.Events {
				types = append(types, string(e.Type()))
				delivered = append(delivered, toGeneric(t, e))
			}
			if exp.EventTypes != nil && strings.Join(types, ",") != strings.Join(exp.EventTypes, ",") {
				t.Errorf("eventTypes:\n got  %v\n want %v", types, exp.EventTypes)
			}
			for _, absent := range exp.EventTypesAbsent {
				for _, ty := range types {
					if ty == absent {
						t.Errorf("%s reached the application", absent)
					}
				}
			}
			for path, want := range exp.EventPaths {
				got, ok := eventPath(delivered, path)
				if !ok || !subset(toGeneric(t, want), got) {
					t.Errorf("eventPaths[%s]: got %v (present=%v), want %v", path, got, ok, want)
				}
			}
			for _, path := range exp.EventAbsentPaths {
				if got, ok := eventPath(delivered, path); ok {
					t.Errorf("eventAbsentPaths[%s]: present with %v", path, got)
				}
			}
			for _, w := range exp.Warnings {
				found := false
				for _, got := range warnings {
					if strings.Contains(got, w) {
						found = true
					}
				}
				if !found {
					t.Errorf("no warning contains %q", w)
				}
			}
			if exp.NoWarnings && len(warnings) > 0 {
				t.Errorf("expected no warnings, got:\n  %s", strings.Join(warnings, "\n  "))
			}
			if exp.MessageCount != nil && len(agent.Store.Messages) != *exp.MessageCount {
				t.Errorf("messageCount: got %d want %d", len(agent.Store.Messages), *exp.MessageCount)
			}
			if exp.Messages != nil {
				if got := toGeneric(t, agent.Store.Messages); !subset(toGeneric(t, exp.Messages), got) {
					b, _ := json.Marshal(got)
					t.Errorf("messages:\n got  %s\n want %s", b, mustJSON(exp.Messages))
				}
			}
			if exp.HasState {
				if got := toGeneric(t, agent.Store.State); !subset(exp.State, got) {
					t.Errorf("state: got %s want %s", mustJSON(got), mustJSON(exp.State))
				}
			}
			if exp.Request != nil || exp.RequestAbsentPaths != nil {
				req, gerr := generic(requestBody)
				if gerr != nil {
					t.Fatalf("request body: %v", gerr)
				}
				if exp.Request != nil && !subset(toGeneric(t, exp.Request), req) {
					t.Errorf("request:\n got  %s\n want %s", requestBody, mustJSON(exp.Request))
				}
				for _, p := range exp.RequestAbsentPaths {
					if v, ok := dotted(req, p); ok {
						t.Errorf("requestAbsentPaths[%s]: present with %v", p, v)
					}
				}
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// eventPath reads "<index>.<dotted path>" from the delivered events.
func eventPath(events []any, path string) (any, bool) {
	idx, rest, _ := strings.Cut(path, ".")
	i, err := strconv.Atoi(idx)
	if err != nil || i < 0 || i >= len(events) {
		return nil, false
	}
	if rest == "" {
		return events[i], true
	}
	return dotted(events[i], rest)
}

func dotted(v any, path string) (any, bool) {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch t := cur.(type) {
		case map[string]any:
			x, ok := t[seg]
			if !ok {
				return nil, false
			}
			cur = x
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			cur = t[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// subset matches as the corpus specifies: objects on the keys named, arrays
// by length and element, scalars by value.
func subset(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !subset(wv, gv) {
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
			if !subset(w[i], g[i]) {
				return false
			}
		}
		return true
	case json.Number:
		g, ok := got.(json.Number)
		if !ok {
			return false
		}
		wf, _ := w.Float64()
		gf, _ := g.Float64()
		return wf == gf
	default:
		return fmt.Sprint(want) == fmt.Sprint(got) && (want == nil) == (got == nil)
	}
}

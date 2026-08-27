package agui_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
)

// ── the simplest path: one text turn / o caminho mais simples: um turno de texto ──

func TestTextTurnEmitsThePythonAdapterSequence(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Chunks: []string{"Hello", ", ", "world"}})
	ag := newAgent(t, m)

	evs := aguitest.Run(t, ag, aguitest.Input("hi"))

	aguitest.RequireSequence(t, evs,
		agui.EventRunStarted,
		agui.EventStepStarted,
		agui.EventTextMessageStart,
		agui.EventTextMessageChunk, agui.EventTextMessageChunk, agui.EventTextMessageChunk,
		agui.EventTextMessageEnd,
		agui.EventStepFinished,
		agui.EventStateSnapshot,
		agui.EventMessagesSnapshot,
		agui.EventRunFinished,
	)
	aguitest.RequireBalanced(t, evs)

	if got := aguitest.ConcatenatedText(evs); got != "Hello, world" {
		t.Fatalf("concatenated text = %q, want %q", got, "Hello, world")
	}
}

// ── server tools / tools de servidor ─────────────────────────────────────────

type addArgs struct {
	A int `json:"a" jsonschema:"description=first operand"`
	B int `json:"b" jsonschema:"description=second operand"`
}

func addTool() agui.Tool {
	return agui.NewTool("add", "Adds two numbers.",
		func(_ context.Context, in addArgs) (int, error) { return in.A + in.B, nil })
}

func TestServerToolRunsHereAndTheLoopContinues(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
			{ID: "call_1", Name: "add", Args: `{"a":2,"b":3}`},
		}},
		aguitest.Turn{Text: "It's 5."},
	)
	ag := newAgent(t, m, agui.WithTools(addTool()))

	evs := aguitest.Run(t, ag, aguitest.Input("what is 2+3?"))

	aguitest.RequireBalanced(t, evs)
	aguitest.RequireContains(t, evs,
		agui.EventToolCallStart, agui.EventToolCallArgs, agui.EventToolCallEnd,
		agui.EventToolCallResult,
		agui.EventTextMessageStart, agui.EventRunFinished)

	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("missing TOOL_CALL_RESULT")
	}
	if got := res.String("content"); got != "5" {
		t.Fatalf("tool result = %q, want %q", got, "5")
	}
}

func TestTypedToolSchemaReachesTheModelWithoutJSONSchemaScaffolding(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m, agui.WithTools(addTool()))

	_ = aguitest.Run(t, ag, aguitest.Input("hi"))

	bound := m.BoundTools()
	if len(bound) == 0 {
		t.Fatal("no tool was bound to the model")
	}
	// ParamsOneOf keeps the schema in unexported fields; ToJSONSchema is the
	// accessor — which is also the path the provider uses.
	//
	// PT: ParamsOneOf guarda o schema em campos não exportados; ToJSONSchema é o
	// acessor — que é também o caminho que o provedor usa.
	js, err := bound[0][0].ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("extracting the JSON Schema: %v", err)
	}
	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("serializing the schema: %v", err)
	}
	// Eino's reflector is configured with DoNotReference and strips $schema. If
	// that changes in an update, the providers that reject this scaffolding
	// start failing — and this test warns first.
	//
	// PT: O reflector do Eino é configurado com DoNotReference e limpa $schema. Se
	// isso mudar numa atualização, os provedores que recusam essa
	// infraestrutura passam a falhar — e este teste avisa antes.
	for _, forbidden := range []string{`"$ref"`, `"$defs"`, `"$schema"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("the tool schema carries %s, which must be sanitized:\n%s", forbidden, raw)
		}
	}
	if !strings.Contains(string(raw), "first operand") {
		t.Errorf("the struct tag description did not reach the schema:\n%s", raw)
	}
}

func TestUnknownToolBecomesAnErrorResultAndTheLoopContinues(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
			{ID: "call_1", Name: "nonexistent", Args: `{}`},
		}},
		aguitest.Turn{Text: "sorry"},
	)
	ag := newAgent(t, m, agui.WithTools(addTool()))

	evs := aguitest.Run(t, ag, aguitest.Input("do something"))

	aguitest.RequireBalanced(t, evs)
	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("an unknown tool must be ANSWERED, not ignored")
	}
	if !strings.Contains(res.String("content"), "unknown tool") {
		t.Fatalf("result = %q, want it to mention 'unknown tool'", res.String("content"))
	}
	// The run must end well: the model corrects itself on the next turn.
	//
	// PT: O run precisa terminar bem: o modelo se corrige no turno seguinte.
	if evs[len(evs)-1].Type() != agui.EventRunFinished {
		t.Fatalf("the run ended with %s, want RUN_FINISHED", evs[len(evs)-1].Type())
	}
}

// ── frontend tools: the hand-back / tools de frontend: o hand-back ───────────

func TestFrontendToolHandsControlBackToTheClient(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "showCard", Args: `{"title":"hi"}`},
	}})
	ag := newAgent(t, m, agui.WithTools(addTool()))

	in := aguitest.Input("show a card")
	in.Tools = []agui.ClientTool{{
		Name:        "showCard",
		Description: "Renders a card.",
		Parameters: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string"},
			},
		},
	}}

	evs := aguitest.Run(t, ag, in)

	aguitest.RequireBalanced(t, evs)
	// The server does NOT execute it: there is no TOOL_CALL_RESULT.
	//
	// PT: O servidor NÃO executa: não há TOOL_CALL_RESULT.
	if _, ok := aguitest.First(evs, agui.EventToolCallResult); ok {
		t.Fatal("the server must not execute a frontend tool")
	}
	if evs[len(evs)-1].Type() != agui.EventRunFinished {
		t.Fatalf("ended with %s, want RUN_FINISHED", evs[len(evs)-1].Type())
	}
	// The frontend tool must have been offered to the model alongside the backend ones.
	//
	// PT: A tool de frontend precisa ter sido oferecida ao modelo junto das de backend.
	names := m.LastBoundNames()
	if !contains(names, "add") || !contains(names, "showCard") {
		t.Fatalf("bound tools = %v, want to contain 'add' and 'showCard'", names)
	}
	// And Zod's `$schema` must have been removed.
	//
	// PT: E o `$schema` do Zod precisa ter sido removido.
	raw, _ := json.Marshal(m.BoundTools()[0])
	if strings.Contains(string(raw), `"$schema"`) {
		t.Errorf("Zod's $schema reached the provider:\n%s", raw)
	}
}

func TestFrontendToolWithInvalidSchemaIsDroppedWithoutFailingTheRun(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m)

	in := aguitest.Input("hi")
	in.Tools = []agui.ClientTool{
		{Name: "broken", Parameters: "this is not a schema"},
		{Name: "good", Parameters: map[string]any{"type": "object"}},
	}

	evs := aguitest.Run(t, ag, in)

	// A useFrontendTool with broken Zod must not kill the whole conversation.
	//
	// PT: Um useFrontendTool com Zod quebrado não pode matar a conversa inteira.
	if evs[len(evs)-1].Type() != agui.EventRunFinished {
		t.Fatalf("ended with %s, want RUN_FINISHED", evs[len(evs)-1].Type())
	}
	names := m.LastBoundNames()
	if contains(names, "broken") {
		t.Errorf("the invalid tool was offered to the model: %v", names)
	}
	if !contains(names, "good") {
		t.Errorf("the valid tool was dropped along with it: %v", names)
	}
}

// ── request context / contexto da requisição ─────────────────────────────────

func TestRequestContextBecomesASystemBlock(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m)

	in := aguitest.Input("hi")
	in.Context = []agui.Context{{Description: "catalog", Value: "Card, Row, Metric"}}

	_ = aguitest.Run(t, ag, in)

	var found bool
	for _, msg := range m.LastCall() {
		if strings.HasPrefix(msg.Content, "App Context:\n") &&
			strings.Contains(msg.Content, "Card, Row, Metric") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the App Context block did not reach the model; messages: %+v", m.LastCall())
	}
}

func TestNoContextDisablesTheContextBlock(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m, agui.NoContext())

	in := aguitest.Input("hi")
	in.Context = []agui.Context{{Description: "catalog", Value: "Card"}}

	_ = aguitest.Run(t, ag, in)

	for _, msg := range m.LastCall() {
		if strings.Contains(msg.Content, "App Context:") {
			t.Fatal("NoContext should not forward the context")
		}
	}
}

// ── construction / construção ────────────────────────────────────────────────

func TestNewAggregatesAllToolErrorsAtOnce(t *testing.T) {
	m := aguitest.NewFakeModel()
	_, err := agui.New(context.Background(), m, agui.WithTools(
		agui.NewRawTool("first", "d", json.RawMessage(`{ this is not json }`), noop),
		agui.NewRawTool("second", "d", json.RawMessage(`{ neither is this }`), noop),
	))
	if err == nil {
		t.Fatal("expected a construction error")
	}
	for _, name := range []string{"first", "second"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the error does not mention tool %q: %v", name, err)
		}
	}
}

func TestNewRejectsDuplicateToolNames(t *testing.T) {
	m := aguitest.NewFakeModel()
	_, err := agui.New(context.Background(), m, agui.WithTools(addTool(), addTool()))
	if err == nil || !strings.Contains(err.Error(), "registered twice") {
		t.Fatalf("expected a duplicate name error, got: %v", err)
	}
}

func TestNewRejectsMaxIterationsOutOfRange(t *testing.T) {
	m := aguitest.NewFakeModel()
	for _, n := range []int{0, -1, agui.MaxIterationsCeiling + 1} {
		if _, err := agui.New(context.Background(), m, agui.WithMaxIterations(n)); err == nil {
			t.Errorf("WithMaxIterations(%d) should fail", n)
		}
	}
}

func TestNonConvergenceEndsWithRunError(t *testing.T) {
	// The model insists on the same tool forever.
	//
	// PT: O modelo insiste na mesma tool para sempre.
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "add", Args: `{"a":1,"b":1}`},
	}})
	ag := newAgent(t, m, agui.WithTools(addTool()), agui.WithMaxIterations(2))

	evs := aguitest.Run(t, ag, aguitest.Input("add forever"))

	aguitest.RequireBalanced(t, evs)
	if evs[len(evs)-1].Type() != agui.EventRunError {
		t.Fatalf("ended with %s, want RUN_ERROR", evs[len(evs)-1].Type())
	}
}

// ── helpers / auxiliares ─────────────────────────────────────────────────────

func newAgent(t *testing.T, m *aguitest.FakeModel, opts ...agui.Option) *agui.Agent {
	t.Helper()
	ag, err := agui.New(context.Background(), m, opts...)
	if err != nil {
		t.Fatalf("building the agent: %v", err)
	}
	return ag
}

func noop(context.Context, string) (string, error) { return "", nil }

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

package agui_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
)

func TestStateAppliesAndEmitsTheSameOperation(t *testing.T) {
	// This is THE invariant of State: the patch applied locally is exactly what
	// goes on the wire. A client that starts from the STATE_SNAPSHOT and applies the
	// STATE_DELTAs in order rebuilds the server's document.
	//
	// PT: Este é O invariante do State: o patch aplicado localmente é exatamente o
	// que vai no fio. Um cliente que parta do STATE_SNAPSHOT e aplique os
	// STATE_DELTA em ordem reconstrói o documento do servidor.
	tool := agui.NewTool("mark", "Marks an item.",
		func(ctx context.Context, _ struct{}) (string, error) {
			st := agui.MustFromContext(ctx).State()
			if err := st.Set("/done", true); err != nil {
				return "", err
			}
			return `{"ok":true}`, nil
		})

	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "c1", Name: "mark", Args: `{}`}}},
		aguitest.Turn{Text: "done"},
	)
	ag := newAgent(t, m, agui.WithTools(tool))

	in := aguitest.Input("mark it")
	in.State = map[string]any{"done": false, "title": "task"}

	evs := aguitest.Run(t, ag, in)
	aguitest.RequireBalanced(t, evs)

	// The emitted delta...
	//
	// PT: O delta emitido...
	delta, ok := aguitest.First(evs, agui.EventStateDelta)
	if !ok {
		t.Fatal("missing STATE_DELTA")
	}
	var ops []agui.PatchOp
	remarshal(t, delta["delta"], &ops)
	if len(ops) != 1 || ops[0].Op != "add" || ops[0].Path != "/done" || ops[0].Value != true {
		t.Fatalf("unexpected delta: %+v", ops)
	}

	// ...and the final snapshot must agree.
	//
	// PT: ...e o snapshot final têm de concordar.
	snap, ok := last(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("missing STATE_SNAPSHOT")
	}
	var doc map[string]any
	remarshal(t, snap["snapshot"], &doc)
	want := map[string]any{"done": true, "title": "task"}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("final state = %+v, expected %+v", doc, want)
	}
}

func TestClientStateDoesNotRegressThroughTheAgent(t *testing.T) {
	// The default is a pure echo: an agent with no state of its own returns what it received.
	// Without this, the client's `agent.state` would be reset on every message.
	//
	// PT: O padrão é eco puro: um agente sem estado próprio devolve o que recebeu.
	// Sem isto, o `agent.state` do cliente seria zerado a cada mensagem.
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m)

	in := aguitest.Input("hi")
	in.State = map[string]any{"theme": "dark", "items": []any{"a", "b"}}

	evs := aguitest.Run(t, ag, in)

	snap, ok := last(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("missing STATE_SNAPSHOT")
	}
	var doc map[string]any
	remarshal(t, snap["snapshot"], &doc)
	if doc["theme"] != "dark" {
		t.Fatalf("returned state = %+v, expected theme=dark to be preserved", doc)
	}
}

func TestStateSnapshotIsNeverNull(t *testing.T) {
	// `null` is not a valid state on the client side.
	//
	// PT: `null` não é um estado válido do lado do cliente.
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := newAgent(t, m)

	evs := aguitest.Run(t, ag, aguitest.Input("hi")) // no State in the input (PT: sem State no input)

	snap, ok := last(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("missing STATE_SNAPSHOT")
	}
	if snap["snapshot"] == nil {
		t.Fatal("the snapshot came out as null")
	}
}

func TestStateAppliesBatchesAtomically(t *testing.T) {
	st := agui.NewState(map[string]any{"a": 1})

	err := st.Apply([]agui.PatchOp{
		{Op: "add", Path: "/b", Value: 2},
		{Op: "remove", Path: "/nonexistent"}, // fails (PT: falha)
	})
	if err == nil {
		t.Fatal("expected an error from the invalid patch")
	}
	// Nothing may have been applied: otherwise client and server diverge.
	//
	// PT: Nada pode ter sido aplicado: senão o cliente e o servidor divergem.
	if _, ok := st.Get("/b"); ok {
		t.Fatal("the batch failed but was partially applied")
	}
}

func TestStateGetResolvesJSONPointer(t *testing.T) {
	st := agui.NewState(map[string]any{
		"todos": []any{
			map[string]any{"id": "t1", "title": "buy bread"},
		},
		"a/b": "escaped slash",
	})

	cases := map[string]any{
		"/todos/0/title": "buy bread",
		"/a~1b":          "escaped slash",
	}
	for path, want := range cases {
		got, ok := st.Get(path)
		if !ok || got != want {
			t.Errorf("Get(%q) = %v, %v; expected %v", path, got, ok, want)
		}
	}
	if _, ok := st.Get("/todos/9"); ok {
		t.Error("index out of the array should return false")
	}
}

// ── HITL ─────────────────────────────────────────────────────────────────────

type purchaseArgs struct {
	Amount int `json:"amount" jsonschema:"description=amount in dollars"`
}

// purchaseTool asks for human confirmation before acting. It is the pattern documented in
// [agui.Interrupt].
//
// PT: purchaseTool pede confirmação humana antes de agir. É o padrão documentado em
// [agui.Interrupt].
func purchaseTool(t *testing.T, executed *int) agui.Tool {
	return agui.NewTool("purchase", "Buys an item.",
		func(ctx context.Context, in purchaseArgs) (string, error) {
			rc := agui.MustFromContext(ctx)
			if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
				*executed++
				return `{"purchased":true,"approval":` + mustJSON(t, payload) + `}`, nil
			}
			return "", agui.Interrupt(agui.InterruptRequest{
				Reason:  "needs_approval",
				Message: "Confirm the purchase?",
			})
		})
}

func TestToolRequestingADecisionPausesTheRunWithInterruptOutcome(t *testing.T) {
	var executed int
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "purchase", Args: `{"amount":100}`},
	}})
	ag := newAgent(t, m, agui.WithTools(purchaseTool(t, &executed)))

	evs := aguitest.Run(t, ag, aguitest.Input("buy it"))

	aguitest.RequireBalanced(t, evs)
	if executed != 0 {
		t.Fatal("the tool must not have acted before the approval")
	}
	// The call is left UNANSWERED on purpose: it is what the client renders
	// as the question.
	//
	// PT: A chamada fica SEM resposta de propósito: é ela que o cliente renderiza
	// como a pergunta.
	if _, ok := aguitest.First(evs, agui.EventToolCallResult); ok {
		t.Fatal("a paused call must not have a TOOL_CALL_RESULT")
	}

	final := evs[len(evs)-1]
	if final.Type() != agui.EventRunFinished {
		t.Fatalf("ended with %s, expected RUN_FINISHED", final.Type())
	}
	raw := mustJSON(t, final["outcome"])
	if !strings.Contains(raw, "needs_approval") || !strings.Contains(raw, "call_1") {
		t.Fatalf("the outcome does not describe the interrupt: %s", raw)
	}
}

func TestResumeRunsThePausedToolBeforeCallingTheModel(t *testing.T) {
	var executed int
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "purchase done"})
	ag := newAgent(t, m, agui.WithTools(purchaseTool(t, &executed)))

	// The next run: the client resends the history with the pending call and the
	// decision. There is no store on the server — the call ARRIVES in the conversation.
	//
	// PT: O run seguinte: o cliente reenvia o histórico com a chamada pendente e a
	// decisão. Não há store no servidor — a chamada CHEGA na conversa.
	in := aguitest.Input("buy it")
	in.Messages = append(in.Messages, &agui.AssistantMessage{
		ID: "msg_2",
		ToolCalls: []agui.ToolCall{{
			ID:       "call_1",
			Function: agui.FunctionCall{Name: "purchase", Arguments: `{"amount":100}`},
		}},
	})
	in.Resume = []agui.ResumeEntry{{
		InterruptID: "call_1",
		Status:      agui.ResumeStatusResolved,
		Payload:     map[string]any{"by": "ricardo"},
	}}

	evs := aguitest.Run(t, ag, in)

	aguitest.RequireBalanced(t, evs)
	if executed != 1 {
		t.Fatalf("the tool ran %d times on resume, expected 1", executed)
	}
	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("the resume should close the call with a TOOL_CALL_RESULT")
	}
	if !strings.Contains(res.String("content"), "ricardo") {
		t.Fatalf("the approval payload did not reach the tool: %s", res.String("content"))
	}
	if evs[len(evs)-1].Type() != agui.EventRunFinished {
		t.Fatalf("ended with %s, expected RUN_FINISHED", evs[len(evs)-1].Type())
	}
}

func TestCancelledDecisionDoesNotRunTheToolAndTheModelContinues(t *testing.T) {
	var executed int
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "alright, cancelled"})
	ag := newAgent(t, m, agui.WithTools(purchaseTool(t, &executed)))

	in := aguitest.Input("buy it")
	in.Messages = append(in.Messages, &agui.AssistantMessage{
		ID: "msg_2",
		ToolCalls: []agui.ToolCall{{
			ID:       "call_1",
			Function: agui.FunctionCall{Name: "purchase", Arguments: `{"amount":100}`},
		}},
	})
	in.Resume = []agui.ResumeEntry{{
		InterruptID: "call_1",
		Status:      agui.ResumeStatusCancelled,
	}}

	evs := aguitest.Run(t, ag, in)

	// Without this the loop would spin forever: the tool would request an interrupt
	// again and again, because ResumeFor returns nothing on a cancellation.
	//
	// PT: Sem isto o laço entraria em loop: a tool pediria interrupt de novo para
	// sempre, porque ResumeFor não devolve nada num cancelamento.
	if executed != 0 {
		t.Fatal("a cancelled decision must not execute the tool")
	}
	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("the cancelled call needs an answer for the model to continue")
	}
	if !strings.Contains(res.String("content"), "cancelled") {
		t.Fatalf("result = %q, expected it to indicate cancellation", res.String("content"))
	}
	if evs[len(evs)-1].Type() != agui.EventRunFinished {
		t.Fatalf("ended with %s, expected RUN_FINISHED", evs[len(evs)-1].Type())
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func last(evs []aguitest.Event, typ agui.EventType) (aguitest.Event, bool) {
	all := aguitest.All(evs, typ)
	if len(all) == 0 {
		return nil, false
	}
	return all[len(all)-1], true
}

func remarshal(t *testing.T, from any, into any) {
	t.Helper()
	b, err := json.Marshal(from)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	return string(b)
}

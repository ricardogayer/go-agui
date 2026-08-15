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

func TestStateAplicaEEmiteAMesmaOperacao(t *testing.T) {
	// Este é O invariante do State: o patch aplicado localmente é exatamente o
	// que vai no fio. Um cliente que parta do STATE_SNAPSHOT e aplique os
	// STATE_DELTA em ordem reconstrói o documento do servidor.
	tool := agui.NewTool("marcar", "Marca um item.",
		func(ctx context.Context, _ struct{}) (string, error) {
			st := agui.MustFromContext(ctx).State()
			if err := st.Set("/feito", true); err != nil {
				return "", err
			}
			return `{"ok":true}`, nil
		})

	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "c1", Name: "marcar", Args: `{}`}}},
		aguitest.Turn{Text: "pronto"},
	)
	ag := novoAgente(t, m, agui.WithTools(tool))

	in := aguitest.Input("marque")
	in.State = map[string]any{"feito": false, "titulo": "tarefa"}

	evs := aguitest.Run(t, ag, in)
	aguitest.RequireBalanced(t, evs)

	// O delta emitido...
	delta, ok := aguitest.Primeiro(evs, agui.EventStateDelta)
	if !ok {
		t.Fatal("faltou o STATE_DELTA")
	}
	var ops []agui.PatchOp
	remarshal(t, delta["delta"], &ops)
	if len(ops) != 1 || ops[0].Op != "add" || ops[0].Path != "/feito" || ops[0].Value != true {
		t.Fatalf("delta inesperado: %+v", ops)
	}

	// ...e o snapshot final têm de concordar.
	snap, ok := ultimo(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("faltou o STATE_SNAPSHOT")
	}
	var doc map[string]any
	remarshal(t, snap["snapshot"], &doc)
	esperado := map[string]any{"feito": true, "titulo": "tarefa"}
	if !reflect.DeepEqual(doc, esperado) {
		t.Fatalf("estado final = %+v, esperava %+v", doc, esperado)
	}
}

func TestEstadoDoClienteNaoRegridePorPassarPeloAgente(t *testing.T) {
	// O padrão é eco puro: um agente sem estado próprio devolve o que recebeu.
	// Sem isto, o `agent.state` do cliente seria zerado a cada mensagem.
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m)

	in := aguitest.Input("oi")
	in.State = map[string]any{"tema": "escuro", "itens": []any{"a", "b"}}

	evs := aguitest.Run(t, ag, in)

	snap, ok := ultimo(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("faltou o STATE_SNAPSHOT")
	}
	var doc map[string]any
	remarshal(t, snap["snapshot"], &doc)
	if doc["tema"] != "escuro" {
		t.Fatalf("estado devolvido = %+v, esperava preservar tema=escuro", doc)
	}
}

func TestStateSnapshotNuncaSaiComoNull(t *testing.T) {
	// `null` não é um estado válido do lado do cliente.
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m)

	evs := aguitest.Run(t, ag, aguitest.Input("oi")) // sem State no input

	snap, ok := ultimo(evs, agui.EventStateSnapshot)
	if !ok {
		t.Fatal("faltou o STATE_SNAPSHOT")
	}
	if snap["snapshot"] == nil {
		t.Fatal("o snapshot saiu como null")
	}
}

func TestStateAplicaEmLoteDeFormaAtomica(t *testing.T) {
	st := agui.NewState(map[string]any{"a": 1})

	err := st.Apply([]agui.PatchOp{
		{Op: "add", Path: "/b", Value: 2},
		{Op: "remove", Path: "/inexistente"}, // falha
	})
	if err == nil {
		t.Fatal("esperava erro do patch inválido")
	}
	// Nada pode ter sido aplicado: senão o cliente e o servidor divergem.
	if _, ok := st.Get("/b"); ok {
		t.Fatal("o lote falhou mas aplicou parcialmente")
	}
}

func TestStateGetResolveJSONPointer(t *testing.T) {
	st := agui.NewState(map[string]any{
		"todos": []any{
			map[string]any{"id": "t1", "titulo": "comprar pão"},
		},
		"a/b": "barra escapada",
	})

	casos := map[string]any{
		"/todos/0/titulo": "comprar pão",
		"/a~1b":           "barra escapada",
	}
	for path, esperado := range casos {
		got, ok := st.Get(path)
		if !ok || got != esperado {
			t.Errorf("Get(%q) = %v, %v; esperava %v", path, got, ok, esperado)
		}
	}
	if _, ok := st.Get("/todos/9"); ok {
		t.Error("índice fora do array deveria devolver false")
	}
}

// ── HITL ─────────────────────────────────────────────────────────────────────

type argsCompra struct {
	Valor int `json:"valor" jsonschema:"description=valor em reais"`
}

// toolComprar pede confirmação humana antes de agir. É o padrão documentado em
// [agui.Interrupt].
func toolComprar(t *testing.T, executou *int) agui.Tool {
	return agui.NewTool("comprar", "Compra um item.",
		func(ctx context.Context, in argsCompra) (string, error) {
			rc := agui.MustFromContext(ctx)
			if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
				*executou++
				return `{"comprado":true,"aprovacao":` + mustJSON(t, payload) + `}`, nil
			}
			return "", agui.Interrupt(agui.InterruptRequest{
				Reason:  "needs_approval",
				Message: "Confirmar a compra?",
			})
		})
}

func TestToolQuePedeDecisaoPausaORunComOutcomeDeInterrupt(t *testing.T) {
	var executou int
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "comprar", Args: `{"valor":100}`},
	}})
	ag := novoAgente(t, m, agui.WithTools(toolComprar(t, &executou)))

	evs := aguitest.Run(t, ag, aguitest.Input("compre"))

	aguitest.RequireBalanced(t, evs)
	if executou != 0 {
		t.Fatal("a tool não pode ter agido antes da aprovação")
	}
	// A chamada fica SEM resposta de propósito: é ela que o cliente renderiza
	// como a pergunta.
	if _, ok := aguitest.Primeiro(evs, agui.EventToolCallResult); ok {
		t.Fatal("uma chamada pausada não pode ter TOOL_CALL_RESULT")
	}

	fim := evs[len(evs)-1]
	if fim.Tipo() != agui.EventRunFinished {
		t.Fatalf("terminou com %s, esperava RUN_FINISHED", fim.Tipo())
	}
	raw := mustJSON(t, fim["outcome"])
	if !strings.Contains(raw, "needs_approval") || !strings.Contains(raw, "call_1") {
		t.Fatalf("o outcome não descreve o interrupt: %s", raw)
	}
}

func TestRetomadaExecutaAToolPausadaSemChamarOModeloAntes(t *testing.T) {
	var executou int
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "compra feita"})
	ag := novoAgente(t, m, agui.WithTools(toolComprar(t, &executou)))

	// O run seguinte: o cliente reenvia o histórico com a chamada pendente e a
	// decisão. Não há store no servidor — a chamada CHEGA na conversa.
	in := aguitest.Input("compre")
	in.Messages = append(in.Messages, agui.Message{
		ID:   "msg_2",
		Role: agui.RoleAssistant,
		ToolCalls: []agui.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: agui.FunctionCall{Name: "comprar", Arguments: `{"valor":100}`},
		}},
	})
	in.Resume = []agui.ResumeEntry{{
		InterruptID: "call_1",
		Status:      agui.ResumeStatusResolved,
		Payload:     map[string]any{"por": "ricardo"},
	}}

	evs := aguitest.Run(t, ag, in)

	aguitest.RequireBalanced(t, evs)
	if executou != 1 {
		t.Fatalf("a tool rodou %d vezes na retomada, esperava 1", executou)
	}
	res, ok := aguitest.Primeiro(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("a retomada deveria fechar a chamada com um TOOL_CALL_RESULT")
	}
	if !strings.Contains(res.String("content"), "ricardo") {
		t.Fatalf("o payload da aprovação não chegou à tool: %s", res.String("content"))
	}
	if evs[len(evs)-1].Tipo() != agui.EventRunFinished {
		t.Fatalf("terminou com %s, esperava RUN_FINISHED", evs[len(evs)-1].Tipo())
	}
}

func TestDecisaoCanceladaNaoExecutaAToolEOModeloSegue(t *testing.T) {
	var executou int
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "tudo bem, cancelei"})
	ag := novoAgente(t, m, agui.WithTools(toolComprar(t, &executou)))

	in := aguitest.Input("compre")
	in.Messages = append(in.Messages, agui.Message{
		ID:   "msg_2",
		Role: agui.RoleAssistant,
		ToolCalls: []agui.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: agui.FunctionCall{Name: "comprar", Arguments: `{"valor":100}`},
		}},
	})
	in.Resume = []agui.ResumeEntry{{
		InterruptID: "call_1",
		Status:      agui.ResumeStatusCancelled,
	}}

	evs := aguitest.Run(t, ag, in)

	// Sem isto o laço entraria em loop: a tool pediria interrupt de novo para
	// sempre, porque ResumeFor não devolve nada num cancelamento.
	if executou != 0 {
		t.Fatal("uma decisão cancelada não pode executar a tool")
	}
	res, ok := aguitest.Primeiro(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("a chamada cancelada precisa ser respondida para o modelo seguir")
	}
	if !strings.Contains(res.String("content"), "cancelled") {
		t.Fatalf("resultado = %q, esperava indicar cancelamento", res.String("content"))
	}
	if evs[len(evs)-1].Tipo() != agui.EventRunFinished {
		t.Fatalf("terminou com %s, esperava RUN_FINISHED", evs[len(evs)-1].Tipo())
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ultimo(evs []aguitest.Evento, tipo agui.EventType) (aguitest.Evento, bool) {
	todos := aguitest.Todos(evs, tipo)
	if len(todos) == 0 {
		return nil, false
	}
	return todos[len(todos)-1], true
}

func remarshal(t *testing.T, from any, into any) {
	t.Helper()
	b, err := json.Marshal(from)
	if err != nil {
		t.Fatalf("serializando: %v", err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatalf("desserializando: %v", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("serializando: %v", err)
	}
	return string(b)
}

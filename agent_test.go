package agui_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
)

// ── o caminho mais simples: um turno de texto ────────────────────────────────

func TestTurnoDeTextoEmiteASequenciaDoAdaptadorPython(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Chunks: []string{"Olá", ", ", "mundo"}})
	ag := novoAgente(t, m)

	evs := aguitest.Run(t, ag, aguitest.Input("oi"))

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

	if got := aguitest.TextoConcatenado(evs); got != "Olá, mundo" {
		t.Fatalf("texto concatenado = %q, esperava %q", got, "Olá, mundo")
	}
}

// ── tools de servidor ────────────────────────────────────────────────────────

type argsSoma struct {
	A int `json:"a" jsonschema:"description=primeira parcela"`
	B int `json:"b" jsonschema:"description=segunda parcela"`
}

func toolSoma() agui.Tool {
	return agui.NewTool("somar", "Soma dois números.",
		func(_ context.Context, in argsSoma) (int, error) { return in.A + in.B, nil })
}

func TestToolDeServidorRodaAquiEOLacoContinua(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
			{ID: "call_1", Name: "somar", Args: `{"a":2,"b":3}`},
		}},
		aguitest.Turn{Text: "São 5."},
	)
	ag := novoAgente(t, m, agui.WithTools(toolSoma()))

	evs := aguitest.Run(t, ag, aguitest.Input("quanto é 2+3?"))

	aguitest.RequireBalanced(t, evs)
	aguitest.RequireContains(t, evs,
		agui.EventToolCallStart, agui.EventToolCallArgs, agui.EventToolCallEnd,
		agui.EventToolCallResult,
		agui.EventTextMessageStart, agui.EventRunFinished)

	res, ok := aguitest.Primeiro(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("faltou o TOOL_CALL_RESULT")
	}
	if got := res.String("content"); got != "5" {
		t.Fatalf("resultado da tool = %q, esperava %q", got, "5")
	}
}

func TestSchemaDaToolTipadaChegaAoModeloSemInfraestruturaJSONSchema(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m, agui.WithTools(toolSoma()))

	_ = aguitest.Run(t, ag, aguitest.Input("oi"))

	bound := m.BoundTools()
	if len(bound) == 0 {
		t.Fatal("nenhuma tool foi ligada ao modelo")
	}
	// ParamsOneOf guarda o schema em campos não exportados; ToJSONSchema é o
	// acessor — que é também o caminho que o provedor usa.
	js, err := bound[0][0].ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("extraindo o JSON Schema: %v", err)
	}
	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("serializando o schema: %v", err)
	}
	// O reflector do Eino é configurado com DoNotReference e limpa $schema. Se
	// isso mudar numa atualização, os provedores que recusam essa
	// infraestrutura passam a falhar — e este teste avisa antes.
	for _, proibido := range []string{`"$ref"`, `"$defs"`, `"$schema"`} {
		if strings.Contains(string(raw), proibido) {
			t.Errorf("o schema da tool traz %s, que precisa ser saneado:\n%s", proibido, raw)
		}
	}
	if !strings.Contains(string(raw), "primeira parcela") {
		t.Errorf("a descrição da struct tag não chegou ao schema:\n%s", raw)
	}
}

func TestToolInexistenteViraResultadoDeErroEOLacoSegue(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
			{ID: "call_1", Name: "inexistente", Args: `{}`},
		}},
		aguitest.Turn{Text: "desculpe"},
	)
	ag := novoAgente(t, m, agui.WithTools(toolSoma()))

	evs := aguitest.Run(t, ag, aguitest.Input("faça algo"))

	aguitest.RequireBalanced(t, evs)
	res, ok := aguitest.Primeiro(evs, agui.EventToolCallResult)
	if !ok {
		t.Fatal("uma tool desconhecida deve ser RESPONDIDA, não ignorada")
	}
	if !strings.Contains(res.String("content"), "unknown tool") {
		t.Fatalf("resultado = %q, esperava mencionar 'unknown tool'", res.String("content"))
	}
	// O run precisa terminar bem: o modelo se corrige no turno seguinte.
	if evs[len(evs)-1].Tipo() != agui.EventRunFinished {
		t.Fatalf("o run terminou com %s, esperava RUN_FINISHED", evs[len(evs)-1].Tipo())
	}
}

// ── tools de frontend: o hand-back ───────────────────────────────────────────

func TestToolDeFrontendDevolveOControleAoCliente(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "mostrarCartao", Args: `{"titulo":"oi"}`},
	}})
	ag := novoAgente(t, m, agui.WithTools(toolSoma()))

	in := aguitest.Input("mostre um cartão")
	in.Tools = []agui.ClientTool{{
		Name:        "mostrarCartao",
		Description: "Renderiza um cartão.",
		Parameters: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
			"properties": map[string]any{
				"titulo": map[string]any{"type": "string"},
			},
		},
	}}

	evs := aguitest.Run(t, ag, in)

	aguitest.RequireBalanced(t, evs)
	// O servidor NÃO executa: não há TOOL_CALL_RESULT.
	if _, ok := aguitest.Primeiro(evs, agui.EventToolCallResult); ok {
		t.Fatal("o servidor não pode executar uma tool de frontend")
	}
	if evs[len(evs)-1].Tipo() != agui.EventRunFinished {
		t.Fatalf("terminou com %s, esperava RUN_FINISHED", evs[len(evs)-1].Tipo())
	}
	// A tool de frontend precisa ter sido oferecida ao modelo junto das de backend.
	nomes := m.LastBoundNames()
	if !contem(nomes, "somar") || !contem(nomes, "mostrarCartao") {
		t.Fatalf("tools ligadas = %v, esperava conter 'somar' e 'mostrarCartao'", nomes)
	}
	// E o `$schema` do Zod precisa ter sido removido.
	raw, _ := json.Marshal(m.BoundTools()[0])
	if strings.Contains(string(raw), `"$schema"`) {
		t.Errorf("o $schema do Zod chegou ao provedor:\n%s", raw)
	}
}

func TestToolDeFrontendComSchemaInvalidoEDescartadaSemDerrubarORun(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m)

	in := aguitest.Input("oi")
	in.Tools = []agui.ClientTool{
		{Name: "quebrada", Parameters: "isto não é um schema"},
		{Name: "boa", Parameters: map[string]any{"type": "object"}},
	}

	evs := aguitest.Run(t, ag, in)

	// Um useFrontendTool com Zod quebrado não pode matar a conversa inteira.
	if evs[len(evs)-1].Tipo() != agui.EventRunFinished {
		t.Fatalf("terminou com %s, esperava RUN_FINISHED", evs[len(evs)-1].Tipo())
	}
	nomes := m.LastBoundNames()
	if contem(nomes, "quebrada") {
		t.Errorf("a tool inválida foi oferecida ao modelo: %v", nomes)
	}
	if !contem(nomes, "boa") {
		t.Errorf("a tool válida foi descartada junto: %v", nomes)
	}
}

// ── contexto da requisição ───────────────────────────────────────────────────

func TestContextoDaRequisicaoViraBlocoDeSistema(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m)

	in := aguitest.Input("oi")
	in.Context = []agui.Context{{Description: "catálogo", Value: "Card, Row, Metric"}}

	_ = aguitest.Run(t, ag, in)

	var achou bool
	for _, msg := range m.LastCall() {
		if strings.HasPrefix(msg.Content, "App Context:\n") &&
			strings.Contains(msg.Content, "Card, Row, Metric") {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("o bloco App Context não chegou ao modelo; mensagens: %+v", m.LastCall())
	}
}

func TestNoContextDesligaOBlocoDeContexto(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "ok"})
	ag := novoAgente(t, m, agui.NoContext())

	in := aguitest.Input("oi")
	in.Context = []agui.Context{{Description: "catálogo", Value: "Card"}}

	_ = aguitest.Run(t, ag, in)

	for _, msg := range m.LastCall() {
		if strings.Contains(msg.Content, "App Context:") {
			t.Fatal("NoContext não deveria repassar o contexto")
		}
	}
}

// ── construção ───────────────────────────────────────────────────────────────

func TestNewAgregaOsErrosDeTodasAsToolsDeUmaVez(t *testing.T) {
	m := aguitest.NewFakeModel()
	_, err := agui.New(context.Background(), m, agui.WithTools(
		agui.NewRawTool("primeira", "d", json.RawMessage(`{ isto não é json }`), nada),
		agui.NewRawTool("segunda", "d", json.RawMessage(`{ também não }`), nada),
	))
	if err == nil {
		t.Fatal("esperava erro de construção")
	}
	for _, nome := range []string{"primeira", "segunda"} {
		if !strings.Contains(err.Error(), nome) {
			t.Errorf("o erro não menciona a tool %q: %v", nome, err)
		}
	}
}

func TestNewRecusaToolsComNomeRepetido(t *testing.T) {
	m := aguitest.NewFakeModel()
	_, err := agui.New(context.Background(), m, agui.WithTools(toolSoma(), toolSoma()))
	if err == nil || !strings.Contains(err.Error(), "duas vezes") {
		t.Fatalf("esperava erro de nome duplicado, veio: %v", err)
	}
}

func TestNewRecusaMaxIterationsForaDaFaixa(t *testing.T) {
	m := aguitest.NewFakeModel()
	for _, n := range []int{0, -1, agui.MaxIterationsCeiling + 1} {
		if _, err := agui.New(context.Background(), m, agui.WithMaxIterations(n)); err == nil {
			t.Errorf("WithMaxIterations(%d) deveria falhar", n)
		}
	}
}

func TestNaoConvergirTerminaComRunError(t *testing.T) {
	// O modelo insiste na mesma tool para sempre.
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_1", Name: "somar", Args: `{"a":1,"b":1}`},
	}})
	ag := novoAgente(t, m, agui.WithTools(toolSoma()), agui.WithMaxIterations(2))

	evs := aguitest.Run(t, ag, aguitest.Input("some para sempre"))

	aguitest.RequireBalanced(t, evs)
	if evs[len(evs)-1].Tipo() != agui.EventRunError {
		t.Fatalf("terminou com %s, esperava RUN_ERROR", evs[len(evs)-1].Tipo())
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func novoAgente(t *testing.T, m *aguitest.FakeModel, opts ...agui.Option) *agui.Agent {
	t.Helper()
	ag, err := agui.New(context.Background(), m, opts...)
	if err != nil {
		t.Fatalf("construindo o agente: %v", err)
	}
	return ag
}

func nada(context.Context, string) (string, error) { return "", nil }

func contem(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

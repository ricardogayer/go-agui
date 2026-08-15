package agui

import (
	"strings"
	"testing"

	types "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/cloudwego/eino/schema"
)

func assistantWithCalls(ids ...string) *schema.Message {
	calls := make([]schema.ToolCall, 0, len(ids))
	for _, id := range ids {
		calls = append(calls, schema.ToolCall{ID: id, Function: schema.FunctionCall{Name: "generate_a2ui"}})
	}
	return &schema.Message{Role: schema.Assistant, ToolCalls: calls}
}

func describe(msgs []*schema.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case schema.Tool:
			parts = append(parts, "tool:"+m.ToolCallID)
		case schema.Assistant:
			ids := make([]string, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				ids = append(ids, tc.ID)
			}
			parts = append(parts, "assistant["+strings.Join(ids, "+")+"]")
		default:
			parts = append(parts, string(m.Role))
		}
	}
	return strings.Join(parts, ",")
}

// O caminho normal da lição: o modelo chama a tool de frontend `generate_a2ui`, o run
// termina, o cliente renderiza e reenvia o histórico com a tool call sem
// resposta. Sem o reparo, a OpenAI recusa a requisição inteira com 400.
func TestRepairAnswersComponentCallLeftOpenByTheClient(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("monte um dashboard"),
		assistantWithCalls("call_gen"),
		schema.UserMessage("agora some com ele"),
	}
	got := describe(RepairToolPairs(msgs))
	want := "user,assistant[call_gen],tool:call_gen,user"
	if got != want {
		t.Fatalf("reparo =\n  %s\nesperado\n  %s", got, want)
	}
}

// Uma resposta real não pode ser duplicada por uma sintética.
func TestRepairLeavesAnsweredCallsAlone(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("quantas linhas?"),
		assistantWithCalls("call_q"),
		schema.ToolMessage("[]", "call_q"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "user,assistant[call_q],tool:call_q" {
		t.Fatalf("reparo mexeu num histórico já íntegro: %s", got)
	}
	// Sem reparo a fatia original é devolvida como está — sem cópia.
	if &out[0] != &msgs[0] {
		t.Fatal("o histórico íntegro foi copiado à toa")
	}
}

// Numa mensagem com duas chamadas, só a que ficou aberta recebe resposta.
func TestRepairFillsOnlyTheMissingCall(t *testing.T) {
	msgs := []*schema.Message{
		assistantWithCalls("call_a", "call_b"),
		schema.ToolMessage("[]", "call_b"),
	}
	got := describe(RepairToolPairs(msgs))
	want := "assistant[call_a+call_b],tool:call_a,tool:call_b"
	if got != want {
		t.Fatalf("reparo =\n  %s\nesperado\n  %s", got, want)
	}
}

// TestRepairPreencheOResultadoVazioDaToolDeFrontend recupera um teste que
// existia na lição 2 e foi perdido nas cópias seguintes, embora o caminho de
// código (noToolResult) continue vivo nas três.
func TestRepairPreencheOResultadoVazioDaToolDeFrontend(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("mostre um gráfico"),
		assistantWithCalls("call_pie"),
		schema.ToolMessage("", "call_pie"),
		schema.UserMessage("e agora?"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "user,assistant[call_pie],tool:call_pie,user" {
		t.Fatalf("reparo = %s", got)
	}
	if out[2].Content == "" {
		t.Fatal("a resposta vazia sobreviveu; o modelo fica sem saber se a tool rodou")
	}
	if msgs[2].Content != "" {
		t.Fatalf("o reparo mutou a mensagem original: %q", msgs[2].Content)
	}
}

// O histórico exato que o cliente reenvia depois de renderizar uma superfície:
// as respostas vêm depois das DUAS mensagens do assistente, e a da superfície
// vem vazia. Cada resposta tem de voltar para logo abaixo da sua chamada (é a
// ordem que a API exige) e a vazia tem de ganhar conteúdo.
func TestRepairReordersTheHistoryTheClientSendsBack(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("monte um dashboard"),
		assistantWithCalls("call_sales"),
		assistantWithCalls("call_gen"),
		schema.ToolMessage("", "call_gen"), // superfície renderizada: sem retorno
		schema.ToolMessage("[{...}]", "call_sales"),
		schema.UserMessage("e a maior despesa?"),
	}
	out := RepairToolPairs(msgs)

	got := describe(out)
	want := "user,assistant[call_sales],tool:call_sales,assistant[call_gen],tool:call_gen,user"
	if got != want {
		t.Fatalf("reparo =\n  %s\nesperado\n  %s", got, want)
	}
	for _, m := range out {
		if m.Role == schema.Tool && m.Content == "" {
			t.Fatalf("resposta vazia sobreviveu: %v", m)
		}
	}
	// A resposta real de get_sales_data não pode ter sido substituída.
	if out[2].Content != "[{...}]" {
		t.Fatalf("a resposta real foi descartada: %q", out[2].Content)
	}
	// E a mensagem original não pode ter sido mutada — ela ainda vai no snapshot.
	if msgs[3].Content != "" {
		t.Fatalf("o reparo mutou a mensagem original do histórico: %q", msgs[3].Content)
	}
}

// A sobra de um turno interrompido — mensagem do assistente sem conteúdo e sem
// tool calls — é recusada pela API ("expected a string, got null") e não diz
// nada ao modelo, então sai do input.
func TestRepairDropsEmptyAssistantLeftovers(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("oi"),
		{Role: schema.Assistant, Content: ""},
		schema.UserMessage("ainda está aí?"),
	}
	if got := describe(RepairToolPairs(msgs)); got != "user,user" {
		t.Fatalf("a mensagem vazia do assistente sobreviveu: %s", got)
	}
	// Uma mensagem do assistente COM texto continua intacta.
	ok := []*schema.Message{schema.UserMessage("oi"), schema.AssistantMessage("olá", nil)}
	if got := describe(RepairToolPairs(ok)); got != "user,assistant[]" {
		t.Fatalf("o reparo descartou uma resposta legítima: %s", got)
	}
}

// Duas respostas para a mesma chamada: o provedor recusa: fica a primeira.
func TestRepairDropsDuplicateAnswers(t *testing.T) {
	msgs := []*schema.Message{
		assistantWithCalls("call_a"),
		schema.ToolMessage("primeira", "call_a"),
		schema.ToolMessage("segunda", "call_a"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "assistant[call_a],tool:call_a" {
		t.Fatalf("reparo = %s", got)
	}
	if out[1].Content != "primeira" {
		t.Fatalf("resposta mantida = %q, esperado a primeira", out[1].Content)
	}
}

// Uma mensagem `tool` sem chamada correspondente também é recusada pelo
// provedor ("must be a response to a preceding tool call"), então sai.
func TestRepairDropsOrphanToolMessages(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("oi"),
		schema.ToolMessage("resultado sem chamada", "call_fantasma"),
	}
	if got := describe(RepairToolPairs(msgs)); got != "user" {
		t.Fatalf("a mensagem tool órfã sobreviveu: %s", got)
	}
}

// O reparo é só para o modelo: a conversa devolvida ao cliente no
// MESSAGES_SNAPSHOT continua sendo o histórico real.
func TestRepairDoesNotLeakIntoTheSnapshot(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.UserMessage("monte um dashboard"), "u1")
	c.AppendWithID(assistantWithCalls("call_gen"), "a1")

	if n := len(c.ModelInput("prompt")); n != 4 { // system + user + assistant + tool sintética
		t.Fatalf("ModelInput tem %d mensagens, esperado 4", n)
	}
	snap := c.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("o snapshot tem %d mensagens, esperado 2 (user + assistant): %v", len(snap), snap)
	}
	for _, m := range snap {
		if m.Role == "tool" {
			t.Fatalf("resposta sintética vazou para o cliente: %v", m)
		}
	}
}

// ── contexto da requisição ───────────────────────────────────────────

// O bloco de contexto tem o cabeçalho e o formato do CopilotKitMiddleware do
// Python, e carrega description+value de cada entrada.
func TestAppContextMatchesThePythonFormat(t *testing.T) {
	got := AppContextBlock([]types.Context{
		{Description: "A2UI Component Schema", Value: `[{"component":"LineChart"}]`},
		{Description: "A2UI render tool usage guide", Value: "call generate_a2ui"},
	})

	if !strings.HasPrefix(got, "App Context:\n[") {
		t.Fatalf("o bloco não começa com o cabeçalho do Python:\n%s", got)
	}
	for _, want := range []string{
		`"description": "A2UI Component Schema"`, // JSON indentado: chave, espaço, valor
		"LineChart",
		"call generate_a2ui",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("o bloco perdeu %q:\n%s", want, got)
		}
	}
}

// Sem contexto não há bloco — uma mensagem de sistema com "App Context:\nnull"
// só gastaria tokens e confundiria o modelo.
func TestAppContextIsEmptyWithoutEntries(t *testing.T) {
	if got := AppContextBlock(nil); got != "" {
		t.Fatalf("contexto vazio produziu %q", got)
	}
	if got := AppContextBlock([]types.Context{}); got != "" {
		t.Fatalf("contexto vazio produziu %q", got)
	}
}

// O prompt do agente é um padrão que a mensagem de sistema do cliente sobrepõe;
// o contexto da requisição, não — ele é informação daquela requisição e entra
// sempre, senão o catálogo A2UI sumiria justamente quando o cliente manda o
// próprio prompt.
func TestModelInputAlwaysCarriesTheContextBlock(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.SystemMessage("prompt do cliente"), "s1")
	c.AppendWithID(schema.UserMessage("oi"), "u1")

	msgs := c.ModelInput("prompt do agente", "App Context:\n[]")

	var systems []string
	for _, m := range msgs {
		if m.Role == schema.System {
			systems = append(systems, m.Content)
		}
	}
	if len(systems) != 2 {
		t.Fatalf("o modelo recebeu %d mensagens de sistema, esperado 2: %v", len(systems), systems)
	}
	for _, s := range systems {
		if s == "prompt do agente" {
			t.Fatalf("o prompt do agente venceu a mensagem de sistema do cliente: %v", systems)
		}
	}
	if !strings.Contains(strings.Join(systems, "\n"), "App Context:") {
		t.Fatalf("o bloco de contexto não chegou ao modelo: %v", systems)
	}
}

// Um bloco extra vazio não vira mensagem.
func TestModelInputSkipsEmptyExtraSystemBlocks(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.UserMessage("oi"), "u1")

	msgs := c.ModelInput("prompt do agente", "")
	if len(msgs) != 2 {
		t.Fatalf("o modelo recebeu %d mensagens, esperado 2 (system + user): %d", len(msgs), len(msgs))
	}
	if msgs[0].Role != schema.System || msgs[0].Content != "prompt do agente" {
		t.Fatalf("primeira mensagem inesperada: %v", msgs[0])
	}
}

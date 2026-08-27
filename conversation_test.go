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

// The normal path of the lesson: the model calls the frontend tool `generate_a2ui`, the run
// ends, the client renders and resends the history with the tool call left
// unanswered. Without the repair, OpenAI rejects the whole request with 400.
//
// PT: O caminho normal da lição: o modelo chama a tool de frontend `generate_a2ui`, o run
// termina, o cliente renderiza e reenvia o histórico com a tool call sem
// resposta. Sem o reparo, a OpenAI recusa a requisição inteira com 400.
func TestRepairAnswersComponentCallLeftOpenByTheClient(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("build a dashboard"),
		assistantWithCalls("call_gen"),
		schema.UserMessage("now make it go away"),
	}
	got := describe(RepairToolPairs(msgs))
	want := "user,assistant[call_gen],tool:call_gen,user"
	if got != want {
		t.Fatalf("repair =\n  %s\nexpected\n  %s", got, want)
	}
}

// A real answer must not be duplicated by a synthetic one.
//
// PT: Uma resposta real não pode ser duplicada por uma sintética.
func TestRepairLeavesAnsweredCallsAlone(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("how many rows?"),
		assistantWithCalls("call_q"),
		schema.ToolMessage("[]", "call_q"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "user,assistant[call_q],tool:call_q" {
		t.Fatalf("repair touched an already clean history: %s", got)
	}
	// Without a repair the original slice is returned as is — no copy.
	//
	// PT: Sem reparo a fatia original é devolvida como está — sem cópia.
	if &out[0] != &msgs[0] {
		t.Fatal("the clean history was copied for nothing")
	}
}

// In a message with two calls, only the one left open gets an answer.
//
// PT: Numa mensagem com duas chamadas, só a que ficou aberta recebe resposta.
func TestRepairFillsOnlyTheMissingCall(t *testing.T) {
	msgs := []*schema.Message{
		assistantWithCalls("call_a", "call_b"),
		schema.ToolMessage("[]", "call_b"),
	}
	got := describe(RepairToolPairs(msgs))
	want := "assistant[call_a+call_b],tool:call_a,tool:call_b"
	if got != want {
		t.Fatalf("repair =\n  %s\nexpected\n  %s", got, want)
	}
}

// TestRepairFillsTheEmptyFrontendToolResult recovers a test that
// existed in lesson 2 and was lost in the later copies, even though the code
// path (noToolResult) is still alive in all three.
//
// PT: TestRepairFillsTheEmptyFrontendToolResult recupera um teste que
// existia na lição 2 e foi perdido nas cópias seguintes, embora o caminho de
// código (noToolResult) continue vivo nas três.
func TestRepairFillsTheEmptyFrontendToolResult(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("show a chart"),
		assistantWithCalls("call_pie"),
		schema.ToolMessage("", "call_pie"),
		schema.UserMessage("what now?"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "user,assistant[call_pie],tool:call_pie,user" {
		t.Fatalf("repair = %s", got)
	}
	if out[2].Content == "" {
		t.Fatal("the empty answer survived; the model cannot tell whether the tool ran")
	}
	if msgs[2].Content != "" {
		t.Fatalf("the repair mutated the original message: %q", msgs[2].Content)
	}
}

// The exact history the client resends after rendering a surface:
// the answers come after BOTH assistant messages, and the surface's one
// comes empty. Each answer must go back right below its call (that is the
// order the API requires) and the empty one must gain content.
//
// PT: O histórico exato que o cliente reenvia depois de renderizar uma superfície:
// as respostas vêm depois das DUAS mensagens do assistente, e a da superfície
// vem vazia. Cada resposta tem de voltar para logo abaixo da sua chamada (é a
// ordem que a API exige) e a vazia tem de ganhar conteúdo.
func TestRepairReordersTheHistoryTheClientSendsBack(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("build a dashboard"),
		assistantWithCalls("call_sales"),
		assistantWithCalls("call_gen"),
		schema.ToolMessage("", "call_gen"), // rendered surface: no return value (PT: superfície renderizada: sem retorno)
		schema.ToolMessage("[{...}]", "call_sales"),
		schema.UserMessage("and the biggest expense?"),
	}
	out := RepairToolPairs(msgs)

	got := describe(out)
	want := "user,assistant[call_sales],tool:call_sales,assistant[call_gen],tool:call_gen,user"
	if got != want {
		t.Fatalf("repair =\n  %s\nexpected\n  %s", got, want)
	}
	for _, m := range out {
		if m.Role == schema.Tool && m.Content == "" {
			t.Fatalf("empty answer survived: %v", m)
		}
	}
	// The real answer from get_sales_data must not have been replaced.
	//
	// PT: A resposta real de get_sales_data não pode ter sido substituída.
	if out[2].Content != "[{...}]" {
		t.Fatalf("the real answer was discarded: %q", out[2].Content)
	}
	// And the original message must not have been mutated — it still goes in the snapshot.
	//
	// PT: E a mensagem original não pode ter sido mutada — ela ainda vai no snapshot.
	if msgs[3].Content != "" {
		t.Fatalf("the repair mutated the original history message: %q", msgs[3].Content)
	}
}

// The leftover of an interrupted turn — an assistant message with no content and no
// tool calls — is rejected by the API ("expected a string, got null") and tells
// the model nothing, so it leaves the input.
//
// PT: A sobra de um turno interrompido — mensagem do assistente sem conteúdo e sem
// tool calls — é recusada pela API ("expected a string, got null") e não diz
// nada ao modelo, então sai do input.
func TestRepairDropsEmptyAssistantLeftovers(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("hi"),
		{Role: schema.Assistant, Content: ""},
		schema.UserMessage("still there?"),
	}
	if got := describe(RepairToolPairs(msgs)); got != "user,user" {
		t.Fatalf("the empty assistant message survived: %s", got)
	}
	// An assistant message WITH text stays intact.
	//
	// PT: Uma mensagem do assistente COM texto continua intacta.
	ok := []*schema.Message{schema.UserMessage("hi"), schema.AssistantMessage("hello", nil)}
	if got := describe(RepairToolPairs(ok)); got != "user,assistant[]" {
		t.Fatalf("the repair discarded a legitimate answer: %s", got)
	}
}

// Two answers for the same call: the provider rejects it: the first one stays.
//
// PT: Duas respostas para a mesma chamada: o provedor recusa: fica a primeira.
func TestRepairDropsDuplicateAnswers(t *testing.T) {
	msgs := []*schema.Message{
		assistantWithCalls("call_a"),
		schema.ToolMessage("first", "call_a"),
		schema.ToolMessage("second", "call_a"),
	}
	out := RepairToolPairs(msgs)
	if got := describe(out); got != "assistant[call_a],tool:call_a" {
		t.Fatalf("repair = %s", got)
	}
	if out[1].Content != "first" {
		t.Fatalf("kept answer = %q, expected the first one", out[1].Content)
	}
}

// A `tool` message with no matching call is also rejected by the
// provider ("must be a response to a preceding tool call"), so it leaves.
//
// PT: Uma mensagem `tool` sem chamada correspondente também é recusada pelo
// provedor ("must be a response to a preceding tool call"), então sai.
func TestRepairDropsOrphanToolMessages(t *testing.T) {
	msgs := []*schema.Message{
		schema.UserMessage("hi"),
		schema.ToolMessage("result with no call", "call_ghost"),
	}
	if got := describe(RepairToolPairs(msgs)); got != "user" {
		t.Fatalf("the orphan tool message survived: %s", got)
	}
}

// The repair is for the model only: the conversation returned to the client in the
// MESSAGES_SNAPSHOT is still the real history.
//
// PT: O reparo é só para o modelo: a conversa devolvida ao cliente no
// MESSAGES_SNAPSHOT continua sendo o histórico real.
func TestRepairDoesNotLeakIntoTheSnapshot(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.UserMessage("build a dashboard"), "u1")
	c.AppendWithID(assistantWithCalls("call_gen"), "a1")

	if n := len(c.ModelInput("prompt")); n != 4 { // system + user + assistant + synthetic tool (PT: tool sintética)
		t.Fatalf("ModelInput has %d messages, expected 4", n)
	}
	snap := c.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("the snapshot has %d messages, expected 2 (user + assistant): %v", len(snap), snap)
	}
	for _, m := range snap {
		if m.Role == "tool" {
			t.Fatalf("synthetic answer leaked to the client: %v", m)
		}
	}
}

// ── request context / contexto da requisição ─────────────────────────

// The context block has the header and format of the Python
// CopilotKitMiddleware, and carries description+value of each entry.
//
// PT: O bloco de contexto tem o cabeçalho e o formato do CopilotKitMiddleware do
// Python, e carrega description+value de cada entrada.
func TestAppContextMatchesThePythonFormat(t *testing.T) {
	got := AppContextBlock([]types.Context{
		{Description: "A2UI Component Schema", Value: `[{"component":"LineChart"}]`},
		{Description: "A2UI render tool usage guide", Value: "call generate_a2ui"},
	})

	if !strings.HasPrefix(got, "App Context:\n[") {
		t.Fatalf("the block does not start with the Python header:\n%s", got)
	}
	for _, want := range []string{
		`"description": "A2UI Component Schema"`, // indented JSON: key, space, value (PT: JSON indentado: chave, espaço, valor)
		"LineChart",
		"call generate_a2ui",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("the block lost %q:\n%s", want, got)
		}
	}
}

// Without context there is no block — a system message with "App Context:\nnull"
// would only waste tokens and confuse the model.
//
// PT: Sem contexto não há bloco — uma mensagem de sistema com "App Context:\nnull"
// só gastaria tokens e confundiria o modelo.
func TestAppContextIsEmptyWithoutEntries(t *testing.T) {
	if got := AppContextBlock(nil); got != "" {
		t.Fatalf("empty context produced %q", got)
	}
	if got := AppContextBlock([]types.Context{}); got != "" {
		t.Fatalf("empty context produced %q", got)
	}
}

// The agent prompt is a default that the client's system message overrides;
// the request context is not — it is information about that request and always
// goes in, otherwise the A2UI catalog would vanish precisely when the client sends
// its own prompt.
//
// PT: O prompt do agente é um padrão que a mensagem de sistema do cliente sobrepõe;
// o contexto da requisição, não — ele é informação daquela requisição e entra
// sempre, senão o catálogo A2UI sumiria justamente quando o cliente manda o
// próprio prompt.
func TestModelInputAlwaysCarriesTheContextBlock(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.SystemMessage("client prompt"), "s1")
	c.AppendWithID(schema.UserMessage("hi"), "u1")

	msgs := c.ModelInput("agent prompt", "App Context:\n[]")

	var systems []string
	for _, m := range msgs {
		if m.Role == schema.System {
			systems = append(systems, m.Content)
		}
	}
	if len(systems) != 2 {
		t.Fatalf("the model received %d system messages, expected 2: %v", len(systems), systems)
	}
	for _, s := range systems {
		if s == "agent prompt" {
			t.Fatalf("the agent prompt won over the client's system message: %v", systems)
		}
	}
	if !strings.Contains(strings.Join(systems, "\n"), "App Context:") {
		t.Fatalf("the context block did not reach the model: %v", systems)
	}
}

// An empty extra block does not become a message.
//
// PT: Um bloco extra vazio não vira mensagem.
func TestModelInputSkipsEmptyExtraSystemBlocks(t *testing.T) {
	c := &Conversation{}
	c.AppendWithID(schema.UserMessage("hi"), "u1")

	msgs := c.ModelInput("agent prompt", "")
	if len(msgs) != 2 {
		t.Fatalf("the model received %d messages, expected 2 (system + user): %d", len(msgs), len(msgs))
	}
	if msgs[0].Role != schema.System || msgs[0].Content != "agent prompt" {
		t.Fatalf("unexpected first message: %v", msgs[0])
	}
}

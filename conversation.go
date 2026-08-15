package agui

import (
	"encoding/json"
	"strings"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/cloudwego/eino/schema"
)

// Conversation é o histórico de um run em formato Eino, preservando o id AG-UI
// original de cada mensagem.
//
// Preservar os ids importa: o MESSAGES_SNAPSHOT do fim do run substitui a lista
// de mensagens do frontend. Se cada snapshot inventasse ids novos, o cliente
// veria toda a conversa como mensagens novas a cada turno. Mensagens vindas da
// requisição mantêm o id que o cliente enviou; mensagens produzidas neste run
// recebem o id que já foi anunciado nos eventos de streaming.
//
// Uma Conversation é por run e não é segura para uso concorrente.
type Conversation struct {
	msgs []*schema.Message
	ids  []string
}

// FromRunInput monta a conversa a partir do histórico recebido em
// RunAgentInput.
//
// O system prompt do agente NÃO entra aqui: ele é injetado apenas na chamada ao
// modelo (ver [Conversation.ModelInput]). É assim que o LangGraph se comporta —
// `system_prompt` não vira mensagem de estado — e evita vazar o prompt para o
// frontend pelo MESSAGES_SNAPSHOT.
func FromRunInput(in []types.Message) *Conversation {
	c := &Conversation{}
	for _, m := range in {
		switch m.Role {
		case types.RoleUser:
			if text := messageText(m); text != "" {
				c.AppendWithID(schema.UserMessage(text), m.ID)
			}
		case types.RoleSystem, types.RoleDeveloper:
			if text := messageText(m); text != "" {
				c.AppendWithID(schema.SystemMessage(text), m.ID)
			}
		case types.RoleAssistant:
			content, _ := m.ContentString()
			c.AppendWithID(&schema.Message{
				Role:      schema.Assistant,
				Content:   content,
				ToolCalls: toEinoToolCalls(m.ToolCalls),
			}, m.ID)
		case types.RoleTool:
			content, _ := m.ContentString()
			c.AppendWithID(schema.ToolMessage(content, m.ToolCallID), m.ID)
		}
		// Papéis sem uso para o modelo (reasoning, activity) são ignorados.
	}
	return c
}

// Append acrescenta uma mensagem produzida neste run. O id AG-UI é gerado no
// momento do snapshot.
func (c *Conversation) Append(m *schema.Message) { c.AppendWithID(m, "") }

// AppendWithID acrescenta uma mensagem com um id AG-UI já conhecido — o que foi
// anunciado nos eventos de streaming, por exemplo. Um id vazio equivale a
// [Conversation.Append].
func (c *Conversation) AppendWithID(m *schema.Message, id string) {
	c.msgs = append(c.msgs, m)
	c.ids = append(c.ids, id)
}

// Len devolve o número de mensagens da conversa.
func (c *Conversation) Len() int { return len(c.msgs) }

// Messages devolve as mensagens cruas, sem reparo nem system prompt. Serve para
// inspeção e para um [MessagePreprocessor]; não mute o resultado.
func (c *Conversation) Messages() []*schema.Message { return c.msgs }

// ModelInput devolve as mensagens a enviar ao modelo, com o system prompt à
// frente. Quando o cliente já mandou uma mensagem de sistema, ela vence e o
// prompt do agente não é injetado.
//
// extraSystem são blocos de sistema adicionais — na prática, o contexto da
// requisição (ver [AppContextBlock]). Diferente do prompt do agente, eles
// entram SEMPRE, mesmo que o cliente já tenha mandado uma mensagem de sistema:
// é informação da requisição, não um padrão que o cliente possa sobrepor. O
// CopilotKitMiddleware do Python faz o mesmo, inserindo o bloco logo após a
// primeira mensagem de sistema; aqui ele vai à frente, o que não muda o que o
// modelo lê.
//
// O histórico passa por [RepairToolPairs] antes de ir ao modelo — ver lá o
// porquê.
func (c *Conversation) ModelInput(systemPrompt string, extraSystem ...string) []*schema.Message {
	msgs := RepairToolPairs(c.msgs)

	prefix := make([]*schema.Message, 0, 1+len(extraSystem))
	if systemPrompt != "" && !hasSystemMessage(msgs) {
		prefix = append(prefix, schema.SystemMessage(systemPrompt))
	}
	for _, extra := range extraSystem {
		if extra != "" {
			prefix = append(prefix, schema.SystemMessage(extra))
		}
	}
	if len(prefix) == 0 {
		return msgs
	}
	return append(prefix, msgs...)
}

func hasSystemMessage(msgs []*schema.Message) bool {
	for _, m := range msgs {
		if m.Role == schema.System {
			return true
		}
	}
	return false
}

// appContextPrefix é o cabeçalho do bloco de contexto, byte a byte igual ao do
// CopilotKitMiddleware do Python (`f"App Context:\n{context_content}"`).
const appContextPrefix = "App Context:\n"

// AppContextBlock monta o bloco de sistema com o `context` da requisição, no
// formato que o backend Python produzia. É o [ContextFormatter] padrão.
//
// Isto NÃO é cosmético: é por `RunAgentInput.context` que middlewares do
// runtime CopilotKit entregam ao agente informação que ele não teria de outra
// forma. No caso do A2UI são (a) o CATÁLOGO de componentes disponíveis e (b) o
// guia de uso da tool `generate_a2ui`. Sem repassar esse contexto ao modelo, ele
// chama a tool inventando nomes de componente que o catálogo React não conhece,
// e o frontend renderiza "Unknown component: ..." — observado no fio antes deste
// bloco existir.
//
// O formato (JSON indentado com 2 espaços, sob "App Context:") é o do
// CopilotKitMiddleware; manter a forma importa porque é a que o modelo viu ao
// ser exercitado contra o backend Python.
func AppContextBlock(entries []Context) string {
	if len(entries) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		// Context é struct de duas strings: só falharia se o SDK mudasse de
		// forma. Perder o contexto degrada a resposta, mas não justifica
		// derrubar o run.
		return ""
	}
	return appContextPrefix + string(b)
}

// noToolResult substitui uma resposta de tool ausente ou vazia.
//
// Uma resposta vazia não quebra a API — para o papel `tool` o cliente serializa
// `content: ""`, que é uma string válida (verificado contra a API). O problema é
// outro: `""` não diz nada ao modelo, que fica sem saber se a tool rodou. Um
// status curto custa nada e remove a ambiguidade.
const noToolResult = `{"status":"handled","note":"the client handled this tool call and returned no value"}`

// RepairToolPairs põe o histórico na forma que os provedores de chat exigem:
// cada mensagem do assistente que faz tool calls é seguida IMEDIATAMENTE pelas
// respostas das suas chamadas, todas presentes e com conteúdo não vazio, e
// nenhuma mensagem `tool` fica órfã.
//
// Isto NÃO é defesa paranoica — é o caminho normal de qualquer agente com tools
// de frontend, e sem ele o chat funciona uma vez e toda mensagem seguinte na
// mesma thread falha. Quando o modelo chama uma tool de frontend, o run termina
// e o controle volta ao cliente; o histórico que ele reenvia na mensagem
// seguinte chega assim (observado no fio):
//
//	user
//	assistant[get_sales_data]
//	assistant[generate_a2ui]
//	tool "" → generate_a2ui   ← superfície renderizada: sem valor de retorno
//	tool {métricas} → get_sales_data
//
// O defeito que derruba o run é a ORDEM: as respostas vêm depois das DUAS
// mensagens do assistente, em vez de logo após a sua. A OpenAI recusa com 400
// ("must be followed by tool messages responding to each 'tool_call_id'"),
// verificado contra a API. A resposta vazia do componente não derruba nada — é
// serializada como `""` —, mas também é preenchida aqui: ver noToolResult.
//
// O reparo vale só para o que vai ao MODELO. O MESSAGES_SNAPSHOT devolvido ao
// cliente continua sendo o histórico real, sem reordenação nem respostas
// inventadas.
func RepairToolPairs(msgs []*schema.Message) []*schema.Message {
	if toolPairsOK(msgs) {
		return msgs // caminho íntegro: nada a fazer, nenhuma alocação
	}

	// Primeira resposta encontrada para cada chamada; duplicatas são descartadas
	// (o provedor recusa duas respostas para o mesmo tool_call_id).
	answers := make(map[string]*schema.Message, len(msgs))
	for _, m := range msgs {
		if m.Role == schema.Tool && m.ToolCallID != "" {
			if _, dup := answers[m.ToolCallID]; !dup {
				answers[m.ToolCallID] = m
			}
		}
	}

	out := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == schema.Tool {
			continue // reposicionada abaixo; se for órfã, some
		}
		if m.Role == schema.Assistant && m.Content == "" && len(m.ToolCalls) == 0 {
			// Sobra de um turno que morreu no meio (o cliente guarda a mensagem
			// do assistente que começou a ser transmitida). Sem conteúdo e sem
			// tool calls ela não diz nada ao modelo, e a API a recusa: o campo
			// `content` sai ausente e vem de volta um 400 "Invalid value for
			// 'content': expected a string, got null".
			continue
		}
		out = append(out, m)
		if m.Role != schema.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			answer, ok := answers[tc.ID]
			if !ok || answer.Content == "" {
				// Mensagem nova, nunca mutação da original: a mesma mensagem
				// está no histórico que vai virar MESSAGES_SNAPSHOT.
				answer = schema.ToolMessage(noToolResult, tc.ID)
			}
			out = append(out, answer)
		}
	}
	return out
}

// toolPairsOK diz se o histórico já satisfaz o invariante, numa única passada.
func toolPairsOK(msgs []*schema.Message) bool {
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role == schema.Tool {
			return false // toda `tool` legítima é consumida pelo bloco abaixo
		}
		if m.Role == schema.Assistant && m.Content == "" && len(m.ToolCalls) == 0 {
			return false // mensagem vazia de um turno interrompido
		}
		if m.Role != schema.Assistant || len(m.ToolCalls) == 0 {
			continue
		}
		// As len(ToolCalls) mensagens seguintes têm de ser exatamente as
		// respostas destas chamadas, não vazias.
		pending := make(map[string]bool, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			pending[tc.ID] = true
		}
		for range m.ToolCalls {
			i++
			if i >= len(msgs) {
				return false
			}
			next := msgs[i]
			if next.Role != schema.Tool || next.Content == "" || !pending[next.ToolCallID] {
				return false
			}
			delete(pending, next.ToolCallID)
		}
	}
	return true
}

// Snapshot converte a conversa para mensagens AG-UI, prontas para o
// MESSAGES_SNAPSHOT, respeitando as regras de validação por papel do SDK.
func (c *Conversation) Snapshot() []types.Message {
	out := make([]types.Message, 0, len(c.msgs))
	for i, m := range c.msgs {
		id := c.ids[i]
		if id == "" {
			id = events.GenerateMessageID()
			c.ids[i] = id // estável entre snapshots do mesmo run
		}
		am := types.Message{ID: id, Content: m.Content}
		switch m.Role {
		case schema.System:
			am.Role = types.RoleSystem
		case schema.User:
			am.Role = types.RoleUser
		case schema.Assistant:
			am.Role = types.RoleAssistant
			am.ToolCalls = toAGUIToolCalls(m.ToolCalls)
		case schema.Tool:
			am.Role = types.RoleTool
			am.ToolCallID = m.ToolCallID
		default:
			continue
		}
		out = append(out, am)
	}
	return out
}

// messageText extrai o texto de uma mensagem AG-UI.
//
// ContentString cobre o conteúdo string simples; para conteúdo estruturado
// ([]InputContent) ele devolve ok=false, e então juntamos os fragmentos de
// texto. Fragmentos não-texto (imagem/áudio/binário) são descartados — esta
// versão não tem caminho multimodal —, mas o texto digitado pelo usuário é
// preservado em vez de o turno inteiro sumir em silêncio.
func messageText(m types.Message) string {
	if s, ok := m.ContentString(); ok {
		return s
	}
	parts, ok := m.ContentInputContents()
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == types.InputContentTypeText && p.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func toEinoToolCalls(tcs []types.ToolCall) []schema.ToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]schema.ToolCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, schema.ToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: schema.FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		})
	}
	return out
}

func toAGUIToolCalls(tcs []schema.ToolCall) []types.ToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]types.ToolCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, types.ToolCall{
			ID:       tc.ID,
			Type:     types.ToolCallTypeFunction,
			Function: types.FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		})
	}
	return out
}

package agui

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	wire "github.com/ricardogayer/go-agui/pkg/agui"
)

// Conversation is the history of a run in Eino format, preserving the
// original AG-UI id of each message.
//
// Preserving the ids matters: the MESSAGES_SNAPSHOT at the end of the run
// replaces the frontend's message list. If every snapshot invented new ids,
// the client would see the whole conversation as new messages on every turn.
// Messages coming from the request keep the id the client sent; messages
// produced in this run receive the id already announced in the streaming
// events.
//
// Messages from the request are also kept in their original wire form and go
// back verbatim in the snapshot: multimodal parts, names, metadata and the
// messages the model never sees (reasoning, a user message made only of an
// image) survive the reconciliation instead of being erased from the client.
//
// A Conversation is per run and is not safe for concurrent use.
//
// PT: Conversation é o histórico de um run em formato Eino, preservando o id
// AG-UI original de cada mensagem.
//
// Preservar os ids importa: o MESSAGES_SNAPSHOT do fim do run substitui a lista
// de mensagens do frontend. Se cada snapshot inventasse ids novos, o cliente
// veria toda a conversa como mensagens novas a cada turno. Mensagens vindas da
// requisição mantêm o id que o cliente enviou; mensagens produzidas neste run
// recebem o id que já foi anunciado nos eventos de streaming.
//
// As mensagens da requisição também são guardadas na forma original do fio e
// voltam como estão no snapshot: partes multimodais, nomes, metadata e as
// mensagens que o modelo nunca vê (reasoning, uma mensagem de usuário feita só
// de imagem) sobrevivem à reconciliação em vez de serem apagadas do cliente.
//
// Uma Conversation é por run e não é segura para uso concorrente.
type Conversation struct {
	msgs []*schema.Message
	ids  []string
	wire []wire.Message // original wire form, nil for messages produced here (PT: forma original do fio, nil para mensagens produzidas aqui)

	// hidden holds request messages the model does not see, keyed by the
	// index of the next model message, so the snapshot keeps their order.
	// PT: hidden guarda mensagens da requisição que o modelo não vê, pelo
	// índice da próxima mensagem do modelo, para o snapshot manter a ordem.
	hidden map[int][]wire.Message
}

// FromRunInput builds the conversation from the history received in
// RunAgentInput.
//
// The agent's system prompt does NOT go in here: it is injected only in the
// call to the model (see [Conversation.ModelInput]). This is how LangGraph
// behaves — `system_prompt` does not become a state message — and it avoids
// leaking the prompt to the frontend through the MESSAGES_SNAPSHOT.
//
// PT: FromRunInput monta a conversa a partir do histórico recebido em
// RunAgentInput.
//
// O system prompt do agente NÃO entra aqui: ele é injetado apenas na chamada ao
// modelo (ver [Conversation.ModelInput]). É assim que o LangGraph se comporta —
// `system_prompt` não vira mensagem de estado — e evita vazar o prompt para o
// frontend pelo MESSAGES_SNAPSHOT.
func FromRunInput(in []Message) *Conversation {
	c := &Conversation{}
	for _, m := range in {
		var em *schema.Message
		switch t := m.(type) {
		case *wire.UserMessage:
			if text := contentText(t.Content); text != "" {
				em = schema.UserMessage(text)
			}
		case *wire.SystemMessage:
			if t.Content != "" {
				em = schema.SystemMessage(t.Content)
			}
		case *wire.DeveloperMessage:
			if t.Content != "" {
				em = schema.SystemMessage(t.Content)
			}
		case *wire.AssistantMessage:
			em = &schema.Message{
				Role:      schema.Assistant,
				Content:   wire.Deref(t.Content),
				ToolCalls: toEinoToolCalls(t.ToolCalls),
			}
		case *wire.ToolMessage:
			content := contentText(t.Content)
			if content == "" && t.Error != nil {
				content = *t.Error
			}
			em = schema.ToolMessage(content, t.ToolCallID)
		}
		// Roles of no use to the model (reasoning, activity) and messages
		// without text are kept only for the snapshot.
		//
		// PT: Papéis sem uso para o modelo (reasoning, activity) e mensagens
		// sem texto ficam só para o snapshot.
		if em == nil {
			if m != nil {
				if c.hidden == nil {
					c.hidden = map[int][]wire.Message{}
				}
				c.hidden[len(c.msgs)] = append(c.hidden[len(c.msgs)], m)
			}
			continue
		}
		c.AppendWithID(em, m.MessageID())
		c.wire[len(c.wire)-1] = m
	}
	return c
}

// Append adds a message produced in this run. The AG-UI id is generated at
// snapshot time.
//
// PT: Append acrescenta uma mensagem produzida neste run. O id AG-UI é gerado
// no momento do snapshot.
func (c *Conversation) Append(m *schema.Message) { c.AppendWithID(m, "") }

// AppendWithID adds a message with an already known AG-UI id — the one
// announced in the streaming events, for example. An empty id is equivalent
// to [Conversation.Append].
//
// PT: AppendWithID acrescenta uma mensagem com um id AG-UI já conhecido — o que
// foi anunciado nos eventos de streaming, por exemplo. Um id vazio equivale a
// [Conversation.Append].
func (c *Conversation) AppendWithID(m *schema.Message, id string) {
	c.msgs = append(c.msgs, m)
	c.ids = append(c.ids, id)
	c.wire = append(c.wire, nil)
}

// Len returns the number of messages in the conversation.
//
// PT: Len devolve o número de mensagens da conversa.
func (c *Conversation) Len() int { return len(c.msgs) }

// Messages returns the raw messages, without repair or system prompt. It is
// meant for inspection (e.g. from a custom [Runner]); do not mutate the result.
//
// PT: Messages devolve as mensagens cruas, sem reparo nem system prompt. Serve
// para inspeção (ex.: de um [Runner] customizado); não mute o resultado.
func (c *Conversation) Messages() []*schema.Message { return c.msgs }

// ModelInput returns the messages to send to the model, with the system
// prompt in front. When the client has already sent a system message, it wins
// and the agent's prompt is not injected.
//
// extraSystem are additional system blocks — in practice, the request context
// (see [AppContextBlock]). Unlike the agent's prompt, they ALWAYS go in, even
// if the client has already sent a system message: it is request information,
// not a default the client may override. Python's CopilotKitMiddleware does
// the same, inserting the block right after the first system message; here it
// goes in front, which does not change what the model reads.
//
// The history goes through [RepairToolPairs] before going to the model — see
// there for why.
//
// PT: ModelInput devolve as mensagens a enviar ao modelo, com o system prompt à
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

// appContextPrefix is the header of the context block, byte for byte equal to
// the one in Python's CopilotKitMiddleware (`f"App Context:\n{context_content}"`).
//
// PT: appContextPrefix é o cabeçalho do bloco de contexto, byte a byte igual ao
// do CopilotKitMiddleware do Python (`f"App Context:\n{context_content}"`).
const appContextPrefix = "App Context:\n"

// AppContextBlock builds the system block with the request's `context`, in the
// format the Python backend produced. It is the default [ContextFormatter].
//
// This is NOT cosmetic: it is through `RunAgentInput.context` that CopilotKit
// runtime middlewares deliver to the agent information it would not otherwise
// have. In the A2UI case these are (a) the CATALOG of available components and
// (b) the usage guide of the `generate_a2ui` tool. Without passing that context
// on to the model, it calls the tool inventing component names the React
// catalog does not know, and the frontend renders "Unknown component: ..." —
// observed on the wire before this block existed.
//
// The format (JSON indented with 2 spaces, under "App Context:") is that of
// CopilotKitMiddleware; keeping the shape matters because it is the one the
// model saw when exercised against the Python backend.
//
// PT: AppContextBlock monta o bloco de sistema com o `context` da requisição,
// no formato que o backend Python produzia. É o [ContextFormatter] padrão.
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
		// Context is a struct of two strings: it would only fail if the data model
		// changed shape. Losing the context degrades the answer, but does not
		// justify bringing down the run.
		//
		// PT: Context é struct de duas strings: só falharia se o modelo de dados mudasse de
		// forma. Perder o contexto degrada a resposta, mas não justifica
		// derrubar o run.
		return ""
	}
	return appContextPrefix + string(b)
}

// noToolResult replaces a missing or empty tool response.
//
// An empty response does not break the API — for the `tool` role the client
// serializes `content: ""`, which is a valid string (verified against the
// API). The problem is another: `""` tells the model nothing, leaving it
// unsure whether the tool ran. A short status costs nothing and removes the
// ambiguity.
//
// PT: noToolResult substitui uma resposta de tool ausente ou vazia.
//
// Uma resposta vazia não quebra a API — para o papel `tool` o cliente serializa
// `content: ""`, que é uma string válida (verificado contra a API). O problema é
// outro: `""` não diz nada ao modelo, que fica sem saber se a tool rodou. Um
// status curto custa nada e remove a ambiguidade.
const noToolResult = `{"status":"handled","note":"the client handled this tool call and returned no value"}`

// RepairToolPairs puts the history in the shape chat providers require: every
// assistant message that makes tool calls is IMMEDIATELY followed by the
// responses to its calls, all present and with non-empty content, and no
// `tool` message is left orphaned.
//
// This is NOT paranoid defense — it is the normal path of any agent with
// frontend tools, and without it the chat works once and every following
// message in the same thread fails. When the model calls a frontend tool, the
// run ends and control returns to the client; the history it resends on the
// next message arrives like this (observed on the wire):
//
//	user
//	assistant[get_sales_data]
//	assistant[generate_a2ui]
//	tool "" → generate_a2ui   ← rendered surface: no return value
//	tool {metrics} → get_sales_data
//
// The defect that brings down the run is the ORDER: the responses come after
// BOTH assistant messages, instead of right after their own. OpenAI rejects it
// with 400 ("must be followed by tool messages responding to each
// 'tool_call_id'"), verified against the API. The component's empty response
// brings nothing down — it is serialized as `""` —, but it is also filled in
// here: see noToolResult.
//
// The repair applies only to what goes to the MODEL. The MESSAGES_SNAPSHOT
// returned to the client remains the real history, with no reordering and no
// invented responses.
//
// PT: RepairToolPairs põe o histórico na forma que os provedores de chat
// exigem: cada mensagem do assistente que faz tool calls é seguida
// IMEDIATAMENTE pelas respostas das suas chamadas, todas presentes e com
// conteúdo não vazio, e nenhuma mensagem `tool` fica órfã.
//
// Isto NÃO é defesa paranoica — é o caminho normal de qualquer agente com tools
// de frontend, e sem ele o chat funciona uma vez e toda mensagem seguinte na
// mesma thread falha. Quando o modelo chama uma tool de frontend, o run termina
// e o controle volta ao cliente; o histórico que ele reenvia na mensagem
// seguinte chega como no exemplo acima (observado no fio).
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
		return msgs // clean path: nothing to do, no allocation (PT: caminho íntegro: nada a fazer, nenhuma alocação)
	}

	// First response found for each call; duplicates are discarded (the
	// provider rejects two responses for the same tool_call_id).
	//
	// PT: Primeira resposta encontrada para cada chamada; duplicatas são
	// descartadas (o provedor recusa duas respostas para o mesmo tool_call_id).
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
			continue // repositioned below; if orphaned, it disappears (PT: reposicionada abaixo; se for órfã, some)
		}
		if m.Role == schema.Assistant && m.Content == "" && len(m.ToolCalls) == 0 {
			// Leftover of a turn that died midway (the client keeps the
			// assistant message that started being streamed). With no content
			// and no tool calls it tells the model nothing, and the API rejects
			// it: the `content` field goes out absent and a 400 "Invalid value
			// for 'content': expected a string, got null" comes back.
			//
			// PT: Sobra de um turno que morreu no meio (o cliente guarda a
			// mensagem do assistente que começou a ser transmitida). Sem
			// conteúdo e sem tool calls ela não diz nada ao modelo, e a API a
			// recusa: o campo `content` sai ausente e vem de volta um 400
			// "Invalid value for 'content': expected a string, got null".
			continue
		}
		out = append(out, m)
		if m.Role != schema.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			answer, ok := answers[tc.ID]
			if !ok || answer.Content == "" {
				// A new message, never a mutation of the original: the same
				// message is in the history that will become the
				// MESSAGES_SNAPSHOT.
				//
				// PT: Mensagem nova, nunca mutação da original: a mesma
				// mensagem está no histórico que vai virar MESSAGES_SNAPSHOT.
				answer = schema.ToolMessage(noToolResult, tc.ID)
			}
			out = append(out, answer)
		}
	}
	return out
}

// toolPairsOK reports whether the history already satisfies the invariant, in
// a single pass.
//
// PT: toolPairsOK diz se o histórico já satisfaz o invariante, numa única
// passada.
func toolPairsOK(msgs []*schema.Message) bool {
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role == schema.Tool {
			return false // every legitimate `tool` is consumed by the block below (PT: toda `tool` legítima é consumida pelo bloco abaixo)
		}
		if m.Role == schema.Assistant && m.Content == "" && len(m.ToolCalls) == 0 {
			return false // empty message from an interrupted turn (PT: mensagem vazia de um turno interrompido)
		}
		if m.Role != schema.Assistant || len(m.ToolCalls) == 0 {
			continue
		}
		// The next len(ToolCalls) messages must be exactly the responses to
		// these calls, non-empty.
		//
		// PT: As len(ToolCalls) mensagens seguintes têm de ser exatamente as
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

// Snapshot converts the conversation to AG-UI messages, ready for the
// MESSAGES_SNAPSHOT. Messages from the request go back in their original wire
// form; messages produced in this run are converted from Eino.
//
// PT: Snapshot converte a conversa para mensagens AG-UI, prontas para o
// MESSAGES_SNAPSHOT. Mensagens da requisição voltam na forma original do fio;
// mensagens produzidas neste run são convertidas do Eino.
func (c *Conversation) Snapshot() []Message {
	out := make([]Message, 0, len(c.msgs))
	for i, m := range c.msgs {
		out = append(out, c.hidden[i]...)
		if c.wire[i] != nil {
			out = append(out, c.wire[i])
			continue
		}
		id := c.ids[i]
		if id == "" {
			id = uuid.NewString()
			c.ids[i] = id // stable across snapshots of the same run (PT: estável entre snapshots do mesmo run)
		}
		switch m.Role {
		case schema.System:
			out = append(out, &wire.SystemMessage{ID: id, Content: m.Content})
		case schema.User:
			out = append(out, &wire.UserMessage{ID: id, Content: wire.TextContent(m.Content)})
		case schema.Assistant:
			am := &wire.AssistantMessage{ID: id, ToolCalls: toAGUIToolCalls(m.ToolCalls)}
			if m.Content != "" {
				am.Content = &m.Content
			}
			out = append(out, am)
		case schema.Tool:
			out = append(out, &wire.ToolMessage{
				ID: id, Content: wire.TextContent(m.Content), ToolCallID: m.ToolCallID,
			})
		}
	}
	return append(out, c.hidden[len(c.msgs)]...)
}

// contentText extracts the text of user/tool content. With multimodal parts,
// the text parts are joined with a newline. Non-text parts (image, audio,
// video, document) are dropped — this version has no multimodal path to the
// model, which AG-UI 1.0 allows as long as the run does not fail because of
// it — but the text the user typed is preserved
// instead of the whole turn silently vanishing.
//
// PT: contentText extrai o texto de um conteúdo de usuário/tool. Com partes
// multimodais, as partes de texto são unidas por quebra de linha. Partes
// não-texto (imagem, áudio, vídeo, documento) são descartadas — esta versão
// não tem caminho multimodal até o modelo, o que o AG-UI 1.0 permite —, mas o
// texto digitado pelo usuário é preservado em vez de o turno inteiro sumir em
// silêncio.
func contentText(c Content) string {
	if !c.IsParts() {
		return c.String()
	}
	var b strings.Builder
	for _, p := range c.Parts() {
		if t, ok := p.(*wire.TextPart); ok && t.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

func toEinoToolCalls(tcs []ToolCall) []schema.ToolCall {
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

func toAGUIToolCalls(tcs []schema.ToolCall) []ToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, wire.NewToolCall(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	return out
}

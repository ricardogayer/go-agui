package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// turnResult is what one model turn produced.
//
// PT: turnResult é o que um turno do modelo produziu.
type turnResult struct {
	message *schema.Message
	// msgID is the AG-UI id announced for the assistant message. It is the
	// same in TEXT_MESSAGE_*, in the parentMessageId of the tool calls and in
	// the MESSAGES_SNAPSHOT.
	//
	// PT: msgID é o id AG-UI anunciado para a mensagem do assistente. É o mesmo
	// em TEXT_MESSAGE_*, no parentMessageId das tool calls e no
	// MESSAGES_SNAPSHOT.
	msgID string
	usage *schema.TokenUsage
}

// streamTurn streams one model turn, emitting text and tool calls as the
// chunks arrive, and returns the already concatenated assistant message.
//
// About tool calls: the model opens the call (chunk with id+name), sends
// argument fragments and closes it. A TOOL_CALL_START without a toolCallId
// could not be correlated with its arguments or its result, so the fragments
// are buffered until id and name are known.
// Whoever streams the tool calls must NOT re-emit them afterwards — that would
// duplicate every call (the same trap ag_ui_adk documents with
// `_translator_emitted_tool_call_ids`).
//
// PT: streamTurn faz o streaming de um turno do modelo, emitindo texto e tool
// calls conforme os chunks chegam, e devolve a mensagem do assistente já
// concatenada.
//
// Sobre as tool calls: o modelo abre a chamada (chunk com id+nome), manda
// fragmentos de argumento e fecha. Um TOOL_CALL_START sem toolCallId não
// poderia ser correlacionado com seus argumentos nem com seu resultado, então
// os fragmentos ficam em buffer até id e nome serem conhecidos.
// Quem faz streaming das tool calls NÃO pode reemiti-las depois — isso
// duplicaria cada chamada (a mesma armadilha que o ag_ui_adk documenta com
// `_translator_emitted_tool_call_ids`).
func streamTurn(
	ctx context.Context,
	emit *Emitter,
	cm model.ToolCallingChatModel,
	messages []*schema.Message,
	ids toolCallIDs,
) (turnResult, error) {
	// The assistant message id is decided before the first chunk: it is the
	// messageId of the text block, the parentMessageId of the tool calls and
	// the id in the MESSAGES_SNAPSHOT. A single id keeps the three things
	// coherent on the client.
	//
	// PT: O id da mensagem do assistente é decidido antes do primeiro chunk: ele
	// é o messageId do bloco de texto, o parentMessageId das tool calls e o id
	// no MESSAGES_SNAPSHOT. Um id só mantém as três coisas coerentes no cliente.
	res := turnResult{msgID: uuid.NewString()}

	sr, err := cm.Stream(ctx, messages)
	if err != nil {
		return res, err
	}
	defer sr.Close()

	var chunks []*schema.Message
	textOpen := false

	// State of the tool-call tap, indexed by the call's identity (Index is
	// stable across the chunks of the same call).
	//
	// PT: Estado do tap de tool calls, indexado pela identidade da chamada
	// (Index é estável entre os chunks de uma mesma chamada).
	type tcStream struct {
		started  bool
		id, name string
		wireID   string   // id announced on the wire, unique in the thread (PT: id anunciado no fio, único na thread)
		buffered []string // argument fragments held until id+name exist (PT: fragmentos de argumento retidos até id+nome existirem)
	}
	tcs := map[string]*tcStream{}
	var tcOrder []string

	closeText := func() {
		if textOpen {
			emit.TextEnd(res.msgID)
			textOpen = false
		}
	}

	streamToolCallChunk := func(chunk *schema.Message) {
		if len(chunk.ToolCalls) == 0 {
			return
		}
		// A tool call closes the open text block, so that blocks never
		// overlap on the wire.
		//
		// PT: Uma tool call encerra o bloco de texto aberto, para que os blocos
		// nunca se sobreponham no fio.
		closeText()
		for _, tc := range chunk.ToolCalls {
			key := toolCallKey(tc)
			st := tcs[key]
			if st == nil {
				st = &tcStream{}
				tcs[key] = st
				tcOrder = append(tcOrder, key)
			}
			if tc.Function.Name != "" {
				st.name = tc.Function.Name
			}
			if tc.ID != "" && st.id == "" {
				st.id = tc.ID
				st.wireID = ids.claim(tc.ID)
			}
			frag := tc.Function.Arguments
			switch {
			case st.started:
				emit.ToolArgs(st.wireID, frag) // the emitter ignores an empty fragment (PT: o emitter ignora fragmento vazio)
			case st.id != "" && st.name != "":
				emit.ToolStart(st.wireID, st.name, res.msgID)
				st.started = true
				for _, b := range st.buffered {
					emit.ToolArgs(st.wireID, b)
				}
				st.buffered = nil
				emit.ToolArgs(st.wireID, frag)
			case frag != "":
				st.buffered = append(st.buffered, frag)
			}
		}
	}

	// closeOpenBlocks balances every open block. It lives in the defer so that
	// ANY exit — EOF, an error in the middle of Recv, cancellation — closes the
	// text block and the open tool calls, instead of leaving the client hanging
	// on a TOOL_CALL_START with no end.
	//
	// PT: closeOpenBlocks equilibra todo bloco aberto. Fica no defer para que
	// QUALQUER saída — EOF, erro no meio do Recv, cancelamento — feche o bloco
	// de texto e as tool calls abertas, em vez de deixar o cliente pendurado
	// num TOOL_CALL_START sem fim.
	defer func() {
		closeText()
		for _, key := range tcOrder {
			if st := tcs[key]; st.started {
				emit.ToolEnd(st.wireID)
				st.started = false // idempotent: never two ENDs for the same call (PT: idempotente: nunca dois END para a mesma chamada)
			}
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return res, err // the client left; stop draining the model (PT: cliente foi embora; para de drenar o modelo)
		}
		if err := emit.Err(); err != nil {
			return res, err
		}
		chunk, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return res, recvErr
		}
		if chunk.Content != "" {
			if !textOpen {
				emit.TextStart(res.msgID)
				textOpen = true
			}
			emit.TextContent(res.msgID, chunk.Content)
		}
		streamToolCallChunk(chunk)
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		return res, errors.New("the model stream came back empty")
	}
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return res, err
	}
	// The conversation must carry the ids the client saw.
	// PT: A conversa precisa carregar os ids que o cliente viu.
	for i, tc := range msg.ToolCalls {
		if st := tcs[toolCallKey(tc)]; st != nil && st.wireID != "" && tc.ID == st.id {
			msg.ToolCalls[i].ID = st.wireID
		}
	}
	res.message = msg
	if msg.ResponseMeta != nil {
		res.usage = msg.ResponseMeta.Usage
	}
	return res, nil
}

// toolCallIDs is the set of tool call ids already used in the thread: the
// ones in the history the client sent and the ones announced in this run.
//
// AG-UI 1.0 identifies a tool call by its id: reusing one (some providers
// number calls "call_0", "call_1", ... again on every turn) makes the second
// TOOL_CALL_START a "reopening" of the first, which the protocol refuses when
// it disagrees with the opener — and the client would merge two different
// calls. A repeated id is replaced by a fresh one, both on the wire and in the
// conversation.
//
// PT: toolCallIDs é o conjunto de ids de tool call já usados na thread: os do
// histórico que o cliente enviou e os anunciados neste run.
//
// O AG-UI 1.0 identifica uma tool call pelo id: reusar um (alguns provedores
// numeram as chamadas "call_0", "call_1", ... de novo a cada turno) faz o
// segundo TOOL_CALL_START virar uma "reabertura" do primeiro, que o protocolo
// recusa quando discorda do original — e o cliente fundiria duas chamadas
// diferentes. Um id repetido é trocado por um novo, no fio e na conversa.
type toolCallIDs map[string]bool

// usedToolCallIDs collects the tool call ids of the conversation.
//
// PT: usedToolCallIDs coleta os ids de tool call da conversa.
func usedToolCallIDs(conv *Conversation) toolCallIDs {
	ids := toolCallIDs{}
	for _, m := range conv.Messages() {
		for _, tc := range m.ToolCalls {
			if tc.ID != "" {
				ids[tc.ID] = true
			}
		}
	}
	return ids
}

// claim returns id when it is still unused, or a fresh id otherwise, and
// marks the result as used.
//
// PT: claim devolve id quando ainda não foi usado, ou um id novo caso
// contrário, e marca o resultado como usado.
func (ids toolCallIDs) claim(id string) string {
	if ids[id] {
		id = "call_" + uuid.NewString()
	}
	ids[id] = true
	return id
}

// toolCallKey identifies a streaming tool call stably across its chunks.
// Without this, two calls in the same turn collapse into one.
//
// PT: toolCallKey identifica uma tool call em streaming de forma estável ao
// longo dos seus chunks. Sem isto, duas chamadas no mesmo turno colapsam numa
// só.
func toolCallKey(tc schema.ToolCall) string {
	if tc.Index != nil {
		return "i" + strconv.Itoa(*tc.Index)
	}
	if tc.ID != "" {
		return "d" + tc.ID
	}
	return "p0"
}

// validateToolCalls splits the tool calls emitted by the model between those
// that stay in the assistant message (every kept call needs a matching
// response, a provider requirement) and the subset worth forwarding.
//
// A call with an empty name or arguments that are not JSON is malformed: it
// stays in the message and receives a corrective response, so the model can
// recover on the next turn, but it is not forwarded. A call without an id is
// uncorrelatable (no response can be associated with it) and leaves the
// message entirely.
//
// A corrective response to a call the client already saw (TOOL_CALL_START
// went out) is also emitted as TOOL_CALL_RESULT: AG-UI 1.0 counts every
// started call without a result as pending.
//
// PT: validateToolCalls separa as tool calls emitidas pelo modelo entre as que
// ficam na mensagem do assistente (toda chamada mantida precisa de uma resposta
// correspondente, exigência do provedor) e o subconjunto que vale encaminhar.
//
// Uma chamada com nome vazio ou argumentos que não são JSON é malformada: fica
// na mensagem e recebe uma resposta corretiva, para o modelo se recuperar no
// turno seguinte, mas não é encaminhada. Uma chamada sem id é incorrelacionável
// (nenhuma resposta pode ser associada a ela) e sai da mensagem por completo.
//
// Uma resposta corretiva a uma chamada que o cliente já viu (o
// TOOL_CALL_START saiu) também é emitida como TOOL_CALL_RESULT: o AG-UI 1.0
// conta toda chamada iniciada sem resultado como pendente.
func validateToolCalls(logger *slog.Logger, emit *Emitter, assistant *schema.Message, conv *Conversation) []schema.ToolCall {
	kept := make([]schema.ToolCall, 0, len(assistant.ToolCalls))
	actionable := make([]schema.ToolCall, 0, len(assistant.ToolCalls))
	corrective := func(tc schema.ToolCall, result string) {
		conv.Append(schema.ToolMessage(result, tc.ID))
		kept = append(kept, tc)
		if emit.startedToolCall(tc.ID) {
			emit.ToolResult(uuid.NewString(), tc.ID, result)
		}
	}
	for _, tc := range assistant.ToolCalls {
		switch {
		case tc.ID == "":
			logger.Warn("discarding tool call without id", "tool", tc.Function.Name)
		case tc.Function.Name == "":
			corrective(tc, `{"error":"tool call had an empty function name"}`)
		case !json.Valid([]byte(tc.Function.Arguments)):
			corrective(tc, fmt.Sprintf(
				`{"error":"tool arguments for %q were not valid JSON"}`, tc.Function.Name))
		default:
			kept = append(kept, tc)
			actionable = append(actionable, tc)
		}
	}
	assistant.ToolCalls = kept
	return actionable
}

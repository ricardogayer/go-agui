package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// turnResult é o que um turno do modelo produziu.
type turnResult struct {
	message *schema.Message
	// msgID é o id AG-UI anunciado para a mensagem do assistente. É o mesmo em
	// TEXT_MESSAGE_*, no parentMessageId das tool calls e no MESSAGES_SNAPSHOT.
	msgID string
	usage *schema.TokenUsage
}

// streamTurn faz o streaming de um turno do modelo, emitindo texto e tool calls
// conforme os chunks chegam, e devolve a mensagem do assistente já concatenada.
//
// Sobre as tool calls: o modelo abre a chamada (chunk com id+nome), manda
// fragmentos de argumento e fecha. O SDK recusa TOOL_CALL_START com toolCallId
// vazio, então os fragmentos ficam em buffer até id e nome serem conhecidos.
// Quem faz streaming das tool calls NÃO pode reemiti-las depois — isso
// duplicaria cada chamada (a mesma armadilha que o ag_ui_adk documenta com
// `_translator_emitted_tool_call_ids`).
func streamTurn(
	ctx context.Context,
	emit *Emitter,
	cm model.ToolCallingChatModel,
	messages []*schema.Message,
) (turnResult, error) {
	// O id da mensagem do assistente é decidido antes do primeiro chunk: ele é
	// o messageId do bloco de texto, o parentMessageId das tool calls e o id no
	// MESSAGES_SNAPSHOT. Um id só mantém as três coisas coerentes no cliente.
	res := turnResult{msgID: events.GenerateMessageID()}

	sr, err := cm.Stream(ctx, messages)
	if err != nil {
		return res, err
	}
	defer sr.Close()

	var chunks []*schema.Message
	textOpen := false

	// Estado do tap de tool calls, indexado pela identidade da chamada
	// (Index é estável entre os chunks de uma mesma chamada).
	type tcStream struct {
		started  bool
		id, name string
		buffered []string // fragmentos de argumento retidos até id+nome existirem
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
		// Uma tool call encerra o bloco de texto aberto, para que os blocos
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
			if tc.ID != "" {
				st.id = tc.ID
			}
			frag := tc.Function.Arguments
			switch {
			case st.started:
				emit.ToolArgs(st.id, frag) // o emitter ignora fragmento vazio
			case st.id != "" && st.name != "":
				emit.ToolStart(st.id, st.name, res.msgID)
				st.started = true
				for _, b := range st.buffered {
					emit.ToolArgs(st.id, b)
				}
				st.buffered = nil
				emit.ToolArgs(st.id, frag)
			case frag != "":
				st.buffered = append(st.buffered, frag)
			}
		}
	}

	// closeOpenBlocks equilibra todo bloco aberto. Fica no defer para que
	// QUALQUER saída — EOF, erro no meio do Recv, cancelamento — feche o bloco
	// de texto e as tool calls abertas, em vez de deixar o cliente pendurado
	// num TOOL_CALL_START sem fim.
	defer func() {
		closeText()
		for _, key := range tcOrder {
			if st := tcs[key]; st.started {
				emit.ToolEnd(st.id)
				st.started = false // idempotente: nunca dois END para a mesma chamada
			}
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return res, err // cliente foi embora; para de drenar o modelo
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
		return res, errors.New("o stream do modelo veio vazio")
	}
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return res, err
	}
	res.message = msg
	if msg.ResponseMeta != nil {
		res.usage = msg.ResponseMeta.Usage
	}
	return res, nil
}

// toolCallKey identifica uma tool call em streaming de forma estável ao longo
// dos seus chunks. Sem isto, duas chamadas no mesmo turno colapsam numa só.
func toolCallKey(tc schema.ToolCall) string {
	if tc.Index != nil {
		return "i" + strconv.Itoa(*tc.Index)
	}
	if tc.ID != "" {
		return "d" + tc.ID
	}
	return "p0"
}

// validateToolCalls separa as tool calls emitidas pelo modelo entre as que
// ficam na mensagem do assistente (toda chamada mantida precisa de uma resposta
// correspondente, exigência do provedor) e o subconjunto que vale encaminhar.
//
// Uma chamada com nome vazio ou argumentos que não são JSON é malformada: fica
// na mensagem e recebe uma resposta corretiva, para o modelo se recuperar no
// turno seguinte, mas não é encaminhada. Uma chamada sem id é incorrelacionável
// (nenhuma resposta pode ser associada a ela) e sai da mensagem por completo.
//
// Isto não é zelo: um toolCallId vazio é recusado pelo SDK, e um evento
// recusado seria lido pelo Emitter como desconexão — o run morreria em
// silêncio, sem RUN_ERROR.
func validateToolCalls(logger *slog.Logger, assistant *schema.Message, conv *Conversation) []schema.ToolCall {
	kept := make([]schema.ToolCall, 0, len(assistant.ToolCalls))
	actionable := make([]schema.ToolCall, 0, len(assistant.ToolCalls))
	corrective := func(tc schema.ToolCall, result string) {
		conv.Append(schema.ToolMessage(result, tc.ID))
		kept = append(kept, tc)
	}
	for _, tc := range assistant.ToolCalls {
		switch {
		case tc.ID == "":
			logger.Warn("descartando tool call sem id", "tool", tc.Function.Name)
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

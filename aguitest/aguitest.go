// Package aguitest traz o que é preciso para testar um agente AG-UI sem tocar
// na rede nem gastar com um provedor.
//
// O centro é o [FakeModel]: ele implementa model.ToolCallingChatModel
// DIRETAMENTE, em processo, sem servidor HTTP falso no meio. Isso importa
// porque o que se quer testar é o laço, o streaming e a sequência de eventos —
// não o adaptador HTTP do eino-ext, que é código de terceiro já testado. Um
// servidor falso tornaria os testes mais lentos e os faria quebrar por motivos
// irrelevantes, como uma mudança no formato de chunk do provedor.
//
// Um teste típico cabe em dez linhas:
//
//	m := aguitest.NewFakeModel(aguitest.Turn{Text: "olá"})
//	ag := agui.Must(agui.New(ctx, m))
//	evs := aguitest.Run(t, ag, aguitest.Input("oi"))
//	aguitest.RequireSequence(t, evs,
//		agui.EventRunStarted, agui.EventStepStarted,
//		agui.EventTextMessageStart, agui.EventTextMessageChunk,
//		agui.EventTextMessageEnd, agui.EventStepFinished,
//		agui.EventStateSnapshot, agui.EventMessagesSnapshot, agui.EventRunFinished)
package aguitest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// FakeToolCall descreve uma tool call que o modelo falso vai emitir.
type FakeToolCall struct {
	// ID é o id da chamada. Vazio gera um id determinístico ("call_0", ...).
	ID string
	// Name é o nome da tool.
	Name string
	// Args são os argumentos em JSON, emitidos de uma vez.
	Args string
	// SplitArgs, quando presente, substitui Args e emite os argumentos em
	// fragmentos — é assim que se exercita o buffer que segura os fragmentos
	// até id e nome serem conhecidos.
	SplitArgs []string
}

// Turn é um turno do modelo falso.
type Turn struct {
	// Text é o texto do turno, emitido num único chunk.
	Text string
	// Chunks, quando presente, substitui Text e emite o texto em pedaços — para
	// exercitar o fatiamento do streaming.
	Chunks []string
	// ToolCalls são as chamadas emitidas neste turno.
	ToolCalls []FakeToolCall
	// Err, quando presente, faz o turno falhar no meio do stream.
	Err error
	// Usage, quando presente, acompanha a mensagem concatenada.
	Usage *schema.TokenUsage
}

// FakeModel é um model.ToolCallingChatModel roteirizado.
//
// Cada chamada consome o próximo Turn; o último se repete indefinidamente, o
// que evita ter que roteirizar turnos extras só porque o laço deu mais uma
// volta do que o esperado.
//
// É seguro para uso concorrente.
type FakeModel struct {
	turns []Turn

	mu     sync.Mutex
	calls  [][]*schema.Message
	bound  [][]*schema.ToolInfo
	cursor int
}

var _ model.ToolCallingChatModel = (*FakeModel)(nil)

// NewFakeModel cria o modelo falso com os turnos dados. Sem turnos, responde
// sempre com um texto curto.
func NewFakeModel(turns ...Turn) *FakeModel {
	if len(turns) == 0 {
		turns = []Turn{{Text: "ok"}}
	}
	return &FakeModel{turns: turns}
}

// Calls devolve, em ordem, as mensagens que o modelo recebeu em cada chamada.
//
// É o que permite afirmar sobre o que o agente MONTOU — se o system prompt foi
// injetado, se o bloco de contexto está lá, se o RepairToolPairs reordenou o
// histórico —, e não só sobre o que ele emitiu.
func (f *FakeModel) Calls() [][]*schema.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]*schema.Message, len(f.calls))
	copy(out, f.calls)
	return out
}

// LastCall devolve as mensagens da última chamada ao modelo.
func (f *FakeModel) LastCall() []*schema.Message {
	calls := f.Calls()
	if len(calls) == 0 {
		return nil
	}
	return calls[len(calls)-1]
}

// BoundTools devolve, em ordem, os conjuntos de tools ligados por WithTools.
// Serve para afirmar que as tools de frontend chegaram ao modelo.
func (f *FakeModel) BoundTools() [][]*schema.ToolInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]*schema.ToolInfo, len(f.bound))
	copy(out, f.bound)
	return out
}

// LastBoundNames devolve os nomes das tools ligadas na última chamada a
// WithTools.
func (f *FakeModel) LastBoundNames() []string {
	bound := f.BoundTools()
	if len(bound) == 0 {
		return nil
	}
	last := bound[len(bound)-1]
	names := make([]string, 0, len(last))
	for _, t := range last {
		names = append(names, t.Name)
	}
	return names
}

// WithTools implementa model.ToolCallingChatModel devolvendo uma instância que
// compartilha o roteiro e o registro — a raiz continua sendo o ponto de
// inspeção, o que reproduz o uso real (o agente liga tools por requisição).
func (f *FakeModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	f.mu.Lock()
	f.bound = append(f.bound, tools)
	f.mu.Unlock()
	return f, nil
}

// Generate implementa model.BaseChatModel drenando o stream.
func (f *FakeModel) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	sr, err := f.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	defer sr.Close()
	var chunks []*schema.Message
	for {
		c, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return schema.ConcatMessages(chunks)
}

// Stream implementa model.BaseChatModel emitindo o próximo turno do roteiro.
func (f *FakeModel) Stream(_ context.Context, in []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	f.mu.Lock()
	snapshot := make([]*schema.Message, len(in))
	copy(snapshot, in)
	f.calls = append(f.calls, snapshot)
	turn := f.turns[min(f.cursor, len(f.turns)-1)]
	f.cursor++
	f.mu.Unlock()

	sr, sw := schema.Pipe[*schema.Message](8)
	go func() {
		defer sw.Close()
		for _, c := range turn.chunksOf() {
			if sw.Send(c, nil) {
				return // o leitor fechou
			}
		}
		if turn.Err != nil {
			sw.Send(nil, turn.Err)
		}
	}()
	return sr, nil
}

// chunksOf transforma o roteiro do turno na sequência de chunks que o modelo
// emitiria.
func (t Turn) chunksOf() []*schema.Message {
	var out []*schema.Message

	texts := t.Chunks
	if len(texts) == 0 && t.Text != "" {
		texts = []string{t.Text}
	}
	for _, s := range texts {
		out = append(out, &schema.Message{Role: schema.Assistant, Content: s})
	}

	for i, tc := range t.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		idx := i
		args := tc.SplitArgs
		if len(args) == 0 {
			args = []string{tc.Args}
		}
		for j, frag := range args {
			call := schema.ToolCall{Index: &idx, Type: "function"}
			// Como no fio real, id e nome vêm só no primeiro chunk da chamada;
			// os seguintes carregam apenas fragmentos de argumento.
			if j == 0 {
				call.ID = id
				call.Function.Name = tc.Name
			}
			call.Function.Arguments = frag
			out = append(out, &schema.Message{
				Role:      schema.Assistant,
				ToolCalls: []schema.ToolCall{call},
			})
		}
	}

	if t.Usage != nil && len(out) > 0 {
		out[len(out)-1].ResponseMeta = &schema.ResponseMeta{Usage: t.Usage}
	}
	return out
}

package aguitest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agui "github.com/ricardogayer/go-agui"
	wire "github.com/ricardogayer/go-agui/pkg/agui"
)

// Event is an AG-UI event decoded from the SSE stream, with the raw fields
// preserved — which allows asserting on anything the server wrote, not only
// on the type.
//
// PT: Event é um evento AG-UI decodificado do fluxo SSE, com os campos crus
// preservados — o que permite afirmar sobre qualquer coisa que o servidor
// escreveu, não só sobre o tipo.
type Event map[string]any

// Type returns the event's `type` field.
//
// PT: Type devolve o field `type` do evento.
func (e Event) Type() agui.EventType {
	t, _ := e["type"].(string)
	return agui.EventType(t)
}

// String returns a text field of the event.
//
// PT: String devolve um field de texto do evento.
func (e Event) String(field string) string {
	s, _ := e[field].(string)
	return s
}

// Input builds a minimal RunAgentInput with one user message. It is the body
// almost every test needs.
//
// PT: Input monta um RunAgentInput mínimo com uma mensagem de usuário. É o
// body que quase todo teste precisa.
func Input(text string) *agui.RunAgentInput {
	return &agui.RunAgentInput{
		ThreadID: "thread_test",
		RunID:    "run_test",
		Messages: agui.Messages{
			&agui.UserMessage{ID: "msg_1", Content: agui.TextContent(text)},
		},
	}
}

// Run executes a Runner in memory and returns the emitted events.
//
// It does not start a server: it builds the [agui.Handler] and exercises it
// with an httptest.ResponseRecorder. It is the shortest path between an agent
// and the event sequence it produces.
//
// An event the Emitter refuses as non-conformant to AG-UI 1.0 fails the test
// (pass agui.WithOnDropped to override), and the stream is checked with
// [RequireConformant].
//
// PT: Run executa um Runner em memória e devolve os eventos emitidos.
//
// Não sobe servidor: monta o [agui.Handler] e o exercita com um
// httptest.ResponseRecorder. É o caminho mais curto entre um agente e a
// sequência de eventos que ele produz.
//
// Um evento recusado pelo Emitter por não conformidade com o AG-UI 1.0
// reprova o teste (passe agui.WithOnDropped para mudar isso), e o stream é
// verificado com [RequireConformant].
func Run(t *testing.T, r agui.Runner, in *agui.RunAgentInput, opts ...agui.HandlerOption) []Event {
	t.Helper()
	failOnDrop := agui.WithOnDropped(func(_ context.Context, ev agui.Event, err error) {
		t.Errorf("aguitest: %s dropped: %v", ev.Type(), err)
	})
	return Collect(t, agui.Handler(r, append([]agui.HandlerOption{failOnDrop}, opts...)...), in)
}

// Collect exercises an http.Handler with the given input and returns the SSE
// events, after checking them with [RequireConformant].
//
// PT: Collect exercita um http.Handler com o input dado e devolve os eventos
// SSE, depois de verificá-los com [RequireConformant].
func Collect(t *testing.T, h http.Handler, in *agui.RunAgentInput) []Event {
	t.Helper()

	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("aguitest: marshalling the RunAgentInput: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(),
		http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("aguitest: status %d, body %s", rec.Code, rec.Body.String())
	}
	evs := ParseSSE(t, rec.Body.Bytes())
	RequireConformant(t, evs)
	return evs
}

// RequireConformant checks a stream the way a conformant AG-UI 1.0 consumer
// would: every event against the official JSON Schema, and the whole sequence
// against the protocol rules (run lifecycle, open/content/close discipline,
// tool calls, steps, ...). The stream must end with its run closed.
//
// PT: RequireConformant verifica um stream como um consumidor AG-UI 1.0
// conforme faria: cada evento contra o JSON Schema oficial, e a sequência
// inteira contra as regras do protocolo (ciclo de vida do run, disciplina
// abrir/conteúdo/fechar, tool calls, steps, ...). O stream deve terminar com
// o run fechado.
func RequireConformant(t *testing.T, evs []Event) {
	t.Helper()
	var checker wire.StreamChecker
	for i, e := range evs {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("aguitest: event %d: %v", i, err)
		}
		ev, err := wire.ParseEvent(raw)
		if err != nil {
			t.Fatalf("aguitest: event %d (%s) does not conform to the AG-UI %s schema: %v\n  %s",
				i, e.Type(), wire.ProtocolVersion, err, raw)
		}
		if _, err := checker.Check(ev); err != nil {
			t.Fatalf("aguitest: event %d (%s) breaks the AG-UI %s sequence rules: %v\n  got: %v",
				i, e.Type(), wire.ProtocolVersion, err, Types(evs))
		}
	}
	if len(evs) > 0 && !checker.Verifier.RunClosed() {
		t.Fatalf("aguitest: the stream ended with its run still open\n  got: %v", Types(evs))
	}
}

// ParseSSE decodes an SSE stream into events.
//
// PT: ParseSSE decodifica um fluxo SSE em eventos.
func ParseSSE(t *testing.T, raw []byte) []Event {
	t.Helper()
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue // `id:`/`event:`/blank lines carry no event (PT: não carregam evento)
		}
		var ev Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("aguitest: unreadable SSE frame %q: %v", line, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("aguitest: reading the SSE stream: %v", err)
	}
	return out
}

// Types returns, in order, the types of the events.
//
// PT: Types devolve, em ordem, os tipos dos eventos.
func Types(evs []Event) []agui.EventType {
	out := make([]agui.EventType, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Type())
	}
	return out
}

// RequireSequence requires the event types to be EXACTLY want, in order.
//
// It takes [agui.EventType] rather than string on purpose: a typo in a string
// ("TEXT_MESAGE_START") compiles and the test passes comparing two equally
// wrong values.
//
// PT: RequireSequence exige que os tipos dos eventos sejam EXATAMENTE os want,
// na ordem.
//
// Recebe [agui.EventType] e não string de propósito: um typo numa string
// ("TEXT_MESAGE_START") compila e o teste passa comparando dois valores
// igualmente errados.
func RequireSequence(t *testing.T, evs []Event, want ...agui.EventType) {
	t.Helper()
	got := Types(evs)
	if len(got) != len(want) {
		t.Fatalf("aguitest: expected %d events, got %d\n  expected: %v\n  got:      %v",
			len(want), len(got), want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aguitest: event %d is %s, expected %s\n  expected: %v\n  got:      %v",
				i, got[i], want[i], want, got)
		}
	}
}

// RequireContains requires the types to appear in the given order, allowing
// other events in between. Use it when the test cares about the relative order
// and not about the whole sequence.
//
// PT: RequireContains exige que os tipos apareçam na ordem dada, admitindo
// outros eventos no meio. Use quando o teste se importa com a ordem relativa e
// não com a sequência inteira.
func RequireContains(t *testing.T, evs []Event, want ...agui.EventType) {
	t.Helper()
	got := Types(evs)
	i := 0
	for _, got := range got {
		if i < len(want) && got == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("aguitest: subsequence %v not found\n  got: %v", want, got)
	}
}

// RequireBalanced checks the protocol's structural invariants:
//
//   - every TEXT_MESSAGE_START has an END, with no overlapping blocks;
//   - every TOOL_CALL_START has an END, and no ARGS arrives outside an open block;
//   - the run has EXACTLY one terminal event, and it is the last one.
//
// It is the most valuable assertion in this package: on their own, nobody
// writes this test, and it is precisely the invariant that a bug in block
// closing breaks — with the symptom "the UI hung", three weeks later.
//
// PT: RequireBalanced verifica os invariantes estruturais do protocolo:
//
//   - todo TEXT_MESSAGE_START tem END, sem blocos sobrepostos;
//   - todo TOOL_CALL_START tem END, e nenhum ARGS chega fora de um bloco aberto;
//   - o run tem EXATAMENTE um evento terminal, e ele é o último.
//
// É a asserção mais valiosa deste pacote: sozinho, ninguém escreve este teste,
// e é justamente o invariante que um bug no fechamento de blocos quebra — com
// sintoma de "a UI ficou pendurada", três semanas depois.
func RequireBalanced(t *testing.T, evs []Event) {
	t.Helper()

	openText := ""
	openTools := map[string]bool{}
	terminals := 0

	for i, e := range evs {
		switch e.Type() {
		case agui.EventTextMessageStart:
			if openText != "" {
				t.Fatalf("aguitest: event %d opens a text block with %q still open", i, openText)
			}
			openText = e.String("messageId")
		case agui.EventTextMessageContent:
			if openText == "" {
				t.Fatalf("aguitest: event %d is text content outside an open block", i)
			}
		case agui.EventTextMessageEnd:
			if openText == "" {
				t.Fatalf("aguitest: event %d closes a text block that was not open", i)
			}
			openText = ""
		case agui.EventToolCallStart:
			id := e.String("toolCallId")
			if id == "" {
				t.Fatalf("aguitest: event %d opens a tool call with an empty toolCallId", i)
			}
			if openTools[id] {
				t.Fatalf("aguitest: event %d reopens tool call %q", i, id)
			}
			openTools[id] = true
		case agui.EventToolCallArgs:
			if id := e.String("toolCallId"); !openTools[id] {
				t.Fatalf("aguitest: event %d sends arguments to tool call %q, which is not open", i, id)
			}
		case agui.EventToolCallEnd:
			id := e.String("toolCallId")
			if !openTools[id] {
				t.Fatalf("aguitest: event %d closes tool call %q, which was not open", i, id)
			}
			delete(openTools, id)
		case agui.EventRunFinished, agui.EventRunError:
			terminals++
			if i != len(evs)-1 {
				t.Fatalf("aguitest: terminal event %s at position %d, but there are %d events",
					e.Type(), i, len(evs))
			}
		}
	}

	if openText != "" {
		t.Fatalf("aguitest: text block %q left open at the end of the run", openText)
	}
	if len(openTools) > 0 {
		t.Fatalf("aguitest: tool calls left open at the end of the run: %v", keys(openTools))
	}
	if terminals != 1 {
		t.Fatalf("aguitest: the run has %d terminal events, expected exactly 1\n  got: %v",
			terminals, Types(evs))
	}
}

// First returns the first event of a type.
//
// PT: First devolve o primeiro evento de um tipo.
func First(evs []Event, typ agui.EventType) (Event, bool) {
	for _, e := range evs {
		if e.Type() == typ {
			return e, true
		}
	}
	return nil, false
}

// All returns all the events of a type.
//
// PT: All devolve todos os eventos de um tipo.
func All(evs []Event, typ agui.EventType) []Event {
	var out []Event
	for _, e := range evs {
		if e.Type() == typ {
			out = append(out, e)
		}
	}
	return out
}

// ConcatenatedText joins the TEXT_MESSAGE_CONTENT deltas, which is the text
// the user would end up seeing.
//
// PT: ConcatenatedText junta os deltas de TEXT_MESSAGE_CONTENT, que é o texto
// que o usuário acabaria vendo.
func ConcatenatedText(evs []Event) string {
	var b strings.Builder
	for _, e := range All(evs, agui.EventTextMessageContent) {
		b.WriteString(e.String("delta"))
	}
	return b.String()
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

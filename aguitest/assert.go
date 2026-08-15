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
)

// Evento é um evento AG-UI decodificado do fluxo SSE, com os campos crus
// preservados — o que permite afirmar sobre qualquer coisa que o servidor
// escreveu, não só sobre o tipo.
type Evento map[string]any

// Tipo devolve o campo `type` do evento.
func (e Evento) Tipo() agui.EventType {
	t, _ := e["type"].(string)
	return agui.EventType(t)
}

// String devolve um campo de texto do evento.
func (e Evento) String(campo string) string {
	s, _ := e[campo].(string)
	return s
}

// Input monta um RunAgentInput mínimo com uma mensagem de usuário. É o corpo
// que quase todo teste precisa.
func Input(texto string) *agui.RunAgentInput {
	return &agui.RunAgentInput{
		ThreadID: "thread_teste",
		RunID:    "run_teste",
		Messages: []agui.Message{
			{ID: "msg_1", Role: agui.RoleUser, Content: texto},
		},
	}
}

// Run executa um Runner em memória e devolve os eventos emitidos.
//
// Não sobe servidor: monta o [agui.Handler] e o exercita com um
// httptest.ResponseRecorder. É o caminho mais curto entre um agente e a
// sequência de eventos que ele produz.
func Run(t *testing.T, r agui.Runner, in *agui.RunAgentInput, opts ...agui.HandlerOption) []Evento {
	t.Helper()
	return Collect(t, agui.Handler(r, opts...), in)
}

// Collect exercita um http.Handler com o input dado e devolve os eventos SSE.
func Collect(t *testing.T, h http.Handler, in *agui.RunAgentInput) []Evento {
	t.Helper()

	corpo, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("aguitest: serializando o RunAgentInput: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(),
		http.MethodPost, "/", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("aguitest: status %d, corpo %s", rec.Code, rec.Body.String())
	}
	return ParseSSE(t, rec.Body.Bytes())
}

// ParseSSE decodifica um fluxo SSE em eventos.
func ParseSSE(t *testing.T, raw []byte) []Evento {
	t.Helper()
	var out []Evento
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		linha := sc.Text()
		if !strings.HasPrefix(linha, "data: ") {
			continue // `id:`/`event:`/linhas em branco não carregam evento
		}
		var ev Evento
		if err := json.Unmarshal([]byte(strings.TrimPrefix(linha, "data: ")), &ev); err != nil {
			t.Fatalf("aguitest: quadro SSE ilegível %q: %v", linha, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("aguitest: lendo o fluxo SSE: %v", err)
	}
	return out
}

// Tipos devolve, em ordem, os tipos dos eventos.
func Tipos(evs []Evento) []agui.EventType {
	out := make([]agui.EventType, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Tipo())
	}
	return out
}

// RequireSequence exige que os tipos dos eventos sejam EXATAMENTE os esperados,
// na ordem.
//
// Recebe [agui.EventType] e não string de propósito: um typo numa string
// ("TEXT_MESAGE_START") compila e o teste passa comparando dois valores
// igualmente errados.
func RequireSequence(t *testing.T, evs []Evento, esperados ...agui.EventType) {
	t.Helper()
	obtidos := Tipos(evs)
	if len(obtidos) != len(esperados) {
		t.Fatalf("aguitest: esperava %d eventos, veio %d\n  esperado: %v\n  obtido:   %v",
			len(esperados), len(obtidos), esperados, obtidos)
	}
	for i := range esperados {
		if obtidos[i] != esperados[i] {
			t.Fatalf("aguitest: evento %d é %s, esperava %s\n  esperado: %v\n  obtido:   %v",
				i, obtidos[i], esperados[i], esperados, obtidos)
		}
	}
}

// RequireContains exige que os tipos apareçam na ordem dada, admitindo outros
// eventos no meio. Use quando o teste se importa com a ordem relativa e não com
// a sequência inteira.
func RequireContains(t *testing.T, evs []Evento, esperados ...agui.EventType) {
	t.Helper()
	obtidos := Tipos(evs)
	i := 0
	for _, got := range obtidos {
		if i < len(esperados) && got == esperados[i] {
			i++
		}
	}
	if i != len(esperados) {
		t.Fatalf("aguitest: não achei a subsequência %v\n  obtido: %v", esperados, obtidos)
	}
}

// RequireBalanced verifica os invariantes estruturais do protocolo:
//
//   - todo TEXT_MESSAGE_START tem END, sem blocos sobrepostos;
//   - todo TOOL_CALL_START tem END, e nenhum ARGS chega fora de um bloco aberto;
//   - o run tem EXATAMENTE um evento terminal, e ele é o último.
//
// É a asserção mais valiosa deste pacote: sozinho, ninguém escreve este teste,
// e é justamente o invariante que um bug no fechamento de blocos quebra — com
// sintoma de "a UI ficou pendurada", três semanas depois.
func RequireBalanced(t *testing.T, evs []Evento) {
	t.Helper()

	textoAberto := ""
	toolsAbertas := map[string]bool{}
	terminais := 0

	for i, e := range evs {
		switch e.Tipo() {
		case agui.EventTextMessageStart:
			if textoAberto != "" {
				t.Fatalf("aguitest: evento %d abre um bloco de texto com %q ainda aberto", i, textoAberto)
			}
			textoAberto = e.String("messageId")
		case agui.EventTextMessageChunk:
			if textoAberto == "" {
				t.Fatalf("aguitest: evento %d é conteúdo de texto fora de um bloco aberto", i)
			}
		case agui.EventTextMessageEnd:
			if textoAberto == "" {
				t.Fatalf("aguitest: evento %d fecha um bloco de texto que não estava aberto", i)
			}
			textoAberto = ""
		case agui.EventToolCallStart:
			id := e.String("toolCallId")
			if id == "" {
				t.Fatalf("aguitest: evento %d abre uma tool call com toolCallId vazio", i)
			}
			if toolsAbertas[id] {
				t.Fatalf("aguitest: evento %d reabre a tool call %q", i, id)
			}
			toolsAbertas[id] = true
		case agui.EventToolCallArgs:
			if id := e.String("toolCallId"); !toolsAbertas[id] {
				t.Fatalf("aguitest: evento %d manda argumentos para a tool call %q, que não está aberta", i, id)
			}
		case agui.EventToolCallEnd:
			id := e.String("toolCallId")
			if !toolsAbertas[id] {
				t.Fatalf("aguitest: evento %d fecha a tool call %q, que não estava aberta", i, id)
			}
			delete(toolsAbertas, id)
		case agui.EventRunFinished, agui.EventRunError:
			terminais++
			if i != len(evs)-1 {
				t.Fatalf("aguitest: evento terminal %s na posição %d, mas há %d eventos",
					e.Tipo(), i, len(evs))
			}
		}
	}

	if textoAberto != "" {
		t.Fatalf("aguitest: bloco de texto %q ficou aberto no fim do run", textoAberto)
	}
	if len(toolsAbertas) > 0 {
		t.Fatalf("aguitest: tool calls abertas no fim do run: %v", chaves(toolsAbertas))
	}
	if terminais != 1 {
		t.Fatalf("aguitest: o run tem %d eventos terminais, esperava exatamente 1\n  obtido: %v",
			terminais, Tipos(evs))
	}
}

// Primeiro devolve o primeiro evento de um tipo.
func Primeiro(evs []Evento, tipo agui.EventType) (Evento, bool) {
	for _, e := range evs {
		if e.Tipo() == tipo {
			return e, true
		}
	}
	return nil, false
}

// Todos devolve todos os eventos de um tipo.
func Todos(evs []Evento, tipo agui.EventType) []Evento {
	var out []Evento
	for _, e := range evs {
		if e.Tipo() == tipo {
			out = append(out, e)
		}
	}
	return out
}

// TextoConcatenado junta os deltas de TEXT_MESSAGE_CONTENT, que é o texto que o
// usuário acabaria vendo.
func TextoConcatenado(evs []Evento) string {
	var b strings.Builder
	for _, e := range Todos(evs, agui.EventTextMessageChunk) {
		b.WriteString(e.String("delta"))
	}
	return b.String()
}

func chaves(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

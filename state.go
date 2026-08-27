package agui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

// State is the shared state of a run: a free-form JSON document, with no
// reserved keys, mirrored in the client's `agent.state`.
//
// The contract this type guarantees — and the reason it exists instead of you
// assembling patches by hand — is that **the operation applied here is exactly
// the one that goes on the wire**. A client that starts from the STATE_SNAPSHOT
// and applies every STATE_DELTA in order rebuilds the server's document byte
// for byte. There is no "I mutated and forgot to emit" mode.
//
// A State obtained via [RunContext.State] is already bound to the run's
// Emitter. A State created by [NewState] outside a run emits nothing and is
// meant for tests.
//
// It is safe for concurrent use.
//
// PT: State é o estado compartilhado de um run: um documento JSON livre, sem chaves
// reservadas, espelhado no `agent.state` do cliente.
//
// O contrato que este tipo garante — e a razão de ele existir em vez de você
// montar os patches à mão — é que **a operação aplicada aqui é exatamente a que
// vai no fio**. Um cliente que parta do STATE_SNAPSHOT e aplique todos os
// STATE_DELTA em ordem reconstrói byte a byte o documento do servidor. Não há o
// modo "mutei e esqueci de emitir".
//
// Um State obtido por [RunContext.State] já está ligado ao Emitter do run. Um
// State criado por [NewState] fora de um run não emite nada e serve para
// testes.
//
// É seguro para uso concorrente.
type State struct {
	mu   sync.Mutex
	doc  map[string]any
	emit *Emitter
}

// NewState creates a state seeded with seed. A nil seed becomes an empty
// document. The seed is copied; later mutations of the original map do not
// affect the State.
//
// PT: NewState cria um estado semeado com seed. Um seed nil vira um documento
// vazio. O seed é copiado; mutações posteriores no mapa original não afetam o
// State.
func NewState(seed map[string]any) *State {
	return &State{doc: deepCopyJSONMap(seed)}
}

// stateFromAny seeds a State from RunAgentInput.state, which is `any`.
// Anything that is not a JSON object becomes an empty document: the client's
// state is an object by definition of the protocol.
//
// PT: stateFromAny semeia um State a partir do RunAgentInput.state, que é `any`.
// Qualquer coisa que não seja um objeto JSON vira documento vazio: o estado do
// cliente é um objeto por definição do protocolo.
func stateFromAny(v any) *State {
	if v == nil {
		return NewState(nil)
	}
	if m, ok := v.(map[string]any); ok {
		return NewState(m)
	}
	// It came from a decode with a concrete type (a caller's struct, for
	// example): round-trip through JSON to turn it into a map.
	//
	// PT: Veio de um decode com tipo concreto (um struct do chamador, por exemplo):
	// passa por JSON para virar mapa.
	b, err := json.Marshal(v)
	if err != nil {
		return NewState(nil)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return NewState(nil)
	}
	return NewState(m)
}

// bind ties the State to the run's Emitter. Called by the loop, not by the
// user.
//
// PT: bind liga o State ao Emitter do run. Chamado pelo laço, não pelo usuário.
func (s *State) bind(e *Emitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit = e
}

// Snapshot returns a deep copy of the document. Mutating the result does not
// affect the State.
//
// PT: Snapshot devolve uma cópia profunda do documento. Mutar o resultado não afeta
// o State.
func (s *State) Snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return deepCopyJSONMap(s.doc)
}

// Get reads the value at a JSON Pointer path (RFC 6901), e.g. "/todos/0/id".
//
// A deliberate deviation from the RFC: both "" and "/" return the whole
// document (in the RFC, "/" would point to a member with an empty key — which
// nobody has in a UI state, whereas "read the root" is what everyone wants).
// Note that [State.Set] follows the RFC to the letter via JSON Patch, so
// Set("/", v) is NOT the mirror of Get("/"); to replace the whole document,
// use [NewState] or write the members one by one.
//
// PT: Get lê o valor num caminho JSON Pointer (RFC 6901), por exemplo "/todos/0/id".
//
// Um desvio deliberado da RFC: tanto "" quanto "/" devolvem o documento
// inteiro (na RFC, "/" apontaria para um membro de chave vazia — que ninguém
// tem num estado de UI, enquanto "ler a raiz" é o que todo mundo quer). Note
// que [State.Set] segue a RFC à risca via JSON Patch, então Set("/", v) NÃO é
// o espelho de Get("/"); para substituir o documento inteiro, use [NewState]
// ou escreva os membros um a um.
func (s *State) Get(path string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return getPointer(s.doc, path)
}

// Set writes v at the JSON Pointer path, applying and emitting a STATE_DELTA
// with the corresponding operation.
//
// It uses `add`, not `replace`: in RFC 6902 `add` replaces a member that
// already exists and creates one that does not, so it works in both cases. A
// path whose PARENT does not exist is an error — create the parent first.
//
// PT: Set escreve v no caminho JSON Pointer path, aplicando e emitindo um
// STATE_DELTA com a operação correspondente.
//
// Usa `add`, e não `replace`: em RFC 6902 o `add` substitui um membro que já
// existe e cria um que não existe, então funciona nos dois casos. Um caminho
// cujo PAI não existe é erro — crie o pai antes.
func (s *State) Set(path string, v any) error {
	return s.Apply([]PatchOp{{Op: "add", Path: path, Value: v}})
}

// Remove deletes path, applying and emitting the STATE_DELTA.
//
// PT: Remove apaga o caminho path, aplicando e emitindo o STATE_DELTA.
func (s *State) Remove(path string) error {
	return s.Apply([]PatchOp{{Op: "remove", Path: path}})
}

// Apply applies the operations to the document and emits the corresponding
// STATE_DELTA.
//
// The application is atomic: if any operation fails, the document does not
// change and nothing is emitted — which preserves the invariant that client
// and server saw the same sequence of patches.
//
// PT: Apply aplica as operações ao documento e emite o STATE_DELTA correspondente.
//
// A aplicação é atômica: se qualquer operação falhar, o documento não muda e
// nada é emitido — o que preserva o invariante de que cliente e servidor viram
// a mesma sequência de patches.
func (s *State) Apply(ops []PatchOp) error {
	if len(ops) == 0 {
		return nil
	}

	s.mu.Lock()
	next, err := applyPatch(s.doc, ops)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.doc = next
	emit := s.emit
	s.mu.Unlock()

	if emit != nil {
		emit.StateDelta(ops)
	}
	return nil
}

// Emit publishes the whole document as a STATE_SNAPSHOT.
//
// PT: Emit publica o documento inteiro como STATE_SNAPSHOT.
func (s *State) Emit() {
	s.mu.Lock()
	emit, snap := s.emit, deepCopyJSONMap(s.doc)
	s.mu.Unlock()
	if emit != nil {
		emit.StateSnapshot(snap)
	}
}

// applyPatch runs the operations over a copy and returns the new document.
//
// PT: applyPatch roda as operações sobre uma cópia e devolve o documento novo.
func applyPatch(doc map[string]any, ops []PatchOp) (map[string]any, error) {
	rawOps, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("serializing JSON Patch operations: %w", err)
	}
	patch, err := jsonpatch.DecodePatch(rawOps)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON Patch operations: %w", err)
	}
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("serializing state: %w", err)
	}
	out, err := patch.Apply(rawDoc)
	if err != nil {
		return nil, fmt.Errorf("applying JSON Patch to state: %w", err)
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		return nil, fmt.Errorf("resulting state is not a JSON object: %w", err)
	}
	if next == nil {
		next = map[string]any{}
	}
	return next, nil
}

// deepCopyJSONMap copies via a JSON round-trip. It is the simplest way to
// guarantee that no sub-map or slice remains shared with the caller, and the
// cost is irrelevant for documents the size of a UI state.
//
// PT: deepCopyJSONMap copia via round-trip JSON. É o jeito mais simples de garantir
// que nenhum sub-mapa ou slice continue compartilhado com o chamador, e o custo
// é irrelevante para documentos do tamanho de um estado de UI.
func deepCopyJSONMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}
	return out
}

// getPointer resolves a JSON Pointer (RFC 6901) over the document.
//
// PT: getPointer resolve um JSON Pointer (RFC 6901) sobre o documento.
func getPointer(doc map[string]any, path string) (any, bool) {
	if path == "" || path == "/" {
		return doc, true
	}
	if path[0] != '/' {
		return nil, false
	}
	var cur any = doc
	for _, tok := range splitPointer(path[1:]) {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[tok]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := pointerIndex(tok, len(node))
			if err != nil {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func splitPointer(s string) []string {
	parts := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			parts = append(parts, unescapePointerToken(s[start:i]))
			start = i + 1
		}
	}
	return parts
}

// unescapePointerToken undoes the RFC 6901 escaping: ~1 is "/" and ~0 is "~",
// necessarily in that order (otherwise "~01" would become "/" instead of "~1").
//
// PT: unescapePointerToken desfaz o escape do RFC 6901: ~1 é "/" e ~0 é "~",
// necessariamente nessa ordem (senão "~01" viraria "/" em vez de "~1").
func unescapePointerToken(t string) string {
	return strings.ReplaceAll(strings.ReplaceAll(t, "~1", "/"), "~0", "~")
}

func pointerIndex(tok string, n int) (int, error) {
	i, err := strconv.Atoi(tok)
	if err != nil {
		return 0, fmt.Errorf("non-numeric index %q", tok)
	}
	if i < 0 || i >= n {
		return 0, fmt.Errorf("index %d out of range for array of %d", i, n)
	}
	return i, nil
}

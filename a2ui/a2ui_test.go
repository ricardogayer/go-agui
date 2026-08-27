package a2ui_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/a2ui"
)

// decode returns Render's envelope already decoded, failing the test if it is
// not JSON.
//
// PT: decode devolve o envelope de Render já decodificado, falhando o teste se ele
// não for JSON.
func decode(t *testing.T, payload string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("Render did not produce valid JSON: %v (%q)", err, payload)
	}
	return out
}

// The wire format is what `@copilotkit/a2ui-renderer` looks for: an object with
// the `a2ui_operations` key. Getting this key wrong makes the tool result be
// displayed as raw text in the chat instead of becoming a surface.
//
// PT: O formato do fio é o que o `@copilotkit/a2ui-renderer` procura: um objeto com
// a chave `a2ui_operations`. Errar essa chave faz o resultado da tool ser
// exibido como texto cru no chat, em vez de virar uma superfície.
func TestRenderWrapsOperationsInTheDetectionEnvelope(t *testing.T) {
	payload, err := a2ui.Render(
		a2ui.CreateSurface("s1", "copilotkit://app-dashboard-catalog"),
		a2ui.UpdateComponents("s1", []a2ui.Component{{"id": "root", "component": "Card"}}),
		a2ui.UpdateDataModel("s1", "/", map[string]any{"flights": []any{}}),
	)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	env := decode(t, payload)
	if len(env) != 1 {
		t.Fatalf("the envelope has %d keys, expected only `a2ui_operations`: %v", len(env), env)
	}
	ops, ok := env["a2ui_operations"].([]any)
	if !ok {
		t.Fatalf("`a2ui_operations` key missing or not a list: %v", env)
	}
	if len(ops) != 3 {
		t.Fatalf("the envelope has %d operations, expected 3", len(ops))
	}

	// Every operation carries the protocol version and exactly one command.
	//
	// PT: Toda operação carrega a versão do protocolo e exatamente um comando.
	wantCommand := []string{"createSurface", "updateComponents", "updateDataModel"}
	for i, raw := range ops {
		op, _ := raw.(map[string]any)
		if op["version"] != "v0.9" {
			t.Fatalf("operation %d has version %v, expected v0.9", i, op["version"])
		}
		if len(op) != 2 {
			t.Fatalf("operation %d has %d keys, expected version + 1 command: %v", i, len(op), op)
		}
		if _, ok := op[wantCommand[i]]; !ok {
			t.Fatalf("operation %d is not a %s: %v", i, wantCommand[i], op)
		}
	}
}

// createSurface without a catalog falls back to the specification's basic
// catalog — the same default as the Python helper.
//
// PT: createSurface sem catálogo cai no catálogo básico da especificação — o mesmo
// default do helper Python.
func TestCreateSurfaceDefaultsToTheBasicCatalog(t *testing.T) {
	op := a2ui.CreateSurface("s1", "")
	cmd, _ := op["createSurface"].(map[string]any)
	if cmd["catalogId"] != a2ui.BasicCatalogID {
		t.Fatalf("catalogId = %v, expected the basic catalog", cmd["catalogId"])
	}
	if cmd["surfaceId"] != "s1" {
		t.Fatalf("surfaceId = %v", cmd["surfaceId"])
	}
}

// updateDataModel without a path writes at the root.
//
// PT: updateDataModel sem caminho escreve na raiz.
func TestUpdateDataModelDefaultsToRoot(t *testing.T) {
	op := a2ui.UpdateDataModel("s1", "", map[string]any{"points": []any{}})
	cmd, _ := op["updateDataModel"].(map[string]any)
	if cmd["path"] != "/" {
		t.Fatalf("path = %v, expected /", cmd["path"])
	}
}

// An empty component list must come out as `[]`, not as `null`: the renderer
// iterates over the array and a null would break the application of the
// operation.
//
// PT: Uma lista de componentes vazia tem de sair como `[]`, e não como `null`: o
// renderer itera sobre o array e um nulo derrubaria a aplicação da operação.
func TestEmptyComponentsSerializeAsArrayNotNull(t *testing.T) {
	payload, err := a2ui.Render(a2ui.UpdateComponents("s1", nil))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(payload, `"components":null`) {
		t.Fatalf("empty components came out as null: %s", payload)
	}
	if !strings.Contains(payload, `"components":[]`) {
		t.Fatalf("empty components did not come out as []: %s", payload)
	}
}

// A value that is not serializable must become an error, not a truncated tool
// result — which would render an empty surface with no sign of what happened.
//
// PT: Um valor que não é serializável tem de virar erro, e não um resultado de tool
// truncado — que renderizaria uma superfície vazia sem nenhum sinal do que houve.
func TestRenderReportsUnserializableValues(t *testing.T) {
	if _, err := a2ui.Render(a2ui.UpdateDataModel("s1", "/", make(chan int))); err == nil {
		t.Fatal("Render accepted an unserializable value")
	}
}

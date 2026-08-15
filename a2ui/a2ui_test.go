package a2ui_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/a2ui"
)

// decode devolve o envelope de Render já decodificado, falhando o teste se ele
// não for JSON.
func decode(t *testing.T, payload string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("Render não produziu JSON válido: %v (%q)", err, payload)
	}
	return out
}

// O formato do fio é o que o `@copilotkit/a2ui-renderer` procura: um objeto com
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
		t.Fatalf("o envelope tem %d chaves, esperado só `a2ui_operations`: %v", len(env), env)
	}
	ops, ok := env["a2ui_operations"].([]any)
	if !ok {
		t.Fatalf("chave `a2ui_operations` ausente ou não é uma lista: %v", env)
	}
	if len(ops) != 3 {
		t.Fatalf("o envelope tem %d operações, esperado 3", len(ops))
	}

	// Toda operação carrega a versão do protocolo e exatamente um comando.
	wantCommand := []string{"createSurface", "updateComponents", "updateDataModel"}
	for i, raw := range ops {
		op, _ := raw.(map[string]any)
		if op["version"] != "v0.9" {
			t.Fatalf("operação %d com versão %v, esperado v0.9", i, op["version"])
		}
		if len(op) != 2 {
			t.Fatalf("operação %d tem %d chaves, esperado version + 1 comando: %v", i, len(op), op)
		}
		if _, ok := op[wantCommand[i]]; !ok {
			t.Fatalf("operação %d não é um %s: %v", i, wantCommand[i], op)
		}
	}
}

// createSurface sem catálogo cai no catálogo básico da especificação — o mesmo
// default do helper Python.
func TestCreateSurfaceDefaultsToTheBasicCatalog(t *testing.T) {
	op := a2ui.CreateSurface("s1", "")
	cmd, _ := op["createSurface"].(map[string]any)
	if cmd["catalogId"] != a2ui.BasicCatalogID {
		t.Fatalf("catalogId = %v, esperado o catálogo básico", cmd["catalogId"])
	}
	if cmd["surfaceId"] != "s1" {
		t.Fatalf("surfaceId = %v", cmd["surfaceId"])
	}
}

// updateDataModel sem caminho escreve na raiz.
func TestUpdateDataModelDefaultsToRoot(t *testing.T) {
	op := a2ui.UpdateDataModel("s1", "", map[string]any{"points": []any{}})
	cmd, _ := op["updateDataModel"].(map[string]any)
	if cmd["path"] != "/" {
		t.Fatalf("path = %v, esperado /", cmd["path"])
	}
}

// Uma lista de componentes vazia tem de sair como `[]`, e não como `null`: o
// renderer itera sobre o array e um nulo derrubaria a aplicação da operação.
func TestEmptyComponentsSerializeAsArrayNotNull(t *testing.T) {
	payload, err := a2ui.Render(a2ui.UpdateComponents("s1", nil))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(payload, `"components":null`) {
		t.Fatalf("componentes vazios saíram como null: %s", payload)
	}
	if !strings.Contains(payload, `"components":[]`) {
		t.Fatalf("componentes vazios não saíram como []: %s", payload)
	}
}

// Um valor que não é serializável tem de virar erro, e não um resultado de tool
// truncado — que renderizaria uma superfície vazia sem nenhum sinal do que houve.
func TestRenderReportsUnserializableValues(t *testing.T) {
	if _, err := a2ui.Render(a2ui.UpdateDataModel("s1", "/", make(chan int))); err == nil {
		t.Fatal("Render aceitou um valor não serializável")
	}
}

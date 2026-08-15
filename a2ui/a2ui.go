// Package a2ui é o port em Go de `copilotkit/a2ui.py` — o gerador das operações
// A2UI v0.9 que uma tool de backend devolve como resultado.
//
// Não há nada de modelo aqui: A2UI é um formato JSON, e o que o Python fazia em
// 246 linhas (a maior parte delas texto de prompt que o runtime TypeScript
// injeta sozinho) são três construtores e um envelope.
//
// O caminho é este: a tool de backend (`display_flights`, por exemplo) devolve a
// string produzida por Render como conteúdo do TOOL_CALL_RESULT; o
// `@copilotkit/a2ui-renderer` do frontend reconhece a chave `a2ui_operations`,
// aplica as operações na superfície e renderiza os componentes do catálogo React
// (`frontend/src/catalog/`).
//
// Não confundir com a tool `generate_a2ui`: aquela é injetada pelo
// A2UIMiddleware do runtime, chega como tool de FRONTEND em RunAgentInput.tools
// e nunca é executada aqui — é o caminho de schema dinâmico, em que o próprio
// modelo desenha a superfície. Este pacote serve o caminho de schema FIXO, em
// que o servidor manda um layout que ele já conhece.
package a2ui

import (
	"encoding/json"
	"fmt"
)

const (
	// Version é a versão do protocolo carregada em cada operação.
	Version = "v0.9"

	// OperationsKey é a chave do envelope que o renderer procura para detectar
	// que o resultado de uma tool é uma carga A2UI (A2UI_OPERATIONS_KEY no
	// Python).
	OperationsKey = "a2ui_operations"

	// BasicCatalogID é o catálogo padrão da especificação v0.9. Esta lição não o
	// usa — ela registra o próprio catálogo em `frontend/src/catalog/` —, mas o
	// valor é mantido porque é o default do helper Python.
	BasicCatalogID = "https://a2ui.org/specification/v0_9/basic_catalog.json"
)

// Operation é uma operação A2UI serializável. O formato é um objeto com
// `version` e exatamente uma chave de comando (createSurface, updateComponents
// ou updateDataModel).
type Operation map[string]any

// Component é um nó do schema de componentes: `{"id": ..., "component": ...}`
// mais as props do componente do catálogo.
type Component map[string]any

// CreateSurface monta a operação `createSurface`.
//
// catalogID vazio cai no catálogo básico da especificação, espelhando o
// argumento com valor padrão do helper Python.
func CreateSurface(surfaceID, catalogID string) Operation {
	if catalogID == "" {
		catalogID = BasicCatalogID
	}
	return Operation{
		"version": Version,
		"createSurface": map[string]any{
			"surfaceId": surfaceID,
			"catalogId": catalogID,
		},
	}
}

// UpdateComponents monta a operação `updateComponents` — o schema declarativo da
// superfície.
func UpdateComponents(surfaceID string, components []Component) Operation {
	// Uma lista nil sairia como `null` no JSON, e o renderer espera um array.
	if components == nil {
		components = []Component{}
	}
	return Operation{
		"version": Version,
		"updateComponents": map[string]any{
			"surfaceId":  surfaceID,
			"components": components,
		},
	}
}

// UpdateDataModel monta a operação `updateDataModel`, escrevendo `value` no
// caminho `path` do modelo de dados da superfície.
//
// path vazio vira "/" (a raiz), que é o default do helper Python.
func UpdateDataModel(surfaceID, path string, value any) Operation {
	if path == "" {
		path = "/"
	}
	return Operation{
		"version": Version,
		"updateDataModel": map[string]any{
			"surfaceId": surfaceID,
			"path":      path,
			"value":     value,
		},
	}
}

// Render embrulha as operações no envelope `a2ui_operations` e serializa.
//
// O erro nunca deveria acontecer com os construtores deste pacote (tudo é
// map/slice de valores serializáveis), mas `value` de UpdateDataModel vem de
// fora — de argumentos que o modelo produziu —, então a falha é devolvida em vez
// de engolida: um resultado de tool truncado renderizaria uma superfície vazia
// sem nenhum sinal do que houve.
func Render(operations ...Operation) (string, error) {
	if operations == nil {
		operations = []Operation{}
	}
	b, err := json.Marshal(map[string]any{OperationsKey: operations})
	if err != nil {
		return "", fmt.Errorf("serializando operações A2UI: %w", err)
	}
	return string(b), nil
}

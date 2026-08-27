// Package a2ui generates the A2UI v0.9 operations that a backend tool returns
// as its result (the Go counterpart of CopilotKit's Python `a2ui` helper).
//
// There is nothing model-related here: A2UI is a JSON format, and the package
// is three constructors and an envelope.
//
// The path is this: the backend tool (`display_flights`, for example) returns
// the string produced by Render as the content of the TOOL_CALL_RESULT; the
// frontend's `@copilotkit/a2ui-renderer` recognizes the `a2ui_operations` key,
// applies the operations to the surface and renders the components from the
// application's React catalog.
//
// Not to be confused with the `generate_a2ui` tool: that one is injected by the
// runtime's A2UIMiddleware, arrives as a FRONTEND tool in RunAgentInput.tools
// and is never executed here — it is the dynamic-schema path, in which the
// model itself draws the surface. This package serves the FIXED-schema path, in
// which the server sends a layout it already knows.
//
// PT: Package a2ui gera as operações A2UI v0.9 que uma tool de backend devolve
// como resultado (o equivalente em Go do helper Python `a2ui` do CopilotKit).
//
// Não há nada de modelo aqui: A2UI é um formato JSON, e o pacote são três
// construtores e um envelope.
//
// O caminho é este: a tool de backend (`display_flights`, por exemplo) devolve a
// string produzida por Render como conteúdo do TOOL_CALL_RESULT; o
// `@copilotkit/a2ui-renderer` do frontend reconhece a chave `a2ui_operations`,
// aplica as operações na superfície e renderiza os componentes do catálogo React
// da aplicação.
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
	// Version is the protocol version carried in every operation.
	//
	// PT: Version é a versão do protocolo carregada em cada operação.
	Version = "v0.9"

	// OperationsKey is the envelope key the renderer looks for to detect that a
	// tool result is an A2UI payload (A2UI_OPERATIONS_KEY in Python).
	//
	// PT: OperationsKey é a chave do envelope que o renderer procura para detectar
	// que o resultado de uma tool é uma carga A2UI (A2UI_OPERATIONS_KEY no
	// Python).
	OperationsKey = "a2ui_operations"

	// BasicCatalogID is the default catalog of the v0.9 specification and the
	// default of the Python helper. Applications that register their own React
	// catalog pass its id to [Render] explicitly.
	//
	// PT: BasicCatalogID é o catálogo padrão da especificação v0.9 e o default
	// do helper Python. Aplicações que registram o próprio catálogo React
	// passam o id dele a [Render] explicitamente.
	BasicCatalogID = "https://a2ui.org/specification/v0_9/basic_catalog.json"
)

// Operation is a serializable A2UI operation. The format is an object with
// `version` and exactly one command key (createSurface, updateComponents or
// updateDataModel).
//
// PT: Operation é uma operação A2UI serializável. O formato é um objeto com
// `version` e exatamente uma chave de comando (createSurface, updateComponents
// ou updateDataModel).
type Operation map[string]any

// Component is a node of the component schema: `{"id": ..., "component": ...}`
// plus the props of the catalog component.
//
// PT: Component é um nó do schema de componentes: `{"id": ..., "component": ...}`
// mais as props do componente do catálogo.
type Component map[string]any

// CreateSurface builds the `createSurface` operation.
//
// An empty catalogID falls back to the specification's basic catalog,
// mirroring the defaulted argument of the Python helper.
//
// PT: CreateSurface monta a operação `createSurface`.
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

// UpdateComponents builds the `updateComponents` operation — the declarative
// schema of the surface.
//
// PT: UpdateComponents monta a operação `updateComponents` — o schema declarativo da
// superfície.
func UpdateComponents(surfaceID string, components []Component) Operation {
	// A nil list would come out as `null` in the JSON, and the renderer expects an array.
	//
	// PT: Uma lista nil sairia como `null` no JSON, e o renderer espera um array.
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

// UpdateDataModel builds the `updateDataModel` operation, writing `value` at
// path `path` of the surface's data model.
//
// An empty path becomes "/" (the root), which is the default of the Python helper.
//
// PT: UpdateDataModel monta a operação `updateDataModel`, escrevendo `value` no
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

// Render wraps the operations in the `a2ui_operations` envelope and serializes.
//
// The error should never happen with this package's constructors (everything is
// a map/slice of serializable values), but UpdateDataModel's `value` comes from
// outside — from arguments the model produced — so the failure is returned
// instead of swallowed: a truncated tool result would render an empty surface
// with no sign of what happened.
//
// PT: Render embrulha as operações no envelope `a2ui_operations` e serializa.
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
		return "", fmt.Errorf("serializing A2UI operations: %w", err)
	}
	return string(b), nil
}

package agui

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// clientToolInfos converte as definições que chegam em RunAgentInput.tools (as
// tools de frontend registradas por useFrontendTool/useComponent no React) em
// ToolInfos do Eino, prontas para WithTools.
//
// Uma tool inválida é DESCARTADA com aviso, não derruba o run. O motivo é
// prático: esses schemas são gerados por Zod no navegador, e um único
// useFrontendTool com schema quebrado mataria a conversa inteira — o usuário
// veria o chat morrer sem nenhuma pista. Descartar degrada uma funcionalidade;
// abortar degrada tudo.
//
// Nome duplicado idem: fica a primeira ocorrência.
func clientToolInfos(tools []ClientTool, logger *slog.Logger) []*schema.ToolInfo {
	out := make([]*schema.ToolInfo, 0, len(tools))
	seen := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t.Name == "" {
			logger.Warn("descartando tool de frontend sem nome")
			continue
		}
		if seen[t.Name] {
			logger.Warn("descartando tool de frontend duplicada", "tool", t.Name)
			continue
		}

		js, err := toJSONSchema(t.Parameters)
		if err != nil {
			logger.Warn("descartando tool de frontend com schema inválido",
				"tool", t.Name, "error", err)
			continue
		}

		seen[t.Name] = true
		info := &schema.ToolInfo{Name: t.Name, Desc: t.Description}
		if js != nil {
			info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
		}
		out = append(out, info)
	}
	return out
}

// ParseJSONSchema converte um JSON Schema escrito como JSON cru no
// *jsonschema.Schema do Eino.
//
// Existe para que uma tool de backend possa declarar seus parâmetros na mesma
// forma em que as tools de frontend chegam pelo fio — um JSON Schema — em vez
// de montar a estrutura campo a campo. O que o modelo recebe passa pelo mesmo
// caminho de conversão nos dois casos. Ver [NewRawTool].
func ParseJSONSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var params any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("não é um JSON válido: %w", err)
	}
	return toJSONSchema(params)
}

// ClientToolNames indexa por nome as tools que só o cliente pode executar.
func ClientToolNames(tools []ClientTool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t.Name != "" {
			names[t.Name] = true
		}
	}
	return names
}

// toJSONSchema converte o JSON Schema arbitrário vindo do cliente (o campo
// `parameters` da tool, decodificado como `any`) num *jsonschema.Schema do
// Eino. Um schema ausente/nil devolve nil (tool sem argumentos).
//
// Nota sobre saneamento de schema: os schemas que chegam aqui são gerados por
// Zod no React e trazem infraestrutura JSON Schema (`$schema`, e possivelmente
// `$ref`/`$defs` quando há sub-schemas reaproveitados). Adaptadores para
// provedores que não entendem essa infraestrutura precisam removê-la — é o que
// `_clean_schema_for_genai` faz em ag_ui_adk para o Gemini. A API da OpenAI
// aceita `$ref`/`$defs` em `function.parameters`, então removê-los aqui
// quebraria justamente os schemas que dependem deles. Só o `$schema` do topo é
// descartado, por ser ruído: ele não descreve nenhum parâmetro.
func toJSONSchema(params any) (*jsonschema.Schema, error) {
	if params == nil {
		return nil, nil
	}
	if m, ok := params.(map[string]any); ok {
		if _, has := m["$schema"]; has {
			cleaned := make(map[string]any, len(m))
			for k, v := range m {
				if k == "$schema" {
					continue
				}
				cleaned[k] = v
			}
			params = cleaned
		}
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("não é um JSON Schema válido: %w", err)
	}
	return &s, nil
}

// ClassifyToolCalls separa as chamadas que o servidor pode executar das que
// pertencem ao cliente.
//
// Um nome presente em clientNames é tool de frontend: o servidor devolve o
// controle e o cliente executa. Qualquer outro nome — uma tool de backend ou um
// nome alucinado pelo modelo — vai para o lado do servidor, onde é executado ou
// respondido com um erro que o modelo consegue corrigir.
func ClassifyToolCalls(calls []schema.ToolCall, clientNames map[string]bool) (server, client []schema.ToolCall) {
	for _, tc := range calls {
		if clientNames[tc.Function.Name] {
			client = append(client, tc)
		} else {
			server = append(server, tc)
		}
	}
	return server, client
}

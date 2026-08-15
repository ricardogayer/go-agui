package agui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// Os construtores deste arquivo NÃO devolvem erro, e isso é deliberado.
//
// Em Go uma expressão de dois valores só pode ser a lista COMPLETA de
// argumentos de uma chamada. Se NewTool devolvesse (Tool, error), então
//
//	WithTools(NewTool(...), NewRawTool(...))
//
// nem compilaria, e o call site viraria um `t1, err := ...; if err != nil` por
// tool — três linhas de cerimônia para um erro que só acontece se você escrever
// uma struct tag inválida, ou seja, um erro de programador, detectado na
// subida, nunca em produção.
//
// Em vez disso o erro fica retido na própria tool (ver brokenTool) e é colhido
// por [New], que já devolve error, agregado com errors.Join e nomeado:
//
//	tool "display_flights": schema de parâmetros inválido: ...

// NewTool cria uma tool de servidor inferindo o JSON Schema dos parâmetros a
// partir das struct tags de In.
//
// A entrada é decodificada de JSON para In antes de fn ser chamada, e a saída é
// codificada de volta — você trabalha com tipos, não com strings:
//
//	type BuscaVoos struct {
//		Origem  string `json:"origem"  jsonschema:"description=Código IATA de origem"`
//		Destino string `json:"destino" jsonschema:"description=Código IATA de destino"`
//	}
//
//	agui.NewTool("search_flights", "Busca voos entre dois aeroportos.",
//		func(ctx context.Context, in BuscaVoos) ([]Voo, error) { ... })
//
// O reflector do Eino gera o schema sem `$ref`/`$defs`/`$schema`, que é a forma
// que os provedores aceitam sem saneamento adicional.
//
// Para alcançar o emitter, o estado ou o id da chamada corrente de dentro de
// fn, use [FromContext].
func NewTool[In, Out any](nome, desc string, fn func(context.Context, In) (Out, error)) Tool {
	t, err := utils.InferTool(nome, desc, fn)
	if err != nil {
		return brokenTool{nome: nome, err: fmt.Errorf("inferindo schema dos parâmetros: %w", err)}
	}
	return t
}

// NewRawTool cria uma tool de servidor a partir de um JSON Schema escrito à
// mão, recebendo e devolvendo strings cruas.
//
// Existe para dois casos concretos: schemas que a inferência não expressa, e
// schemas que precisam ser preservados byte a byte porque o texto das
// descrições faz parte do prompt — trocar a redação muda o comportamento do
// modelo.
//
// params é json.RawMessage e não string de propósito: com três strings
// adjacentes o compilador não pega uma troca de ordem entre nome, descrição e
// schema.
func NewRawTool(nome, desc string, params json.RawMessage, fn func(ctx context.Context, args string) (string, error)) Tool {
	js, err := ParseJSONSchema(params)
	if err != nil {
		return brokenTool{nome: nome, err: fmt.Errorf("schema de parâmetros inválido: %w", err)}
	}
	info := &schema.ToolInfo{Name: nome, Desc: desc}
	if js != nil {
		info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
	}
	return NewToolFromInfo(info, fn)
}

// NewToolFromInfo cria uma tool de servidor a partir de um *schema.ToolInfo já
// montado. É o construtor de último recurso, para quem monta o schema
// programaticamente.
func NewToolFromInfo(info *schema.ToolInfo, fn func(ctx context.Context, args string) (string, error)) Tool {
	if info == nil || info.Name == "" {
		return brokenTool{nome: "(sem nome)", err: fmt.Errorf("ToolInfo precisa de um nome")}
	}
	if fn == nil {
		return brokenTool{nome: info.Name, err: fmt.Errorf("função de execução nula")}
	}
	return &funcTool{info: info, fn: fn}
}

// NewClientTool declara uma tool que o SERVIDOR anuncia ao modelo mas NÃO
// executa: ao vê-la ser chamada, o run termina e o controle volta ao cliente.
//
// É o mesmo caminho das tools que chegam em RunAgentInput.tools, com uma
// diferença: aqui a declaração é do servidor, o que permite anunciar uma
// capacidade do frontend sem depender de o React tê-la registrado.
//
// Implementa apenas Info — é essa ausência de InvokableRun que o laço lê como
// "não sou eu que executo".
func NewClientTool(nome, desc string, params json.RawMessage) Tool {
	js, err := ParseJSONSchema(params)
	if err != nil {
		return brokenTool{nome: nome, err: fmt.Errorf("schema de parâmetros inválido: %w", err)}
	}
	if nome == "" {
		return brokenTool{nome: "(sem nome)", err: fmt.Errorf("tool de cliente precisa de um nome")}
	}
	info := &schema.ToolInfo{Name: nome, Desc: desc}
	if js != nil {
		info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
	}
	return announceOnlyTool{info: info}
}

// funcTool é a Tool de servidor construída a partir de uma função sobre strings.
type funcTool struct {
	info *schema.ToolInfo
	fn   func(ctx context.Context, args string) (string, error)
}

func (t *funcTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *funcTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	return t.fn(ctx, args)
}

// announceOnlyTool implementa apenas tool.BaseTool: anuncia o schema e nada
// mais. Ver [NewClientTool].
type announceOnlyTool struct{ info *schema.ToolInfo }

func (t announceOnlyTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

// brokenTool carrega um erro de construção até [New] colhê-lo. Implementa
// tool.BaseTool devolvendo o erro em Info, que é exatamente onde New o procura.
type brokenTool struct {
	nome string
	err  error
}

func (t brokenTool) Info(context.Context) (*schema.ToolInfo, error) {
	return nil, fmt.Errorf("tool %q: %w", t.nome, t.err)
}

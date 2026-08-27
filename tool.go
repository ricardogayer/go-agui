package agui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// The constructors in this file do NOT return an error, and that is deliberate.
//
// In Go a two-value expression can only be the COMPLETE argument list of a
// call. If NewTool returned (Tool, error), then
//
//	WithTools(NewTool(...), NewRawTool(...))
//
// would not even compile, and the call site would become a
// `t1, err := ...; if err != nil` per tool — three lines of ceremony for an
// error that only happens if you write an invalid struct tag, i.e. a
// programmer error, caught at startup, never in production.
//
// Instead, the error is held inside the tool itself (see brokenTool) and is
// collected by [New], which already returns error, aggregated with errors.Join
// and named:
//
//	tool "display_flights": invalid parameter schema: ...
//
// PT: Os construtores deste arquivo NÃO devolvem erro, e isso é deliberado.
//
// Em Go uma expressão de dois valores só pode ser a lista COMPLETA de
// argumentos de uma chamada. Se NewTool devolvesse (Tool, error), então o
// exemplo acima nem compilaria, e o call site viraria um
// `t1, err := ...; if err != nil` por tool — três linhas de cerimônia para um
// erro que só acontece se você escrever uma struct tag inválida, ou seja, um
// erro de programador, detectado na subida, nunca em produção.
//
// Em vez disso o erro fica retido na própria tool (ver brokenTool) e é colhido
// por [New], que já devolve error, agregado com errors.Join e nomeado, como no
// exemplo acima.

// NewTool creates a server tool, inferring the parameters' JSON Schema from
// the struct tags of In.
//
// The input is decoded from JSON into In before fn is called, and the output
// is encoded back — you work with types, not strings:
//
//	type FlightSearch struct {
//		Origin      string `json:"origin"      jsonschema:"description=Origin IATA code"`
//		Destination string `json:"destination" jsonschema:"description=Destination IATA code"`
//	}
//
//	agui.NewTool("search_flights", "Searches flights between two airports.",
//		func(ctx context.Context, in FlightSearch) ([]Flight, error) { ... })
//
// Eino's reflector generates the schema without `$ref`/`$defs`/`$schema`,
// which is the form providers accept without further sanitizing.
//
// To reach the emitter, the state, or the id of the current call from inside
// fn, use [FromContext].
//
// PT: NewTool cria uma tool de servidor inferindo o JSON Schema dos parâmetros
// a partir das struct tags de In.
//
// A entrada é decodificada de JSON para In antes de fn ser chamada, e a saída é
// codificada de volta — você trabalha com tipos, não com strings (ver o
// exemplo acima).
//
// O reflector do Eino gera o schema sem `$ref`/`$defs`/`$schema`, que é a forma
// que os provedores aceitam sem saneamento adicional.
//
// Para alcançar o emitter, o estado ou o id da chamada corrente de dentro de
// fn, use [FromContext].
func NewTool[In, Out any](name, desc string, fn func(context.Context, In) (Out, error)) Tool {
	t, err := utils.InferTool(name, desc, fn)
	if err != nil {
		return brokenTool{name: name, err: fmt.Errorf("inferring parameter schema: %w", err)}
	}
	return t
}

// NewRawTool creates a server tool from a hand-written JSON Schema, receiving
// and returning raw strings.
//
// It exists for two concrete cases: schemas that inference cannot express, and
// schemas that must be preserved byte for byte because the wording of the
// descriptions is part of the prompt — changing the wording changes the
// model's behavior.
//
// params is json.RawMessage rather than string on purpose: with three adjacent
// strings the compiler would not catch a swapped order between name,
// description and schema.
//
// PT: NewRawTool cria uma tool de servidor a partir de um JSON Schema escrito à
// mão, recebendo e devolvendo strings cruas.
//
// Existe para dois casos concretos: schemas que a inferência não expressa, e
// schemas que precisam ser preservados byte a byte porque o texto das
// descrições faz parte do prompt — trocar a redação muda o comportamento do
// modelo.
//
// params é json.RawMessage e não string de propósito: com três strings
// adjacentes o compilador não pega uma troca de ordem entre name, descrição e
// schema.
func NewRawTool(name, desc string, params json.RawMessage, fn func(ctx context.Context, args string) (string, error)) Tool {
	js, err := ParseJSONSchema(params)
	if err != nil {
		return brokenTool{name: name, err: fmt.Errorf("invalid parameter schema: %w", err)}
	}
	info := &schema.ToolInfo{Name: name, Desc: desc}
	if js != nil {
		info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
	}
	return NewToolFromInfo(info, fn)
}

// NewToolFromInfo creates a server tool from an already assembled
// *schema.ToolInfo. It is the constructor of last resort, for those who build
// the schema programmatically.
//
// PT: NewToolFromInfo cria uma tool de servidor a partir de um *schema.ToolInfo
// já montado. É o construtor de último recurso, para quem monta o schema
// programaticamente.
func NewToolFromInfo(info *schema.ToolInfo, fn func(ctx context.Context, args string) (string, error)) Tool {
	if info == nil || info.Name == "" {
		return brokenTool{name: "(unnamed)", err: fmt.Errorf("ToolInfo requires a name")}
	}
	if fn == nil {
		return brokenTool{name: info.Name, err: fmt.Errorf("nil execution function")}
	}
	return &funcTool{info: info, fn: fn}
}

// NewClientTool declares a tool that the SERVER announces to the model but
// does NOT execute: upon seeing it called, the run ends and control returns to
// the client.
//
// It is the same path as the tools arriving in RunAgentInput.tools, with one
// difference: here the declaration belongs to the server, which allows
// announcing a frontend capability without depending on React having
// registered it.
//
// It implements only Info — it is this absence of InvokableRun that the loop
// reads as "I am not the one who executes".
//
// PT: NewClientTool declara uma tool que o SERVIDOR anuncia ao modelo mas NÃO
// executa: ao vê-la ser chamada, o run termina e o controle volta ao cliente.
//
// É o mesmo caminho das tools que chegam em RunAgentInput.tools, com uma
// diferença: aqui a declaração é do servidor, o que permite anunciar uma
// capacidade do frontend sem depender de o React tê-la registrado.
//
// Implementa apenas Info — é essa ausência de InvokableRun que o laço lê como
// "não sou eu que executo".
func NewClientTool(name, desc string, params json.RawMessage) Tool {
	js, err := ParseJSONSchema(params)
	if err != nil {
		return brokenTool{name: name, err: fmt.Errorf("invalid parameter schema: %w", err)}
	}
	if name == "" {
		return brokenTool{name: "(unnamed)", err: fmt.Errorf("client tool requires a name")}
	}
	info := &schema.ToolInfo{Name: name, Desc: desc}
	if js != nil {
		info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
	}
	return announceOnlyTool{info: info}
}

// funcTool is the server Tool built from a function over strings.
//
// PT: funcTool é a Tool de servidor construída a partir de uma função sobre
// strings.
type funcTool struct {
	info *schema.ToolInfo
	fn   func(ctx context.Context, args string) (string, error)
}

func (t *funcTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *funcTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	return t.fn(ctx, args)
}

// announceOnlyTool implements only tool.BaseTool: it announces the schema and
// nothing else. See [NewClientTool].
//
// PT: announceOnlyTool implementa apenas tool.BaseTool: anuncia o schema e nada
// mais. Ver [NewClientTool].
type announceOnlyTool struct{ info *schema.ToolInfo }

func (t announceOnlyTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

// brokenTool carries a construction error until [New] collects it. It
// implements tool.BaseTool by returning the error from Info, which is exactly
// where New looks for it.
//
// PT: brokenTool carrega um erro de construção até [New] colhê-lo. Implementa
// tool.BaseTool devolvendo o erro em Info, que é exatamente onde New o procura.
type brokenTool struct {
	name string
	err  error
}

func (t brokenTool) Info(context.Context) (*schema.ToolInfo, error) {
	return nil, fmt.Errorf("tool %q: %w", t.name, t.err)
}

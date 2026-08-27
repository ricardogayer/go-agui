# go-agui

*Leia em [Inglês](README.md).*

**Biblioteca Go que implementa o protocolo [AG-UI](https://docs.ag-ui.com) sobre
SSE, com o laço modelo↔tools construído no [CloudWeGo Eino](https://github.com/cloudwego/eino).**

Você escreve o agente em Go — prompt, tools, estado — e a biblioteca cuida do
resto do contrato: o `POST /` que recebe um `RunAgentInput`, o
`text/event-stream` de volta, a sequência de eventos, o healthcheck, o
cancelamento e o desligamento gracioso.

Do outro lado da linha, qualquer runtime que fale AG-UI conversa com este
servidor — o [CopilotKit](https://copilotkit.ai), por exemplo, através de um
`HttpAgent` genérico. O frontend não sabe (nem precisa saber) que há Go aqui.

```go
func main() {
	ctx := context.Background()
	m := agui.Must(model.FromEnv(ctx))
	ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt("Você é um assistente útil.")))
	log.Fatal(ginx.ListenAndServe(ctx, ":8000", ag, "meu-agente"))
}
```

São quatro linhas para um agente com streaming, healthcheck, desligamento
gracioso, cancelamento na desconexão do cliente e a sequência de eventos AG-UI
completa.

---

## Sumário

- [O que a biblioteca é](#o-que-a-biblioteca-é)
- [Instalação](#instalação)
- [Experimentar sem subir nada](#experimentar-sem-subir-nada)
- **Uso**
  - [Criando o agente](#criando-o-agente)
  - [Tools](#tools)
  - [Dentro de uma tool](#dentro-de-uma-tool)
  - [Estado compartilhado](#estado-compartilhado)
  - [Human-in-the-loop](#human-in-the-loop)
  - [Renderizando UI: A2UI](#renderizando-ui-a2ui)
  - [Observabilidade](#observabilidade)
- **Servidor**
  - [Setup de um servidor Go com Gin](#setup-de-um-servidor-go-com-gin) ← passo a passo
  - [Sem Gin: net/http puro](#sem-gin-nethttp-puro)
  - [Níveis de controle](#níveis-de-controle)
- [Testes](#testes)
- [Armadilhas do protocolo que a biblioteca já resolve](#armadilhas-do-protocolo-que-a-biblioteca-já-resolve)
- [Variáveis de ambiente](#variáveis-de-ambiente)
- [Healthcheck em container distroless](#healthcheck-em-container-distroless)
- [Desenvolvimento](#desenvolvimento)

---

## O que a biblioteca é

O contrato AG-UI que ela implementa é o mesmo que o adaptador Python
`ag_ui_langgraph` monta:

| Rota | O que faz |
|---|---|
| `POST /` | recebe um `RunAgentInput` em JSON, responde `text/event-stream` com eventos AG-UI |
| `GET /health` | healthcheck com `{"status":"ok","agent":{"name":...}}` |
| `GET //health` | idem, reescrito para `/health` (compatibilidade com sondas escritas contra o backend Python) |

Um run completo emite, na ordem: `RUN_STARTED`, `STEP_STARTED`, os blocos de
texto (`TEXT_MESSAGE_START` / `_CONTENT` / `_END`) e de tool call
(`TOOL_CALL_START` / `_ARGS` / `_END` / `_RESULT`), `STEP_FINISHED`,
`STATE_SNAPSHOT`, `MESSAGES_SNAPSHOT` e exatamente **um** evento terminal
(`RUN_FINISHED` ou `RUN_ERROR`).

O laço modelo↔tools por trás disso é o Eino: o chat model é um
`model.ToolCallingChatModel`, e uma tool é um `tool.BaseTool`. Isso não é
detalhe de implementação — é o que faz qualquer tool do ecossistema `eino-ext`
(MCP, busca, o que for) entrar sem adaptador, e o que permite trazer o seu
próprio `ChatModel` sem tocar na biblioteca.

**Requisitos:** Go 1.24.4+ (o núcleo e o `model`); Go 1.25+ para o `ginx` e os
exemplos.

## Instalação

```bash
go get github.com/ricardogayer/go-agui@v0.2.0
go get github.com/ricardogayer/go-agui/model@v0.2.0   # provedores de modelo (opcional)
go get github.com/ricardogayer/go-agui/ginx@v0.2.0    # adaptador Gin (opcional)
```

São três módulos de propósito. O núcleo depende apenas do SDK AG-UI, do Eino e
de dois utilitários — nada de framework web, nada de cliente de provedor. Quem
usa `chi` não leva o Gin; quem já tem o próprio `ChatModel` não leva o `eino-ext`.

| Módulo | Import | Traz |
|---|---|---|
| núcleo | `github.com/ricardogayer/go-agui` | agente, handler, emitter, estado, tools |
| `model` | `.../go-agui/model` | OpenAI e Ollama via `eino-ext` |
| `ginx` | `.../go-agui/ginx` | Gin: rotas, engine, CORS, `ListenAndServe` |
| `aguitest` | `.../go-agui/aguitest` | `FakeModel` e asserções (subpacote do núcleo) |
| `a2ui` | `.../go-agui/a2ui` | operações A2UI v0.9 para renderizar UI (subpacote do núcleo) |

## Experimentar sem subir nada

O backend é testável sozinho, sem runtime TypeScript e sem navegador:

```bash
export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
go run ./examples/quickstart

curl -N -X POST localhost:8000 -H 'content-type: application/json' \
  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"oi"}]}'
```

Você vê os quadros SSE crus: `RUN_STARTED`, `TEXT_MESSAGE_START`, os deltas,
`MESSAGES_SNAPSHOT`, `RUN_FINISHED`.

---

# Uso

## Criando o agente

`agui.New` recebe um `model.ToolCallingChatModel` do Eino e devolve um `*Agent`,
que já é um `agui.Runner` — o que qualquer transporte precisa.

```go
ag, err := agui.New(ctx, m,
	agui.WithSystemPrompt("Você ajuda a planejar viagens."),
	agui.WithTools(searchFlights, setTheme),
	agui.WithMaxIterations(6),
)
```

Ele recebe `ctx` porque os schemas das tools são resolvidos aí — e `Info()` pode
fazer I/O de rede, como faz uma tool MCP. Resolver na construção, e não a cada
requisição, é o que permite falhar na subida do processo em vez de no meio de
uma conversa.

Um `*Agent` é **seguro para uso concorrente** e não é mutado por requisição: as
tools de frontend são ligadas ao modelo com `WithTools`, que devolve uma
instância nova.

### Opções

| Opção | Efeito |
|---|---|
| `WithSystemPrompt(s)` | prompt de sistema; só é injetado se o cliente não mandou um `system` próprio |
| `WithTools(...)` | registra as tools; chamadas sucessivas acumulam |
| `WithMaxIterations(n)` | teto de voltas do laço por run. Padrão `DefaultMaxIterations` = 8, teto absoluto 64 |
| `WithLogger(l)` | `*slog.Logger` do agente (padrão `slog.Default()`) |
| `WithContextFormatter(f)` | troca a formatação do `context` da requisição (padrão `AppContextBlock`) |
| `NoContext()` | desliga o repasse do `context` da requisição ao modelo |
| `WithMessagePreprocessor(f)` | última transformação nas mensagens antes de cada chamada — truncar histórico, injetar few-shots, redigir dados sensíveis |
| `WithState(f)` | troca a construção do estado compartilhado do run |
| `WithHooks(h)` | observadores do run — ver [Observabilidade](#observabilidade) |
| `WithStepNames(model, tools)` | rótulos dos `STEP_STARTED`/`STEP_FINISHED` (padrões `"model"` e `"tools"`) |

Por que 8 iterações: cobre com folga o caso real (chamar uma tool de dados,
depois uma de apresentação, depois responder) e limita o prejuízo de um laço
patológico, em que o modelo insiste numa tool que sempre falha — cada volta é
uma chamada paga ao provedor.

### O chat model

O módulo `model` é a conveniência para os dois provedores comuns:

```go
m, err := model.FromEnv(ctx)                       // lê MODEL_PROVIDER, MODEL, ...
m, err := model.New(ctx, model.Config{             // ou explicitamente
	Provider: model.ProviderOpenAI,
	Model:    "gpt-4o-mini",
	APIKey:   os.Getenv("OPENAI_API_KEY"),
})
```

`Config.Validate` recusa na subida o que só falharia na primeira requisição —
que é o comportamento certo para um container com healthcheck. Quem já tem o
próprio `model.ToolCallingChatModel` não precisa deste módulo: `agui.New` aceita
qualquer implementação.

## Tools

Uma tool é um `tool.BaseTool` do Eino, e é a **própria interface** que decide
quem executa o quê:

| A tool implementa | Quem executa | O que acontece |
|---|---|---|
| `Info` + `InvokableRun` | o **servidor** | roda aqui, o resultado volta ao modelo, o laço continua |
| só `Info` | o **cliente** | o run termina e devolve o controle ao frontend |

O segundo caso é o mesmo caminho das tools que chegam em `RunAgentInput.tools`
(as registradas por `useFrontendTool`/`useComponent` no React). A diferença é
que declarar a tool no servidor permite anunciar uma capacidade do frontend sem
depender de o React tê-la registrado.

### Schema inferido — `NewTool`

```go
type FlightSearch struct {
	Origin      string `json:"origin"  jsonschema:"required,description=Código IATA de origem"`
	Destination string `json:"destination" jsonschema:"required,description=Código IATA de destino"`
}

agui.NewTool("search_flights", "Busca voos entre dois aeroportos.",
	func(ctx context.Context, in FlightSearch) ([]Flight, error) {
		return search(in.Origin, in.Destination), nil
	})
```

Entrada e saída são tipadas; a serialização é da biblioteca. O schema sai sem
`$ref`/`$defs`/`$schema`, que é a forma que os provedores aceitam sem
saneamento.

### Schema escrito à mão — `NewRawTool`

Quando o texto das descrições faz parte do prompt e precisa sobreviver byte a
byte, ou quando a inferência não expressa o schema:

```go
agui.NewRawTool("display_flights", descricao, json.RawMessage(schemaJSON),
	func(ctx context.Context, args string) (string, error) { ... })
```

`params` é `json.RawMessage` e não `string` de propósito: com três strings
adjacentes o compilador não pegaria uma troca de ordem entre nome, descrição e
schema. Para quem monta o `*schema.ToolInfo` programaticamente há ainda
`NewToolFromInfo`.

### Tool de cliente — `NewClientTool`

Anuncia o schema ao modelo mas não executa: ao ver a chamada, o run termina e o
controle volta ao frontend.

```go
agui.NewClientTool("show_chart", "Renderiza um gráfico na tela.",
	json.RawMessage(`{"type":"object","properties":{"titulo":{"type":"string"}}}`))
```

### Erros de construção

Os construtores não devolvem `error` — o erro fica retido na tool e é colhido
por `New`, agregado e nomeado. É o que permite escrever a lista de tools sem
três linhas de cerimônia entre cada uma:

```go
ag, err := agui.New(ctx, m, agui.WithTools(
	agui.NewTool("a", "...", fnA),
	agui.NewRawTool("b", "...", schemaB, fnB),
))
// err: tool "b": invalid parameter schema: ...
```

O motivo é de linguagem: em Go, uma expressão de dois valores só pode ser a
lista **completa** de argumentos de uma chamada — se `NewTool` devolvesse
`(Tool, error)`, `WithTools(NewTool(...), NewRawTool(...))` nem compilaria.

## Dentro de uma tool

A função de uma tool recebe só `(ctx, entrada)` — é assim que o Eino a invoca.
Todo o resto do run chega pelo contexto:

```go
func changeTheme(ctx context.Context, in ThemeArgs) (string, error) {
	rc := agui.MustFromContext(ctx)
	rc.State().Set("/theme", in.Theme)   // aplica E emite o STATE_DELTA
	return `{"ok":true}`, nil
}
```

O `*RunContext` dá acesso a:

| Método | Para quê |
|---|---|
| `Emitter()` | emitir eventos no fio (nunca eventos terminais: quem encerra o run é o laço) |
| `State()` | o estado compartilhado do run |
| `Input()` | o `RunAgentInput` cru — `forwardedProps`, `threadId`, contexto da aplicação |
| `Logger()` | `*slog.Logger` já anotado com thread e run |
| `ThreadID()` / `RunID()` | identificadores do run |
| `ToolCallID()` / `ToolName()` | qual chamada esta tool está atendendo |
| `Resume()` / `ResumeFor(id)` | dados de retomada — ver [Human-in-the-loop](#human-in-the-loop) |

Use `FromContext` (que devolve `ok`) em vez de `MustFromContext` quando a tool
precisar ser chamável direto num teste, fora de um run.

## Estado compartilhado

`State` é um documento JSON livre espelhado no `agent.state` do cliente. O
contrato: **a operação aplicada localmente é exatamente a que vai no fio**. Um
cliente que parta do `STATE_SNAPSHOT` e aplique os `STATE_DELTA` em ordem
reconstrói o documento do servidor. Não existe o modo "mutei e esqueci de
emitir".

```go
st.Set("/todos/0/feito", true)   // aplica + emite
st.Remove("/rascunho")           // idem
st.Apply(ops)                    // lote atômico: ou tudo, ou nada
v, ok := st.Get("/theme")         // leitura por JSON Pointer (RFC 6901)
snap := st.Snapshot()            // cópia profunda
```

`Set` usa `add` e não `replace` porque, na RFC 6902, `add` substitui um membro
que já existe e cria um que não existe — funciona nos dois casos. Um caminho
cujo **pai** não existe é erro; crie o pai antes. `State` é seguro para uso
concorrente.

Para semear o estado com um valor próprio, use `WithState`:

```go
agui.WithState(func(ctx context.Context, in *agui.RunAgentInput) (*agui.State, error) {
	s := agui.NewState(map[string]any{"todos": []any{}})
	return s, s.Apply(seedDoCliente(in.State))
})
```

O padrão semeia com `RunAgentInput.state` e devolve intacto — o `agent.state` do
cliente não pode regredir só porque passou por aqui.

## Human-in-the-loop

Uma tool pausa o run devolvendo `agui.Interrupt`. O cliente responde em
`RunAgentInput.resume`, e na passagem seguinte a tool recupera a resposta:

```go
func approvePurchase(ctx context.Context, in Purchase) (string, error) {
	rc := agui.MustFromContext(ctx)
	if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
		return process(in, payload)      // o humano respondeu
	}
	return "", agui.Interrupt(agui.InterruptRequest{
		Reason:  "needs_approval",
		Message: "Confirmar a compra?",
	})
}
```

`ResumeFor` só considera entradas com status `resolved`: uma decisão
`cancelled` não é um valor de retomada, e devolvê-la como se fosse faria a tool
executar justamente o que o usuário recusou — use `Resume()` para inspecionar os
cancelamentos.

Não há store no servidor, e isso é deliberado: o AG-UI reenvia o histórico
inteiro a cada requisição, então a chamada pendente **chega** na conversa. Um
store em memória não acrescentaria nada e passaria a estar errado assim que
houvesse mais de uma réplica.

> Por que um sinal próprio e não o `tool.Interrupt` do Eino: a metade de
> sinalização dele funcionaria, mas a de **retomada** não — `GetInterruptState`
> e `GetResumeContext` leem uma chave de contexto não exportada, populada apenas
> pelo runner de grafo do pacote `compose`. Como este laço invoca as tools
> diretamente (que é o que permite tratar tools que o servidor não executa), não
> há como preencher essa chave. O transporte da pausa é, então, o do próprio
> AG-UI, que já tem `Interrupt` e `ResumeEntry` desenhados para isto.

## Renderizando UI: A2UI

O subpacote `a2ui` monta as operações **A2UI v0.9** que uma tool de backend
devolve como resultado — é o port em Go de `copilotkit/a2ui.py`. Não há nada de
modelo aqui: A2UI é um formato JSON, e o que o Python fazia em 246 linhas (a
maior parte delas texto de prompt que o runtime TypeScript injeta sozinho) são
três construtores e um envelope.

O caminho no fio é este: a tool devolve a string produzida por `a2ui.Render`
como conteúdo do `TOOL_CALL_RESULT`; o `@copilotkit/a2ui-renderer` do frontend
reconhece a chave `a2ui_operations`, aplica as operações na superfície e
renderiza os componentes do seu catálogo React.

```go
func showFlights(ctx context.Context, in FlightSearch) (string, error) {
	voos := search(in.Origin, in.Destination)

	return a2ui.Render(
		a2ui.CreateSurface("voos", "copilotkit://app-dashboard-catalog"),
		// `component` e as demais chaves são os nomes e as props do SEU
		// catálogo React; a biblioteca não os interpreta.
		a2ui.UpdateComponents("voos", []a2ui.Component{
			{"id": "root", "component": "Card", "children": []any{"lista"}},
			{"id": "lista", "component": "FlightList", "itemsPath": "/flights"},
		}),
		a2ui.UpdateDataModel("voos", "/", map[string]any{"flights": voos}),
	)
}
```

| Construtor | Operação | Padrão quando vazio |
|---|---|---|
| `CreateSurface(surfaceID, catalogID)` | `createSurface` | `catalogID` cai em `a2ui.BasicCatalogID` |
| `UpdateComponents(surfaceID, comps)` | `updateComponents` — o schema declarativo da superfície | uma lista nil sai como `[]` |
| `UpdateDataModel(surfaceID, path, v)` | `updateDataModel` — escreve `v` no caminho do modelo de dados | `path` vazio vira `"/"` (a raiz) |
| `Render(ops...)` | embrulha tudo em `{"a2ui_operations": [...]}` e serializa | — |

Cada operação carrega `version: "v0.9"` (a constante `a2ui.Version`) e
exatamente uma chave de comando.

Três detalhes que o pacote garante, e que custam caro quando faltam:

- **A chave do envelope é a detecção.** Errar `a2ui_operations`
  (`a2ui.OperationsKey`) faz o resultado da tool aparecer como texto cru no
  chat, em vez de virar uma superfície.
- **Lista de componentes vazia sai como `[]`, nunca `null`.** O renderer itera
  sobre o array, e um nulo derrubaria a aplicação da operação.
- **`Render` devolve erro em vez de engolir.** O `value` de `UpdateDataModel`
  vem de fora — de argumentos que o modelo produziu —, e um resultado de tool
  truncado renderizaria uma superfície vazia sem nenhum sinal do que houve.

### Não confundir com `generate_a2ui`

São dois caminhos distintos, e só um passa por este pacote:

| | Schema **fixo** (este pacote) | Schema **dinâmico** (`generate_a2ui`) |
|---|---|---|
| Quem desenha a superfície | o servidor, que já conhece o layout | o próprio modelo |
| De onde vem a tool | você a declara com `agui.NewTool` | injetada pelo A2UIMiddleware do runtime |
| Como ela chega | é sua, roda no servidor | como tool de **frontend** em `RunAgentInput.tools` |
| Quem executa | o servidor, devolvendo `a2ui.Render(...)` | o cliente — nunca é executada aqui |

Nos dois casos vale a armadilha do `context` da requisição: é por
`RunAgentInput.context` que o runtime entrega o **catálogo de componentes** ao
agente. Sem repassá-lo ao modelo, ele inventa nomes que o catálogo React não
conhece e o frontend renderiza "Unknown component" — por isso o
`AppContextBlock` é o formatador padrão, e por isso `NoContext()` não deve ser
usado num agente que renderiza A2UI.

## Observabilidade

`WithHooks` existe porque um logger não responde as perguntas que se faz em
produção: quanto custou, quais tools rodaram, quanto tempo cada turno levou.

```go
agui.WithHooks(agui.Hooks{
	OnRunStart:   func(ctx context.Context, in *agui.RunAgentInput) { ... },
	OnModelTurn:  func(ctx context.Context, t agui.TurnInfo) {
		if t.Usage != nil {
			log.Printf("turno %d: %d tokens em %s",
				t.Iteration, t.Usage.TotalTokens, t.Duration)
		}
	},
	OnToolResult: func(ctx context.Context, t agui.ToolRunInfo) { ... }, // nome, args, duração, erro, interrompida
	OnRunEnd:     func(ctx context.Context, r agui.RunInfo) { ... },     // iterações, duração, "success"|"interrupt"|"error"
})
```

Todos os campos são opcionais.

---

# Servidor

## Setup de um servidor Go com Gin

O núcleo fala `http.Handler` e não conhece framework nenhum. O módulo `ginx` é a
camada de conveniência para quem usa Gin — e é um módulo **à parte** justamente
para que o Gin não entre no `go.mod` de quem usa `chi`, `echo` ou `net/http`
puro.

### 1. O projeto

```bash
mkdir meu-agente && cd meu-agente
go mod init github.com/voce/meu-agente

go get github.com/ricardogayer/go-agui@v0.2.0
go get github.com/ricardogayer/go-agui/ginx@v0.2.0
go get github.com/ricardogayer/go-agui/model@v0.2.0
```

### 2. O `main.go`

```go
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/ginx"
	"github.com/ricardogayer/go-agui/model"
)

func main() {
	// Um SIGINT/SIGTERM cancela este contexto, que por sua vez dispara o
	// desligamento gracioso do servidor.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	m := agui.Must(model.FromEnv(ctx))

	ag := agui.Must(agui.New(ctx, m,
		agui.WithSystemPrompt("Você é um assistente útil e direto."),
	))

	// agui.Addr lê HOST e PORT, caindo em 0.0.0.0:8000.
	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "meu-agente"))
}
```

`agui.Must` desembrulha `(valor, erro)` e entra em pânico se houver erro — existe
para o `main()`, onde um erro de construção não tem para onde ir e um
`if err != nil { log.Fatal(err) }` por linha só adiciona ruído. Não use fora da
inicialização.

### 3. Subir e verificar

```bash
export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
go run .

curl -s localhost:8000/health
# {"agent":{"name":"meu-agente"},"status":"ok"}

curl -N -X POST localhost:8000 -H 'content-type: application/json' \
  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"oi"}]}'
```

`ListenAndServe` bloqueia até o contexto ser cancelado; aí drena as conexões em
voo (10s por padrão) e devolve. Um `Ctrl-C` no terminal exercita o caminho
inteiro.

### 4. O que `ginx.ListenAndServe` faz por você

Uma chamada monta esta pilha:

```
gin.Engine (modo release)
 └─ gin.Recovery()
 └─ [CORS, se WithCORS()]
 └─ POST /       → agui.Handler(ag)   ← SSE
 └─ GET  /health → agui.Health(nome)
agui.NormalizeHealthPath(...)          ← //health → /health
http.Server sem WriteTimeout, com ReadHeaderTimeout=15s
 └─ Shutdown gracioso quando ctx é cancelado
```

E cada requisição, dentro do handler: decodificação do `RunAgentInput`, geração
de `threadId`/`runId` quando vierem vazios, os cabeçalhos SSE (incluindo
`X-Accel-Buffering: no`), a remoção do prazo de escrita, o `RUN_STARTED`, o
contexto do run ligado à desconexão do cliente **e** ao desligamento do
processo, a recuperação de pânico virando `RUN_ERROR`, e a garantia de que sai
exatamente um evento terminal.

### 5. Opções do servidor

```go
ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "meu-agente",
	ginx.WithCORS(),                                   // CORS permissivo (ver abaixo)
	ginx.WithHandlerOptions(
		agui.WithHandlerLogger(logger),                // logger do handler e do writer SSE
		agui.WithShutdownContext(ctx),                 // SIGTERM cancela os streams em voo
		agui.WithMaxBodyBytes(8<<20),                  // 413 acima de 8 MiB (padrão: sem limite)
		agui.WithOnDropped(func(ctx context.Context, ev agui.Event, err error) {
			metrics.EventoDescartado(ev.Type())        // padrão: log em ERROR
		}),
	),
	ginx.WithServeOptions(
		agui.WithServeLogger(logger),
		agui.WithShutdownTimeout(30*time.Second),      // padrão 10s
	),
)
```

`WithCORS` é **desligado por padrão**, de propósito: o runtime CopilotKit chama
o backend server-side, então não há origem cruzada no caminho normal. Ligue
quando for depurar o backend direto do navegador.

### 6. Montando num roteador Gin que você já tem

Quando o agente é uma rota entre outras — porque o mesmo binário serve uma API
REST, métricas, ou vários agentes:

```go
r := gin.New()
r.Use(gin.Recovery(), meuLogger(), minhaAutenticacao())

// As duas rotas do contrato, na raiz:
ginx.Register(r, ag, "meu-agente")

// Ou num prefixo — vira POST /agents/viagens/ e GET /agents/viagens/health
// (o POST fica com a barra final; o Gin redireciona /agents/viagens para ela):
ginx.Register(r.Group("/agents/viagens"), agViagens, "viagens")
ginx.Register(r.Group("/agents/suporte"), agSuporte, "suporte")

r.GET("/metrics", metricsHandler)

// NormalizeHealthPath cuida do `//health` das sondas legadas; ele não pode ser
// uma rota (o roteador do Gin recusa um caminho com barra dobrada).
log.Fatal(agui.ListenAndServe(ctx, agui.Addr(8000), agui.NormalizeHealthPath(r)))
```

Se quiser só o handler, sem as convenções de rota:

```go
r.POST("/chat", ginx.Handler(ag))
r.GET("/chat/health", ginx.Health("meu-agente"))
```

`ginx.Handler` é `gin.WrapH(agui.Handler(...))`. O `gin.ResponseWriter`
implementa `http.Flusher` e `http.ResponseWriter`, então o streaming SSE
atravessa o wrap sem perda. Se preferir não depender do `ginx`, `r.POST("/",
gin.WrapH(agui.Handler(ag)))` faz a mesma coisa.

Para um `*gin.Engine` pronto (release, `Recovery`, as duas rotas, CORS opcional)
que você mesmo serve, use `ginx.Engine(ag, "meu-agente", opts...)` — lembrando
de embrulhá-lo em `agui.NormalizeHealthPath` se as sondas usarem `//health`.

### 7. Middleware e SSE

Um middleware que **embrulha o `ResponseWriter`** (compressão, cache, gravação
do corpo) quebra o streaming: ou segura os quadros num buffer, ou esconde o
`Flusher` do handler. Não use `gzip` nem `gin.Logger` com writer customizado na
rota do agente. Autenticação, rate limit, tracing e recuperação de pânico são
seguros — nenhum toca no corpo da resposta.

Atrás de um NGINX, o `X-Accel-Buffering: no` que o handler já envia é o que
evita que o proxy transforme o stream em lote. Em outros proxies (Cloudflare,
ALB, Envoy) confira o buffering de resposta e desligue-o para a rota do agente.

### 8. Produção

- **Não configure `WriteTimeout`.** É a recomendação padrão de qualquer guia de
  produção e cortaria o stream SSE no meio da conversa.
  `agui.ListenAndServe` não o define de propósito, e mantém
  `ReadHeaderTimeout: 15s`, que é o que protege contra conexões ociosas.
- **Ligue o `WithShutdownContext`** ao mesmo contexto de sinal, para que um
  SIGTERM cancele os runs em voo em vez de matá-los no meio.
- **Defina `WithMaxBodyBytes`** num servidor exposto. O AG-UI reenvia o
  histórico inteiro a cada requisição, então o teto certo depende da aplicação
  — mas sem nenhum, qualquer cliente pode fazer o decode alocar sem limite.
- **O healthcheck testa o endpoint de verdade** — ver
  [Healthcheck em container distroless](#healthcheck-em-container-distroless).
- **Escale horizontalmente à vontade.** Não há estado de sessão no servidor: o
  AG-UI reenvia o histórico inteiro a cada requisição.

## Sem Gin: net/http puro

O núcleo sozinho já monta o contrato inteiro:

```go
log.Fatal(agui.ListenAndServe(ctx, agui.Addr(8000), agui.Mux(ag, "meu-agente")))
```

`agui.Mux` devolve um `http.Handler` com `POST /`, `GET /health` e a
normalização do `//health`. Num roteador seu (`chi`, `echo`, `http.ServeMux`),
monte `agui.Handler(ag)` e `agui.Health(nome)` onde quiser.

## Níveis de controle

| Nível | API | Para quem |
|---|---|---|
| **0** | `ginx.ListenAndServe(ctx, addr, ag, nome)` | sobe o servidor inteiro |
| **1** | `ginx.Register(r, ag, nome)` / `agui.Handler(ag)` / `agui.Mux(...)` | você já tem um roteador |
| **2** | implementar `agui.Runner` | você quer o próprio laço, mantendo SSE e ciclo de vida |
| **3** | `Emitter`, `Conversation`, `ClassifyToolCalls` | você quer controlar cada evento no fio |

Nenhum nível é uma parede: dá para descer um degrau sem reescrever o resto. No
nível 2, um `Runner` é uma interface de um método —
`Run(ctx, *Emitter, *RunAgentInput) error` — e `RunnerFunc` adapta uma função a
ela. O `RUN_STARTED` já foi emitido quando `Run` é chamado; devolver erro basta
para encerrar o run, porque o handler emite o `RUN_ERROR` se nenhum terminal
tiver saído.

---

## Testes

O pacote `aguitest` traz um `FakeModel` que implementa
`model.ToolCallingChatModel` **em processo**, sem servidor HTTP falso — o que se
testa é o laço e a sequência de eventos, não o adaptador HTTP de terceiro.

```go
m := aguitest.NewFakeModel(
	aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "c1", Name: "add", Args: `{"a":2,"b":3}`}}},
	aguitest.Turn{Text: "That is 5."},
)
ag := agui.Must(agui.New(ctx, m, agui.WithTools(addTool())))

evs := aguitest.Run(t, ag, aguitest.Input("what is 2+3?"))
aguitest.RequireBalanced(t, evs)
aguitest.RequireContains(t, evs, agui.EventToolCallResult, agui.EventRunFinished)
```

`RequireBalanced` é a asserção que ninguém escreve sozinho e que mais paga: todo
`START` tem `END`, nenhum bloco se sobrepõe, e o run tem exatamente **um** evento
terminal, no fim. Há ainda `RequireSequence` (a sequência exata),
`Collect`/`ParseSSE` (para exercitar um `http.Handler`), `First`/`All`/`Types`
e `ConcatenatedText`.

Do outro lado, `FakeModel.Calls()` devolve as mensagens que o modelo recebeu em
cada chamada, e `BoundTools()` os conjuntos de tools ligados — é o que permite
afirmar sobre o que o agente **montou** (o system prompt foi injetado? o bloco de
contexto está lá? o `RepairToolPairs` reordenou o histórico?), e não só sobre o
que ele emitiu.

## Armadilhas do protocolo que a biblioteca já resolve

Este é o conhecimento caro — o que custou depuração contra o fio e que você não
precisa redescobrir:

- **Um evento inválido derruba o stream inteiro.** O `@ag-ui/client` faz
  `EventSchemas.parse` e chama `eventSubject.error` no catch. A biblioteca
  descarta o evento recusado (`ErrEventDropped`) e mantém o stream vivo, para que
  um `RUN_ERROR` ainda alcance o cliente.
- **`toolCallId` vazio é recusado pelo SDK.** No streaming, id e nome só chegam
  no primeiro chunk. Os fragmentos de argumento ficam em buffer até os dois
  serem conhecidos.
- **Não emita `TOOL_CALL_*` duas vezes.** Quem faz streaming das chamadas não
  pode reemiti-las depois.
- **`MESSAGES_SNAPSHOT` é obrigatório.** Sem ele a lista de mensagens do
  frontend fica sem reconciliação após tool calls. E os ids das mensagens
  precisam ser estáveis: se cada snapshot inventasse ids novos, o cliente veria
  a conversa inteira como mensagens novas a cada turno — daí a `Conversation`
  preservar o id AG-UI original de cada mensagem.
- **A ordem das respostas de tool importa.** Quando o controle volta ao cliente,
  o histórico que ele reenvia traz as respostas fora de posição, e a OpenAI
  recusa com 400. `RepairToolPairs` conserta o que vai ao modelo — sem tocar no
  `MESSAGES_SNAPSHOT`, que continua sendo o histórico real.
- **O `context` da requisição precisa chegar ao modelo.** É por ele que o
  runtime entrega coisas como o catálogo de componentes; sem repassar, o modelo
  inventa nomes e a UI renderiza "Unknown component".
- **`X-Accel-Buffering: no`.** Sem isso um NGINX à frente segura os quadros e o
  streaming vira lote.
- **Zere o prazo de escrita.** Um `http.Server` com `WriteTimeout` — a
  recomendação padrão de produção — cortaria o stream SSE no meio.
- **O cliente que desconecta ainda custa dinheiro.** A primeira escrita que
  falha cancela o contexto do run, o que aborta o stream do modelo em voo em vez
  de continuar gerando — e pagando — contra quem já foi embora.

## Variáveis de ambiente

Lidas por `model.ConfigFromEnv` e `agui.Addr`. São as mesmas que os backends
Python liam, para que um `.env` existente continue valendo.

| Variável | Para que serve |
|---|---|
| `MODEL_PROVIDER` | `openai` (padrão) ou `ollama` |
| `MODEL` | id do modelo |
| `OPENAI_API_KEY` | credencial, obrigatória para `openai` |
| `OPENAI_BASE_URL` | endpoint alternativo (gateway compatível) |
| `OPENAI_REASONING_EFFORT` | `none` é **obrigatório** nos modelos de raciocínio da OpenAI: eles recusam function tools com o esforço padrão, e um agente AG-UI passa tudo por function tools |
| `OLLAMA_BASE_URL` | endpoint do Ollama (padrão `http://localhost:11434`) |
| `HOST` / `PORT` | endereço de escuta |

## Healthcheck em container distroless

Uma imagem distroless não tem shell nem curl, então o próprio binário vira a
sonda. A biblioteca **não** registra flags — isso quebraria o `FlagSet` de quem
a usa e o `go test`:

```go
var probe = flag.Bool("healthcheck", false, "consulta o /health local e sai 0/1")

func main() {
	flag.Parse()
	if *probe {
		if err := agui.CheckHealth("http://127.0.0.1:8000/health"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// ...
}
```

```dockerfile
HEALTHCHECK --interval=10s CMD ["/app/servidor", "-healthcheck"]
```

Isto testa o endpoint de verdade, diferente de um healthcheck que só abre um
socket TCP e reporta "healthy" com o agente quebrado.

## Desenvolvimento

O repositório tem quatro módulos amarrados por um `go.work`. O `Makefile` roda
os alvos em todos:

```bash
make build      # compila todos os módulos
make test       # roda os testes de todos
make lint       # gofmt + go vet
make tidy       # go mod tidy em todos
make quickstart # sobe o exemplo mínimo em :8000
```

Exemplos:

| Exemplo | Mostra |
|---|---|
| `examples/quickstart` | o menor agente possível: chat com streaming, sem tools |
| `examples/backend-tools` | schema inferido, estado compartilhado, tool de cliente declarada no servidor e hooks de uso de tokens |

## Licença

MIT.

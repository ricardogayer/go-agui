// Package agui exposes an agent that speaks the AG-UI protocol over SSE, with
// the model↔tools loop built on top of CloudWeGo Eino.
//
// The contract is the same one the Python adapter `ag_ui_langgraph` sets up: a
// `POST /` route that receives a RunAgentInput as JSON and returns
// `text/event-stream` with AG-UI events, plus a `GET /health`. Any runtime that
// speaks AG-UI — CopilotKit, for example, through a generic HttpAgent — talks
// to this server without knowing that there is Go and Eino on the other side.
//
// # The smallest possible server
//
//	func main() {
//		ctx := context.Background()
//		m := agui.Must(model.FromEnv(ctx))
//		ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt("You are a helpful assistant.")))
//		log.Fatal(agui.ListenAndServe(ctx, ":8000", agui.Mux(ag, "my_agent")))
//	}
//
// # Tools: who executes what
//
// A Tool is an Eino [tool.BaseTool], and the interface itself decides the path
// a call takes:
//
//   - Implements `InvokableRun` (that is, it is a [tool.InvokableTool]) → the
//     SERVER executes the tool and hands the result back to the model, which
//     continues the loop.
//   - Implements only `Info` → the schema is announced to the model, but the
//     server does not execute it: upon seeing the call, it ends the run and
//     hands control back to the client, which executes and resends the history.
//
// The second case is the same path taken by the tools that arrive in
// `RunAgentInput.tools` (the ones registered by `useFrontendTool`/`useComponent`
// in React). The difference is that declaring a [tool.BaseTool] lets you
// announce a client tool from the server.
//
// # Per-request data inside a tool
//
// A tool's function receives only `(ctx, input)`. Everything else about the run
// — emitter, shared state, id of the call in progress, resume data — arrives
// through the context, via [FromContext]:
//
//	func switchTheme(ctx context.Context, in ThemeArgs) (string, error) {
//		agui.MustFromContext(ctx).State().Set("/theme", in.Theme)
//		return `{"ok":true}`, nil
//	}
//
// # Concurrency safety
//
// An *Agent is safe for concurrent use and is not mutated per request: the
// frontend tools are bound to the model with WithTools, which returns a new
// instance. An *Emitter is also safe (writes are serialized), precisely because
// tools reach it through the context and may emit from another goroutine. A
// *Conversation is per run and must not be shared.
//
// PT: O pacote agui expõe um agente que fala o protocolo AG-UI sobre SSE, com o
// laço modelo↔tools construído sobre o CloudWeGo Eino.
//
// O contrato é o mesmo que o adaptador Python `ag_ui_langgraph` monta: uma rota
// `POST /` que recebe um RunAgentInput em JSON e devolve `text/event-stream`
// com eventos AG-UI, mais um `GET /health`. Qualquer runtime que fale AG-UI —
// o CopilotKit, por exemplo, através de um HttpAgent genérico — conversa com
// este servidor sem saber que do outro lado há Go e Eino.
//
// # O menor servidor possível
//
// Ver o exemplo acima.
//
// # Tools: quem executa o quê
//
// Uma Tool é um [tool.BaseTool] do Eino, e é a própria interface que decide o
// caminho que a chamada percorre:
//
//   - Implementa `InvokableRun` (isto é, é um [tool.InvokableTool]) → o
//     SERVIDOR executa a tool e devolve o resultado ao modelo, que segue o laço.
//   - Implementa apenas `Info` → o schema é anunciado ao modelo, mas o servidor
//     não executa: ao ver a chamada, ele encerra o run e devolve o controle ao
//     cliente, que executa e reenvia o histórico.
//
// O segundo caso é o mesmo caminho das tools que chegam em
// `RunAgentInput.tools` (as registradas por `useFrontendTool`/`useComponent` no
// React). A diferença é que declarar uma [tool.BaseTool] permite anunciar uma
// tool de cliente a partir do servidor.
//
// # Dados por requisição dentro de uma tool
//
// A função de uma tool recebe apenas `(ctx, entrada)`. Todo o resto do run —
// emitter, estado compartilhado, id da chamada em curso, dados de retomada —
// chega pelo contexto, via [FromContext] (ver o exemplo `switchTheme` acima).
//
// # Segurança para uso concorrente
//
// Um *Agent é seguro para uso concorrente e não é mutado por requisição: as
// tools de frontend são ligadas ao modelo com WithTools, que devolve uma
// instância nova. Um *Emitter também é seguro (as escritas são serializadas),
// justamente porque as tools o alcançam pelo contexto e podem emitir de outra
// goroutine. Uma *Conversation é por run e não deve ser compartilhada.
package agui

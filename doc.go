// Package agui expõe um agente que fala o protocolo AG-UI sobre SSE, com o
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
//	func main() {
//		ctx := context.Background()
//		m := agui.Must(model.FromEnv(ctx))
//		ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt("Você é um assistente útil.")))
//		log.Fatal(agui.ListenAndServe(ctx, ":8000", agui.Mux(ag, "meu_agente")))
//	}
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
// chega pelo contexto, via [FromContext]:
//
//	func trocarTema(ctx context.Context, in TemaArgs) (string, error) {
//		agui.MustFromContext(ctx).State().Set("/tema", in.Tema)
//		return `{"ok":true}`, nil
//	}
//
// # Segurança para uso concorrente
//
// Um *Agent é seguro para uso concorrente e não é mutado por requisição: as
// tools de frontend são ligadas ao modelo com WithTools, que devolve uma
// instância nova. Um *Emitter também é seguro (as escritas são serializadas),
// justamente porque as tools o alcançam pelo contexto e podem emitir de outra
// goroutine. Uma *Conversation é por run e não deve ser compartilhada.
package agui

// Command backend-tools mostra os três tipos de tool que um agente AG-UI
// distingue, e o estado compartilhado.
//
//   - buscar_voos  — tool de servidor com schema INFERIDO das struct tags.
//   - definir_tema — tool de servidor que escreve no estado compartilhado, o que
//     faz o `agent.state` do cliente mudar sozinho.
//   - mostrar_grafico — tool de CLIENTE declarada no servidor: o modelo a vê,
//     mas quem executa é o React; o run termina devolvendo o controle.
//
// Rodar:
//
//	export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
//	go run ./examples/backend-tools
package main

import (
	"context"
	"encoding/json"
	"log"
	"os/signal"
	"syscall"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/ginx"
	"github.com/ricardogayer/go-agui/model"
)

// BuscaVoos é a entrada da tool. O JSON Schema anunciado ao modelo sai daqui —
// não há schema escrito à mão.
type BuscaVoos struct {
	Origem  string `json:"origem"  jsonschema:"required,description=Código IATA de origem (ex.: GRU)"`
	Destino string `json:"destino" jsonschema:"required,description=Código IATA de destino (ex.: JFK)"`
}

type Voo struct {
	Companhia string `json:"companhia"`
	Numero    string `json:"numero"`
	Partida   string `json:"partida"`
	Preco     string `json:"preco"`
}

func buscarVoos(_ context.Context, in BuscaVoos) ([]Voo, error) {
	return []Voo{
		{Companhia: "LATAM", Numero: "LA8084", Partida: "22:40", Preco: "R$ 3.120"},
		{Companhia: "American", Numero: "AA974", Partida: "21:55", Preco: "R$ 2.870"},
	}, nil
}

type DefinirTema struct {
	Tema string `json:"tema" jsonschema:"required,description=claro ou escuro"`
}

// definirTema escreve no estado compartilhado. O State aplica o patch E emite o
// STATE_DELTA na mesma operação, então o cliente nunca fica dessincronizado.
func definirTema(ctx context.Context, in DefinirTema) (string, error) {
	if err := agui.MustFromContext(ctx).State().Set("/tema", in.Tema); err != nil {
		return "", err
	}
	return `{"ok":true}`, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	m := agui.Must(model.FromEnv(ctx))

	ag := agui.Must(agui.New(ctx, m,
		agui.WithSystemPrompt(
			"Você ajuda a planejar viagens. Para voos, chame buscar_voos e depois "+
				"apresente os resultados. Para gráficos, chame mostrar_grafico."),
		agui.WithTools(
			agui.NewTool("buscar_voos", "Busca voos entre dois aeroportos.", buscarVoos),
			agui.NewTool("definir_tema", "Muda o tema da interface.", definirTema),
			// Só anuncia: quem executa é o cliente.
			agui.NewClientTool("mostrar_grafico", "Renderiza um gráfico na tela.",
				json.RawMessage(`{
					"type": "object",
					"properties": {
						"titulo": {"type": "string"},
						"pontos": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"rotulo": {"type": "string"},
									"valor":  {"type": "number"}
								}
							}
						}
					},
					"required": ["titulo", "pontos"]
				}`)),
		),
		agui.WithHooks(agui.Hooks{
			OnModelTurn: func(_ context.Context, t agui.TurnInfo) {
				if t.Usage != nil {
					log.Printf("turno %d: %d tokens em %s",
						t.Iteration, t.Usage.TotalTokens, t.Duration.Round(1e6))
				}
			},
		}),
	))

	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "backend-tools"))
}

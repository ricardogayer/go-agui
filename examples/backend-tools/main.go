// Command backend-tools shows the three kinds of tool an AG-UI agent
// distinguishes, plus shared state.
//
//   - search_flights  — server tool with the schema INFERRED from struct tags.
//   - set_theme — server tool that writes to the shared state, which makes the
//     client's `agent.state` change on its own.
//   - show_chart — CLIENT tool declared on the server: the model sees it, but
//     React is who executes it; the run ends handing back control.
//
// Run:
//
//	export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
//	go run ./examples/backend-tools
//
// PT: Command backend-tools mostra os três tipos de tool que um agente AG-UI
// distingue, e o estado compartilhado.
//
//   - search_flights  — tool de servidor com schema INFERIDO das struct tags.
//   - set_theme — tool de servidor que escreve no estado compartilhado, o que
//     faz o `agent.state` do cliente mudar sozinho.
//   - show_chart — tool de CLIENTE declarada no servidor: o modelo a vê,
//     mas quem executa é o React; o run termina devolvendo o controle.
//
// O exemplo acima mostra como rodar.
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

// FlightSearch is the tool's input. The JSON Schema announced to the model
// comes from here — there is no hand-written schema.
//
// PT: FlightSearch é a entrada da tool. O JSON Schema anunciado ao modelo sai daqui —
// não há schema escrito à mão.
type FlightSearch struct {
	Origin      string `json:"origin"  jsonschema:"required,description=Origin IATA code (e.g. GRU)"`
	Destination string `json:"destination" jsonschema:"required,description=Destination IATA code (e.g. JFK)"`
}

type Flight struct {
	Airline   string `json:"airline"`
	Number    string `json:"number"`
	Departure string `json:"departure"`
	Price     string `json:"price"`
}

func searchFlights(_ context.Context, in FlightSearch) ([]Flight, error) {
	return []Flight{
		{Airline: "LATAM", Number: "LA8084", Departure: "22:40", Price: "$620"},
		{Airline: "American", Number: "AA974", Departure: "21:55", Price: "$575"},
	}, nil
}

type SetTheme struct {
	Theme string `json:"theme" jsonschema:"required,description=light or dark"`
}

// setTheme writes to the shared state. State applies the patch AND emits the
// STATE_DELTA in the same operation, so the client never gets out of sync.
//
// PT: setTheme escreve no estado compartilhado. O State aplica o patch E emite o
// STATE_DELTA na mesma operação, então o cliente nunca fica dessincronizado.
func setTheme(ctx context.Context, in SetTheme) (string, error) {
	if err := agui.MustFromContext(ctx).State().Set("/theme", in.Theme); err != nil {
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
			"You help plan trips. For flights, call search_flights and then "+
				"present the results. For charts, call show_chart."),
		agui.WithTools(
			agui.NewTool("search_flights", "Searches flights between two airports.", searchFlights),
			agui.NewTool("set_theme", "Changes the UI theme.", setTheme),
			// Only announces it: the client is who executes it.
			//
			// PT: Só anuncia: quem executa é o cliente.
			agui.NewClientTool("show_chart", "Renders a chart on screen.",
				json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {"type": "string"},
						"points": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"label": {"type": "string"},
									"value":  {"type": "number"}
								}
							}
						}
					},
					"required": ["title", "points"]
				}`)),
		),
		agui.WithHooks(agui.Hooks{
			OnModelTurn: func(_ context.Context, t agui.TurnInfo) {
				if t.Usage != nil {
					log.Printf("turn %d: %d tokens in %s",
						t.Iteration, t.Usage.TotalTokens, t.Duration.Round(1e6))
				}
			},
		}),
	))

	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "backend-tools"))
}

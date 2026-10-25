# go-agui

*Read this in [Portuguese (pt-BR)](README_PT.md).*

**A Go library implementing the [AG-UI 1.0](https://docs.ag-ui.com/spec/1.0)
protocol (HTTP + SSE and HTTP + Protobuf), with the model↔tools loop built on
[CloudWeGo Eino](https://github.com/cloudwego/eino).**

You write the agent in Go — prompt, tools, state — and the library takes care of
the rest of the contract: the `POST /` that receives a `RunAgentInput`, the
`text/event-stream` going back, the event sequence, the healthcheck, cancellation
and graceful shutdown.

On the other side of the wire, any runtime that speaks AG-UI talks to this
server — [CopilotKit](https://copilotkit.ai), for instance, through a generic
`HttpAgent`. The frontend doesn't know (and doesn't need to know) there's Go here.

```go
func main() {
	ctx := context.Background()
	m := agui.Must(model.FromEnv(ctx))
	ag := agui.Must(agui.New(ctx, m, agui.WithSystemPrompt("You are a helpful assistant.")))
	log.Fatal(ginx.ListenAndServe(ctx, ":8000", ag, "my-agent"))
}
```

That's four lines for an agent with streaming, healthcheck, graceful shutdown,
cancellation on client disconnect and the complete AG-UI event sequence.

---

## Contents

- [What the library is](#what-the-library-is)
- [AG-UI 1.0 conformance](#ag-ui-10-conformance)
- [Installation](#installation)
- [Try it without deploying anything](#try-it-without-deploying-anything)
- **Usage**
  - [Creating the agent](#creating-the-agent)
  - [Tools](#tools)
  - [Inside a tool](#inside-a-tool)
  - [Shared state](#shared-state)
  - [Human-in-the-loop](#human-in-the-loop)
  - [Rendering UI: A2UI](#rendering-ui-a2ui)
  - [Observability](#observability)
- **Server**
  - [Setting up a Go server with Gin](#setting-up-a-go-server-with-gin) ← step by step
  - [Without Gin: plain net/http](#without-gin-plain-nethttp)
  - [Levels of control](#levels-of-control)
- [Testing](#testing)
- [Protocol pitfalls the library already solves](#protocol-pitfalls-the-library-already-solves)
- [Environment variables](#environment-variables)
- [Healthcheck in a distroless container](#healthcheck-in-a-distroless-container)
- [Development](#development)

---

## What the library is

The AG-UI contract it implements is the same one the Python adapter
`ag_ui_langgraph` assembles:

| Route | What it does |
|---|---|
| `POST /` | receives a `RunAgentInput` in JSON, responds with AG-UI events: `text/event-stream`, or `application/vnd.ag-ui.event+proto` when the client's `Accept` admits it |
| `GET /health` | healthcheck with `{"agent":{"name":...},"status":"ok"}` |
| `GET //health` | same, rewritten to `/health` (compatibility with probes written against the Python backend) |

A complete run emits, in order: `RUN_STARTED` (declaring `protocolVersion: "1.0"`); then, per iteration of the loop,
a `STEP_STARTED` / `STEP_FINISHED` pair (step `"model"`) enclosing the text
blocks (`TEXT_MESSAGE_START` / `_CONTENT` / `_END`) and tool call blocks
(`TOOL_CALL_START` / `_ARGS` / `_END`), and — when the server executes tools — a
second pair (step `"tools"`) enclosing the `TOOL_CALL_RESULT`s and any
`STATE_DELTA`s the tools emit; then `STATE_SNAPSHOT`, `MESSAGES_SNAPSHOT` and
exactly **one** terminal event (`RUN_FINISHED` or `RUN_ERROR`). On a fatal error
(model failure, tool binding) `RUN_ERROR` goes out without the two snapshots.
`RUN_FINISHED` carries the 1.0 outcome: plain success, success with
`pendingToolCallIds` when frontend tools were handed to the client, or the
interrupt outcome when a tool asked for a human decision; the terminal event
also reports the run's token `usage`.

The model↔tools loop behind this is Eino: the chat model is a
`model.ToolCallingChatModel`, and a tool is a `tool.BaseTool`. That's not an
implementation detail — it's what lets any tool from the `eino-ext` ecosystem
(MCP, search, whatever) plug in without an adapter, and what lets you bring your
own `ChatModel` without touching the library.

**Requirements:** Go 1.24.4+ (the core and `model`); Go 1.25+ for `ginx` and the
examples.

## AG-UI 1.0 conformance

The library follows the two sources of authority of the
[specification](https://docs.ag-ui.com/spec/1.0):

- **Structure**: the official [`schema.json`](https://docs.ag-ui.com/spec/1.0/schema.json)
  is embedded unchanged (`pkg/agui/spec/1.0/schema.json`). Every validation is an
  interpretation of that file, not a hand-written copy.
- **Behaviour**: the normative rules (MUST/MUST NOT) of the prose are implemented
  for both roles, **producer** (agent) and **consumer** (client): run lifecycle,
  open/content/close discipline, chunks, subagent attribution, the processing
  model (unknown material dropped with a warning; a malformed known value is
  fatal), metadata, snapshot/delta, interrupt/resume, versioning and both
  transport bindings.

Every event the agent emits goes through that layer **before** it is written:
it is validated against the schema and checked against the sequence rules. A
non-conformant event never reaches the wire (`ErrEventDropped`), and a
`RUN_FINISHED` the protocol would refuse is replaced by a `RUN_ERROR`, so the
client always gets a terminal event. On top of that, the agent loop does what
1.0 asks of a producer:

| Rule | What the library does |
|---|---|
| Input is read under the processing model | unknown fields dropped with a warning; a malformed known value answers **HTTP 400 before any stream**; retired forms (`binary` parts, legacy `null`s) are converted |
| Version negotiation | `RUN_STARTED` declares `protocolVersion: "1.0"`; newer minors are served, another major can be refused with `WithRejectOtherMajorVersions()` |
| Two bindings | HTTP + SSE always; HTTP + Protobuf negotiated through `Accept` (`WithoutProtobuf()` turns it off) |
| Frontend tool calls | the run ends with the success outcome and `pendingToolCallIds` naming exactly the calls left unanswered, in call order |
| Every started call is answered or pending | skipped server calls and malformed calls get a `TOOL_CALL_RESULT` |
| Tool call ids identify a call | an id the provider repeats across turns (`call_0` again) is renamed, on the wire and in the history |
| Interrupt / resume | interrupt outcome with unique ids; a resume that leaves a paused call uncovered ends with the interrupt outcome again, without running it; entries naming no open interrupt are logged and ignored |
| Snapshots | `MESSAGES_SNAPSHOT` returns the request messages verbatim (multimodal parts, reasoning, metadata) and keeps ids stable |
| Token usage | the run's total `usage` goes on the terminal event |
| Capabilities | `agui.Capabilities(caps)` serves a schema-validated `AgentCapabilities` |

The protocol layer lives in the core module, under `pkg/`, and is usable on its
own:

| Package | What it is |
|---|---|
| `pkg/agui` (imported as `wire` by the core) | the 1.0 data model (31 events, messages, multimodal content, outcomes, capabilities), schema validation, `Sanitize`, sequence verifier, chunk expansion, JSON Patch, versioning, compatibility shims |
| `pkg/sse` | SSE framing (LF writing, full-grammar reading) |
| `pkg/protobuf` | HTTP + Protobuf binding: codec, framing, negotiation |
| `pkg/stream` | producer: writes only conformant events to an HTTP response |
| `pkg/server` | a low-level `net/http` handler for agents that drive a `stream.SSE` directly |
| `pkg/client` | consumer: `HttpAgent`-style client, processing pipeline, `Store` |

What the tests verify (`make test`):

| Check | Result |
| --- | --- |
| [Official schema fixtures](https://github.com/ag-ui-protocol/ag-ui/tree/main/spec/1.0/fixtures): valid documents accepted, invalid ones rejected by the same rule | 219/219 |
| Typed round trip: every valid fixture decoded into the Go type and re-encoded produces the same JSON | 135/135 |
| [Official consumer conformance corpus](https://github.com/ag-ui-protocol/ag-ui/tree/main/spec/1.0/conformance/streams), replayed over real HTTP | 68/68 (1 with a documented override) |
| Protobuf byte corpus (TypeScript and .NET reference encoders) | 98/98 byte-identical |
| [Official Python SDK tests](https://github.com/ag-ui-protocol/ag-ui/tree/main/sdks/python/tests) ported to Go (`pkg/agui/pytests`) | 268 tests: 242 pass, 26 N/A in Go (Python-only APIs, reason recorded in each test) |
| [Official .NET SDK tests](https://github.com/ag-ui-protocol/ag-ui/tree/main/sdks/dotnet) ported to Go (`pkg/agui/dotnettests`), including the Protobuf binding | 887 tests: 805 pass, 82 N/A in Go (A2UI, cross-language integration and .NET-only APIs, reason recorded in each test) |
| Every stream of the agent's own tests replayed through a conformant consumer (`aguitest.RequireConformant`) | always |
| The reference client (`pkg/client`) talking to an `agui.Agent` over real HTTP, on SSE and on Protobuf | no errors, no warnings |

The override: the `unknown-enum-value-role-fatal` fixture pins an *admitted gap*
of the reference client (an unknown `role` value is fatal there). The
specification (`basic/processing.mdx`) requires it to be dropped with a warning,
and that is what this library does.

The official fixtures and conformance corpus are in
`pkg/agui/testdata/spec-1.0` (MIT license of the AG-UI project; the source
commit is in `UPSTREAM_COMMIT`).

## Installation

```bash
go get github.com/ricardogayer/go-agui@v0.4.0
go get github.com/ricardogayer/go-agui/model@v0.4.0   # model providers (optional)
go get github.com/ricardogayer/go-agui/ginx@v0.4.0    # Gin adapter (optional)
```

Three published modules, on purpose (a fourth, `examples`, exists only in the
repository). The core depends only on Eino and a few utilities (its AG-UI 1.0
protocol layer is part of the module, under `pkg/`) — no web framework, no
provider client. Whoever uses `chi` doesn't drag
in Gin; whoever already has their own `ChatModel` doesn't drag in `eino-ext`.

| Module | Import | Brings |
|---|---|---|
| core | `github.com/ricardogayer/go-agui` | agent, handler, emitter, state, tools, and the AG-UI 1.0 protocol layer (`pkg/...`) |
| `model` | `.../go-agui/model` | OpenAI and Ollama via `eino-ext` |
| `ginx` | `.../go-agui/ginx` | Gin: routes, engine, CORS, `ListenAndServe` |
| `aguitest` | `.../go-agui/aguitest` | `FakeModel` and assertions (subpackage of the core) |
| `a2ui` | `.../go-agui/a2ui` | A2UI v0.9 operations for rendering UI (subpackage of the core) |

## Try it without deploying anything

The backend is testable on its own, with no TypeScript runtime and no browser:

```bash
export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
go run ./examples/quickstart

curl -N -X POST localhost:8000 -H 'content-type: application/json' \
  -H 'accept: text/event-stream' \
  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"hi"}]}'
```

(`accept: text/event-stream` matters: curl sends `Accept: */*` by default, which
admits the Protobuf binding — and AG-UI 1.0 negotiation would answer in binary.)

You see the raw SSE frames: `RUN_STARTED`, `STEP_STARTED`, `TEXT_MESSAGE_START`,
the deltas, `TEXT_MESSAGE_END`, `STEP_FINISHED`, `STATE_SNAPSHOT`,
`MESSAGES_SNAPSHOT`, `RUN_FINISHED`.

---

# Usage

## Creating the agent

`agui.New` takes an Eino `model.ToolCallingChatModel` and returns an `*Agent`,
which is already an `agui.Runner` — what any transport needs.

```go
ag, err := agui.New(ctx, m,
	agui.WithSystemPrompt("You help plan trips."),
	agui.WithTools(
		agui.NewTool("search_flights", "Searches flights.", searchFlights),
		agui.NewTool("set_theme", "Changes the UI theme.", setTheme),
	),
	agui.WithMaxIterations(6),
)
```

It takes `ctx` because the tool schemas are resolved there — and `Info()` may do
network I/O, as an MCP tool does. Resolving at construction time, rather than on
every request, is what lets it fail at process startup instead of in the middle
of a conversation.

An `*Agent` is **safe for concurrent use** and is not mutated per request:
frontend tools are bound to the model with `WithTools`, which returns a new
instance.

### Options

| Option | Effect |
|---|---|
| `WithSystemPrompt(s)` | system prompt; only injected if the client didn't send its own `system` |
| `WithTools(...)` | registers the tools; successive calls accumulate |
| `WithMaxIterations(n)` | cap on loop turns per run. Default `DefaultMaxIterations` = 8, absolute cap 64 |
| `WithLogger(l)` | the agent's `*slog.Logger` (default `slog.Default()`) |
| `WithContextFormatter(f)` | swaps how the request `context` is formatted (default `AppContextBlock`) |
| `NoContext()` | turns off forwarding the request `context` to the model |
| `WithMessagePreprocessor(f)` | last transformation on messages before each call — truncate history, inject few-shots, redact sensitive data |
| `WithState(f)` | swaps how the run's shared state is built |
| `WithHooks(h)` | run observers — see [Observability](#observability) |
| `WithStepNames(model, tools)` | labels for `STEP_STARTED`/`STEP_FINISHED` (defaults `"model"` and `"tools"`) |

Why 8 iterations: it comfortably covers the real case (call a data tool, then a
presentation tool, then answer) and limits the damage of a pathological loop, in
which the model insists on a tool that always fails — every turn is a paid call
to the provider.

### The chat model

The `model` module is the convenience layer for the two common providers:

```go
m, err := model.FromEnv(ctx)                       // reads MODEL_PROVIDER, MODEL, ...
m, err := model.New(ctx, model.Config{             // or explicitly
	Provider: model.ProviderOpenAI,
	Model:    "gpt-4o-mini",
	APIKey:   os.Getenv("OPENAI_API_KEY"),
})
```

`Config.Validate` rejects at startup what would otherwise only fail on the first
request — which is the right behavior for a container with a healthcheck.
Whoever already has their own `model.ToolCallingChatModel` doesn't need this
module: `agui.New` accepts any implementation.

## Tools

A tool is an Eino `tool.BaseTool`, and it's the **interface itself** that decides
who executes what:

| The tool implements | Who executes | What happens |
|---|---|---|
| `Info` + `InvokableRun` | the **server** | runs here, the result goes back to the model, the loop continues |
| `Info` only | the **client** | the run ends and control returns to the frontend |

The second case is the same path as tools arriving in `RunAgentInput.tools`
(those registered by `useFrontendTool`/`useComponent` in React). The difference is
that declaring the tool on the server lets you announce a frontend capability
without depending on React having registered it.

### Inferred schema — `NewTool`

```go
type FlightSearch struct {
	Origin      string `json:"origin"      jsonschema:"required,description=Origin IATA code"`
	Destination string `json:"destination" jsonschema:"required,description=Destination IATA code"`
}

agui.NewTool("search_flights", "Searches flights between two airports.",
	func(ctx context.Context, in FlightSearch) ([]Flight, error) {
		return search(in.Origin, in.Destination), nil
	})
```

Input and output are typed; serialization is the library's job. The schema comes
out without `$ref`/`$defs`/`$schema`, which is the form providers accept without
sanitization.

### Hand-written schema — `NewRawTool`

When the description text is part of the prompt and has to survive byte for byte,
or when inference can't express the schema:

```go
agui.NewRawTool("display_flights", description, json.RawMessage(schemaJSON),
	func(ctx context.Context, args string) (string, error) { ... })
```

`params` is a `json.RawMessage` and not a `string` on purpose: with three
adjacent strings the compiler wouldn't catch a swap between name, description and
schema. For those who build the `*schema.ToolInfo` programmatically there's also
`NewToolFromInfo`.

### Client tool — `NewClientTool`

Announces the schema to the model but doesn't execute: on seeing the call, the
run ends and control returns to the frontend.

```go
agui.NewClientTool("show_chart", "Renders a chart on screen.",
	json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`))
```

### Construction errors

The constructors don't return an `error` — the error is held inside the tool and
collected by `New`, aggregated and named. That's what lets you write the tool list
without three lines of ceremony between each one:

```go
ag, err := agui.New(ctx, m, agui.WithTools(
	agui.NewTool("a", "...", fnA),
	agui.NewRawTool("b", "...", schemaB, fnB),
))
// err: agui: tool "b": invalid parameter schema: ...
```

The reason is a language one: in Go, a two-value expression can only be the
**complete** argument list of a call — if `NewTool` returned `(Tool, error)`,
`WithTools(NewTool(...), NewRawTool(...))` wouldn't even compile.

## Inside a tool

A tool's function receives only `(ctx, input)` — that's how Eino invokes it.
Everything else about the run arrives through the context:

```go
func changeTheme(ctx context.Context, in ThemeArgs) (string, error) {
	rc := agui.MustFromContext(ctx)
	// applies AND emits the STATE_DELTA; errors if the parent path is missing
	if err := rc.State().Set("/theme", in.Theme); err != nil {
		return "", err
	}
	return `{"ok":true}`, nil
}
```

The `*RunContext` gives access to:

| Method | What for |
|---|---|
| `Emitter()` | emit events on the wire (never terminal events: the loop is what ends the run) |
| `State()` | the run's shared state |
| `Input()` | the raw `RunAgentInput` — `forwardedProps`, `threadId`, application context |
| `Logger()` | `*slog.Logger` already annotated with thread and run |
| `ThreadID()` / `RunID()` | run identifiers |
| `ToolCallID()` / `ToolName()` | which call this tool is serving |
| `Resume()` / `ResumeFor(id)` | resume data — see [Human-in-the-loop](#human-in-the-loop) |

Use `FromContext` (which returns `ok`) instead of `MustFromContext` when the tool
needs to be callable directly in a test, outside a run.

## Shared state

`State` is a free-form JSON document mirrored into the client's `agent.state`.
The contract: **the operation applied locally is exactly the one that goes on the
wire**. A client that starts from the `STATE_SNAPSHOT` and applies the
`STATE_DELTA`s in order rebuilds the server's document. There is no "I mutated it
and forgot to emit" mode.

```go
st.Set("/todos/0/done", true)   // applies + emits
st.Remove("/draft")             // same
st.Apply(ops)                   // atomic batch: all or nothing
v, ok := st.Get("/theme")       // read by JSON Pointer (RFC 6901)
snap := st.Snapshot()           // deep copy
st.Emit()                       // publishes the whole document as STATE_SNAPSHOT
```

`Set` uses `add` and not `replace` because, in RFC 6902, `add` replaces a member
that already exists and creates one that doesn't — it works in both cases. A path
whose **parent** doesn't exist is an error; create the parent first. `State` is
safe for concurrent use.

To seed the state with your own value, use `WithState`:

```go
agui.WithState(func(ctx context.Context, in *agui.RunAgentInput) (*agui.State, error) {
	s := agui.NewState(map[string]any{"todos": []any{}})
	return s, s.Apply(seedFromClient(in.State))
})
```

The default seeds from `RunAgentInput.state` and returns it untouched — the
client's `agent.state` must not regress just because it passed through here.

## Human-in-the-loop

A tool pauses the run by returning `agui.Interrupt`. The client answers in
`RunAgentInput.resume`, and on the next pass the tool recovers the answer:

```go
func approvePurchase(ctx context.Context, in Purchase) (string, error) {
	rc := agui.MustFromContext(ctx)
	if payload, ok := rc.ResumeFor(rc.ToolCallID()); ok {
		return process(in, payload)        // the human answered
	}
	return "", agui.Interrupt(agui.InterruptRequest{
		Reason:  "needs_approval",
		Message: "Confirm the purchase?",
	})
}
```

`ResumeFor` only considers entries with status `resolved`: a `cancelled` decision
is not a resume value, and returning it as if it were would make the tool execute
precisely what the user refused — use `Resume()` to inspect cancellations.

AG-UI 1.0 requires the resume to cover every interrupt of the run being
continued. If a paused call is left without an entry, the library does not run
it and does not move past it: the run ends with the interrupt outcome again.
Entries naming no open interrupt are logged and ignored.

There is no store on the server, and that's deliberate: AG-UI resends the entire
history on every request, so the pending call **arrives** with the conversation.
An in-memory store would add nothing and would start being wrong the moment there
was more than one replica.

> Why a signal of our own and not Eino's `tool.Interrupt`: its signaling half
> would work, but the **resume** half wouldn't — `GetInterruptState` and
> `GetResumeContext` read an unexported context key, populated only by the graph
> runner in the `compose` package. Since this loop invokes tools directly (which
> is what allows handling tools the server doesn't execute), there's no way to
> populate that key. The transport for the pause is therefore AG-UI's own, which
> already has `Interrupt` and `ResumeEntry` designed for exactly this.

## Rendering UI: A2UI

The `a2ui` subpackage builds the **A2UI v0.9** operations that a backend tool
returns as its result — it's the Go port of `copilotkit/a2ui.py`. There's nothing
model-related here: A2UI is a JSON format, and what Python did in 246 lines (most
of them prompt text that the TypeScript runtime injects on its own) is three
constructors and an envelope.

The path on the wire is this: the tool returns the string produced by
`a2ui.Render` as the content of the `TOOL_CALL_RESULT`; the frontend's
`@copilotkit/a2ui-renderer` recognizes the `a2ui_operations` key, applies the
operations to the surface and renders the components from your React catalog.

```go
func showFlights(ctx context.Context, in FlightSearch) (string, error) {
	flights := search(in.Origin, in.Destination)

	return a2ui.Render(
		a2ui.CreateSurface("flights", "copilotkit://app-dashboard-catalog"),
		// `component` and the other keys are the names and props of YOUR
		// React catalog; the library does not interpret them.
		a2ui.UpdateComponents("flights", []a2ui.Component{
			{"id": "root", "component": "Card", "children": []any{"list"}},
			{"id": "list", "component": "FlightList", "itemsPath": "/flights"},
		}),
		a2ui.UpdateDataModel("flights", "/", map[string]any{"flights": flights}),
	)
}
```

| Constructor | Operation | Default when empty |
|---|---|---|
| `CreateSurface(surfaceID, catalogID)` | `createSurface` | `catalogID` falls back to `a2ui.BasicCatalogID` |
| `UpdateComponents(surfaceID, comps)` | `updateComponents` — the surface's declarative schema | a nil list comes out as `[]` |
| `UpdateDataModel(surfaceID, path, v)` | `updateDataModel` — writes `v` at the data model path | an empty `path` becomes `"/"` (the root) |
| `Render(ops...)` | wraps everything in `{"a2ui_operations": [...]}` and serializes | — |

Each operation carries `version: "v0.9"` (the `a2ui.Version` constant) and exactly
one command key.

Three details the package guarantees, and which cost dearly when missing:

- **The envelope key is the detection.** Getting `a2ui_operations`
  (`a2ui.OperationsKey`) wrong makes the tool result show up as raw text in the
  chat, instead of becoming a surface.
- **An empty component list comes out as `[]`, never `null`.** The renderer
  iterates over the array, and a null would break applying the operation.
- **`Render` returns an error instead of swallowing it.** The `value` of
  `UpdateDataModel` comes from outside — from arguments the model produced — and a
  truncated tool result would render an empty surface with no sign of what
  happened.

### Not to be confused with `generate_a2ui`

They are two distinct paths, and only one goes through this package:

| | **Fixed** schema (this package) | **Dynamic** schema (`generate_a2ui`) |
|---|---|---|
| Who designs the surface | the server, which already knows the layout | the model itself |
| Where the tool comes from | you declare it with `agui.NewTool` | injected by the runtime's A2UIMiddleware |
| How it arrives | it's yours, runs on the server | as a **frontend** tool in `RunAgentInput.tools` |
| Who executes it | the server, returning `a2ui.Render(...)` | the client — it's never executed here |

In both cases the request `context` pitfall applies: it's through
`RunAgentInput.context` that the runtime delivers the **component catalog** to the
agent. Without forwarding it to the model, it invents names the React catalog
doesn't know and the frontend renders "Unknown component" — that's why
`AppContextBlock` is the default formatter, and why `NoContext()` should not be
used in an agent that renders A2UI.

## Observability

`WithHooks` exists because a logger doesn't answer the questions you ask in
production: how much did it cost, which tools ran, how long did each turn take.

```go
agui.WithHooks(agui.Hooks{
	OnRunStart:   func(ctx context.Context, in *agui.RunAgentInput) { ... },
	OnModelTurn:  func(ctx context.Context, t agui.TurnInfo) {
		if t.Usage != nil {
			log.Printf("turn %d: %d tokens in %s",
				t.Iteration, t.Usage.TotalTokens, t.Duration)
		}
	},
	OnToolResult: func(ctx context.Context, t agui.ToolRunInfo) { ... }, // name, args, duration, error, interrupted
	OnRunEnd:     func(ctx context.Context, r agui.RunInfo) { ... },     // iterations, duration, "success"|"interrupt"|"error"
})
```

All fields are optional.

---

# Server

## Setting up a Go server with Gin

The core speaks `http.Handler` and knows no framework at all. The `ginx` module is
the convenience layer for Gin users — and it's a **separate** module precisely so
Gin doesn't end up in the `go.mod` of whoever uses `chi`, `echo` or plain
`net/http`.

### 1. The project

```bash
mkdir my-agent && cd my-agent
go mod init github.com/you/my-agent

go get github.com/ricardogayer/go-agui@v0.4.0
go get github.com/ricardogayer/go-agui/ginx@v0.4.0
go get github.com/ricardogayer/go-agui/model@v0.4.0
```

### 2. The `main.go`

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
	// A SIGINT/SIGTERM cancels this context, which in turn triggers the
	// server's graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	m := agui.Must(model.FromEnv(ctx))

	ag := agui.Must(agui.New(ctx, m,
		agui.WithSystemPrompt("You are a helpful, straightforward assistant."),
	))

	// agui.Addr reads HOST and PORT, falling back to 0.0.0.0:8000.
	log.Fatal(ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "my-agent"))
}
```

`agui.Must` unwraps `(value, error)` and panics on error — it exists for `main()`,
where a construction error has nowhere to go and an `if err != nil { log.Fatal(err) }`
per line only adds noise. Don't use it outside initialization.

### 3. Run and verify

```bash
export MODEL_PROVIDER=openai MODEL=gpt-4o-mini OPENAI_API_KEY=sk-...
go run .

curl -s localhost:8000/health
# {"agent":{"name":"my-agent"},"status":"ok"}

curl -N -X POST localhost:8000 -H 'content-type: application/json' \
  -H 'accept: text/event-stream' \
  -d '{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"hi"}]}'
```

`ListenAndServe` blocks until the context is cancelled; then it drains in-flight
connections (10s by default) and returns. A `Ctrl-C` in the terminal exercises the
whole path.

### 4. What `ginx.ListenAndServe` does for you

One call assembles this stack:

```
gin.Engine (release mode)
 └─ gin.Recovery()
 └─ [CORS, if WithCORS()]
 └─ POST /       → agui.Handler(ag)   ← SSE
 └─ GET  /health → agui.Health(name)
agui.NormalizeHealthPath(...)          ← //health → /health
http.Server with no WriteTimeout, with ReadHeaderTimeout=15s
 └─ Graceful shutdown when ctx is cancelled
```

And on every request, inside the handler: decoding of the `RunAgentInput`,
generation of `threadId`/`runId` when they come in empty, the SSE headers
(including `X-Accel-Buffering: no`), removal of the write deadline, the
`RUN_STARTED`, the run context tied to client disconnect **and** to process
shutdown, panic recovery turning into `RUN_ERROR`, and the guarantee that exactly
one terminal event goes out.

### 5. Server options

```go
ginx.ListenAndServe(ctx, agui.Addr(8000), ag, "my-agent",
	ginx.WithCORS(),                                   // permissive CORS (see below)
	ginx.WithHandlerOptions(
		agui.WithHandlerLogger(logger),                // logger for the handler and the SSE writer
		agui.WithShutdownContext(ctx),                 // SIGTERM cancels in-flight streams
		agui.WithMaxBodyBytes(8<<20),                  // 413 above 8 MiB (default: no limit)
		agui.WithRejectOtherMajorVersions(),           // 400 for a client of another major version
		agui.WithoutProtobuf(),                        // always SSE (default: negotiated via Accept)
		agui.WithOnDropped(func(ctx context.Context, ev agui.Event, err error) {
			metrics.DroppedEvent(ev.Type())            // default: log at ERROR
		}),
	),
	ginx.WithServeOptions(
		agui.WithServeLogger(logger),
		agui.WithShutdownTimeout(30*time.Second),      // default 10s
	),
)
```

`WithCORS` is **off by default**, on purpose: the CopilotKit runtime calls the
backend server-side, so there's no cross-origin in the normal path. Turn it on
when debugging the backend straight from the browser.

### 6. Mounting on a Gin router you already have

When the agent is one route among others — because the same binary serves a REST
API, metrics, or several agents:

```go
r := gin.New()
r.Use(gin.Recovery(), myLogger(), myAuth())

// The contract's two routes, at the root:
ginx.Register(r, ag, "my-agent")

// Or under a prefix — becomes POST /agents/travel/ and GET /agents/travel/health
// (the POST keeps the trailing slash; Gin redirects /agents/travel to it):
ginx.Register(r.Group("/agents/travel"), agTravel, "travel")
ginx.Register(r.Group("/agents/support"), agSupport, "support")

r.GET("/metrics", metricsHandler)

// NormalizeHealthPath takes care of the `//health` of legacy probes; it can't be
// a route (Gin's router rejects a path with a doubled slash).
log.Fatal(agui.ListenAndServe(ctx, agui.Addr(8000), agui.NormalizeHealthPath(r)))
```

If you want just the handler, without the routing conventions:

```go
r.POST("/chat", ginx.Handler(ag))
r.GET("/chat/health", ginx.Health("my-agent"))
```

`ginx.Handler` is `gin.WrapH(agui.Handler(...))`. The `gin.ResponseWriter`
implements `http.Flusher` and `http.ResponseWriter`, so SSE streaming crosses the
wrap without loss. If you'd rather not depend on `ginx`, `r.POST("/",
gin.WrapH(agui.Handler(ag)))` does the same thing.

For a ready-made `*gin.Engine` (release, `Recovery`, the two routes, optional
CORS) that you serve yourself, use `ginx.Engine(ag, "my-agent", opts...)` —
remembering to wrap it in `agui.NormalizeHealthPath` if the probes use `//health`.

### 7. Middleware and SSE

A middleware that **wraps the `ResponseWriter`** (compression, cache, body
recording) breaks streaming: it either holds the frames in a buffer, or hides the
`Flusher` from the handler. Don't use `gzip` or `gin.Logger` with a custom writer
on the agent's route. Authentication, rate limiting, tracing and panic recovery are
safe — none of them touch the response body.

Behind an NGINX, the `X-Accel-Buffering: no` the handler already sends is what
keeps the proxy from turning the stream into a batch. On other proxies
(Cloudflare, ALB, Envoy) check response buffering and turn it off for the agent's
route.

### 8. Production

- **Do not configure `WriteTimeout`.** It's the standard recommendation in every
  production guide and it would cut the SSE stream mid-conversation.
  `agui.ListenAndServe` deliberately doesn't set it, and keeps
  `ReadHeaderTimeout: 15s`, which is what protects against idle connections.
- **Wire up `WithShutdownContext`** to the same signal context, so a SIGTERM
  cancels in-flight runs instead of killing them mid-way.
- **Set `WithMaxBodyBytes`** on an exposed server. AG-UI resends the entire
  history on every request, so the right cap depends on the application — but
  without any, any client can make the decode allocate without limit.
- **The healthcheck tests the real endpoint** — see
  [Healthcheck in a distroless container](#healthcheck-in-a-distroless-container).
- **Scale horizontally at will.** There's no session state on the server: AG-UI
  resends the entire history on every request.

## Without Gin: plain net/http

The core alone already assembles the whole contract:

```go
log.Fatal(agui.ListenAndServe(ctx, agui.Addr(8000), agui.Mux(ag, "my-agent")))
```

`agui.Mux` returns an `http.Handler` with `POST /`, `GET /health` and the
`//health` normalization. On a router of your own (`chi`, `echo`,
`http.ServeMux`), mount `agui.Handler(ag)` and `agui.Health(name)` wherever you
like.

## Levels of control

| Level | API | For whom |
|---|---|---|
| **0** | `ginx.ListenAndServe(ctx, addr, ag, name)` | brings up the whole server |
| **1** | `ginx.Register(r, ag, name)` / `agui.Handler(ag)` / `agui.Mux(...)` | you already have a router |
| **2** | implement `agui.Runner` | you want your own loop, keeping SSE and lifecycle |
| **3** | `Emitter`, `Conversation`, `ClassifyToolCalls` | you want to control every event on the wire |
| **4** | `pkg/stream`, `pkg/server`, `pkg/agui` | you want the bare AG-UI 1.0 protocol layer, without the Eino agent |

No level is a wall: you can drop down a step without rewriting the rest. At level
2, a `Runner` is a one-method interface — `Run(ctx, *Emitter, *RunAgentInput) error`
— and `RunnerFunc` adapts a function to it. `RUN_STARTED` has already been emitted
when `Run` is called; returning an error is enough to end the run, because the
handler emits the `RUN_ERROR` if no terminal event went out.

At level 3 the `Emitter` covers the whole 1.0 surface: text, tool calls and
results (`ToolResultParts` for multimodal results), reasoning
(`ReasoningStart`/`ReasoningMessage*`/`ReasoningEnd`), state, activity
(`ActivitySnapshot`/`ActivityDelta`), `Custom`/`Raw`, and every terminal form
(`RunFinished`, `RunFinishedWithResult`, `RunFinishedPending`,
`RunFinishedWithInterrupts`, `RunCancelled`, `RunError`, `RunErrorCode`);
`Send` emits any event of `pkg/agui`. Messages are the 1.0 types
(`*agui.UserMessage`, `*agui.AssistantMessage`, ...), read with a type switch.

---

## Testing

The `aguitest` package brings a `FakeModel` that implements
`model.ToolCallingChatModel` **in process**, with no fake HTTP server — what's
being tested is the loop and the event sequence, not a third-party HTTP adapter.

```go
m := aguitest.NewFakeModel(
	aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "c1", Name: "add", Args: `{"a":2,"b":3}`}}},
	aguitest.Turn{Text: "That's 5."},
)
ag := agui.Must(agui.New(ctx, m, agui.WithTools(addTool())))

evs := aguitest.Run(t, ag, aguitest.Input("what's 2+3?"))
aguitest.RequireBalanced(t, evs)
aguitest.RequireContains(t, evs, agui.EventToolCallResult, agui.EventRunFinished)
```

`RequireBalanced` is the assertion nobody writes on their own and that pays off
the most: every `START` has an `END`, no block overlaps, and the run has exactly
**one** terminal event, at the end. There's also `RequireSequence` (the exact
sequence), `Collect` (to exercise an `http.Handler`), `ParseSSE` (to decode a raw SSE
stream), plus `First`/`All`/`Types` and `ConcatenatedText`.

`Run` and `Collect` also check, on every call, that the stream is AG-UI 1.0
conformant: each event against the official schema and the whole sequence
against the protocol rules, as a conformant consumer would
(`RequireConformant`). An event the `Emitter` refuses fails the test.

On the other side, `FakeModel.Calls()` returns the messages the model received on
each call, and `BoundTools()` the tool sets that were bound (`LastCall()` and
`LastBoundNames()` are shortcuts to the most recent ones) — that's what lets you
assert about what the agent **assembled** (was the system prompt injected? is the
context block there? did `RepairToolPairs` reorder the history?), and not just
about what it emitted.

## Protocol pitfalls the library already solves

This is the expensive knowledge — what cost debugging against the wire and what
you don't have to rediscover:

- **One invalid event kills the entire stream.** A conformant consumer treats a
  malformed event or a sequence violation as fatal. The library validates every
  event against the 1.0 schema and sequence rules before writing it, drops the
  rejected one (`ErrEventDropped`) and keeps the stream alive, so a terminal
  event can still reach the client.
- **A tool call needs its id before it starts.** In streaming, id and name only
  arrive in the first chunk. Argument fragments are buffered until both are known.
- **Providers repeat tool call ids.** Some number the calls `call_0`, `call_1`, ...
  again on every turn; in AG-UI a repeated id "reopens" the earlier call. The
  repeated id is renamed on the wire and in the history.
- **A frontend tool call is not "nothing pending".** AG-UI 1.0 consumers must not
  read a bare `RUN_FINISHED` as "no work left"; the run reports
  `pendingToolCallIds`, and every other started call gets an answer.
- **Don't emit `TOOL_CALL_*` twice.** Whoever streams the calls can't re-emit them
  afterwards.
- **`MESSAGES_SNAPSHOT` is mandatory.** Without it the frontend's message list has
  no reconciliation after tool calls. And the message ids must be stable: if each
  snapshot invented new ids, the client would see the entire conversation as new
  messages every turn — hence `Conversation` preserving each message's original
  AG-UI id.
- **The order of tool responses matters.** When control returns to the client, the
  history it resends brings the responses out of position, and OpenAI rejects it
  with a 400. `RepairToolPairs` fixes what goes to the model — without touching
  the `MESSAGES_SNAPSHOT`, which remains the real history.
- **The request `context` has to reach the model.** It's through it that the
  runtime delivers things like the component catalog; without forwarding it, the
  model invents names and the UI renders "Unknown component".
- **`X-Accel-Buffering: no`.** Without it an NGINX in front holds the frames and
  streaming becomes batching.
- **Zero out the write deadline.** An `http.Server` with `WriteTimeout` — the
  standard production recommendation — would cut the SSE stream in half.
- **A client that disconnects still costs money.** The first write that fails
  cancels the run's context, which aborts the in-flight model stream instead of
  going on generating — and paying — for someone who already left.

## Environment variables

Read by `model.ConfigFromEnv` and `agui.Addr`. They're the same ones the Python
backends read, so an existing `.env` keeps working.

| Variable | What it's for |
|---|---|
| `MODEL_PROVIDER` | `openai` (default) or `ollama` |
| `MODEL` | model id |
| `OPENAI_API_KEY` | credential, required for `openai` |
| `OPENAI_BASE_URL` | alternative endpoint (compatible gateway) |
| `OPENAI_REASONING_EFFORT` | `none`, `low`, `medium` or `high` (empty omits the field). `none` is **mandatory** on OpenAI's reasoning models: they reject function tools at the default effort, and an AG-UI agent puts everything through function tools |
| `OLLAMA_BASE_URL` | Ollama endpoint (default `http://localhost:11434`) |
| `HOST` / `PORT` | listen address, read by `agui.Addr` (defaults `0.0.0.0` and the port passed to `Addr`; an invalid `PORT` is ignored) |

## Healthcheck in a distroless container

A distroless image has no shell and no curl, so the binary itself becomes the
probe. The library does **not** register flags — that would break the `FlagSet` of
whoever uses it, and `go test`:

```go
var probe = flag.Bool("healthcheck", false, "queries the local /health and exits 0/1")

func main() {
	flag.Parse()
	if *probe {
		if err := agui.CheckHealth("http://127.0.0.1:8000/health"); err != nil { // 3 s timeout
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// ...
}
```

```dockerfile
HEALTHCHECK --interval=10s CMD ["/app/server", "-healthcheck"]
```

This tests the real endpoint, unlike a healthcheck that only opens a TCP socket
and reports "healthy" with the agent broken.

## Development

The repository has four modules tied together by a `go.work` (which requires Go
1.25+ to build the repository as a whole). The `Makefile` runs the targets across
all of them:

```bash
make build      # builds all modules
make test       # runs the tests of all of them
make vet        # go vet across all
make fmt        # gofmt -w
make lint       # gofmt check (use `make fmt` to fix) + go vet
make tidy       # go mod tidy across all
make quickstart # brings up the minimal example on :8000
```

Examples:

| Example | Shows |
|---|---|
| `examples/quickstart` | the smallest possible agent: chat with streaming, no tools |
| `examples/backend-tools` | inferred schema, shared state, a client tool declared on the server, and token usage hooks |

## License

MIT.

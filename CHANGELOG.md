# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the modules use
[Semantic Versioning](https://semver.org/). Each release tags the three
published modules together: `vX.Y.Z` (core), `model/vX.Y.Z` and `ginx/vX.Y.Z`.

PT: Todas as mudanças relevantes deste projeto estão documentadas aqui. O
formato segue o Keep a Changelog e os módulos usam Semantic Versioning. Cada
release marca os três módulos publicados juntos: `vX.Y.Z` (núcleo),
`model/vX.Y.Z` e `ginx/vX.Y.Z`.

## [0.4.0] - 2026-10-25

Full AG-UI 1.0 conformance. The protocol layer of
[ag-ui-go](https://github.com/ricardogayer/ag-ui-go) — data model, schema
validation, sequence verifier, SSE and Protobuf bindings, producer, consumer —
and its whole test suite now live in this repository, and the agent runs on
top of it instead of the community AG-UI Go SDK.

### Added
- AG-UI 1.0 protocol layer in the core module, under `pkg/`: `pkg/agui` (1.0
  data model with all 31 events, official `schema.json` embedded, `Sanitize`,
  sequence verifier, chunk expansion, JSON Patch, versioning, compatibility
  shims), `pkg/sse`, `pkg/protobuf` (HTTP + Protobuf binding), `pkg/stream`
  (conformant producer), `pkg/server` (low-level `net/http` handler) and
  `pkg/client` (consumer). Gin-specific helpers of ag-ui-go were left out of
  the core (use `ginx` or `gin.WrapH`).
- The test suites of ag-ui-go: official schema fixtures, consumer conformance
  corpus, Protobuf byte corpus, and the official Python and .NET SDK tests
  ported to Go.
- HTTP + Protobuf binding, negotiated through `Accept`; `WithoutProtobuf()`
  forces SSE. `WithRejectOtherMajorVersions()` refuses clients of another
  major protocol version.
- `RUN_STARTED` declares `protocolVersion: "1.0"` (and echoes `parentRunId`).
- Frontend tool calls end the run with the success outcome and
  `pendingToolCallIds`; skipped server calls and malformed calls are answered
  with `TOOL_CALL_RESULT`.
- Tool call ids repeated by the provider across turns are renamed, on the wire
  and in the history.
- A resume that does not cover a paused call ends the run with the interrupt
  outcome again, without running the call; resume entries naming no open
  interrupt are logged.
- Token usage of the run on the terminal event (`usage`).
- `Emitter`: `Send`, `RunFinishedWithResult`, `RunFinishedPending`,
  `RunCancelled`, `RunErrorCode`, `ToolResultParts`, reasoning, activity and
  `Raw` events, `PendingToolCalls`, `SetUsage`, `Protobuf`;
  `WithEmitterStream`, `WithEmitterParentRunID`.
- `Capabilities(caps)` handler and the `AgentCapabilities` alias.
- `aguitest.RequireConformant`; `aguitest.Run`/`Collect` check every stream
  for AG-UI 1.0 conformance and fail on dropped events.
- Aliases for the 1.0 message types, `Content`, `TokenUsage`, every event type
  constant, `TextContent`, `PartsContent`, `Ptr`, `ProtocolVersion`.

### Changed (breaking)
- The wire types are now the AG-UI 1.0 ones (`pkg/agui`): `Message` is an
  interface (`*UserMessage`, `*AssistantMessage`, ...) and
  `RunAgentInput.Messages` is `Messages`; user/tool content is `Content`;
  optional fields are pointers; `PatchOp` is `wire.PatchOperation`;
  `InterruptPoint.Message`/`ToolCallID` are pointers.
- `EventTextMessageChunk` now names `TEXT_MESSAGE_CHUNK`; the constant for
  `TEXT_MESSAGE_CONTENT` is `EventTextMessageContent`.
- `WithEmitterSSEWriter` was replaced by `WithEmitterStream`.
- The handler reads the input under the AG-UI processing model: unknown
  fields are dropped with a warning, a malformed known value answers 400, and
  only `POST` with a JSON body is accepted.
- Every event is validated against the 1.0 schema and sequence rules before
  it is written; a non-conformant one is dropped (`ErrEventDropped`), and a
  refused `RUN_FINISHED` becomes a `RUN_ERROR`. An `Emitter` used directly
  must open the run with `RunStarted` first.
- `MESSAGES_SNAPSHOT` returns request messages verbatim (multimodal parts,
  reasoning messages, metadata).
- Empty tool results are no longer replaced by `"(empty)"`.
- Dependency on `github.com/ag-ui-protocol/ag-ui/sdks/community/go` removed.

PT:
- Conformidade total com o AG-UI 1.0: a camada de protocolo do ag-ui-go
  (modelo de dados, validação pelo schema, verificador de sequência, bindings
  SSE e Protobuf, produtor, consumidor) e toda a sua suíte de testes passam a
  viver neste repositório, em `pkg/`, e o agente roda sobre ela em vez do SDK
  comunitário.
- Adicionado: binding Protobuf negociado pelo `Accept`; `protocolVersion` no
  `RUN_STARTED`; `pendingToolCallIds` para tools de frontend; ids de tool call
  repetidos renomeados; resume sem cobertura interrompe de novo; `usage` no
  evento terminal; novos métodos do `Emitter`; `Capabilities`;
  `aguitest.RequireConformant`.
- Alterado (incompatível): tipos do fio do 1.0 (`Message` é interface,
  `Content`, ponteiros em campos opcionais), `EventTextMessageChunk` agora é
  `TEXT_MESSAGE_CHUNK` (use `EventTextMessageContent`), `WithEmitterStream` no
  lugar de `WithEmitterSSEWriter`, input lido pelo modelo de processamento
  (400 para valor malformado), todo evento validado antes de ser escrito.

## [0.2.1] - 2026-08-27

### Fixed
- `NewClientTool`: a client tool declared on the server now hands control back
  to the client, as documented. Before, the loop treated it as an unknown tool,
  answered `{"error":"unknown tool ..."}` and called the model again.
- The "unknown tool" result is now valid JSON (the tool name was embedded with
  Go quoting inside a JSON string).
- `ginx.ListenAndServe` now ties in-flight runs to the shutdown context by
  default (`agui.WithShutdownContext(ctx)`); a `WithShutdownContext` passed by
  the caller still wins.
- Doc comments brought in line with the code: `ginx.Register` does not handle
  `//health` (only `ListenAndServe` does), `Interrupt` stores nothing on the
  server, `Agent.Run` terminal-event contract on cancellation, leftover
  references to a course repository in `a2ui` and `types.go`, and others.

### Changed
- README, README_PT and llms.txt: accurate event sequence (steps `"model"` /
  `"tools"`, `STATE_DELTA`), `agui:` prefix on aggregated errors, `/health`
  body key order, `OPENAI_REASONING_EFFORT` values, `HOST`/`PORT` defaults,
  `CheckHealth` timeout, `make vet`/`make fmt` targets, `aguitest`/`a2ui`
  described as packages of the core module.

### Added
- Tests `TestServerDeclaredClientToolHandsControlBackToTheClient` and
  `TestUnknownToolResultIsValidJSON`.

PT:
- Corrigido: `NewClientTool` devolve o controle ao cliente (antes virava
  "unknown tool"); resultado "unknown tool" em JSON válido;
  `ginx.ListenAndServe` liga os runs ao contexto de desligamento por padrão;
  comentários de código alinhados ao comportamento real.
- Alterado: documentação corrigida (sequência de eventos, prefixo `agui:`,
  defaults e alvos do Makefile).
- Adicionado: testes para os dois primeiros casos.

## [0.2.0] - 2026-08-27

### Changed
- Public API, identifiers and strings translated to English; comments kept
  bilingual (English followed by a `PT:` block).
- Documentation: `README.md` in English, `README_PT.md` in Portuguese,
  `llms.txt` added.
- `ginx` requires `go-agui v0.1.0` instead of a `v0.0.0` placeholder.

PT:
- API pública, identificadores e strings traduzidos para inglês; comentários
  bilíngues (inglês seguido de bloco `PT:`).
- Documentação: `README.md` em inglês, `README_PT.md` em português, `llms.txt`
  adicionado.
- `ginx` requer `go-agui v0.1.0` em vez do placeholder `v0.0.0`.

## [0.1.0] - 2026-08-15

### Added
- Initial release: AG-UI protocol over SSE on top of CloudWeGo Eino — agent
  loop, handler, emitter, state (JSON Patch), backend/client tools, interrupts
  and resume, `model` (OpenAI and Ollama), `ginx` (Gin adapter), `aguitest`
  (FakeModel and assertions), `a2ui` (A2UI v0.9 operations), examples.

PT:
- Versão inicial: protocolo AG-UI sobre SSE em cima do CloudWeGo Eino — laço
  do agente, handler, emitter, estado (JSON Patch), tools de backend/cliente,
  interrupts e retomada, `model` (OpenAI e Ollama), `ginx` (adaptador Gin),
  `aguitest` (FakeModel e asserções), `a2ui` (operações A2UI v0.9), exemplos.

[0.4.0]: https://github.com/ricardogayer/go-agui/compare/v0.2.1...v0.4.0
[0.2.1]: https://github.com/ricardogayer/go-agui/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ricardogayer/go-agui/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/ricardogayer/go-agui/releases/tag/v0.1.0

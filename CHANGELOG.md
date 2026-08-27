# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the modules use
[Semantic Versioning](https://semver.org/). Each release tags the three
published modules together: `vX.Y.Z` (core), `model/vX.Y.Z` and `ginx/vX.Y.Z`.

PT: Todas as mudanças relevantes deste projeto estão documentadas aqui. O
formato segue o Keep a Changelog e os módulos usam Semantic Versioning. Cada
release marca os três módulos publicados juntos: `vX.Y.Z` (núcleo),
`model/vX.Y.Z` e `ginx/vX.Y.Z`.

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

[0.2.1]: https://github.com/ricardogayer/go-agui/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ricardogayer/go-agui/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/ricardogayer/go-agui/releases/tag/v0.1.0

# Makefile — go-agui
# The repository has four modules (core, ginx, model, examples), tied together
# by go.work. The targets below run across all of them.
# PT: O repositório tem quatro módulos (núcleo, ginx, model, examples),
# amarrados pelo go.work. Os alvos abaixo rodam em todos.

MODULES := . ./ginx ./model ./examples

.DEFAULT_GOAL := help
.PHONY: help build test vet fmt lint tidy quickstart

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build all modules
	@for m in $(MODULES); do echo "▶ build $$m"; (cd $$m && go build ./...) || exit 1; done

test: ## Run the tests of all modules
	@for m in $(MODULES); do echo "▶ test $$m"; (cd $$m && go test ./...) || exit 1; done

vet: ## Run go vet on all modules
	@for m in $(MODULES); do echo "▶ vet $$m"; (cd $$m && go vet ./...) || exit 1; done

fmt: ## Format the code
	gofmt -w .

# lint does NOT depend on fmt on purpose: if it formatted before checking, the
# check would never fail — and in CI a file outside gofmt would pass silently.
# PT: lint NÃO depende de fmt de propósito: se formatasse antes de verificar, a
# verificação nunca falharia — e em CI um arquivo fora do gofmt passaria calado.
lint: vet ## Check formatting (without fixing) and run vet
	@test -z "$$(gofmt -l .)" || { echo "✗ files outside gofmt (run 'make fmt'):"; gofmt -l .; exit 1; }
	@echo "✓ gofmt clean"

tidy: ## Update the go.mod of all modules
	@for m in $(MODULES); do echo "▶ tidy $$m"; (cd $$m && go mod tidy) || exit 1; done

quickstart: ## Start the minimal example on :8000
	cd examples && go run ./quickstart

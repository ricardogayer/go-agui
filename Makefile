# Makefile — go-agui
# O repositório tem quatro módulos (núcleo, ginx, model, examples), amarrados
# pelo go.work. Os alvos abaixo rodam em todos.

MODULOS := . ./ginx ./model ./examples

.DEFAULT_GOAL := help
.PHONY: help build test vet fmt lint tidy quickstart

help: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compila todos os módulos
	@for m in $(MODULOS); do echo "▶ build $$m"; (cd $$m && go build ./...) || exit 1; done

test: ## Roda os testes de todos os módulos
	@for m in $(MODULOS); do echo "▶ test $$m"; (cd $$m && go test ./...) || exit 1; done

vet: ## Roda go vet em todos os módulos
	@for m in $(MODULOS); do echo "▶ vet $$m"; (cd $$m && go vet ./...) || exit 1; done

fmt: ## Formata o código
	gofmt -w .

# lint NÃO depende de fmt de propósito: se formatasse antes de verificar, a
# verificação nunca falharia — e em CI um arquivo fora do gofmt passaria calado.
lint: vet ## Verifica formatação (sem corrigir) e roda o vet
	@test -z "$$(gofmt -l .)" || { echo "✗ arquivos fora do gofmt (rode 'make fmt'):"; gofmt -l .; exit 1; }
	@echo "✓ gofmt limpo"

tidy: ## Atualiza os go.mod de todos os módulos
	@for m in $(MODULOS); do echo "▶ tidy $$m"; (cd $$m && go mod tidy) || exit 1; done

quickstart: ## Sobe o exemplo mínimo em :8000
	cd examples && go run ./quickstart

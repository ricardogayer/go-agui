// Package model builds the Eino chat model from configuration or from the
// environment.
//
// It is a SEPARATE MODULE from the core, on purpose. Every provider drags in
// its own dependency tree and its own Eino version requirement — the ollama
// component, for example, declares eino v0.7.13 while the core pins v0.9.2. If
// everything lived in a single module, the version resolution of whoever uses
// the library would be held hostage by the most outdated provider, and a
// provider nobody uses could break the core.
//
// Whoever already has their own model.ToolCallingChatModel does not need this
// package: agui.New accepts any implementation.
//
// PT: Package model constrói o chat model do Eino a partir de configuração ou do
// ambiente.
//
// É um MÓDULO À PARTE do núcleo, de propósito. Cada provedor arrasta a própria
// árvore de dependências e a própria exigência de versão do Eino — o componente
// ollama, por exemplo, declara eino v0.7.13 enquanto o núcleo fixa v0.9.2. Se
// tudo morasse num módulo só, a resolução de versões de quem usa a biblioteca
// ficaria refém do provedor mais atrasado, e um provedor que ninguém usa
// poderia quebrar o núcleo.
//
// Quem já tem o próprio model.ToolCallingChatModel não precisa deste pacote:
// agui.New aceita qualquer implementação.
package model

import (
	"context"
	"fmt"
	"os"
	"strings"

	ollama "github.com/cloudwego/eino-ext/components/model/ollama"
	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// Supported providers.
//
// PT: Provedores suportados.
const (
	ProviderOpenAI = "openai"
	ProviderOllama = "ollama"
)

// Config describes the model to build.
//
// It is a narrow struct on purpose: a library should not receive the whole
// application's Config just to read a handful of fields.
//
// PT: Config descreve o modelo a construir.
//
// É um struct estreito de propósito: uma biblioteca não deve receber o Config
// da aplicação inteira só para ler meia dúzia de campos.
type Config struct {
	// Provider is "openai" or "ollama". Empty is equivalent to "openai".
	//
	// PT: Provider é "openai" ou "ollama". Vazio equivale a "openai".
	Provider string

	// Model is the model id passed on to the provider.
	//
	// PT: Model é o id do modelo repassado ao provedor.
	Model string

	// APIKey is the provider credential. Required for openai.
	//
	// PT: APIKey é a credencial do provedor. Obrigatória para openai.
	APIKey string

	// BaseURL overrides the endpoint. Useful for OpenAI-API-compatible gateways
	// and for a fake server in tests. For ollama, the default is
	// http://localhost:11434.
	//
	// PT: BaseURL sobrescreve o endpoint. Útil para gateways compatíveis com a API
	// da OpenAI e para um servidor falso em teste. Para ollama, o padrão é
	// http://localhost:11434.
	BaseURL string

	// ReasoningEffort is the Chat Completions `reasoning_effort`. Empty omits
	// the field and lets the provider apply the model's default.
	//
	// Why this is configurable: OpenAI's reasoning models refuse function tools
	// on /v1/chat/completions with the default effort —
	// "Function tools with reasoning_effort are not supported ... set
	// reasoning_effort to 'none'". An AG-UI agent passes EVERYTHING through
	// function tools, including the frontend ones, so without this key it
	// renders nothing. For non-reasoning models, leave it empty.
	//
	// PT: ReasoningEffort é o `reasoning_effort` do Chat Completions. Vazio omite o
	// campo e deixa o provedor aplicar o padrão do modelo.
	//
	// Por que isto é configurável: os modelos de raciocínio da OpenAI recusam
	// function tools em /v1/chat/completions com o esforço padrão —
	// "Function tools with reasoning_effort are not supported ... set
	// reasoning_effort to 'none'". Um agente AG-UI passa TUDO por function
	// tools, inclusive as de frontend, então sem esta chave ele não renderiza
	// nada. Para os modelos não-raciocinantes, deixe vazio.
	ReasoningEffort string
}

// Validate rejects configurations that would only fail on the first request.
// Failing at startup is the correct behavior for a container with a healthcheck.
//
// PT: Validate rejeita configurações que só falhariam na primeira requisição.
// Falhar na subida é o comportamento correto para um container com healthcheck.
func (c Config) Validate() error {
	switch c.provider() {
	case ProviderOpenAI:
		if c.APIKey == "" {
			return fmt.Errorf("model: OPENAI_API_KEY is required when MODEL_PROVIDER=openai")
		}
	case ProviderOllama:
		// Ollama runs locally and requires no credential.
		//
		// PT: Ollama roda local e não exige credencial.
	default:
		return fmt.Errorf("model: MODEL_PROVIDER=%q is not supported (use %q or %q)",
			c.Provider, ProviderOpenAI, ProviderOllama)
	}
	if c.Model == "" {
		return fmt.Errorf("model: MODEL must not be empty")
	}
	return nil
}

func (c Config) provider() string {
	p := strings.ToLower(strings.TrimSpace(c.Provider))
	if p == "" {
		return ProviderOpenAI
	}
	return p
}

// New builds the chat model described by cfg.
//
// PT: New constrói o chat model descrito por cfg.
func New(ctx context.Context, cfg Config) (model.ToolCallingChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.provider() {
	case ProviderOpenAI:
		conf := &openai.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
		}
		if cfg.ReasoningEffort != "" {
			conf.ReasoningEffort = openai.ReasoningEffortLevel(strings.ToLower(cfg.ReasoningEffort))
		}
		return openai.NewChatModel(ctx, conf)

	case ProviderOllama:
		conf := &ollama.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}
		if conf.BaseURL == "" {
			conf.BaseURL = "http://localhost:11434"
		}
		return ollama.NewChatModel(ctx, conf)
	}

	// Unreachable: Validate has already refused the rest.
	//
	// PT: Inalcançável: Validate já recusou o resto.
	return nil, fmt.Errorf("model: provider %q not supported", cfg.Provider)
}

// ConfigFromEnv reads the configuration from the environment.
//
// The variables are the same ones the Python backends read, so that an
// existing `.env` keeps working:
//
//	MODEL_PROVIDER            openai | ollama          (default: openai)
//	MODEL                     model id
//	OPENAI_API_KEY            credential (openai)
//	OPENAI_BASE_URL           alternative endpoint
//	OPENAI_REASONING_EFFORT   none, low, medium, high
//	OLLAMA_BASE_URL           ollama endpoint
//
// PT: ConfigFromEnv lê a configuração do ambiente.
//
// As variáveis são as mesmas que os backends Python liam, para que um `.env`
// existente continue valendo (ver a tabela acima).
func ConfigFromEnv() Config {
	cfg := Config{
		Provider:        os.Getenv("MODEL_PROVIDER"),
		Model:           os.Getenv("MODEL"),
		APIKey:          os.Getenv("OPENAI_API_KEY"),
		BaseURL:         os.Getenv("OPENAI_BASE_URL"),
		ReasoningEffort: os.Getenv("OPENAI_REASONING_EFFORT"),
	}
	if cfg.provider() == ProviderOllama {
		cfg.BaseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	return cfg
}

// FromEnv builds the chat model from the environment. It is main's shortcut:
//
//	m, err := model.FromEnv(ctx)
//
// PT: FromEnv constrói o chat model a partir do ambiente. É o atalho do main
// (ver o exemplo acima).
func FromEnv(ctx context.Context) (model.ToolCallingChatModel, error) {
	return New(ctx, ConfigFromEnv())
}

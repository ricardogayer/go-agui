// Package model constrói o chat model do Eino a partir de configuração ou do
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

// Provedores suportados.
const (
	ProviderOpenAI = "openai"
	ProviderOllama = "ollama"
)

// Config descreve o modelo a construir.
//
// É um struct estreito de propósito: uma biblioteca não deve receber o Config
// da aplicação inteira só para ler três campos.
type Config struct {
	// Provider é "openai" ou "ollama". Vazio equivale a "openai".
	Provider string

	// Model é o id do modelo repassado ao provedor.
	Model string

	// APIKey é a credencial do provedor. Obrigatória para openai.
	APIKey string

	// BaseURL sobrescreve o endpoint. Útil para gateways compatíveis com a API
	// da OpenAI e para um servidor falso em teste. Para ollama, o padrão é
	// http://localhost:11434.
	BaseURL string

	// ReasoningEffort é o `reasoning_effort` do Chat Completions. Vazio omite o
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

// Validate rejeita configurações que só falhariam na primeira requisição.
// Falhar na subida é o comportamento correto para um container com healthcheck.
func (c Config) Validate() error {
	switch c.provider() {
	case ProviderOpenAI:
		if c.APIKey == "" {
			return fmt.Errorf("model: OPENAI_API_KEY é obrigatório quando MODEL_PROVIDER=openai")
		}
	case ProviderOllama:
		// Ollama roda local e não exige credencial.
	default:
		return fmt.Errorf("model: MODEL_PROVIDER=%q não é suportado (use %q ou %q)",
			c.Provider, ProviderOpenAI, ProviderOllama)
	}
	if c.Model == "" {
		return fmt.Errorf("model: MODEL não pode ser vazio")
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

// New constrói o chat model descrito por cfg.
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

	// Inalcançável: Validate já recusou o resto.
	return nil, fmt.Errorf("model: provedor %q não suportado", cfg.Provider)
}

// ConfigFromEnv lê a configuração do ambiente.
//
// As variáveis são as mesmas que os backends Python liam, para que um `.env`
// existente continue valendo:
//
//	MODEL_PROVIDER            openai | ollama          (padrão: openai)
//	MODEL                     id do modelo
//	OPENAI_API_KEY            credencial (openai)
//	OPENAI_BASE_URL           endpoint alternativo
//	OPENAI_REASONING_EFFORT   none, low, medium, high
//	OLLAMA_BASE_URL           endpoint do ollama
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

// FromEnv constrói o chat model a partir do ambiente. É o atalho do main:
//
//	m, err := model.FromEnv(ctx)
func FromEnv(ctx context.Context) (model.ToolCallingChatModel, error) {
	return New(ctx, ConfigFromEnv())
}

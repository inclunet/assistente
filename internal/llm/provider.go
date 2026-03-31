package llm

import (
	"fmt"
	"strings"
)

// ProviderType representa o tipo de provedor LLM (label de marca).
//
// NOTE: valores são strings para facilitar serialização
// e compatibilidade com configs.
type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderClaude     ProviderType = "claude"
	ProviderGrok       ProviderType = "grok"
	ProviderDeepSeek   ProviderType = "deepseek"
	ProviderMistral    ProviderType = "mistral"
	ProviderGroq       ProviderType = "groq"
	ProviderTogether   ProviderType = "together"
	ProviderFireworks  ProviderType = "fireworks"
	ProviderPerplexity ProviderType = "perplexity"
	ProviderOllama     ProviderType = "ollama"
	ProviderCustom     ProviderType = "custom"
)

// APIFormat determina qual SDK/protocolo usar para comunicação com o provedor.
// Independente de ProviderType (que é apenas um label de marca).
type APIFormat string

const (
	APIFormatOpenAI    APIFormat = "openai"    // openai-go SDK (OpenAI, OpenRouter, Ollama, Together, Groq, etc)
	APIFormatAnthropic APIFormat = "anthropic" // anthropic-sdk-go SDK
	APIFormatGoogle    APIFormat = "google"    // google.golang.org/genai SDK
)

// ProviderConfig descreve um provedor LLM
// Usado pelo ProviderRegistry para inicialização do cliente.
type ProviderConfig struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Type              ProviderType      `json:"type"`
	APIFormat         APIFormat         `json:"api_format,omitempty"`
	BaseURL           string            `json:"base_url"`
	Model             string            `json:"model,omitempty"`
	DefaultModel      string            `json:"default_model,omitempty"`
	IsDefault         bool              `json:"is_default,omitempty"`
	Timeout           int               `json:"timeout,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	CredentialPattern string            `json:"credential_pattern,omitempty"`
}

// GetAPIFormat retorna o api_format efetivo, defaulting para "openai".
func (p *ProviderConfig) GetAPIFormat() APIFormat {
	if p.APIFormat != "" {
		return p.APIFormat
	}
	return APIFormatOpenAI
}

// Validate verifica se o ProviderConfig é válido
func (p *ProviderConfig) Validate() error {
	if p == nil {
		return fmt.Errorf("provider config nil")
	}
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.CredentialPattern = strings.TrimSpace(p.CredentialPattern)

	if p.ID == "" {
		return fmt.Errorf("provider id vazio")
	}
	if p.Name == "" {
		return fmt.Errorf("provider name vazio")
	}
	if p.BaseURL == "" {
		return fmt.Errorf("provider base_url vazio")
	}
	return nil
}

package llm

import (
	"fmt"
	"strings"
)

// ProviderType representa o tipo de provedor LLM
// Exemplos: openai, claude, grok, deepseek, ollama, custom
//
// NOTE: valores são strings para facilitar serialização
// e compatibilidade com configs.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderClaude    ProviderType = "claude"
	ProviderGrok      ProviderType = "grok"
	ProviderDeepSeek  ProviderType = "deepseek"
	ProviderOllama    ProviderType = "ollama"
	ProviderCustom    ProviderType = "custom"
)

// ProviderConfig descreve um provedor LLM
// Usado pelo ProviderRegistry para inicialização do cliente.
type ProviderConfig struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Type              ProviderType      `json:"type"`
	BaseURL           string            `json:"base_url"`
	Model             string            `json:"model,omitempty"`
	Timeout           int               `json:"timeout,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	CredentialPattern string            `json:"credential_pattern,omitempty"`
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

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
	ProviderLocalAI    ProviderType = "localai"
	ProviderLlamaCPP   ProviderType = "llamacpp"
	ProviderCustom     ProviderType = "custom"
)

// AuthMode descreve o tratamento de autenticação para o provedor.
//
// Modos:
//
//   - AuthModeRequired (default): a credencial é obrigatória. Ausência
//     dispara erro explícito ("credencial gerenciada não resolvida"),
//     evitando que o request vá para upstream sem chave e gere um 401
//     opaco. Este é o comportamento esperado para provedores cloud
//     (OpenAI, Anthropic, etc.).
//
//   - AuthModeOptional: a credencial pode ou não existir. Quando existe,
//     o transport injeta normalmente (Authorization header). Quando
//     ausente, o request segue sem header — útil para provedores que
//     suportam autenticação opcional (LocalAI, LiteLLM standalone,
//     Ollama com proxy custom).
//
//   - AuthModeNone: o provedor explicitamente não usa Authorization.
//     O SDK não injeta o placeholder "managed-by-credential-transport",
//     e o transport remove qualquer header Authorization residual.
//     Para Ollama/llama.cpp puros que rejeitam headers desconhecidos.
//
// O default vazio é tratado como AuthModeRequired apenas para CredentialPattern != "".
// Se CredentialPattern == "" e AuthMode == "", trata como AuthModeNone (compatibilidade
// com configs existentes onde "sem pattern" significava "sem auth").
type AuthMode string

const (
	AuthModeRequired AuthMode = "required"
	AuthModeOptional AuthMode = "optional"
	AuthModeNone     AuthMode = "none"
)

// APIFormat determina qual SDK/protocolo usar para comunicação com o provedor.
// Independente de ProviderType (que é apenas um label de marca).
//
// Formatos disponíveis:
//
//   - "openai" (APIFormatOpenAI / APIFormatOpenAICompatible):
//     Chat Completions API only (/v1/chat/completions).
//     Para provedores OpenAI-compatible: OpenRouter, Ollama, Groq, Together, etc.
//     NÃO suporta MCP nativo. Valor legado, default para configs sem api_format.
//
//   - "openai_responses" (APIFormatOpenAIResponses):
//     Responses API first (/v1/responses).
//     Para OpenAI real (api.openai.com). Suporta MCP nativo (type:mcp),
//     reasoning summaries, e todas as features modernas da plataforma OpenAI.
//     Inferido automaticamente quando BaseURL contém "api.openai.com".
//
//   - "anthropic": SDK anthropic-sdk-go. Suporta MCP nativo via Beta Messages API.
//
//   - "google": SDK google.golang.org/genai. NÃO suporta MCP nativo.
type APIFormat string

const (
	// APIFormatOpenAI é o formato Chat Completions only (/v1/chat/completions).
	// Valor wire: "openai". Usado para provedores OpenAI-compatible
	// (OpenRouter, Ollama, Groq, Together, LiteLLM, Azure, etc).
	// NÃO suporta MCP nativo nem Responses API.
	// Este é o valor default para configs legadas sem api_format explícito.
	APIFormatOpenAI APIFormat = "openai"

	// APIFormatOpenAICompatible é um alias semântico para APIFormatOpenAI.
	// Idêntico em valor wire ("openai"). Existe apenas para clareza em código novo —
	// deixa explícito que o provider usa Chat Completions por ser compatível/legado.
	APIFormatOpenAICompatible = APIFormatOpenAI

	// APIFormatOpenAIResponses é o formato Responses API first (/v1/responses).
	// Valor wire: "openai_responses". Para OpenAI real (api.openai.com).
	// Suporta MCP nativo (type:mcp), reasoning summaries, tool_choice, e
	// todas as features modernas. Inferido automaticamente quando BaseURL
	// contém "api.openai.com" e api_format não está definido.
	APIFormatOpenAIResponses APIFormat = "openai_responses"

	APIFormatAnthropic APIFormat = "anthropic" // anthropic-sdk-go SDK — suporta MCP nativo
	APIFormatGoogle    APIFormat = "google"    // google.golang.org/genai SDK — sem MCP nativo
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
	// AuthMode controla o tratamento de credenciais. Ver `AuthMode` para detalhes.
	// Vazio = inferido a partir de CredentialPattern (sem pattern → none, com pattern → required).
	AuthMode AuthMode `json:"auth_mode,omitempty"`
}

// EffectiveAuthMode devolve o AuthMode resolvido, aplicando a inferência
// de compat: configs antigas sem AuthMode tinham `CredentialPattern: ""`
// para indicar "sem auth" (caso ollama). Mantemos esse contrato.
func (p *ProviderConfig) EffectiveAuthMode() AuthMode {
	if p == nil {
		return AuthModeRequired
	}
	if p.AuthMode != "" {
		return p.AuthMode
	}
	if strings.TrimSpace(p.CredentialPattern) == "" {
		return AuthModeNone
	}
	return AuthModeRequired
}

// GetAPIFormat retorna o api_format efetivo.
//
// Precedência:
//  1. APIFormat explícito (se definido) — respeitado incondicionalmente.
//  2. Inferência por URL: api.openai.com → APIFormatOpenAIResponses.
//  3. Default conservador: APIFormatOpenAI (Chat Completions only).
//
// A inferência por URL garante que providers OpenAI reais criados antes
// da introdução de api_format usem automaticamente a Responses API,
// sem exigir migração manual de configs existentes.
func (p *ProviderConfig) GetAPIFormat() APIFormat {
	if p.APIFormat != "" {
		return p.APIFormat
	}
	if isOpenAIRealURL(p.BaseURL) {
		return APIFormatOpenAIResponses
	}
	return APIFormatOpenAI
}

// isOpenAIRealURL retorna true se a URL aponta para a API oficial da OpenAI.
func isOpenAIRealURL(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSuffix(baseURL, "/"))
	return strings.Contains(normalized, "api.openai.com")
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

// SupportsTTS retorna true se o SDK do provider suporta síntese de voz (TTS).
// Atualmente apenas o SDK OpenAI (formatos openai e openai_responses) tem endpoint /audio/speech.
func (p *ProviderConfig) SupportsTTS() bool {
	f := p.GetAPIFormat()
	return f == APIFormatOpenAI || f == APIFormatOpenAIResponses
}

// SupportsSTT retorna true se o SDK do provider suporta transcrição de voz (STT/Whisper).
// Atualmente apenas o SDK OpenAI (formatos openai e openai_responses) tem endpoint /audio/transcriptions.
func (p *ProviderConfig) SupportsSTT() bool {
	f := p.GetAPIFormat()
	return f == APIFormatOpenAI || f == APIFormatOpenAIResponses
}

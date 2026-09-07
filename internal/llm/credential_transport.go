package llm

import (
	"net/http"
	"time"

	"assistente/internal/credentials"
)

// newHTTPClientForProvider cria um http.Client com CredentialTransport configurado.
// Delega ao pacote credentials para injeção automática de credenciais.
// Respeita o EffectiveAuthMode do provider — providers locais (Ollama,
// LocalAI, llama.cpp) marcados como AuthModeNone geram um cliente que NÃO
// envia o header Authorization placeholder ao upstream.
func newHTTPClientForProvider(provider *ProviderConfig, credMgr *credentials.Manager) *http.Client {
	mode := credentialAuthRequirement(provider)
	return credentials.NewHTTPClientWithAuthMode(credMgr, provider.CredentialPattern, mode, providerTimeout(provider))
}

// newStreamingHTTPClientForProvider cria o http.Client dedicado a streaming
// SSE. Sem Timeout global (que cortava streams ativos aos 3 min); ver
// credentials.NewStreamingHTTPClientWithAuthMode.
func newStreamingHTTPClientForProvider(provider *ProviderConfig, credMgr *credentials.Manager) *http.Client {
	mode := credentialAuthRequirement(provider)
	return credentials.NewStreamingHTTPClientWithAuthMode(credMgr, provider.CredentialPattern, mode)
}

// credentialAuthRequirement converte llm.AuthMode em credentials.AuthRequirement.
// Necessário porque o pacote credentials não pode importar llm (ciclo).
func credentialAuthRequirement(p *ProviderConfig) credentials.AuthRequirement {
	switch p.EffectiveAuthMode() {
	case AuthModeNone:
		return credentials.AuthNone
	case AuthModeOptional:
		return credentials.AuthOptional
	default:
		return credentials.AuthRequired
	}
}

// providerUsesPlaceholderAPIKey indica se o SDK deve injetar o placeholder
// "managed-by-credential-transport" para que o transport substitua pelo
// token real. Para AuthModeNone não injetamos — assim mesmo que o transport
// falhe em remover, nenhum header espúrio é gerado pelo SDK.
func providerUsesPlaceholderAPIKey(p *ProviderConfig) bool {
	return p.EffectiveAuthMode() != AuthModeNone
}

func providerTimeout(p *ProviderConfig) time.Duration {
	if p.Timeout > 0 {
		return time.Duration(p.Timeout) * time.Second
	}
	return 3 * time.Minute
}

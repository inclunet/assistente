package llm

import (
	"fmt"
	"net/http"
	"strings"
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
	client := credentials.NewHTTPClientWithAuthMode(credMgr, provider.CredentialPattern, mode, providerTimeout(provider))
	client.CheckRedirect = sameOriginProviderRedirect
	return client
}

// newStreamingHTTPClientForProvider cria o http.Client dedicado a streaming
// SSE. Sem Timeout global (que cortava streams ativos aos 3 min); ver
// credentials.NewStreamingHTTPClientWithAuthMode.
func newStreamingHTTPClientForProvider(provider *ProviderConfig, credMgr *credentials.Manager) *http.Client {
	mode := credentialAuthRequirement(provider)
	client := credentials.NewStreamingHTTPClientWithAuthMode(credMgr, provider.CredentialPattern, mode)
	client.CheckRedirect = sameOriginProviderRedirect
	return client
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

// O SDK e o transport podem reaplicar segredos a cada request. Redirecionamentos
// precisam permanecer na origem inicial, inclusive para listagem de modelos.
func sameOriginProviderRedirect(next *http.Request, via []*http.Request) error {
	if len(via) == 0 || !strings.EqualFold(next.URL.Scheme, via[0].URL.Scheme) || !strings.EqualFold(next.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("redirecionamento para outra origem recusado pelo provedor")
	}
	if len(via) >= 10 {
		return fmt.Errorf("limite de redirecionamentos excedido")
	}
	return nil
}

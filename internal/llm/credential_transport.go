package llm

import (
	"net/http"
	"time"

	"assistente/internal/credentials"
)

// newHTTPClientForProvider cria um http.Client com CredentialTransport configurado.
// Delega ao pacote credentials para injeção automática de credenciais.
func newHTTPClientForProvider(provider *ProviderConfig, credMgr *credentials.Manager) *http.Client {
	return credentials.NewHTTPClient(credMgr, provider.CredentialPattern, providerTimeout(provider))
}

func providerTimeout(p *ProviderConfig) time.Duration {
	if p.Timeout > 0 {
		return time.Duration(p.Timeout) * time.Second
	}
	return 3 * time.Minute
}

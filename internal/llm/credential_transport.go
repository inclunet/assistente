package llm

import (
	"net/http"
	"strings"
	"time"

	"assistente/internal/credentials"
)

// credentialTransport é um http.RoundTripper que injeta credenciais do credMgr
// nos requests HTTP. Projetado para uso com SDKs oficiais (openai-go, anthropic-sdk-go, etc)
// que aceitam http.Client customizado.
type credentialTransport struct {
	base        http.RoundTripper
	credMgr     *credentials.Manager
	credPattern string // padrão para lookup no credMgr (ex: "api.openai.com")
}

// newCredentialTransport cria um transport que injeta credenciais automaticamente.
// credPattern é o padrão registrado no credMgr (ProviderConfig.CredentialPattern).
func newCredentialTransport(credMgr *credentials.Manager, credPattern string) *credentialTransport {
	return &credentialTransport{
		base:        http.DefaultTransport,
		credMgr:     credMgr,
		credPattern: credPattern,
	}
}

func (t *credentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.credMgr == nil || t.credPattern == "" {
		return t.base.RoundTrip(req)
	}

	auth, err := t.credMgr.GetByPattern(t.credPattern)
	if err != nil || auth == nil {
		return t.base.RoundTrip(req)
	}

	switch auth.Type {
	case "bearer":
		if auth.Token != "" {
			if strings.HasPrefix(auth.Token, "Bearer ") {
				req.Header.Set("Authorization", auth.Token)
			} else {
				req.Header.Set("Authorization", "Bearer "+auth.Token)
			}
		}
	case "basic":
		if auth.Username != "" && auth.Password != "" {
			req.SetBasicAuth(auth.Username, auth.Password)
		}
	case "custom":
		for key, val := range auth.Headers {
			req.Header.Set(key, val)
		}
	}

	return t.base.RoundTrip(req)
}

// newHTTPClientForProvider cria um http.Client com credentialTransport configurado.
func newHTTPClientForProvider(provider *ProviderConfig, credMgr *credentials.Manager) *http.Client {
	return &http.Client{
		Transport: newCredentialTransport(credMgr, provider.CredentialPattern),
		Timeout:   providerTimeout(provider),
	}
}

func providerTimeout(p *ProviderConfig) time.Duration {
	if p.Timeout > 0 {
		return time.Duration(p.Timeout) * time.Second
	}
	return 3 * time.Minute
}

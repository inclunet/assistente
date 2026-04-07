package credentials

import (
	"net/http"
	"strings"
	"time"
)

// CredentialTransport ├® um http.RoundTripper que injeta credenciais do Manager
// nos requests HTTP. Projetado para uso com SDKs oficiais (openai-go, etc)
// que aceitam http.Client customizado.
type CredentialTransport struct {
	Base        http.RoundTripper
	CredMgr     *Manager
	CredPattern string // padr├úo para lookup no credMgr (ex: "api.openai.com")
}

// NewCredentialTransport cria um transport que injeta credenciais automaticamente.
// credPattern ├® o padr├úo registrado no Manager (ex: "api.openai.com").
func NewCredentialTransport(credMgr *Manager, credPattern string) *CredentialTransport {
	return &CredentialTransport{
		Base:        http.DefaultTransport,
		CredMgr:     credMgr,
		CredPattern: credPattern,
	}
}

func (t *CredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.CredMgr == nil || t.CredPattern == "" {
		return t.Base.RoundTrip(req)
	}

	auth, err := t.CredMgr.GetByPattern(t.CredPattern)
	if err != nil || auth == nil {
		return t.Base.RoundTrip(req)
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

	return t.Base.RoundTrip(req)
}

// NewHTTPClient cria um http.Client configurado com CredentialTransport.
func NewHTTPClient(credMgr *Manager, credPattern string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewCredentialTransport(credMgr, credPattern),
		Timeout:   timeout,
	}
}

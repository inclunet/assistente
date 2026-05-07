package credentials

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"assistente/internal/database"
)

const managedCredentialPlaceholder = "managed-by-credential-transport"

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
	if t.CredPattern == "" {
		return t.Base.RoundTrip(req)
	}
	if t.CredMgr == nil {
		if hasManagedCredentialPlaceholder(req) {
			return nil, unresolvedCredentialError(req, t.CredPattern)
		}
		return t.Base.RoundTrip(req)
	}

	auth, err := t.CredMgr.GetByPatternWithContext(req.Context(), t.CredPattern)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		if hasManagedCredentialPlaceholder(req) {
			return nil, unresolvedCredentialError(req, t.CredPattern)
		}
		return t.Base.RoundTrip(req)
	}

	switch auth.Type {
	case "bearer":
		if strings.TrimSpace(auth.Token) == "" {
			if hasManagedCredentialPlaceholder(req) {
				return nil, unresolvedCredentialError(req, t.CredPattern)
			}
			break
		}
		if strings.HasPrefix(auth.Token, "Bearer ") {
			req.Header.Set("Authorization", auth.Token)
		} else {
			req.Header.Set("Authorization", "Bearer "+auth.Token)
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

func hasManagedCredentialPlaceholder(req *http.Request) bool {
	if req == nil {
		return false
	}
	return strings.Contains(req.Header.Get("Authorization"), managedCredentialPlaceholder)
}

func unresolvedCredentialError(req *http.Request, pattern string) error {
	userID := ""
	if req != nil {
		if scopedUserID, ok := database.UserIDFromContext(req.Context()); ok {
			userID = scopedUserID
		}
	}
	if strings.TrimSpace(userID) == "" {
		userID = "<sem usuario autenticado>"
	}
	if strings.TrimSpace(pattern) == "" {
		pattern = "<sem credential_pattern>"
	}
	return fmt.Errorf("credencial gerenciada não resolvida para pattern %q e usuário %q", pattern, userID)
}

// NewHTTPClient cria um http.Client configurado com CredentialTransport.
func NewHTTPClient(credMgr *Manager, credPattern string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewCredentialTransport(credMgr, credPattern),
		Timeout:   timeout,
	}
}

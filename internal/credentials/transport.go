package credentials

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"assistente/internal/database"
)

const managedCredentialPlaceholder = "managed-by-credential-transport"

// AuthRequirement classifica o comportamento desejado do transport quando
// a credencial associada ao pattern não pode ser resolvida.
//
// Espelha llm.AuthMode mas vive aqui para evitar ciclo de imports
// (credentials → llm). O conversor está em internal/llm/http_client.go.
type AuthRequirement int

const (
	// AuthRequired (default): ausência de credencial dispara erro
	// "credencial gerenciada não resolvida". Para provedores cloud que
	// vão devolver 401/403 sem header Authorization.
	AuthRequired AuthRequirement = iota
	// AuthOptional: credencial é injetada se existir; ausência segue
	// adiante sem erro e sem header. Para provedores que aceitam auth
	// opcional (LocalAI, LiteLLM standalone, Ollama com proxy custom).
	AuthOptional
	// AuthNone: provedor explicitamente sem auth. Transport remove
	// header Authorization residual antes de enviar (defesa contra
	// servidores estritos que rejeitam Bearer desconhecido).
	AuthNone
)

// CredentialTransport ├® um http.RoundTripper que injeta credenciais do Manager
// nos requests HTTP. Projetado para uso com SDKs oficiais (openai-go, etc)
// que aceitam http.Client customizado.
type CredentialTransport struct {
	Base        http.RoundTripper
	CredMgr     *Manager
	CredPattern string // padr├úo para lookup no credMgr (ex: "api.openai.com")
	// AuthMode classifica como tratar ausência de credencial. Default
	// (zero value) = AuthRequired, mantendo o comportamento histórico
	// para todos os providers cloud.
	AuthMode AuthRequirement
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

// NewCredentialTransportWithMode cria um transport com modo de auth explícito.
func NewCredentialTransportWithMode(credMgr *Manager, credPattern string, mode AuthRequirement) *CredentialTransport {
	return &CredentialTransport{
		Base:        http.DefaultTransport,
		CredMgr:     credMgr,
		CredPattern: credPattern,
		AuthMode:    mode,
	}
}

func (t *CredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// AuthNone: nunca tenta resolver credencial e remove qualquer
	// Authorization residual (placeholder do SDK ou inadvertidamente
	// injetado por upstream wrappers). Isso garante que provedores
	// puramente locais (Ollama, llama.cpp) recebam um request limpo.
	if t.AuthMode == AuthNone {
		stripManagedPlaceholder(req)
		return t.Base.RoundTrip(req)
	}

	if t.CredPattern == "" {
		// Sem pattern + AuthMode != none: dada a inferência feita em
		// EffectiveAuthMode (sem pattern → AuthNone), este caminho só
		// existe quando o caller construiu o transport diretamente
		// sem mode. Compat: passa direto, removendo placeholder.
		stripManagedPlaceholder(req)
		return t.Base.RoundTrip(req)
	}
	if t.CredMgr == nil {
		if t.AuthMode == AuthOptional {
			stripManagedPlaceholder(req)
			return t.Base.RoundTrip(req)
		}
		if hasManagedCredentialPlaceholder(req) {
			return nil, unresolvedCredentialError(req, t.CredPattern)
		}
		return t.Base.RoundTrip(req)
	}

	auth, err := t.CredMgr.GetByPatternWithContext(req.Context(), t.CredPattern)
	if err != nil {
		if t.AuthMode == AuthOptional {
			// AuthOptional + erro de resolução: tratamos como "sem
			// credencial" (segue adiante sem header). Erro silencioso
			// no transport mas o provedor responderá 401 se exigir.
			stripManagedPlaceholder(req)
			return t.Base.RoundTrip(req)
		}
		return nil, err
	}
	if auth == nil {
		if t.AuthMode == AuthOptional {
			stripManagedPlaceholder(req)
			return t.Base.RoundTrip(req)
		}
		if hasManagedCredentialPlaceholder(req) {
			return nil, unresolvedCredentialError(req, t.CredPattern)
		}
		return t.Base.RoundTrip(req)
	}

	switch auth.Type {
	case "bearer":
		if strings.TrimSpace(auth.Token) == "" {
			if t.AuthMode == AuthOptional {
				stripManagedPlaceholder(req)
				break
			}
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

// stripManagedPlaceholder remove o header Authorization se ele contiver o
// placeholder injetado pelos SDKs (openai-go, anthropic-sdk-go). Sem essa
// limpeza, providers locais estritos recebem `Authorization: Bearer
// managed-by-credential-transport` e respondem 401 com mensagem opaca.
func stripManagedPlaceholder(req *http.Request) {
	if req == nil {
		return
	}
	if hasManagedCredentialPlaceholder(req) {
		req.Header.Del("Authorization")
	}
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

// NewHTTPClientWithAuthMode cria um http.Client respeitando o modo de auth.
func NewHTTPClientWithAuthMode(credMgr *Manager, credPattern string, mode AuthRequirement, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewCredentialTransportWithMode(credMgr, credPattern, mode),
		Timeout:   timeout,
	}
}

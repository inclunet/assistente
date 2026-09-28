package credentials

import (
	"fmt"
	"maps"
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
	DisableCommandCache bool // SDKs que capturam uma chave fora deste transport
	Base                http.RoundTripper
	CredMgr             *Manager
	CredPattern         string // padr├úo para lookup no credMgr (ex: "api.openai.com")
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

// CloseIdleConnections preserva o contrato de limpeza do cliente encapsulado.
func (t *CredentialTransport) CloseIdleConnections() {
	if base, ok := t.Base.(interface{ CloseIdleConnections() }); ok {
		base.CloseIdleConnections()
	}
}

func (t *CredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// AuthNone: nunca tenta resolver credencial e remove qualquer
	// Authorization residual (placeholder do SDK ou inadvertidamente
	// injetado por upstream wrappers). Isso garante que provedores
	// puramente locais (Ollama, llama.cpp) recebam um request limpo.
	if t.AuthMode == AuthNone {
		req = req.Clone(req.Context())
		req.Header.Del("Authorization")
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

	auth, err := t.CredMgr.getByPatternWithContext(req.Context(), t.CredPattern, !t.DisableCommandCache)
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

	if t.AuthMode == AuthOptional && auth.Type == "bearer" && strings.TrimSpace(auth.Token) == "" {
		stripManagedPlaceholder(req)
		return t.Base.RoundTrip(req)
	}

	first := req.Clone(req.Context())
	if err := ApplyAuth(first, auth); err != nil {
		return nil, err
	}
	response, err := t.Base.RoundTrip(first)
	if err != nil || response == nil || response.StatusCode != http.StatusUnauthorized || !t.CredMgr.rejectCommandCredential(auth) {
		return response, err
	}
	// Só repetimos uma rejeição explícita de autenticação, nunca falhas de rede,
	// 403 ou streaming já iniciado. Corpos de upload sem GetBody não são repetidos.
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return response, nil
	}
	fresh, err := t.CredMgr.getByPatternWithContext(req.Context(), t.CredPattern, !t.DisableCommandCache)
	if err != nil {
		_ = response.Body.Close()
		return nil, err
	}
	if fresh == nil || fresh.commandEntry != auth.commandEntry {
		return response, nil
	}
	if sameHTTPAuth(auth, fresh) {
		// Preserve a geração renovada para compartilhar o resultado com 401s
		// antigos concorrentes. Uma rejeição dessa nova geração pode renová-la.
		return response, nil
	}
	retry := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		retry.Body, err = req.GetBody()
		if err != nil {
			return response, nil
		}
	}
	if err := ApplyAuth(retry, fresh); err != nil {
		if retry.Body != nil {
			_ = retry.Body.Close()
		}
		_ = response.Body.Close()
		return nil, err
	}
	_ = response.Body.Close()
	response, err = t.Base.RoundTrip(retry)
	if err == nil && response != nil && response.StatusCode == http.StatusUnauthorized {
		t.CredMgr.rejectCommandCredential(fresh)
	}
	return response, err
}

func sameHTTPAuth(a, b *AuthConfig) bool {
	return a.Type == b.Type && a.Token == b.Token && a.Username == b.Username &&
		a.Password == b.Password && maps.Equal(a.Headers, b.Headers)
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

// streamingResponseHeaderTimeout limita quanto tempo se espera pelas
// primeiras linhas de cabeçalho do upstream em requests de streaming SSE.
// Cobre conexão lenta/servidor pendurado antes da resposta, sem colocar um
// teto sobre o corpo — que num stream saudável pode durar minutos.
const streamingResponseHeaderTimeout = 30 * time.Second

// NewStreamingHTTPClientWithAuthMode cria um http.Client para streaming SSE
// (AEP-0010). Diferente de NewHTTPClientWithAuthMode, NÃO define Timeout
// global no client: esse timeout cobria a leitura inteira do body e matava
// streams longos porém ativos no meio. O controle fica por:
//   - ResponseHeaderTimeout no Transport (conexão/cabeçalho travados);
//   - contexto da request (cancelamento pelo usuário e idle watchdog do
//     provider, que detecta servidor que para de enviar sem fechar).
func NewStreamingHTTPClientWithAuthMode(credMgr *Manager, credPattern string, mode AuthRequirement) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ResponseHeaderTimeout = streamingResponseHeaderTimeout
	transport := NewCredentialTransportWithMode(credMgr, credPattern, mode)
	transport.Base = base
	return &http.Client{Transport: transport}
}

// NewHTTPClientWithAuthMode cria um http.Client respeitando o modo de auth.
func NewHTTPClientWithAuthMode(credMgr *Manager, credPattern string, mode AuthRequirement, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewCredentialTransportWithMode(credMgr, credPattern, mode),
		Timeout:   timeout,
	}
}

// ApplyAuth applies an already materialized credential to a request.
func ApplyAuth(req *http.Request, auth *AuthConfig) error {
	if auth == nil {
		return nil
	}
	stripManagedPlaceholder(req)
	switch auth.Type {
	case "none":
		req.Header.Del("Authorization")
	case "bearer":
		if strings.TrimSpace(auth.Token) == "" {
			return fmt.Errorf("token de credencial vazio")
		}
		token := auth.Token
		if !strings.HasPrefix(token, "Bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	case "basic":
		req.SetBasicAuth(auth.Username, auth.Password)
	case "custom":
		for k, v := range auth.Headers {
			req.Header.Set(k, v)
		}
	default:
		return fmt.Errorf("scheme de credencial não suportado para HTTP")
	}
	return nil
}

package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"assistente/internal/credentials"
)

// Config define a configuração do cliente HTTP centralizado
type Config struct {
	CredentialManager *credentials.Manager
	RetryPolicy       *RetryPolicy
	Timeout           time.Duration
	LogFn             func(msg string) // opcional
	// Authorizer, quando presente, é consultado se uma request for barrada pela
	// política anti-SSRF (BlockedIPError). Permite o fluxo de consentimento +
	// allowlist e a reexecução da request. nil => comportamento hard-deny padrão.
	Authorizer NetworkAuthorizer
}

// RetryPolicy define política de retry
type RetryPolicy struct {
	MaxAttempts       int
	BackoffMultiplier float64
	InitialBackoff    time.Duration
}

// Client é o cliente HTTP centralizado com interceptor de autenticação
type Client struct {
	baseClient     *http.Client
	credMgr        *credentials.Manager
	retryPolicy    *RetryPolicy
	logFn          func(msg string)
	domainPatterns map[string]string // domínio → padrão de credencial
	authorizer     NetworkAuthorizer // opcional: fluxo de autorização anti-SSRF
}

// New cria um novo cliente HTTP centralizado
func New(cfg *Config, domainPatterns map[string]string) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RetryPolicy == nil {
		cfg.RetryPolicy = &RetryPolicy{
			MaxAttempts:       1,
			BackoffMultiplier: 2,
			InitialBackoff:    100 * time.Millisecond,
		}
	}
	if cfg.LogFn == nil {
		cfg.LogFn = func(msg string) {} // no-op
	}

	return &Client{
		baseClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		credMgr:        cfg.CredentialManager,
		retryPolicy:    cfg.RetryPolicy,
		logFn:          cfg.LogFn,
		domainPatterns: domainPatterns,
		authorizer:     cfg.Authorizer,
	}
}

// SetNetworkAuthorizer instala (ou substitui) o authorizer anti-SSRF depois da
// construção. Usado pelo wiring de app, que só monta o authorizer (com UI de
// consentimento + store de allowlist) após o Client já existir.
func (c *Client) SetNetworkAuthorizer(a NetworkAuthorizer) {
	c.authorizer = a
}

// Do executa uma requisição HTTP com interceptação de autenticação. Este é o
// ponto comum de enforcement de network scope para tools que usam o cliente
// compartilhado (web_fetch, http_request, web_search/feed).
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.URL == nil {
		return nil, fmt.Errorf("request URL cannot be nil")
	}
	if err := ValidateNetworkScope(ctx, req.URL.Hostname()); err != nil {
		return nil, err
	}

	// Aplicar autenticação. O vazamento de credenciais em redirects que cruzam um
	// limite de confiança é tratado pelo RedirectGuard (allowlist deny-by-default),
	// que cobre tanto estes headers quanto os custom passados pelo chamador.
	c.applyAuth(ctx, req)

	// Executar com retry
	resp, err := c.doWithRetry(ctx, req)
	if err == nil {
		return resp, nil
	}

	// Fluxo anti-SSRF: se o guard pós-DNS barrou o destino e há um authorizer,
	// oferecemos consentimento/allowlist e, se autorizado, reexecutamos a request
	// com os IPs liberados. Caso contrário, devolvemos um erro ACIONÁVEL (não o
	// BlockedIPError seco), preservando a detecção do bloqueio por padrão.
	var blocked *BlockedIPError
	if !errors.As(err, &blocked) {
		return nil, err
	}
	return c.handleBlocked(ctx, req, blocked)
}

// handleBlocked orquestra a autorização de um destino barrado por anti-SSRF.
func (c *Client) handleBlocked(ctx context.Context, req *http.Request, blocked *BlockedIPError) (*http.Response, error) {
	dest := c.buildBlockedDestination(ctx, req, blocked)

	if c.authorizer == nil {
		return nil, &BlockedDestinationError{
			Host:        dest.Host,
			URL:         dest.URL,
			IPs:         dest.IPs,
			Category:    dest.Category,
			Reason:      dest.Reason,
			Suggestions: defaultBlockSuggestions,
		}
	}

	trustedIPs, ok, aerr := c.authorizer.Authorize(ctx, dest)
	if aerr != nil {
		return nil, fmt.Errorf("falha ao solicitar autorização de rede: %w", aerr)
	}
	if !ok || len(trustedIPs) == 0 {
		return nil, &BlockedDestinationError{
			Host:        dest.Host,
			URL:         dest.URL,
			IPs:         dest.IPs,
			Category:    dest.Category,
			Reason:      dest.Reason,
			Suggestions: defaultBlockSuggestions,
		}
	}

	// Reexecuta com o trust por-request. Reseta o body quando disponível
	// (http.NewRequestWithContext popula GetBody para bodies comuns).
	if err := resetRequestBody(req); err != nil {
		return nil, fmt.Errorf("não foi possível reexecutar a request após autorização: %w", err)
	}
	trustedCtx := WithTrustedIPs(ctx, trustedIPs)
	return c.doWithRetry(trustedCtx, req)
}

// buildBlockedDestination resolve os IPs do host e classifica o bloqueio para
// montar o pedido de autorização / a mensagem de erro. Sempre inclui o IP que o
// guard reportou (blocked.IP); complementa com a resolução completa quando
// possível, registrando o(s) IP(s) real(is) — nunca confia só no hostname.
func (c *Client) buildBlockedDestination(ctx context.Context, req *http.Request, blocked *BlockedIPError) BlockedDestination {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		switch req.URL.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}

	category := ClassifyDestination(host, blocked.IP)
	ips := resolveHostIPs(ctx, host)
	if len(ips) == 0 && blocked.IP != nil {
		ips = []net.IP{blocked.IP}
	}

	return BlockedDestination{
		Host:     host,
		Port:     port,
		URL:      req.URL.Redacted(),
		IPs:      ips,
		Category: category,
		Reason:   fmt.Sprintf("%s address blocked by anti-SSRF policy", category),
	}
}

// resolveHostIPs resolve os IPs de um host respeitando o ctx. Se o host já for um
// IP literal, devolve-o direto. Falhas de DNS não são fatais (o chamador tem
// fallback para o IP reportado pelo guard).
func resolveHostIPs(ctx context.Context, host string) []net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips
}

// resetRequestBody reposiciona o body para uma reexecução. Sem body (GET/HEAD),
// é no-op. Com body, usa GetBody (populado pelo net/http para readers conhecidos).
func resetRequestBody(req *http.Request) error {
	if req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	if req.GetBody == nil {
		return errors.New("body não é reproduzível (GetBody ausente)")
	}
	body, err := req.GetBody()
	if err != nil {
		return err
	}
	req.Body = body
	return nil
}

// applyAuth aplica autenticação baseada no domínio da requisição
func (c *Client) applyAuth(ctx context.Context, req *http.Request) {
	if c.credMgr == nil {
		return
	}

	domain := req.URL.Host
	if domain == "" {
		return
	}

	// Se já tem Authorization explícito, não sobrescrever
	if req.Header.Get("Authorization") != "" {
		return
	}

	var auth *credentials.AuthConfig

	// 1) Busca por domainPatterns (mapeamento explícito)
	pattern := ""
	if p, ok := c.domainPatterns[domain]; ok {
		pattern = p
	} else if p, ok := c.domainPatterns["*"]; ok {
		pattern = p
	}

	if pattern != "" {
		resolved, err := c.credMgr.GetByPatternWithContext(ctx, pattern)
		if err == nil {
			auth = resolved
		}
	}

	// 2) Fallback: resolve por URL (regex/wildcard matching no credential manager)
	if auth == nil {
		resolved, err := c.credMgr.ResolveForURLWithContext(ctx, req.URL.String())
		if err == nil {
			auth = resolved
		}
	}

	if auth == nil {
		return
	}

	switch auth.Type {
	case "bearer":
		if auth.Token != "" {
			if !strings.HasPrefix(auth.Token, "Bearer ") {
				req.Header.Set("Authorization", "Bearer "+auth.Token)
			} else {
				req.Header.Set("Authorization", auth.Token)
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
}

// doWithRetry executa requisição com lógica de retry
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	backoff := c.retryPolicy.InitialBackoff

	for attempt := 0; attempt < c.retryPolicy.MaxAttempts; attempt++ {
		if attempt > 0 {
			c.logFn(fmt.Sprintf("Retry attempt %d after %v", attempt, backoff))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			backoff = time.Duration(float64(backoff) * c.retryPolicy.BackoffMultiplier)
		}

		resp, err := c.baseClient.Do(req.WithContext(ctx))
		if err != nil {
			lastErr = err
			// Se for erro de rede, pode tentar retry
			if isRetryableError(err) && attempt < c.retryPolicy.MaxAttempts-1 {
				continue
			}
			return nil, err
		}

		// Se status é sucesso ou erro do cliente, retornar sem retry
		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Status 5xx, pode tentar retry
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
	}

	return nil, lastErr
}

// isRetryableError determina se um erro deve resultar em retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Erros de rede/timeout são retentáveis
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset")
}

// AddDomainPattern adiciona um novo mapeamento domínio → padrão de credencial
func (c *Client) AddDomainPattern(domain, pattern string) {
	if c.domainPatterns == nil {
		c.domainPatterns = make(map[string]string)
	}
	c.domainPatterns[domain] = pattern
}

// GetBaseClient retorna o cliente HTTP base (para casos onde interceptação não é necessária)
func (c *Client) GetBaseClient() *http.Client {
	return c.baseClient
}

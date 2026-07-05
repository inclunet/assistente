package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"assistente/internal/credentials"
)

// redirectTracker sinaliza se uma request seguiu ao menos um redirect. É
// compartilhado por ponteiro no context para que o RedirectGuard (invocado
// dentro do net/http) possa marcar e o Client ler depois que Do retorna. Como o
// net/http chama CheckRedirect de forma síncrona no mesmo fluxo, não há
// concorrência real, mas usamos atomic por segurança com o race detector.
type redirectTracker struct{ redirected atomic.Bool }

type redirectTrackerKey struct{}

func withRedirectTracker(ctx context.Context) context.Context {
	return context.WithValue(ctx, redirectTrackerKey{}, &redirectTracker{})
}

// markRedirected marca que a request seguiu um redirect (chamado pelo guard).
func markRedirected(ctx context.Context) {
	if rt, ok := ctx.Value(redirectTrackerKey{}).(*redirectTracker); ok {
		rt.redirected.Store(true)
	}
}

// didRedirect reporta se a request associada ao ctx seguiu algum redirect.
func didRedirect(ctx context.Context) bool {
	rt, ok := ctx.Value(redirectTrackerKey{}).(*redirectTracker)
	return ok && rt.redirected.Load()
}

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

	// Rastreia se a request seguirá redirects: handleBlocked usa isso para saber
	// se um bloqueio veio do alvo direto (sem redirect) ou de um salto de redirect.
	ctx = withRedirectTracker(ctx)

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
	// Apenas o destino DIRETAMENTE requisitado pode ser autorizado por
	// consentimento. Se a request já seguiu ao menos um redirect, o dial inicial
	// teve sucesso e este bloqueio veio de um salto de redirect (URL inicial
	// pública → host que só revela IP privado no dial): NÃO abrimos prompt — seria
	// um vetor de open-redirect→SSRF, induzindo o usuário a aprovar um destino
	// interno que não escolheu. O sinal é confiável e não depende de re-resolução
	// DNS. Devolvemos erro acionável sem atribuir o IP interno ao host público.
	if didRedirect(ctx) {
		return nil, redirectBlockedError(blocked.IP)
	}

	dest := c.buildBlockedDestination(ctx, req, blocked)

	if c.authorizer == nil {
		return nil, newBlockedDestinationError(dest)
	}

	trustedIPs, ok, aerr := c.authorizer.Authorize(ctx, dest)
	if aerr != nil {
		return nil, fmt.Errorf("falha ao solicitar autorização de rede: %w", aerr)
	}
	if !ok || len(trustedIPs) == 0 {
		return nil, newBlockedDestinationError(dest)
	}

	// Reexecuta com o trust por-request. Reseta o body quando disponível
	// (http.NewRequestWithContext popula GetBody para bodies comuns).
	if err := resetRequestBody(req); err != nil {
		return nil, fmt.Errorf("não foi possível reexecutar a request após autorização: %w", err)
	}
	trustedCtx := withRedirectTracker(WithTrustedIPs(ctx, trustedIPs, dest.Port, dest.PortExplicit))
	resp, err := c.doWithRetry(trustedCtx, req)
	if err == nil {
		return resp, nil
	}
	// A reexecução pós-consentimento pode falhar de novo (redirect para IP não
	// confiável, rotação DNS, Happy Eyeballs num endereço fora do trust).
	// Normalizamos um eventual BlockedIPError para um erro acionável coerente em
	// vez de vazar o erro seco do guard. Não reabrimos prompt (evita laço).
	var blocked2 *BlockedIPError
	if errors.As(err, &blocked2) {
		// Mesma distinção do fluxo inicial: se o bloqueio subsequente veio de um
		// salto de redirect, não atribuímos o IP interno ao host da URL original.
		if didRedirect(trustedCtx) {
			return nil, redirectBlockedError(blocked2.IP)
		}
		return nil, newBlockedDestinationError(c.buildBlockedDestination(trustedCtx, req, blocked2))
	}
	return nil, err
}

// newBlockedDestinationError monta o erro acionável para um destino barrado.
func newBlockedDestinationError(dest BlockedDestination) *BlockedDestinationError {
	return &BlockedDestinationError{
		Host:        dest.Host,
		URL:         dest.URL,
		IPs:         dest.IPs,
		Category:    dest.Category,
		Reason:      dest.Reason,
		Suggestions: defaultBlockSuggestions,
	}
}

// redirectHostBlockedError monta o erro acionável para um redirect barrado ainda
// na checagem pré-dial por hostname (RedirectGuard): host local/privado literal
// (ex.: 127.0.0.1) ou alias (localhost) sem trust por-request. Mantém o mesmo
// formato de BlockedDestinationError (com sugestões) usado no resto do fluxo
// anti-SSRF, em vez de um erro seco.
func redirectHostBlockedError(host string) *BlockedDestinationError {
	return &BlockedDestinationError{
		Host:        host,
		Category:    ClassifyDestination(host, net.ParseIP(host)),
		Reason:      "redirecionamento para host local/privado não permitido",
		Suggestions: defaultBlockSuggestions,
	}
}

// redirectBlockedError descreve um bloqueio ocorrido num salto de redirect, sem
// atribuir o IP interno ao host da URL original (evita mensagem incoerente).
func redirectBlockedError(ip net.IP) *BlockedDestinationError {
	return &BlockedDestinationError{
		IPs:      []net.IP{ip},
		Category: Classify(ip),
		// Reason em pt-BR e sem repetir a categoria (já exibida em Category):
		// acrescenta a informação de que o bloqueio veio de um redirecionamento.
		Reason:      "redirecionamento para endereço interno não permitido",
		Suggestions: defaultBlockSuggestions,
	}
}

// buildBlockedDestination resolve os IPs do host e classifica o bloqueio para
// montar o pedido de autorização / a mensagem de erro. Sempre inclui o IP que o
// guard reportou (blocked.IP); complementa com a resolução completa quando
// possível, registrando o(s) IP(s) real(is) — nunca confia só no hostname.
func (c *Client) buildBlockedDestination(ctx context.Context, req *http.Request, blocked *BlockedIPError) BlockedDestination {
	host := req.URL.Hostname()
	port := req.URL.Port()
	portExplicit := port != ""
	if port == "" {
		switch req.URL.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}

	// Sempre inclui o IP que o guard reportou (o que de fato falhou no dial).
	// A resolução abaixo pode divergir (TTL/cache, IPv4/IPv6), e se o IP barrado
	// ficasse de fora do trust por-request, a reexecução após consentimento
	// continuaria bloqueada nesse endereço.
	ips := appendUniqueIP(resolveHostIPs(ctx, host), blocked.IP)
	// Classifica pelo PIOR caso entre TODOS os IPs (não só o blocked.IP): o trust
	// pós-consentimento libera todos os IPs resolvidos, então o usuário precisa
	// ver a categoria mais sensível que está de fato autorizando (ex.: se o host
	// resolve para cgnat E metadata, o prompt/entrada não pode dizer só "cgnat").
	category := MostSensitiveCategory(host, ips)

	return BlockedDestination{
		Host:         host,
		Port:         port,
		PortExplicit: portExplicit,
		URL:          req.URL.Redacted(),
		IPs:          ips,
		Category:     category,
		// Reason em pt-BR e sem repetir a categoria (já exibida em Category).
		Reason: "host resolve para uma faixa de IP não permitida",
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

// appendUniqueIP acrescenta ip a ips se ainda não estiver presente (comparação
// canônica por normalizeIPKey). ip nil é ignorado.
func appendUniqueIP(ips []net.IP, ip net.IP) []net.IP {
	if ip == nil {
		return ips
	}
	key := normalizeIPKey(ip)
	for _, existing := range ips {
		if normalizeIPKey(existing) == key {
			return ips
		}
	}
	return append(ips, ip)
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

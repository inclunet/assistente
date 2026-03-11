package http

import (
	"context"
	"fmt"
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
}

// RetryPolicy define política de retry
type RetryPolicy struct {
	MaxAttempts      int
	BackoffMultiplier float64
	InitialBackoff   time.Duration
}

// Client é o cliente HTTP centralizado com interceptor de autenticação
type Client struct {
	baseClient    *http.Client
	credMgr       *credentials.Manager
	retryPolicy   *RetryPolicy
	logFn         func(msg string)
	domainPatterns map[string]string // domínio → padrão de credencial
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
	}
}

// Do executa uma requisição HTTP com interceptação de autenticação
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Aplicar autenticação
	c.applyAuth(req)

	// Executar com retry
	return c.doWithRetry(ctx, req)
}

// applyAuth aplica autenticação baseada no domínio da requisição
func (c *Client) applyAuth(req *http.Request) {
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
		resolved, err := c.credMgr.GetByPattern(pattern)
		if err == nil {
			auth = resolved
		}
	}

	// 2) Fallback: resolve por URL (regex/wildcard matching no credential manager)
	if auth == nil {
		resolved, err := c.credMgr.ResolveForURL(req.URL.String())
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
		resp.Body.Close()
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

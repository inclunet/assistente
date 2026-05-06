package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ExternalAuthConfig struct {
	Issuer            string
	Audience          string
	JWKSURL           string
	AllowedAlgorithms []string
	RequiredScopes    []string
}

type ExternalAuthenticator struct {
	cfg       ExternalAuthConfig
	client    *http.Client
	mu        sync.Mutex
	cached    JWKSet
	cacheTime time.Time
}

func NewExternalAuthenticator(cfg ExternalAuthConfig) *ExternalAuthenticator {
	return &ExternalAuthenticator{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *ExternalAuthenticator) Validate(ctx context.Context, token string) (*ExternalClaims, error) {
	if a == nil {
		return nil, errors.New("autenticador externo não configurado")
	}
	jwks, err := a.jwks(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, alg := range a.cfg.AllowedAlgorithms {
		allowed[strings.TrimSpace(alg)] = true
	}
	claims, err := (ExternalValidator{
		Issuer:            a.cfg.Issuer,
		Audience:          a.cfg.Audience,
		AllowedAlgorithms: allowed,
	}).Validate(token, jwks)
	if err != nil {
		return nil, err
	}
	if !claims.HasScopes(a.cfg.RequiredScopes) {
		return nil, errors.New("token externo sem escopos necessários")
	}
	return claims, nil
}

func (a *ExternalAuthenticator) jwks(ctx context.Context) (JWKSet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.cached.Keys) > 0 && time.Since(a.cacheTime) < 10*time.Minute {
		return a.cached, nil
	}
	if strings.TrimSpace(a.cfg.JWKSURL) == "" {
		return JWKSet{}, errors.New("jwks_url externo obrigatório")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.JWKSURL, nil)
	if err != nil {
		return JWKSet{}, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return JWKSet{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return JWKSet{}, errors.New("falha ao buscar JWKS externo")
	}
	var set JWKSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return JWKSet{}, err
	}
	a.cached = set
	a.cacheTime = time.Now()
	return set, nil
}

func (c *ExternalClaims) HasScopes(required []string) bool {
	if len(required) == 0 {
		return true
	}
	available := map[string]bool{}
	for _, scope := range strings.Fields(c.Scope) {
		available[scope] = true
	}
	for _, scope := range required {
		if !available[strings.TrimSpace(scope)] {
			return false
		}
	}
	return true
}

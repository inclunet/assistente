package oauthflow

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Integration isolates protocol extensions from the common lifecycle.
type Route struct{ Method, Path string }
type Integration struct {
	Routes               []Route
	ID, Issuer, Resource string
	Endpoints            Endpoints
	Scopes               []string
	RequiredScopes       []string
	Callback             CallbackConfig
	InitialClientID      string
	RegistrationMethod   string
	// AuthorizationParameters supplies service-specific registration parameters.
	AuthorizationParameters func(Record, string) url.Values
	// CallbackClientID validates a dynamically issued registration.
	CallbackClientID func(Record, url.Values) (string, error)
}

func (i Integration) Validate(r Record) error {
	if r.Integration != i.ID || r.Issuer != i.Issuer || r.Resource != i.Resource || r.Endpoints != i.Endpoints || r.Callback != i.Callback {
		return ErrResource
	}
	return nil
}
func (i Integration) permits(scopes []string) bool {
	for _, required := range i.RequiredScopes {
		found := false
		for _, scope := range scopes {
			if scope == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// NewHTTPClient never follows redirects carrying authorization codes or tokens.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func verifyIdentity(ctx context.Context, client *http.Client, r Record, raw, nonce string) (string, string, error) {
	ctx = oidc.ClientContext(ctx, client)
	keys := oidc.NewRemoteKeySet(ctx, r.Endpoints.JWKS)
	verifier := oidc.NewVerifier(r.Issuer, keys, &oidc.Config{ClientID: r.Client.ID, SupportedSigningAlgs: []string{oidc.RS256}})
	token, err := verifier.Verify(ctx, raw)
	if err != nil || token.Subject == "" {
		return "", "", ErrReauthorize
	}
	if nonce != "" && token.Nonce != nonce {
		return "", "", ErrReauthorize
	}
	if r.Subject != "" && r.Subject != token.Subject {
		return "", "", ErrReauthorize
	}
	var claims struct {
		Email string `json:"email"`
	}
	if token.Claims(&claims) != nil {
		return "", "", ErrReauthorize
	}
	return token.Subject, strings.TrimSpace(claims.Email), nil
}

func (s *Service) AllowsRequest(r Record, method string, target *url.URL) bool {
	i, err := s.integration(r)
	if err != nil {
		return false
	}
	resource, err := url.Parse(i.Resource)
	if err != nil || target == nil {
		return false
	}
	if target.Scheme != resource.Scheme || target.Host != resource.Host || target.User != nil || target.Path != path.Clean(target.Path) {
		return false
	}
	if !strings.HasPrefix(target.Path, strings.TrimRight(resource.Path, "/")+"/") {
		return false
	}
	for _, route := range i.Routes {
		if method == route.Method && target.Path == strings.TrimRight(resource.Path, "/")+route.Path {
			return true
		}
	}
	return false
}

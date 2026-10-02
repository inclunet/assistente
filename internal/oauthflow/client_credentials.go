package oauthflow

import (
	"context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"time"
)

// ClientCredentialsConfig describes protocol inputs, not consumer persistence.
type ClientCredentialsConfig struct {
	Resource, ClientID, ClientSecret, TokenEndpoint string
	Scopes                                          []string
}

// ClientCredentialsTokenSource caches valid tokens and serializes new grants.
// Each grant has its own network approval scope; it never opens a browser.
func ClientCredentialsTokenSource(ctx context.Context, cfg ClientCredentialsConfig, authorize NetworkAuthorizer) oauth2.TokenSource {
	cc := &clientcredentials.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: cfg.TokenEndpoint, Scopes: append([]string(nil), cfg.Scopes...)}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, NewNetworkHTTPClient(cfg.Resource, authorize, 30*time.Second))
	return NewOperationTokenSource(ctx, nil, func(operationCtx context.Context, _ *oauth2.Token) oauth2.TokenSource {
		return cc.TokenSource(operationCtx)
	})
}

// RequestClientCredentialsToken performs one grant within the caller's approval
// scope. Durable consumers preflight before acquiring their short lease; creating
// a fresh scope here would prompt again while that lease is already running.
func RequestClientCredentialsToken(ctx context.Context, cfg ClientCredentialsConfig, authorize NetworkAuthorizer) (*oauth2.Token, error) {
	cc := &clientcredentials.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: cfg.TokenEndpoint, Scopes: append([]string(nil), cfg.Scopes...)}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, NewNetworkHTTPClient(cfg.Resource, authorize, 30*time.Second))
	return cc.TokenSource(ctx).Token()
}

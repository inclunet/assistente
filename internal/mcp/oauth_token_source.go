package mcp

import (
	"assistente/internal/oauthflow"
	"context"
	"golang.org/x/oauth2"
)

type operationTokenSource func() (*oauth2.Token, error)

func (f operationTokenSource) Token() (*oauth2.Token, error) { return f() }

// Cache and serialize access, but create a fresh approval scope for each actual
// refresh. The factory's internal AuthStyle retries share that scope. Retaining
// the successfully returned token preserves refresh-token rotation.
func newScopedTokenSource(ctx context.Context, initial *oauth2.Token, factory func(context.Context, *oauth2.Token) oauth2.TokenSource) oauth2.TokenSource {
	current := initial
	return oauth2.ReuseTokenSource(initial, operationTokenSource(func() (*oauth2.Token, error) {
		token, err := factory(oauthflow.WithNetworkOperation(ctx), current).Token()
		if err == nil {
			current = token
		}
		return token, err
	}))
}

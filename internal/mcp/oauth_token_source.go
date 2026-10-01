package mcp

import (
	"assistente/internal/oauthflow"
	"context"
	"golang.org/x/oauth2"
)

func newScopedTokenSource(ctx context.Context, initial *oauth2.Token, factory func(context.Context, *oauth2.Token) oauth2.TokenSource) oauth2.TokenSource {
	return oauthflow.NewOperationTokenSource(ctx, initial, factory)
}

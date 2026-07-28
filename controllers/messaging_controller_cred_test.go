package controllers

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func TestResolveCredentialRef_RequiresUserScope(t *testing.T) {
	t.Parallel()
	mgr := credentials.NewManager(nil)
	userCtx := database.WithUserID(context.Background(), "user-ana")

	patterns := []struct {
		pattern string
		token   string
	}{
		{"channel:telegram:bot_token", "tg-secret-token"},
		{"channel:slack:bot_token", "slack-bot-token"},
		{"channel:slack:app_token", "slack-app-token"},
		{"channel:signal:api_token", "signal-api-token"},
	}
	for _, p := range patterns {
		if err := mgr.RegisterPatternWithContext(userCtx, p.pattern, &credentials.AuthConfig{
			Type:  "secret",
			Token: p.token,
		}); err != nil {
			t.Fatalf("register %s: %v", p.pattern, err)
		}
	}

	ctrl := NewMessagingController(MessagingControllerConfig{CredMgr: mgr})

	for _, p := range patterns {
		if got := ctrl.resolveCredentialRef(p.pattern); got != "" {
			t.Fatalf("%s sem SetCredentialUserID resolveu %q; esperado vazio", p.pattern, got)
		}
	}

	ctrl.SetCredentialUserID("user-ana")
	for _, p := range patterns {
		if got := ctrl.resolveCredentialRef(p.pattern); got != p.token {
			t.Fatalf("%s com user scope got %q want %q", p.pattern, got, p.token)
		}
	}

	ctrl.SetCredentialUserID("user-other")
	for _, p := range patterns {
		if got := ctrl.resolveCredentialRef(p.pattern); got != "" {
			t.Fatalf("%s user errado resolveu %q; esperado vazio", p.pattern, got)
		}
	}
}

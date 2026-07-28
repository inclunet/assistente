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
	if err := mgr.RegisterPatternWithContext(userCtx, "channel:telegram:bot_token", &credentials.AuthConfig{
		Type:  "secret",
		Token: "tg-secret-token",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctrl := NewMessagingController(MessagingControllerConfig{CredMgr: mgr})

	if got := ctrl.resolveCredentialRef("channel:telegram:bot_token"); got != "" {
		t.Fatalf("sem SetCredentialUserID resolveu %q; esperado vazio (user-scoped)", got)
	}

	ctrl.SetCredentialUserID("user-ana")
	if got := ctrl.resolveCredentialRef("channel:telegram:bot_token"); got != "tg-secret-token" {
		t.Fatalf("com user scope got %q want tg-secret-token", got)
	}

	ctrl.SetCredentialUserID("user-other")
	if got := ctrl.resolveCredentialRef("channel:telegram:bot_token"); got != "" {
		t.Fatalf("user errado resolveu %q; esperado vazio", got)
	}
}

package app

import (
	"errors"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/wailsapi"
)

func TestWireTokensAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
		tokenSvc:       chat.NewTokenService(chat.NewDBMessageStore()),
	}
	api := wailsapi.NewTokens()
	SetTokensAPI(a, api)

	a.wireTokens()

	if a.tokensCtrl == nil {
		t.Fatal("tokensCtrl deve ser criado")
	}
	_, err := api.GetConversationTokenStats("c1")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireAllowlistAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewAllowlists()
	SetAllowlistsAPI(a, api)

	a.wireAllowlist()

	if a.allowlistCtrl == nil {
		t.Fatal("allowlistCtrl deve ser criado")
	}
	_, err := api.GetAllowlists()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

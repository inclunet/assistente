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

func TestWireSkillsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewSkills()
	SetSkillsAPI(a, api)

	a.wireSkills()

	if a.skillsCtrl == nil {
		t.Fatal("skillsCtrl deve ser criado")
	}
	_, err := api.GetSkills()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireToolsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewTools()
	SetToolsAPI(a, api)

	a.wireTools()

	if a.toolsCtrl == nil {
		t.Fatal("toolsCtrl deve ser criado")
	}
	_, err := api.GetAvailableTools()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireUpdaterAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewUpdater()
	SetUpdaterAPI(a, api)

	a.wireUpdater()

	if a.updaterCtrl == nil {
		t.Fatal("updaterCtrl deve ser criado")
	}
	_, err := api.GetAppVersion()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireProfilesAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewProfiles()
	SetProfilesAPI(a, api)

	a.wireProfiles()

	if a.profilesCtrl == nil {
		t.Fatal("profilesCtrl deve ser criado")
	}
	_, err := api.GetProfiles()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireHotkeysAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewHotkeys()
	SetHotkeysAPI(a, api)

	a.wireHotkeys()

	if a.hotkeyCtrl == nil {
		t.Fatal("hotkeyCtrl deve ser criado")
	}
	_, err := api.IsGlobalHotkeySupported()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireNetTrustAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewNetTrust()
	SetNetTrustAPI(a, api)

	a.wireNetTrust()

	if a.netTrustCtrl == nil {
		t.Fatal("netTrustCtrl deve ser criado")
	}
	_, err := api.GetNetworkAllowlist()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireCredentialsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewCredentials()
	SetCredentialsAPI(a, api)

	a.wireCredentials()

	if a.credentialsCtrl == nil {
		t.Fatal("credentialsCtrl deve ser criado")
	}
	_, err := api.ListCredentials()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSettingsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewSettings()
	SetSettingsAPI(a, api)

	a.wireSettings()

	if a.settingsCtrl == nil {
		t.Fatal("settingsCtrl deve ser criado")
	}
	_, err := api.GetNativeTTSProviders()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

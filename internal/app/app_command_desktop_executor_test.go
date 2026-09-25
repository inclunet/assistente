package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandDesktopExecutorUsesFixedRevalidatedSessionWithoutToken(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "desktop.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	user := database.User{Username: "desktop-fixture", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{0x41}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := sessions.IssueSession(ctx, &user, "desktop-first")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{0x42}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x43}, 32))); err != nil {
		t.Fatal(err)
	}
	workspaceManager := workspace.NewManager(filepath.Join(t.TempDir(), "workspace-home"))
	if err := workspaceManager.Initialize(filepath.Join(t.TempDir(), "workspace")); err != nil {
		t.Fatal(err)
	}
	app := &App{sessionSvc: sessions, credMgr: manager, workspaceMgr: workspaceManager}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: first.SessionID, Role: user.Role})

	registry, err := desktopFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, "desktop-v1")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, user.ID, bindings); err != nil {
		t.Fatal(err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return bindings, nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	config := completeFactoryConfig(registry, state, started, user.ID)
	// A fábrica não instala nem altera o HostState: o snapshot autoritativo
	// precisa usar a mesma versão publicada nesta instância.
	config.RegistryVersion = "desktop-v1"
	config.Handlers = map[string]commandexecution.Handler{
		"fixture.desktop": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/fixture/desktop", Classification: commandcatalog.HandlerInternal}, Start: func(_ context.Context, _ commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			done := make(chan commandexecution.Outcome, 1)
			started <- struct{}{}
			done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
			return commandexecution.ExecutionHandle{ID: uuid.Must(uuid.NewV7()).String(), Done: done, Cancel: func() {}}, nil
		}},
	}
	config.Store, err = commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	service, err := app.newCommandDesktopExecutor(config, state)
	if err != nil {
		t.Fatalf("fábrica desktop: %v", err)
	}
	if app.commandHost != nil {
		t.Fatal("fábrica desktop publicou HostState fora da montagem central")
	}
	t.Cleanup(func() { _ = service.Shutdown(ctx) })

	record, err := service.ExecuteEnvelope(ctx, "", desktopCandidate())
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("sessão desktop: status=%s err=%v", record.Status, err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler não foi executado")
	}
	if _, err := service.ExecuteEnvelope(ctx, first.AccessToken, desktopCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("token recebido foi aceito: %v", err)
	}
	if err := sessions.Logout(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteEnvelope(ctx, "", desktopCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("sessão revogada foi aceita: %v", err)
	}
	second, err := sessions.IssueSession(ctx, &user, "desktop-second")
	if err != nil {
		t.Fatal(err)
	}
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: second.SessionID, Role: user.Role})
	if _, err := service.ExecuteEnvelope(ctx, "", desktopCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("troca de sessão reutilizou executor antigo: %v", err)
	}
}

func desktopFactoryRegistry() (*commandcatalog.Registry, error) {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Desktop", Description: "Desktop", Category: "Tests"},
		"en":    {Name: "Desktop", Description: "Desktop", Category: "Tests"},
		"es":    {Name: "Desktop", Description: "Desktop", Category: "Tests"},
	}
	definition := commandcatalog.Definition{
		ID: "fixture.desktop", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Context: commandcatalog.ContextPolicy{None: true},
		Presentation: &commandcatalog.Presentation{Version: "test-v1", Locales: locales},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "internal/fixture/desktop", HandlerClassification: commandcatalog.HandlerInternal,
	}
	return commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerInternal}}})
}

func desktopCandidate() commandexecution.EnvelopeCandidate {
	return commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: "fixture.desktop", Arguments: json.RawMessage(`{}`)}
}

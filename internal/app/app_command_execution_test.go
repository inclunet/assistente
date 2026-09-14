package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandExecutionAppUsesInstanceKeyAndRejectsLogout(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commands.db")), &gorm.Config{})
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
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "executor-fixture", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{4}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := sessions.IssueSession(ctx, &user, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{5}, 32))
	app := &App{ctx: ctx, identitySvc: auth.NewIdentityService(db), sessionSvc: sessions, credMgr: manager, vaultSvc: auth.NewVaultService(nil, nil), authKeyringDelete: func() error { return nil }}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: user.Role})
	locales := map[string]commandcatalog.LocalizedMetadata{}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		locales[locale] = commandcatalog.LocalizedMetadata{Name: "Fixture", Description: "Fixture", Category: "Fixture"}
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{ID: "fixture.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales}}, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	done := make(chan commandexecution.Outcome, 1)
	authorize, err := commandexecution.NewLocalReadAuthorizer(db, map[string][]string{"fixture.read": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	config := commandexecution.Config{Store: store, Registry: registry, RegistryVersion: "1", Source: commandcatalog.Palette,
		Snapshot: func(context.Context, auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
			return commandexecution.Versions{Registry: "1", GlobalConfig: "1", ActiveLayers: "1", Unlocked: true}, nil
		},
		Authorize:  authorize,
		KeyVersion: "v1", Now: time.Now, Retention: time.Hour, ExecutionTimeout: 5 * time.Second, FinalizationTimeout: 2 * time.Second,
		Handlers: map[string]commandexecution.Handler{"fixture.read": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read}, Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			started <- struct{}{}
			return commandexecution.ExecutionHandle{ID: "fixture", Done: done, Cancel: func() {}}, nil
		}}},
	}
	service, err := app.newCommandReadExecutor(config)
	if err != nil {
		t.Fatal(err)
	}
	request := commandexecution.Request{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: "fixture.read"}
	if _, err := service.Execute(ctx, pair.AccessToken, request); !errors.Is(err, commandledger.ErrFingerprintKeyUnavailable) {
		t.Fatalf("chave ausente: %v", err)
	}
	// Apenas chave de fixture em Manager sem store/persistência; nunca keychain.
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		record, err := service.Execute(ctx, pair.AccessToken, request)
		if err == nil && record.Status != commandledger.Succeeded {
			err = errors.New("resultado incorreto")
		}
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Start não ocorreu")
	}
	// Logout real precisa terminar mesmo enquanto o handler aguarda resultado.
	logout := make(chan error, 1)
	go func() { logout <- app.Logout(LogoutRequest{RefreshToken: pair.RefreshToken}) }()
	select {
	case err := <-logout:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("logout bloqueado pelo handler")
	}
	done <- commandexecution.Outcome{Status: commandledger.Succeeded}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não concluiu")
	}
	if _, err := service.Execute(ctx, pair.AccessToken, request); err == nil {
		t.Fatal("replay após logout autorizado")
	}
	select {
	case <-started:
		t.Fatal("Start repetido")
	default:
	}
}

func TestCommandExecutionAppDoesNotBootstrapMissingDependencies(t *testing.T) {
	if _, err := (&App{}).newCommandReadExecutor(commandexecution.Config{}); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal(err)
	}
}

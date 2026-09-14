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
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
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
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{ID: "fixture.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales}}, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
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
	epochs, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, config.RegistryVersion)
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
	service, err := app.newCommandReadExecutor(config, state)
	if err != nil {
		t.Fatal(err)
	}
	alternate, err := commandexecution.NewHostState(epochs, config.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.newCommandReadExecutor(config, alternate); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal("segundo estado substituiu host instalado", err)
	}
	foreignEpochs, err := (&App{}).commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := commandexecution.NewHostState(foreignEpochs, config.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.newCommandReadExecutor(config, foreign); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal("host com outro gate foi aceito", err)
	}
	request := commandexecution.Request{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: "fixture.read"}
	if _, err := service.Execute(ctx, pair.AccessToken, request); !errors.Is(err, commandledger.ErrFingerprintKeyUnavailable) {
		t.Fatalf("chave ausente: %v", err)
	}
	// Apenas chave de fixture em Manager sem store/persistência; nunca keychain.
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))); err != nil {
		t.Fatal(err)
	}
	// O callback permissivo de Config não pode substituir o HostState do App.
	lockedRequest := request
	lockedRequest.InvocationID = uuid.Must(uuid.NewV7()).String()
	locked, err := service.Execute(ctx, pair.AccessToken, lockedRequest)
	if err != nil || locked.Status != commandledger.CancelledStale {
		t.Fatal("OS desconhecido foi admitido", locked, err)
	}
	select {
	case <-started:
		t.Fatal("Start sem estado do SO")
	default:
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	neverBuild := func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		t.Fatal("builder executado sem sessão atual autenticada")
		return nil, nil, nil
	}
	if err := app.rebuildCommandUserConfiguration(ctx, "invalid-token", neverBuild); err == nil {
		t.Fatal("reconstrução aceitou JWT inválido")
	}
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: uuid.Must(uuid.NewV7()).String(), Role: user.Role})
	if err := app.rebuildCommandUserConfiguration(ctx, pair.AccessToken, neverBuild); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatal("JWT de outra sessão do App foi aceito", err)
	}
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: user.Role})
	if err := app.rebuildCommandUserConfiguration(ctx, pair.AccessToken, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		app.resetCommandHostSession(false)
		return bindings, nil, nil
	}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatal("reconstrução atrasada publicou após remoção do mapa", err)
	}
	// Repositório só neste SQLite de fixture. Nenhuma migração no DB do App.
	if err := commandconfig.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	configStore, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildPersistedCommandConfiguration(ctx, pair.AccessToken, configStore, func(context.Context, commandconfig.Snapshot) (*commandbindings.Configuration, error) {
		t.Fatal("projetor chamado sem geração persistida")
		return nil, nil
	}); !errors.Is(err, commandconfig.ErrInvalid) {
		t.Fatal("ausência de geração não recusada", err)
	}
	generation := commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: user.ID, Generation: 1, UpdatedAt: time.Now()}
	if err := db.Create(&generation).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildPersistedCommandConfiguration(ctx, pair.AccessToken, configStore, func(context.Context, commandconfig.Snapshot) (*commandbindings.Configuration, error) {
		// Simula publicação persistida concluída durante a leitura fora do gate.
		if err := db.Model(&commandconfig.Generation{}).Where("id = ?", generation.ID).Update("generation", 2).Error; err != nil {
			return nil, err
		}
		return bindings, nil
	}); !errors.Is(err, commandconfig.ErrStale) {
		t.Fatal("geração persistida obsoleta publicada", err)
	}
	if err := app.rebuildPersistedCommandConfiguration(ctx, pair.AccessToken, configStore, func(_ context.Context, snapshot commandconfig.Snapshot) (*commandbindings.Configuration, error) {
		if snapshot.Scope.UserID != user.ID || snapshot.Scope.WorkspaceID != nil {
			t.Fatal("escopo não derivado da sessão")
		}
		return bindings, nil
	}); err != nil {
		t.Fatal(err)
	}
	// Projeção concreta: documento válido não restaura ativação de camada.
	layer := commandconfig.Layer{ID: uuid.Must(uuid.NewV7()).String(), UserID: user.ID, Name: "fixture", Enabled: true, Source: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	commandID := "fixture.read"
	binding := commandconfig.Binding{ID: uuid.Must(uuid.NewV7()).String(), UserID: user.ID, LayerRefKind: "user", LayerRef: layer.ID,
		TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control"]}`,
		CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: "{}"}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	options := commandconfig.LocalReadProjection{Registry: registry, NoArgumentCommands: []string{commandID}}
	if err := app.rebuildPersistedLocalReadConfiguration(ctx, pair.AccessToken, configStore, options); err != nil {
		t.Fatal(err)
	}
	projected, activeLayers, err := state.UserConfiguration(ctx, user.ID)
	if err != nil || len(activeLayers) != 0 {
		t.Fatal("restore implícito de claims", activeLayers, err)
	}
	selection, err := projected.Resolve("keyboard.local:Control+KeyK", nil, nil)
	if err != nil || selection.Status != commandbindings.NoMatch {
		t.Fatal("camada habilitada foi ativada", selection, err)
	}
	options.ActiveUserLayerIDs = []string{layer.ID}
	if err := app.rebuildPersistedLocalReadConfiguration(ctx, pair.AccessToken, configStore, options); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal("claims aceitas sem restore", err)
	}
	options.ActiveUserLayerIDs = nil
	if err := db.Model(&binding).Update("arguments", `{"secret":"fixture-only"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildPersistedLocalReadConfiguration(ctx, pair.AccessToken, configStore, options); !errors.Is(err, commandconfig.ErrInvalid) {
		t.Fatal("argumentos não suportados publicados", err)
	}
	after, _, err := state.UserConfiguration(ctx, user.ID)
	if err != nil || after != projected {
		t.Fatal("falha de projeção substituiu mapa", err)
	}
	result := make(chan error, 1)
	go func() {
		record, err := service.Execute(ctx, pair.AccessToken, request)
		if err == nil && record.Status != commandledger.OutcomeUnknown {
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
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não concluiu")
	}
	done <- commandexecution.Outcome{Status: commandledger.Succeeded} // Resultado tardio não reabre invocação.
	if _, _, err := state.UserConfiguration(ctx, user.ID); err == nil {
		t.Fatal("logout reteve mapa do usuário")
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
	if _, err := (&App{}).newCommandReadExecutor(commandexecution.Config{}, nil); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal(err)
	}
}

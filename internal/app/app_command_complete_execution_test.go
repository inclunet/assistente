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
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandCompleteExecutorFactoryBindsExactSessionAndRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "complete.db")), &gorm.Config{})
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
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })
	user := database.User{Username: "complete-fixture", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{0x31}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := sessions.IssueSession(ctx, &user, "complete-first")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{0x32}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))); err != nil {
		t.Fatal(err)
	}
	workspaceManager := workspace.NewManager(filepath.Join(t.TempDir(), "workspace-home"))
	if err := workspaceManager.Initialize(filepath.Join(t.TempDir(), "workspace")); err != nil {
		t.Fatal(err)
	}
	questionnaireEvents := make(chan map[string]any, 1)
	questionnaireManager := questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			questionnaireEvents <- data.(map[string]any)
		}
	})
	app := &App{sessionSvc: sessions, credMgr: manager, workspaceMgr: workspaceManager, questionnaireMgr: questionnaireManager}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: first.SessionID, Role: user.Role})

	registry, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, "complete-v1")
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

	started := make(chan struct{}, 2)
	config := completeFactoryConfig(registry, state, started, user.ID)
	config.Store, err = commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	foreignDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "foreign.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	foreignSQLDB, err := foreignDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = foreignSQLDB.Close() })
	foreignStore, err := commandledger.New(foreignDB, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	foreignConfig := config
	foreignConfig.Store = foreignStore
	if _, err := app.newCommandCompleteExecutor(foreignConfig, state); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("store de outro DB foi aceito: %v", err)
	}
	service, err := app.newCommandCompleteExecutor(config, state)
	if err != nil {
		t.Fatalf("fábrica completa: %v", err)
	}
	if service == nil {
		t.Fatal("fábrica devolveu serviço nil")
	}
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

	firstRecord, err := service.ExecuteEnvelope(ctx, first.AccessToken, completeCandidate())
	if err != nil || firstRecord.Status != commandledger.Succeeded {
		t.Fatalf("sessão inicial: status=%s err=%v", firstRecord.Status, err)
	}
	interactiveResult := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, executeErr := service.ExecuteEnvelope(ctx, first.AccessToken, completeCandidateFor("fixture.confirm"))
		interactiveResult <- struct {
			record commandledger.FullRecord
			err    error
		}{record, executeErr}
	}()
	select {
	case payload := <-questionnaireEvents:
		uiID, ok := payload["id"].(string)
		if !ok || uiID == "" {
			t.Fatalf("ID de questionário inválido: %#v", payload["id"])
		}
		if err := questionnaireManager.Respond(uiID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("presenter real não publicou o DecisionDialog")
	}
	interactive := <-interactiveResult
	if interactive.err != nil || interactive.record.Status != commandledger.Succeeded {
		t.Fatalf("decisão interativa: status=%s err=%v", interactive.record.Status, interactive.err)
	}
	select {
	case <-started:
	default:
		t.Fatal("handler real do catálogo não foi chamado")
	}
	if err := state.SetVaultUnlocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteEnvelope(ctx, first.AccessToken, completeCandidate()); err == nil {
		t.Fatal("callback de snapshot bypassou cofre fechado")
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteEnvelope(ctx, first.AccessToken, completeCandidate()); err == nil {
		t.Fatal("callback de snapshot bypassou SO desconhecido")
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

	if err := sessions.Logout(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteEnvelope(ctx, first.AccessToken, completeCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("sessão revogada foi aceita: %v", err)
	}

	second, err := sessions.IssueSession(ctx, &user, "complete-second")
	if err != nil {
		t.Fatal(err)
	}
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: second.SessionID, Role: user.Role})
	if _, err := service.ExecuteEnvelope(ctx, second.AccessToken, completeCandidate()); err == nil {
		t.Fatal("sessão nova executou sem reconstrução do mapa")
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: second.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return bindings, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	secondRecord, err := service.ExecuteEnvelope(ctx, second.AccessToken, completeCandidate())
	if err != nil || secondRecord.Status != commandledger.Succeeded {
		t.Fatalf("sessão reestabelecida: status=%s err=%v", secondRecord.Status, err)
	}
	closingApp := &App{commandLifecycleClosing: true}
	if err := closingApp.installCommandHost(state); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("instalação após closing aceita: %v", err)
	}
	if _, err := epochs.CloseAndDrain(ctx); err != nil {
		t.Fatalf("fechar core de fixture: %v", err)
	}
	if _, err := app.newCommandCompleteExecutor(config, state); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("NewComplete após core fechado retornou %v", err)
	}
}

func completeFactoryRegistry() (*commandcatalog.Registry, error) {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Factory", Description: "Factory", Category: "Tests"},
		"en":    {Name: "Factory", Description: "Factory", Category: "Tests"},
		"es":    {Name: "Factory", Description: "Factory", Category: "Tests"},
	}
	definition := commandcatalog.Definition{
		ID: "fixture.complete", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Context: commandcatalog.ContextPolicy{None: true},
		Presentation:    &commandcatalog.Presentation{Version: "test-v1", Locales: locales},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "internal/fixture/complete", HandlerClassification: commandcatalog.HandlerInternal,
	}
	handler := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerInternal}
	interactive := definition
	interactive.ID = "fixture.confirm"
	interactive.Decision = commandcatalog.Interactive
	interactive.HandlerRoute = "internal/fixture/confirm"
	return commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: handler}, {
		Definition: interactive,
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: interactive.HandlerRoute, Classification: commandcatalog.HandlerInternal},
	}})
}

func completeFactoryConfig(registry *commandcatalog.Registry, state *commandexecution.HostState, started chan<- struct{}, actorID string) commandexecution.Config {
	start := func(_ context.Context, _ commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
		started <- struct{}{}
		done := make(chan commandexecution.Outcome, 1)
		done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
		return commandexecution.ExecutionHandle{ID: uuid.Must(uuid.NewV7()).String(), Done: done, Cancel: func() {}}, nil
	}
	return commandexecution.Config{
		Envelope: &commandexecution.EnvelopeConfig{
			Snapshot: func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
				versions, err := state.Snapshot(ctx, principal)
				if err != nil {
					return commandcontract.Envelope{}, err
				}
				return commandcontract.Envelope{RegistryVersion: versions.Registry, GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers, CorrelationID: candidate.CorrelationID}, nil
			},
			Resolve: func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
				return commandexecution.EnvelopeResolution{}, errors.New("resolver não deve ser usado para command_id direto")
			},
			Authorize: func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error {
				return nil
			},
			AuthorizeLookup: func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error { return nil },
			Actor: func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
				return commandcontract.ActorUser, actorID, nil
			},
			DecisionBody: func(commandcatalog.Definition, commandcontract.Envelope) (string, error) { return `{"version":1}`, nil },
			DecisionTTL:  time.Minute,
		},
		Registry: registry, RegistryVersion: "complete-v1", Source: commandcatalog.Palette,
		Handlers: map[string]commandexecution.Handler{
			"fixture.complete": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/fixture/complete", Classification: commandcatalog.HandlerInternal}, Start: start},
			"fixture.confirm":  {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/fixture/confirm", Classification: commandcatalog.HandlerInternal}, Start: start},
		},
		KeyVersion: "v1", Now: time.Now, Retention: time.Minute, ExecutionTimeout: 5 * time.Second, FinalizationTimeout: time.Second,
	}
}

func completeCandidate() commandexecution.EnvelopeCandidate {
	return completeCandidateFor("fixture.complete")
}

func completeCandidateFor(command string) commandexecution.EnvelopeCandidate {
	return commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: command, Arguments: json.RawMessage(`{}`)}
}

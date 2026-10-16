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
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type desktopDecisionFixture struct {
	db       *gorm.DB
	sessions *auth.SessionService
	user     database.User
	first    *auth.TokenPair
	manager  *questionnaire.Manager
	events   <-chan map[string]any
	started  <-chan commandexecution.Invocation
	service  *commandexecution.Service
	state    *commandexecution.HostState
	app      *App
}

func newDesktopDecisionFixture(t *testing.T) *desktopDecisionFixture {
	t.Helper()
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "desktop-decision.db")), &gorm.Config{})
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

	user := database.User{Username: "desktop-decision-fixture", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{0x61}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := sessions.IssueSession(ctx, &user, "desktop-decision")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{0x62}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x63}, 32))); err != nil {
		t.Fatal(err)
	}
	workspaceManager := workspace.NewManager(filepath.Join(t.TempDir(), "workspace-home"))
	if err := workspaceManager.Initialize(filepath.Join(t.TempDir(), "workspace")); err != nil {
		t.Fatal(err)
	}
	events := make(chan map[string]any, 4)
	questionnaireManager := questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			events <- data.(map[string]any)
		}
	})
	app := &App{sessionSvc: sessions, credMgr: manager, workspaceMgr: workspaceManager, questionnaireMgr: questionnaireManager}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: first.SessionID, Role: user.Role})
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
	if err := state.RebuildUserConfiguration(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) {
			return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
		},
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return bindings, nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	registry, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan commandexecution.Invocation, 2)
	config := completeFactoryConfig(registry, state, make(chan struct{}, 2), user.ID)
	config.Store, err = commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	handler := config.Handlers["fixture.confirm"]
	handler.Start = func(_ context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
		started <- invocation
		done := make(chan commandexecution.Outcome, 1)
		done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
		return commandexecution.ExecutionHandle{ID: newDesktopDecisionID(), Done: done, Cancel: func() {}}, nil
	}
	config.Handlers["fixture.confirm"] = handler
	service, err := app.newCommandDesktopExecutor(config, state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = service.Shutdown(context.Background())
		_, _ = epochs.CloseAndDrain(context.Background())
	})
	return &desktopDecisionFixture{db: db, sessions: sessions, user: user, first: first, manager: questionnaireManager, events: events, started: started, service: service, state: state, app: app}
}

func newDesktopDecisionID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func TestCommandDesktopDecisionAcceptsAndDeliversConsumedReceipt(t *testing.T) {
	f := newDesktopDecisionFixture(t)
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	invocationID := completeCandidateFor("fixture.confirm").InvocationID
	go func() {
		record, err := f.service.ExecuteEnvelope(context.Background(), "", commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: newDesktopDecisionID(), CommandID: "fixture.confirm", Arguments: []byte(`{}`)})
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	var payload map[string]any
	select {
	case payload = <-f.events:
	case early := <-result:
		t.Fatalf("execução terminou antes do diálogo: status=%s err=%v", early.record.Status, early.err)
	case <-time.After(5 * time.Second):
		t.Fatal("o diálogo de decisão não foi emitido")
	}
	finishCommandDecision(t, f.manager, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	got := <-result
	if got.err != nil || got.record.Status != commandledger.Succeeded {
		t.Fatalf("aceite desktop: status=%s err=%v", got.record.Status, got.err)
	}
	invocation := <-f.started
	var receipt struct {
		DecisionID string
		Status     string
	}
	if err := f.db.Raw("SELECT decision_id, status FROM command_decision_receipts WHERE subject_id = ?", invocationID).Scan(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "consumed" || receipt.DecisionID == "" {
		t.Fatalf("receipt não consumido: %+v", receipt)
	}
	if invocation.Envelope == nil || invocation.Envelope.AuthorizationDecisionID == nil || *invocation.Envelope.AuthorizationDecisionID != receipt.DecisionID {
		t.Fatalf("receipt entregue incorretamente: envelope=%+v receipt=%q", invocation.Envelope, receipt.DecisionID)
	}
}

func TestCommandDesktopDecisionRejectAndCancelDoNotStart(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answers   map[string]any
		cancelled bool
	}{
		{name: "negar", answers: map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}},
		{name: "cancelar", cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDesktopDecisionFixture(t)
			candidate := completeCandidateFor("fixture.confirm")
			result := make(chan struct {
				record commandledger.FullRecord
				err    error
			}, 1)
			go func() {
				record, err := f.service.ExecuteEnvelope(context.Background(), "", candidate)
				result <- struct {
					record commandledger.FullRecord
					err    error
				}{record, err}
			}()
			payload := receiveCommandDecisionEvent(t, f.events)
			finishCommandDecision(t, f.manager, payload, tc.answers, tc.cancelled)
			got := <-result
			if got.err != nil || got.record.Status != commandledger.Denied {
				t.Fatalf("%s: status=%s err=%v", tc.name, got.record.Status, got.err)
			}
			select {
			case invocation := <-f.started:
				t.Fatalf("%s iniciou handler: %+v", tc.name, invocation)
			default:
			}
		})
	}
}

func TestCommandDesktopDecisionSessionRevocationDuringDialogDeniesBeforeStart(t *testing.T) {
	f := newDesktopDecisionFixture(t)
	candidate := completeCandidateFor("fixture.confirm")
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(context.Background(), "", candidate)
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	payload := receiveCommandDecisionEvent(t, f.events)
	if err := f.sessions.Logout(context.Background(), f.first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, f.manager, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	got := <-result
	if got.record.Status == commandledger.Succeeded {
		t.Fatalf("sessão revogada durante diálogo executou: status=%s err=%v", got.record.Status, got.err)
	}
	select {
	case invocation := <-f.started:
		t.Fatalf("sessão revogada alcançou handler: %+v", invocation)
	default:
	}
}

func desktopDecisionFactoryConfig(t *testing.T, f *desktopDecisionFixture) commandexecution.Config {
	t.Helper()
	registry, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	config := completeFactoryConfig(registry, f.state, make(chan struct{}, 2), f.user.ID)
	config.Store, err = commandledger.New(f.db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestCommandDesktopDecisionFactoryRejectsIncompleteOrForeignDecisionWiring(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *desktopDecisionFixture, *commandexecution.Config)
	}{
		{name: "ttl ausente", mutate: func(_ *testing.T, _ *desktopDecisionFixture, config *commandexecution.Config) {
			config.Envelope.DecisionTTL = 0
		}},
		{name: "body ausente", mutate: func(_ *testing.T, _ *desktopDecisionFixture, config *commandexecution.Config) {
			config.Envelope.DecisionBody = nil
		}},
		{name: "questionnaire ausente", mutate: func(_ *testing.T, f *desktopDecisionFixture, _ *commandexecution.Config) {
			f.app.questionnaireMgr = nil
		}},
		{name: "store de decisão externo", mutate: func(t *testing.T, f *desktopDecisionFixture, config *commandexecution.Config) {
			store, err := commanddecision.New(f.db, &commandDecisionPresenter{manager: f.manager}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			config.Envelope.Decisions = store
		}},
		{name: "ledger em banco estrangeiro", mutate: func(t *testing.T, f *desktopDecisionFixture, config *commandexecution.Config) {
			foreignDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "foreign-ledger.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			foreignSQLDB, err := foreignDB.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = foreignSQLDB.Close() })
			if err := commandledger.Migrate(context.Background(), foreignDB); err != nil {
				t.Fatal(err)
			}
			foreignStore, err := commandledger.New(foreignDB, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			config.Store = foreignStore
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDesktopDecisionFixture(t)
			config := desktopDecisionFactoryConfig(t, f)
			tc.mutate(t, f, &config)
			if _, err := f.app.newCommandDesktopExecutor(config, f.state); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
				t.Fatalf("configuração inválida aceita: %v", err)
			}
		})
	}
}

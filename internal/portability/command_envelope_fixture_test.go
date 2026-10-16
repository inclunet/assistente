package portability

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	applyImportCommand   = "workspace.tab.new"
	applyImportTrigger   = `{"version":1,"code":"KeyA","modifiers":[]}`
	applyImportCondition = `{"version":1,"clauses":[]}`
)

type applyImportPresenter struct {
	calls int
}

func (p *applyImportPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	p.calls++
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

type applyImportSession struct {
	principal auth.LocalSessionPrincipal
}

func (s applyImportSession) AuthenticateLocalAccess(context.Context, string) (auth.LocalSessionPrincipal, error) {
	return s.principal, nil
}

type applyImportFixture struct {
	db         *gorm.DB
	store      *commandconfig.Store
	service    *commandconfig.CompleteMutationService
	projection *commandconfig.CompleteProjection
	presenter  *applyImportPresenter
	user       string
	session    string
	layer      commandconfig.Layer
	before     commandconfig.Snapshot
}

func newApplyImportFixture(t *testing.T, hook commandconfig.MutationTxHook) applyImportFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "command-import.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	if err := commandconfig.Migrate(ctx, db); err != nil {
		t.Fatalf("commandconfig.Migrate: %v", err)
	}
	if err := commandactivation.Migrate(ctx, db); err != nil {
		t.Fatalf("commandactivation.Migrate: %v", err)
	}
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatalf("commanddecision.Migrate: %v", err)
	}
	store, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	user := applyImportUUID(t)
	session := applyImportUUID(t)
	layer := commandconfig.Layer{
		ID: applyImportUUID(t), UserID: user, Name: "destino", Description: "camada atual",
		Enabled: true, Source: "user", ResolutionPriority: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	generation := commandconfig.Generation{ID: applyImportUUID(t), UserID: user, Generation: 1, UpdatedAt: time.Now().UTC()}
	if err := db.Create(&generation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	before, err := store.Load(ctx, commandconfig.Scope{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	presenter := &applyImportPresenter{}
	receipts, err := commanddecision.New(db, presenter, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	registry := applyImportRegistry(t)
	projection := commandconfig.CompleteProjection{
		Registry: registry,
		TriggerPorts: map[commandcatalog.Source]commandconfig.TriggerPort{
			commandcatalog.KeyboardLocal: commandconfig.TriggerPortFunc(func(_ context.Context, raw []byte) (string, error) {
				if string(raw) != applyImportTrigger {
					return "", errors.New("trigger fixture desconhecido")
				}
				return "keyboard.local:KeyA", nil
			}),
		},
		ActiveUserLayerIDs: []string{layer.ID},
	}
	service, err := commandconfig.NewCompleteMutationService(commandconfig.MutationServiceConfig{
		Store: store, Sessions: applyImportSession{principal: auth.LocalSessionPrincipal{UserID: user, SessionID: session}},
		Epochs: epochs, Receipts: receipts,
		Keys:       func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil },
		KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(_ context.Context, _ auth.LocalSessionPrincipal, _ commandconfig.Scope, operation commandconfig.Operation) error {
			if operation != commandconfig.ConfigImport {
				return errors.New("operação inesperada")
			}
			return nil
		},
		Version:      func(context.Context) (string, error) { return "catalog-v1", nil },
		Render:       func(commandconfig.MutationDiff) (string, error) { return "diff de import", nil },
		OnMutationTx: hook,
	}, func(context.Context, commandconfig.Scope) (commandconfig.CompleteProjection, error) {
		return projection, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return applyImportFixture{db: db, store: store, service: service, projection: &projection, presenter: presenter, user: user, session: session, layer: layer, before: before}
}

func applyImportRegistry(t *testing.T, sensitivePaths ...string) *commandcatalog.Registry {
	t.Helper()
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Novo", Description: "Cria", Category: "Workspace"},
		"en":    {Name: "New", Description: "Create", Category: "Workspace"},
		"es":    {Name: "Nuevo", Description: "Crea", Category: "Workspace"},
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{
		Definition: commandcatalog.Definition{
			ID: applyImportCommand, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal}, Context: commandcatalog.ContextPolicy{None: true},
			Presentation:    &commandcatalog.Presentation{Version: "1", Locales: locales},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"query": {Type: commandcatalog.SchemaString, Optional: true}}},
			SensitivePaths:  commandcatalog.SensitivePaths{Input: sensitivePaths},
			ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk:            commandcatalog.RiskLow,
			Persistence:     commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
			Scopes:          []commandcatalog.Scope{commandcatalog.ScopeGlobal}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/workspace/tab/new", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/workspace/tab/new", Classification: commandcatalog.HandlerInternal},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func applyImportUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

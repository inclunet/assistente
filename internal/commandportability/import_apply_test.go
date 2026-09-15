package commandportability

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"assistente/internal/auth"
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
	db        *gorm.DB
	store     *commandconfig.Store
	service   *commandconfig.CompleteMutationService
	presenter *applyImportPresenter
	user      string
	session   string
	layer     commandconfig.Layer
	before    commandconfig.Snapshot
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
	return applyImportFixture{db: db, store: store, service: service, presenter: presenter, user: user, session: session, layer: layer, before: before}
}

func applyImportRegistry(t *testing.T) *commandcatalog.Registry {
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
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
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

func applyImportRefs(t *testing.T, f applyImportFixture) ReferencePort {
	return ReferencePort{
		Catalog: applyImportRegistry(t),
		Trigger: func(_ context.Context, triggerType, raw string) (string, error) {
			if triggerType != "keyboard.local" || raw != applyImportTrigger {
				return "", ErrInvalid
			}
			return "keyboard.local:KeyA", nil
		},
		Workspace: func(_ context.Context, id string) (string, error) { return id, nil },
	}
}

func applyImportOwner(f applyImportFixture) OwnershipPort {
	return func(_ context.Context, _, id string) (Ownership, error) {
		if id == f.layer.ID {
			return CurrentUserOwner, nil
		}
		return AbsentOwner, nil
	}
}

func applyImportLayer(f applyImportFixture, name string, binding *BindingExport) LayerExport {
	layer := LayerExport{ID: f.layer.ID, Scope: PortableScope{Kind: GlobalScope}, Name: name, Description: "importada", Enabled: true, ResolutionPriority: 1}
	if binding != nil {
		layer.Bindings = []BindingExport{*binding}
	}
	return layer
}

func applyImportBinding(t *testing.T, layerID string, commandID string) BindingExport {
	t.Helper()
	return BindingExport{
		ID: applyImportUUID(t), LayerRefKind: "user", LayerRef: layerID,
		TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger,
		CommandID: &commandID, Arguments: `{}`, Condition: applyImportCondition,
		Effect: "execute", Enabled: true, ResolutionPriority: 2,
		ReviewStatus: "active", Presentation: `{"version":1}`,
	}
}

func TestApplyPlanImportKeepExistenteViraNoopIdempotenteSemDecisao(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	refs := applyImportRefs(t, f)
	input := applyImportLayer(f, "conteúdo diferente", nil)
	_, err := ApplyPlanImport(context.Background(), f.service, "token", nil, []LayerExport{input}, PlanOptions{Mode: KeepMode}, applyImportOwner(f), refs)
	if !errors.Is(err, commandconfig.ErrInvalid) {
		t.Fatalf("Keep idempotente retornou %v, esperado no-op sem mutação", err)
	}
	if f.presenter.calls != 0 {
		t.Fatalf("no-op abriu decisão: calls=%d", f.presenter.calls)
	}
	after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Layers, f.before.Layers) || !reflect.DeepEqual(after.Bindings, f.before.Bindings) || !reflect.DeepEqual(after.Generations, f.before.Generations) {
		t.Fatalf("Keep no-op alterou o estado: before=%+v after=%+v", f.before, after)
	}
}

func TestApplyPlanImportReferenciaAusenteFalhaAntesDaDecisao(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	refs := applyImportRefs(t, f)
	missing := "workspace.tab.missing"
	input := applyImportLayer(f, "com referência", func() *BindingExport {
		binding := applyImportBinding(t, f.layer.ID, missing)
		return &binding
	}())
	_, err := ApplyPlanImport(context.Background(), f.service, "token", nil, []LayerExport{input}, PlanOptions{Mode: ReplaceMode}, applyImportOwner(f), refs)
	if !errors.Is(err, ErrMissingReference) {
		t.Fatalf("referência ausente retornou %v", err)
	}
	if f.presenter.calls != 0 {
		t.Fatalf("referência ausente abriu decisão: calls=%d", f.presenter.calls)
	}
	after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Layers, f.before.Layers) || !reflect.DeepEqual(after.Bindings, f.before.Bindings) || !reflect.DeepEqual(after.Generations, f.before.Generations) {
		t.Fatalf("referência ausente alterou o estado: before=%+v after=%+v", f.before, after)
	}
}

func TestApplyPlanImportReplaceRollbackDoLotePreservaTudo(t *testing.T) {
	hookErr := errors.New("falha do hook do lote")
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return hookErr })
	refs := applyImportRefs(t, f)
	commandID := applyImportCommand
	binding := applyImportBinding(t, f.layer.ID, commandID)
	input := applyImportLayer(f, "substituta", &binding)
	_, err := ApplyPlanImport(context.Background(), f.service, "token", nil, []LayerExport{input}, PlanOptions{Mode: ReplaceMode}, applyImportOwner(f), refs)
	if !errors.Is(err, hookErr) {
		t.Fatalf("falha do hook não propagada: %v", err)
	}
	if f.presenter.calls != 1 {
		t.Fatalf("lote não passou pela decisão: calls=%d", f.presenter.calls)
	}
	after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Layers, f.before.Layers) || !reflect.DeepEqual(after.Bindings, f.before.Bindings) || !reflect.DeepEqual(after.Generations, f.before.Generations) {
		t.Fatalf("rollback não restaurou o lote: before=%+v after=%+v", f.before, after)
	}
	var audits int64
	if err := f.db.Table("command_config_mutations").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatalf("auditoria de mutação sobreviveu ao rollback: %d", audits)
	}
}

func TestApplyPlanImportLoteMultiEscopoFalhaFechadoNaAPIDeEscopoUnico(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	refs := applyImportRefs(t, f)
	workspace := "workspace-destino"
	refs.Workspace = func(_ context.Context, id string) (string, error) {
		if id != workspace {
			return "", ErrWorkspaceResolution
		}
		return id, nil
	}
	localID := applyImportUUID(t)
	local := LayerExport{ID: localID, Scope: PortableScope{Kind: WorkspaceScope, WorkspaceID: "workspace-origem"}, Name: "local", Description: "importada", Enabled: true, ResolutionPriority: 1}
	global := applyImportLayer(f, "global", nil)
	_, err := ApplyPlanImport(context.Background(), f.service, "token", nil, []LayerExport{global, local}, PlanOptions{
		Mode: ReplaceMode, WorkspaceMap: map[string]string{"workspace-origem": workspace},
	}, applyImportOwner(f), refs)
	if !errors.Is(err, commandconfig.ErrInvalid) {
		t.Fatalf("lote multi-escopo retornou %v, esperado falha fechada", err)
	}
	if f.presenter.calls != 0 {
		t.Fatalf("lote multi-escopo abriu decisão: calls=%d", f.presenter.calls)
	}
	after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Layers, f.before.Layers) || !reflect.DeepEqual(after.Generations, f.before.Generations) {
		t.Fatalf("lote multi-escopo alterou estado: before=%+v after=%+v", f.before, after)
	}
}

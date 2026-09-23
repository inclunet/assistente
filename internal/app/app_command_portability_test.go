package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandautomation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"assistente/internal/questionnaire"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAppCommandPortabilityImportConfirmsPersistsRebuildsAndKeeps(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	accessToken, _ := appCommandPortabilitySession(t, a)
	db := database.DB()
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	layer := commandconfig.Layer{
		ID:          appCommandPortabilityUUID(t),
		UserID:      a.currentUserID,
		Name:        "portabilidade-confirmada",
		Description: "camada importada pelo App",
		Enabled:     true,
		Source:      "user",
	}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	commandID := "fixture.complete"
	binding := commandconfig.Binding{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, LayerRefKind: "user", LayerRef: layer.ID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1,"title_key":"commands.fixture"}`}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	refs := appCommandPortabilityRefs(t)
	raw, err := portability.ExportCommandEnvelope(ctx, db, commandconfig.Scope{UserID: a.currentUserID}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&commandconfig.Binding{}, "id = ?", binding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&commandconfig.Layer{}, "id = ?", layer.ID).Error; err != nil {
		t.Fatal(err)
	}

	var decisions chan map[string]any
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	applier := appCommandPortabilityApplier(t, a)
	// A identidade no contexto é deliberadamente estranha: a operação deve
	// substituir esse marker pela identidade autenticada do token antes de
	// escrever ou reconstruir.
	importCtx := database.WithUserID(ctx, "foreign-context-user")
	decisions = make(chan map[string]any, 1)
	resultCh := make(chan struct {
		result commandMutationResult
		err    error
	}, 1)
	operationCtx, cancel := context.WithTimeout(importCtx, 10*time.Second)
	defer cancel()
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case <-resultCh:
		case <-time.After(10 * time.Second):
			t.Errorf("worker do import não encerrou após cancelamento")
		}
	}()
	go func() {
		result, importErr := applier.ImportEnvelope(operationCtx, accessToken, nil, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, refs)
		resultCh <- struct {
			result commandMutationResult
			err    error
		}{result, importErr}
	}()
	var decision map[string]any
	select {
	case decision = <-decisions:
	case completed := <-resultCh:
		finished = true
		t.Fatalf("import terminou antes da confirmação: %+v, erro=%v", completed.result, completed.err)
	case <-time.After(10 * time.Second):
		t.Fatal("import não apresentou confirmação")
	}
	decisionID, ok := decision["id"].(string)
	if !ok || decisionID == "" {
		t.Fatalf("ID de decisão inválido: %#v", decision["id"])
	}
	if err := a.questionnaireMgr.Respond(decisionID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	var completed struct {
		result commandMutationResult
		err    error
	}
	select {
	case completed = <-resultCh:
		finished = true
	case <-time.After(10 * time.Second):
		t.Fatal("import confirmado não terminou")
	}
	if completed.err != nil || !completed.result.Committed || !completed.result.Rebuilt {
		t.Fatalf("import confirmado: result=%+v err=%v", completed.result, completed.err)
	}

	var persisted commandconfig.Layer
	if err := db.Where("id = ?", layer.ID).First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Name != layer.Name || persisted.UserID != a.currentUserID {
		t.Fatalf("camada persistida incorreta: %+v", persisted)
	}
	if _, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID); err != nil {
		t.Fatalf("mapa não foi republicado após import: %v", err)
	}
	configuration, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Bindings) != 1 || snapshot.Bindings[0].ID != binding.ID {
		t.Fatalf("binding importado ausente: %+v", snapshot.Bindings)
	}
	expected, err := commandconfig.ProjectComplete(ctx, snapshot, mutationProjectionOptions(refs.Catalog))
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.Equivalent(expected) {
		t.Fatal("mapa publicado diverge da configuração persistida")
	}

	if _, err := applier.ImportEnvelope(operationCtx, accessToken, nil, raw, commandportability.PlanOptions{Mode: commandportability.KeepMode}, refs); !errors.Is(err, commandportability.ErrNoChanges) {
		t.Fatalf("Keep repetido: %v", err)
	}
}

func TestAppCommandPortabilityImportRevokedRollsBackWithoutRebuild(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	accessToken, refreshToken := appCommandPortabilitySession(t, a)
	db := database.DB()
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	refs := appCommandPortabilityRefs(t)
	layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "portabilidade-revogada", Enabled: true, Source: "user"}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	commandID := "fixture.complete"
	binding := commandconfig.Binding{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, LayerRefKind: "user", LayerRef: layer.ID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1,"title_key":"commands.fixture"}`}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := portability.ExportCommandEnvelope(ctx, db, commandconfig.Scope{UserID: a.currentUserID}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&commandconfig.Binding{}, "id = ?", binding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&commandconfig.Layer{}, "id = ?", layer.ID).Error; err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
	if err != nil {
		t.Fatal(err)
	}

	decisions := make(chan map[string]any, 1)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	applier := appCommandPortabilityApplier(t, a)
	importCtx := database.WithUserID(ctx, "foreign-context-user")
	resultCh := make(chan struct {
		result commandMutationResult
		err    error
	}, 1)
	operationCtx, cancel := context.WithTimeout(importCtx, 10*time.Second)
	defer cancel()
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case <-resultCh:
		case <-time.After(10 * time.Second):
			t.Errorf("worker revogado não encerrou após cancelamento")
		}
	}()
	go func() {
		result, importErr := applier.ImportEnvelope(operationCtx, accessToken, nil, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, refs)
		resultCh <- struct {
			result commandMutationResult
			err    error
		}{result, importErr}
	}()
	var decision map[string]any
	select {
	case decision = <-decisions:
	case completed := <-resultCh:
		finished = true
		t.Fatalf("import revogado terminou antes da confirmação: %+v, erro=%v", completed.result, completed.err)
	case <-time.After(10 * time.Second):
		t.Fatal("import revogado não apresentou confirmação")
	}
	if err := a.sessionSvc.Logout(ctx, refreshToken); err != nil {
		t.Fatal(err)
	}
	if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	var completed struct {
		result commandMutationResult
		err    error
	}
	select {
	case completed = <-resultCh:
		finished = true
	case <-time.After(10 * time.Second):
		t.Fatal("import revogado não terminou")
	}
	if completed.err == nil || completed.result.Committed || completed.result.Rebuilt {
		t.Fatalf("import revogado: result=%+v err=%v", completed.result, completed.err)
	}
	var count int64
	if err := db.Model(&commandconfig.Layer{}).Where("id = ?", layer.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback deixou camada persistida: %d", count)
	}
	afterSnapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeSnapshot, afterSnapshot) {
		t.Fatalf("snapshot persistido mudou após rollback: antes=%+v depois=%+v", beforeSnapshot, afterSnapshot)
	}
}

func appCommandPortabilitySession(t *testing.T, a *App) (string, string) {
	t.Helper()
	var user database.User
	if err := database.DB().Where("id = ?", a.currentUserID).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	pair, err := a.sessionSvc.IssueSession(context.Background(), &user, "command-portability-test")
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: user.Role})
	configuration, active, err := a.commandHost.UserConfiguration(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.commandHost.RebuildUserConfiguration(context.Background(), func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		return a.sessionSvc.AuthenticateLocalAccess(ctx, pair.AccessToken)
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, active, nil
	}); err != nil {
		t.Fatal(err)
	}
	return pair.AccessToken, pair.RefreshToken
}

func appCommandPortabilityUUID(t *testing.T) string {
	t.Helper()
	return uuid.Must(uuid.NewV7()).String()
}

func appCommandPortabilityRefs(t *testing.T) commandportability.ReferencePort {
	t.Helper()
	registry := appCommandPortabilityRegistry(t)
	return commandportability.ReferencePort{
		Catalog: registry,
		Trigger: func(_ context.Context, typ, raw string) (string, error) {
			if typ != "keyboard.local" || raw != `{"version":1,"code":"KeyA","modifiers":[]}` {
				return "", commandportability.ErrInvalid
			}
			return "keyboard.local:KeyA", nil
		},
	}
}

func appCommandPortabilityRegistry(t *testing.T) *commandcatalog.Registry {
	t.Helper()
	registry, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Lookup("fixture.complete")
	if !ok {
		t.Fatal("comando fixture.complete ausente")
	}
	definition.AllowedSources = []commandcatalog.Source{commandcatalog.KeyboardLocal}
	definition.Scopes = []commandcatalog.Scope{commandcatalog.ScopeGlobal}
	registry, err = commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: commandcatalog.HandlerContract{
		Effect: definition.Effect, Route: definition.HandlerRoute, Classification: definition.HandlerClassification,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func appCommandPortabilityApplier(t *testing.T, a *App) *commandMutationApplier {
	t.Helper()
	registry := appCommandPortabilityRegistry(t)
	inputs := commandCompleteMutationInputs{
		Projection: func(context.Context, commandconfig.Scope) (commandconfig.CompleteProjection, error) {
			return mutationProjectionOptions(registry), nil
		},
		Authorize: func(context.Context, auth.LocalSessionPrincipal, commandconfig.Scope, commandconfig.Operation) error {
			return nil
		},
		Version:      func(context.Context) (string, error) { return "portability-test-v1", nil },
		Render:       func(commandconfig.MutationDiff) (string, error) { return "portability", nil },
		OnMutationTx: func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil },
		DecisionTTL:  time.Minute,
	}
	applier, err := a.newCommandMutationApplier(inputs)
	if err != nil {
		t.Fatal(err)
	}
	return applier
}

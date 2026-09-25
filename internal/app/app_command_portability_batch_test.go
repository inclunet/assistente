package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
)

func TestAppCommandPortabilityBatchConfirmaGlobalWorkspaceKeepNoopEMapaAtivo(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		t.Run(strconv.FormatBool(inactive), func(t *testing.T) {
			testAppCommandBatchSuccess(t, inactive)
		})
	}
}

func testAppCommandBatchSuccess(t *testing.T, importInactive bool) {
	a, access, raw, globalID, workspaceID, workspace := appCommandPortabilityBatchFixture(t)
	publicationWorkspace := workspace
	if importInactive {
		other, err := a.workspaceMgr.Create("workspace fora do lote")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.workspaceMgr.Switch(other.ID); err != nil {
			t.Fatal(err)
		}
		publicationWorkspace = other.ID
		store, err := commandconfig.New(database.DB())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureScope(context.Background(), commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &publicationWorkspace}); err != nil {
			t.Fatal(err)
		}
	}
	decisions := make(chan map[string]any, 2)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	applier := appCommandPortabilityApplier(t, a)
	principal := auth.LocalSessionPrincipal{UserID: a.currentUserID, SessionID: a.currentAuthUser.SessionID}
	beforeVersions, err := a.commandHost.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), "foreign-context-user"), 30*time.Second)
	defer cancel()
	type outcome struct {
		result commandMutationBatchResult
		err    error
	}
	resultCh := make(chan outcome, 1)
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case <-resultCh:
		case <-time.After(30 * time.Second):
			t.Errorf("worker batch não encerrou após cancelamento")
		}
	}()
	go func() {
		result, err := applier.ImportEnvelopeBatch(ctx, access, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
		resultCh <- outcome{result: result, err: err}
	}()
	for i := 0; i < 2; i++ {
		var decision map[string]any
		select {
		case decision = <-decisions:
		case completed := <-resultCh:
			finished = true
			t.Fatalf("batch terminou antes da confirmação %d: %+v", i+1, completed)
		case <-time.After(30 * time.Second):
			t.Fatal("confirmação do batch não foi apresentada")
		}
		id, ok := decision["id"].(string)
		if !ok || id == "" {
			t.Fatalf("decisão sem ID: %#v", decision)
		}
		if err := a.questionnaireMgr.Respond(id, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatal(err)
		}
	}
	var completed outcome
	select {
	case completed = <-resultCh:
		finished = true
	case <-time.After(30 * time.Second):
		t.Fatal("batch confirmado não terminou")
	}
	if completed.err != nil || !completed.result.Committed || !completed.result.Rebuilt || len(completed.result.Diffs) != 2 {
		t.Fatalf("batch confirmado: result=%+v err=%v", completed.result, completed.err)
	}
	if completed.result.Report == nil || completed.result.Report.NoChanges || len(completed.result.Report.Layers) != 2 {
		t.Fatalf("relatório não chegou ao App: %+v", completed.result.Report)
	}
	afterVersions, err := a.commandHost.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if beforeVersions.GlobalConfig == afterVersions.GlobalConfig || beforeVersions.ActiveLayers == afterVersions.ActiveLayers {
		t.Fatalf("rebuild não publicou duas gerações observáveis: antes=%+v depois=%+v", beforeVersions, afterVersions)
	}
	// Uma publicação reserva exatamente o par config/camadas. O lote não
	// pode publicar um par intermediário para cada escopo importado.
	sequence := func(version string) uint64 {
		t.Helper()
		_, suffix, ok := strings.Cut(version, ":")
		n, err := strconv.ParseUint(suffix, 10, 64)
		if !ok || err != nil {
			t.Fatalf("geração inválida: %q", version)
		}
		return n
	}
	if sequence(afterVersions.GlobalConfig) != sequence(beforeVersions.ActiveLayers)+1 || sequence(afterVersions.ActiveLayers) != sequence(beforeVersions.ActiveLayers)+2 {
		t.Fatalf("lote publicou mais de uma vez: antes=%+v depois=%+v", beforeVersions, afterVersions)
	}
	assertCommandPortabilityBatchPersisted(t, a, globalID, workspaceID, workspace)
	assertCommandPortabilityBatchMap(t, a, publicationWorkspace)
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}

	keep, err := applier.ImportEnvelopeBatch(ctx, access, raw, commandportability.PlanOptions{Mode: commandportability.KeepMode}, appCommandPortabilityBatchRefs(t))
	if !errors.Is(err, commandportability.ErrNoChanges) || keep.Committed || keep.Rebuilt || len(keep.Diffs) != 0 {
		t.Fatalf("Keep não foi no-op: result=%+v err=%v", keep, err)
	}
	if keep.Report == nil || !keep.Report.NoChanges {
		t.Fatal("Keep deve preservar relatório explícito de nenhuma alteração")
	}
	if err := store.CheckCurrent(ctx, persisted); err != nil {
		t.Fatalf("Keep alterou snapshot persistido: %v", err)
	}
	keepVersions, err := a.commandHost.Snapshot(ctx, principal)
	if err != nil || keepVersions.GlobalConfig != afterVersions.GlobalConfig || keepVersions.ActiveLayers != afterVersions.ActiveLayers {
		t.Fatalf("Keep alterou publicação: %+v err=%v", keepVersions, err)
	}
	var audits int64
	if err := database.DB().Table("command_config_mutations").Where("user_id = ? AND operation = ?", a.currentUserID, string(commandconfig.ConfigImport)).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatalf("lote/Keep deveria gerar somente duas auditorias: %d err=%v", audits, err)
	}
}

func TestAppCommandPortabilityBatchNegaSegundaConfirmacaoSemCommitNemRebuild(t *testing.T) {
	a, access, raw, globalID, workspaceID, workspace := appCommandPortabilityBatchFixture(t)
	before, _, err := a.commandHost.UserConfiguration(context.Background(), a.currentUserID)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make(chan map[string]any, 2)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	applier := appCommandPortabilityApplier(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type outcome struct {
		result commandMutationBatchResult
		err    error
	}
	resultCh := make(chan outcome, 1)
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case <-resultCh:
		case <-time.After(30 * time.Second):
			t.Errorf("worker batch negado não encerrou após cancelamento")
		}
	}()
	go func() {
		result, err := applier.ImportEnvelopeBatch(ctx, access, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
		resultCh <- outcome{result: result, err: err}
	}()
	for i := 0; i < 2; i++ {
		var decision map[string]any
		select {
		case decision = <-decisions:
		case completed := <-resultCh:
			finished = true
			t.Fatalf("batch terminou antes da confirmação %d: %+v", i+1, completed)
		case <-time.After(30 * time.Second):
			t.Fatal("confirmação esperada não foi apresentada")
		}
		action := commanddecision.ApplyAction
		if i == 1 {
			action = commanddecision.DenyAction
		}
		if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: action}, false); err != nil {
			t.Fatal(err)
		}
	}
	var completed outcome
	select {
	case completed = <-resultCh:
		finished = true
	case <-time.After(30 * time.Second):
		t.Fatal("batch negado não terminou")
	}
	if completed.err == nil || completed.result.Committed || completed.result.Rebuilt || len(completed.result.Diffs) != 0 {
		t.Fatalf("negação da segunda confirmação: result=%+v err=%v", completed.result, completed.err)
	}
	if completed.result.Report != nil {
		t.Fatal("importação negada não tem relatório de importação concluída")
	}
	assertCommandPortabilityBatchAbsent(t, a, globalID, workspaceID, workspace)
	after, _, err := a.commandHost.UserConfiguration(context.Background(), a.currentUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Equivalent(after) {
		t.Fatal("negação alterou o mapa publicado")
	}
}

func appCommandPortabilityBatchFixture(t *testing.T) (*App, string, []byte, string, string, string) {
	t.Helper()
	a := readyCommandProduct(t)
	access, _ := appCommandPortabilitySession(t, a)
	ctx := context.Background()
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	if a.workspaceMgr == nil {
		manager := workspace.NewManager(t.TempDir())
		if err := manager.Initialize(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		a.workspaceMgr = manager
	}
	active, err := a.workspaceMgr.Create("batch ativo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceMgr.Switch(active.ID); err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureScope(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &active.ID}); err != nil {
		t.Fatal(err)
	}
	globalID, workspaceID := appCommandPortabilityUUID(t), appCommandPortabilityUUID(t)
	globalCommand, workspaceCommand := "fixture.complete", "fixture.complete"
	layers := []commandportability.LayerExport{
		{ID: globalID, Scope: commandportability.PortableScope{Kind: commandportability.GlobalScope}, Name: "batch global", Enabled: true, Bindings: []commandportability.BindingExport{{ID: appCommandPortabilityUUID(t), LayerRefKind: "user", LayerRef: globalID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &globalCommand, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}},
		{ID: workspaceID, Scope: commandportability.PortableScope{Kind: commandportability.WorkspaceScope, WorkspaceID: active.ID}, Name: "batch workspace", Enabled: true, Bindings: []commandportability.BindingExport{{ID: appCommandPortabilityUUID(t), LayerRefKind: "user", LayerRef: workspaceID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &workspaceCommand, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}},
	}
	raw, err := json.Marshal(portability.ExportFile{Version: portability.ExportVersion, ExportedAt: time.Now().UTC(), Resources: portability.ExportResources{CommandLayers: layers}})
	if err != nil {
		t.Fatal(err)
	}
	return a, access, raw, globalID, workspaceID, active.ID
}

func appCommandPortabilityBatchRefs(t *testing.T) commandportability.ReferencePort {
	refs := appCommandPortabilityRefs(t)
	refs.Workspace = func(_ context.Context, id string) (string, error) { return id, nil }
	return refs
}

func assertCommandPortabilityBatchPersisted(t *testing.T, a *App, globalID, workspaceID, workspace string) {
	t.Helper()
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.Load(context.Background(), commandconfig.Scope{UserID: a.currentUserID})
	if err != nil || !appCommandBatchContainsLayer(global.Layers, globalID) || appCommandBatchContainsLayer(global.Layers, workspaceID) {
		t.Fatalf("persistência global: %+v err=%v", global, err)
	}
	union, err := store.Load(context.Background(), commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &workspace})
	if err != nil || !appCommandBatchContainsLayer(union.Layers, globalID) || !appCommandBatchContainsLayer(union.Layers, workspaceID) {
		t.Fatalf("persistência workspace: %+v err=%v", union, err)
	}
}

func assertCommandPortabilityBatchAbsent(t *testing.T, a *App, globalID, workspaceID, workspace string) {
	t.Helper()
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.Load(context.Background(), commandconfig.Scope{UserID: a.currentUserID})
	if err != nil || appCommandBatchContainsLayer(global.Layers, globalID) || appCommandBatchContainsLayer(global.Layers, workspaceID) {
		t.Fatalf("rollback global: %+v err=%v", global, err)
	}
	union, err := store.Load(context.Background(), commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &workspace})
	if err != nil || appCommandBatchContainsLayer(union.Layers, globalID) || appCommandBatchContainsLayer(union.Layers, workspaceID) {
		t.Fatalf("rollback workspace: %+v err=%v", union, err)
	}
}

func appCommandBatchContainsLayer(layers []commandconfig.Layer, id string) bool {
	for _, layer := range layers {
		if layer.ID == id {
			return true
		}
	}
	return false
}

func assertCommandPortabilityBatchMap(t *testing.T, a *App, workspace string) {
	t.Helper()
	configuration, active, err := a.commandHost.UserConfiguration(context.Background(), a.currentUserID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(context.Background(), commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	options := mutationProjectionOptions(appCommandPortabilityRefs(t).Catalog)
	options.ActiveUserLayerIDs = active
	expected, err := commandconfig.ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.Equivalent(expected) {
		t.Fatal("mapa publicado diverge da projeção do workspace ativo")
	}
}

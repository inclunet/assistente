package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"assistente/internal/questionnaire"
	"github.com/google/uuid"
)

type appCommandImportWailsCopyOutcome struct {
	result *portability.ImportResult
	err    error
}

func appCommandImportWailsCopyFixture(t *testing.T) (*App, context.Context, context.CancelFunc, portability.ImportRequest, []string) {
	t.Helper()
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	current, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	commandID := commandProductWorkspaceListID
	globalID, workspaceID := appCommandPortabilityUUID(t), appCommandPortabilityUUID(t)
	file := portability.ExportFile{Version: portability.ExportVersion, Resources: portability.ExportResources{CommandLayers: []commandportability.LayerExport{
		{ID: globalID, Scope: commandportability.PortableScope{Kind: commandportability.GlobalScope}, Name: "cópia global", Enabled: true,
			Bindings: []commandportability.BindingExport{{ID: appCommandPortabilityUUID(t), LayerRefKind: "user", LayerRef: globalID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}},
		{ID: workspaceID, Scope: commandportability.PortableScope{Kind: commandportability.WorkspaceScope, WorkspaceID: "source-workspace"}, Name: "cópia workspace", Enabled: true,
			Bindings: []commandportability.BindingExport{{ID: appCommandPortabilityUUID(t), LayerRefKind: "user", LayerRef: workspaceID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}},
	}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	// O teste substitui o presenter visual por uma fila de respostas.
	a.questionnaireMgr = questionnaire.NewManager(func(string, any) {})
	a.wireExportImport()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		t.Fatalf("produto: %v", err)
	}
	inputs, err := a.commandDesktopMutationInputs(p, database.WithUserID(ctx, a.currentUserID))
	if err != nil {
		t.Fatalf("portas desktop: %v", err)
	}
	if _, err := a.newCommandDesktopMutationApplier(inputs); err != nil {
		t.Fatalf("factory desktop: %v", err)
	}
	request := portability.ImportRequest{JSONData: string(raw), Resolutions: []portability.ImportResolution{
		{ResourceType: "commandLayers", Identifier: "*", Strategy: portability.ConflictResolutionRename},
		{ResourceType: "commandLayerName", Identifier: globalID, Strategy: portability.ConflictResolutionRename, RenameValue: "cópia global confirmada"},
		{ResourceType: "commandLayerName", Identifier: workspaceID, Strategy: portability.ConflictResolutionRename, RenameValue: "cópia workspace confirmada"},
		{ResourceType: "commandWorkspace", Identifier: "source-workspace", Strategy: portability.ConflictResolutionRename, RenameValue: current.WorkspaceID},
	}}
	return a, ctx, cancel, request, []string{globalID, workspaceID}
}

func startAppCommandImportWailsCopy(t *testing.T, a *App, request portability.ImportRequest, cancel context.CancelFunc) (<-chan appCommandImportWailsCopyOutcome, *bool) {
	t.Helper()
	completed := make(chan appCommandImportWailsCopyOutcome, 1)
	finished := new(bool)
	go func() {
		result, err := a.exportImportAPI.ImportDataWithResolutions(request)
		completed <- appCommandImportWailsCopyOutcome{result: result, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		if *finished {
			return
		}
		select {
		case <-completed:
		case <-time.After(5 * time.Second):
			t.Error("worker do import Wails não encerrou")
		}
	})
	return completed, finished
}

func appCommandImportWailsCopyDecisions(t *testing.T, a *App) <-chan map[string]any {
	t.Helper()
	decisions := make(chan map[string]any, 4)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	return decisions
}

func appCommandImportWailsRespond(t *testing.T, a *App, decisions <-chan map[string]any, action string) {
	t.Helper()
	select {
	case payload := <-decisions:
		id, ok := payload["id"].(string)
		if !ok || id == "" {
			t.Fatalf("decisão pública sem ID: %#v", payload)
		}
		if err := a.questionnaireMgr.Respond(id, map[string]any{questionnaire.AnswerActionID: action}, false); err != nil {
			t.Fatalf("resposta pública: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decisão pública não foi apresentada")
	}
}

func appCommandImportWailsReportTargetIDs(t *testing.T, result *portability.ImportResult) map[string]struct{} {
	t.Helper()
	if result == nil {
		t.Fatal("resultado público ausente")
	}
	ids := make(map[string]struct{})
	for _, warning := range result.Warnings {
		if warning.Code != "commandImport.layerResult" {
			continue
		}
		id := warning.Params["targetId"]
		if id == "" {
			t.Fatalf("aviso público sem targetId: %+v", warning)
		}
		ids[id] = struct{}{}
	}
	return ids
}

func TestCommandImportWailsCopyUsesNewUUIDsFromPersistedReportAndRollsBackAtomically(t *testing.T) {
	t.Run("copia UUID novo e relatório coincide com persistência", func(t *testing.T) {
		a, ctx, cancel, request, sourceIDs := appCommandImportWailsCopyFixture(t)
		decisions := appCommandImportWailsCopyDecisions(t, a)
		completed, finished := startAppCommandImportWailsCopy(t, a, request, cancel)
		for i := 0; i < 2; i++ {
			appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		}
		var outcome appCommandImportWailsCopyOutcome
		select {
		case outcome = <-completed:
			*finished = true
		case <-time.After(5 * time.Second):
			t.Fatal("import público de cópia não terminou")
		}
		if outcome.err != nil || outcome.result == nil || !outcome.result.Success || outcome.result.Imported != 2 {
			t.Fatalf("import público de cópia: result=%+v err=%v", outcome.result, outcome.err)
		}
		reported := appCommandImportWailsReportTargetIDs(t, outcome.result)
		if len(reported) != 2 {
			t.Fatalf("relatório público de cópia: %+v", outcome.result.Warnings)
		}
		for id := range reported {
			if _, err := uuid.Parse(id); err != nil {
				t.Fatalf("targetId não é UUID: %q", id)
			}
			for _, sourceID := range sourceIDs {
				if id == sourceID {
					t.Fatalf("cópia reutilizou UUID de origem: %q", id)
				}
			}
		}
		var persisted []commandconfig.Layer
		if err := database.DB().Where("user_id = ?", a.currentUserID).Find(&persisted).Error; err != nil {
			t.Fatal(err)
		}
		persistedIDs := make(map[string]struct{}, len(persisted))
		for _, layer := range persisted {
			persistedIDs[layer.ID] = struct{}{}
		}
		if len(persistedIDs) != len(reported) {
			t.Fatalf("IDs persistidos=%v relatório=%v", persistedIDs, reported)
		}
		for id := range reported {
			if _, ok := persistedIDs[id]; !ok {
				t.Fatalf("relatório apontou UUID não persistido: %q", id)
			}
		}
		store, err := commandconfig.New(database.DB())
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
		if err != nil || len(snapshot.Layers) != 1 {
			t.Fatalf("snapshot global da cópia: %+v err=%v", snapshot, err)
		}
	})

	t.Run("nega segunda decisão e desfaz o lote", func(t *testing.T) {
		a, ctx, cancel, request, _ := appCommandImportWailsCopyFixture(t)
		before, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID)
		if err != nil {
			t.Fatal(err)
		}
		decisions := appCommandImportWailsCopyDecisions(t, a)
		completed, finished := startAppCommandImportWailsCopy(t, a, request, cancel)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.DenyAction)
		var outcome appCommandImportWailsCopyOutcome
		select {
		case outcome = <-completed:
			*finished = true
		case <-time.After(5 * time.Second):
			t.Fatal("import negado não terminou")
		}
		if outcome.err == nil && outcome.result != nil && outcome.result.Success {
			t.Fatalf("segunda decisão negada reportou sucesso: %+v", outcome.result)
		}
		var count int64
		if err := database.DB().Model(&commandconfig.Layer{}).Where("user_id = ?", a.currentUserID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback deixou %d camadas persistidas", count)
		}
		after, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID)
		if err != nil || !before.Equivalent(after) {
			t.Fatalf("rollback alterou mapa publicado: err=%v", err)
		}
	})
}

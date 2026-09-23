package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"assistente/internal/questionnaire"
)

// Usa a fachada que o frontend chama, a sessão desktop e o catálogo produtivo.
// Não substitui autenticação, writer, hook de ativação nem publicação por mocks.
func TestCommandImportWailsGlobalWorkspaceAndKeep(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	current, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	command := commandProductWorkspaceListID
	globalID, localID := appCommandPortabilityUUID(t), appCommandPortabilityUUID(t)
	file := portability.ExportFile{Version: portability.ExportVersion, Resources: portability.ExportResources{CommandLayers: []commandportability.LayerExport{
		{ID: globalID, Scope: commandportability.PortableScope{Kind: commandportability.GlobalScope}, Name: "public global", Enabled: true,
			Bindings: []commandportability.BindingExport{{ID: appCommandPortabilityUUID(t), LayerRefKind: "user", LayerRef: globalID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &command, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}},
		{ID: localID, Scope: commandportability.PortableScope{Kind: commandportability.WorkspaceScope, WorkspaceID: "source-workspace"}, Name: "public workspace", Enabled: true},
	}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make(chan map[string]any, 4)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	a.wireExportImport()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		t.Fatalf("produto: %v", err)
	}
	inputs, err := a.commandDesktopMutationInputs(p, database.WithUserID(ctx, a.currentUserID))
	if err != nil {
		t.Fatalf("portas: %v", err)
	}
	if _, err := a.newCommandDesktopMutationApplier(inputs); err != nil {
		t.Fatalf("factory: %v", err)
	}
	analysis, err := a.exportImportAPI.AnalyzeImportData(string(raw), "")
	if err != nil || analysis == nil || analysis.CommandLayerCount != 2 {
		t.Fatalf("analysis: %+v %v", analysis, err)
	}
	request := portability.ImportRequest{JSONData: string(raw), Resolutions: []portability.ImportResolution{
		{ResourceType: "commandLayers", Identifier: "*", Strategy: portability.ConflictResolutionOverwrite},
		{ResourceType: "commandWorkspace", Identifier: "source-workspace", Strategy: portability.ConflictResolutionRename, RenameValue: current.WorkspaceID},
	}}
	type outcome struct {
		result *portability.ImportResult
		err    error
	}
	completed := make(chan outcome, 1)
	finished := false
	defer func() {
		cancel()
		if !finished {
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				t.Error("worker não encerrou")
			}
		}
	}()
	go func() {
		result, err := a.exportImportAPI.ImportDataWithResolutions(request)
		completed <- outcome{result, err}
	}()
	for i := 0; i < 2; i++ {
		select {
		case decision := <-decisions:
			if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
				t.Fatal(err)
			}
		case early := <-completed:
			finished = true
			t.Fatalf("antes da decisão %d: resultado=%+v erro=%v", i, early.result, early.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	var got outcome
	select {
	case got = <-completed:
		finished = true
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got.err != nil || got.result == nil || !got.result.Success {
		t.Fatalf("import público: %+v %v", got.result, got.err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &current.WorkspaceID})
	if err != nil || !slices.ContainsFunc(snapshot.Layers, func(layer commandconfig.Layer) bool { return layer.ID == globalID }) || !slices.ContainsFunc(snapshot.Layers, func(layer commandconfig.Layer) bool { return layer.ID == localID }) || len(snapshot.Bindings) != 1 {
		t.Fatalf("persistência pública: %+v %v", snapshot, err)
	}
	if _, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID); err != nil {
		t.Fatalf("mapa não publicado: %v", err)
	}
	request.Resolutions[0].Strategy = portability.ConflictResolutionSkip
	kept, err := a.exportImportAPI.ImportDataWithResolutions(request)
	if err != nil || kept == nil || !kept.Success || kept.Imported != 0 {
		t.Fatalf("keep público: %+v %v", kept, err)
	}
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		t.Fatalf("keep alterou dados: %v", err)
	}
}

func TestCommandImportWailsRejectsInvalidInputsWithoutConfirmation(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	decisions := make(chan struct{}, 8)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, _ any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- struct{}{}
		}
	})
	a.wireExportImport()
	foreignID := appCommandPortabilityUUID(t)
	if err := database.DB().Create(&commandconfig.Layer{ID: foreignID, UserID: appCommandPortabilityUUID(t), Name: "PRIVATE_FOREIGN_LAYER", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"no_policy", "unknown_workspace", "mixed_resources", "foreign_owner"} {
		t.Run(scenario, func(t *testing.T) {
			layer := commandportability.LayerExport{ID: appCommandPortabilityUUID(t), Scope: commandportability.PortableScope{Kind: commandportability.GlobalScope}, Name: "candidate", Enabled: true}
			if scenario == "foreign_owner" {
				layer.ID = foreignID
			}
			if scenario == "unknown_workspace" {
				layer.Scope = commandportability.PortableScope{Kind: commandportability.WorkspaceScope, WorkspaceID: "not-authorized"}
			}
			file := portability.ExportFile{Version: portability.ExportVersion, Resources: portability.ExportResources{CommandLayers: []commandportability.LayerExport{layer}}}
			if scenario == "mixed_resources" {
				file.Options.IncludeCredentials = true
			}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			req := portability.ImportRequest{JSONData: string(raw)}
			if scenario != "no_policy" {
				req.Resolutions = []portability.ImportResolution{{ResourceType: "commandLayers", Identifier: "*", Strategy: portability.ConflictResolutionOverwrite}}
			}
			result, err := a.exportImportAPI.ImportDataWithResolutions(req)
			if err == nil && (result == nil || result.Success) {
				t.Fatalf("entrada inválida aceita: %+v", result)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "PRIVATE_FOREIGN_LAYER") || err != nil && strings.Contains(err.Error(), "PRIVATE_FOREIGN_LAYER") {
				t.Fatal("vazou nome privado")
			}
			select {
			case <-decisions:
				t.Fatal("entrada inválida abriu decisão")
			default:
			}
			var count int64
			if err := database.DB().Model(&commandconfig.Layer{}).Where("user_id = ?", a.currentUserID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("camadas criadas por tentativa recusada: %d %v", count, err)
			}
		})
	}
}

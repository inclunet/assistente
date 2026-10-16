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
)

func TestCommandExportWailsRoundTripCopyPreservesSourceAndPublication(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	current, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	globalID, workspaceID := appCommandPortabilityUUID(t), appCommandPortabilityUUID(t)
	workspace := current.WorkspaceID
	now := time.Now().UTC()
	layers := []commandconfig.Layer{
		{ID: globalID, UserID: a.currentUserID, Name: "export original global", Description: "global de teste", Enabled: true, Source: "user", ResolutionPriority: 1, CreatedAt: now, UpdatedAt: now},
		{ID: workspaceID, UserID: a.currentUserID, WorkspaceID: &workspace, Name: "export original workspace", Description: "workspace de teste", Enabled: true, Source: "user", ResolutionPriority: 2, CreatedAt: now, UpdatedAt: now},
	}
	if err := database.DB().Create(&layers).Error; err != nil {
		t.Fatalf("camadas de origem: %v", err)
	}
	for _, layer := range layers {
		commandID := commandProductWorkspaceListID
		binding := commandconfig.Binding{
			ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, WorkspaceID: cloneCommandWorkspace(layer.WorkspaceID),
			LayerRefKind: "user", LayerRef: layer.ID, TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID,
			Arguments: `{}`, Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true,
			Source: "user", ReviewStatus: "active", Presentation: `{"version":1}`,
		}
		if err := database.DB().Create(&binding).Error; err != nil {
			t.Fatalf("binding de origem %s: %v", layer.ID, err)
		}
	}

	a.wireExportImport()
	req := portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeWorkspace: true}
	raw, err := a.exportImportAPI.ExportData(req)
	if err != nil {
		t.Fatalf("ExportData: %v", err)
	}

	var exported portability.ExportFile
	if err := json.Unmarshal([]byte(raw), &exported); err != nil {
		t.Fatalf("envelope exportado: %v", err)
	}
	if len(exported.Resources.CommandLayers) != 2 {
		t.Fatalf("camadas exportadas = %d, want 2", len(exported.Resources.CommandLayers))
	}
	if strings.Contains(raw, "userId") || strings.Contains(raw, "claims") || strings.Contains(raw, "grants") || strings.Contains(raw, "owner") {
		t.Fatalf("envelope exportado contém estado não portátil: %s", raw)
	}
	parsed, err := portability.ParseCommandImportEnvelope([]byte(raw))
	if err != nil || len(parsed) != 2 {
		t.Fatalf("ParseCommandImportEnvelope: camadas=%d err=%v", len(parsed), err)
	}
	if !slices.ContainsFunc(parsed, func(layer commandportability.LayerExport) bool { return layer.ID == globalID }) || !slices.ContainsFunc(parsed, func(layer commandportability.LayerExport) bool { return layer.ID == workspaceID }) {
		t.Fatalf("parser não preservou UUIDs de origem: %+v", parsed)
	}

	request := portability.ImportRequest{JSONData: raw, Resolutions: []portability.ImportResolution{
		{ResourceType: "commandLayers", Identifier: "*", Strategy: portability.ConflictResolutionRename},
		{ResourceType: "commandLayerName", Identifier: globalID, Strategy: portability.ConflictResolutionRename, RenameValue: "export cópia global"},
		{ResourceType: "commandLayerName", Identifier: workspaceID, Strategy: portability.ConflictResolutionRename, RenameValue: "export cópia workspace"},
		{ResourceType: "commandWorkspace", Identifier: workspace, Strategy: portability.ConflictResolutionRename, RenameValue: workspace},
	}}
	decisions := appCommandImportWailsCopyDecisions(t, a)
	completed, finished := startAppCommandImportWailsCopy(t, a, request, cancel)
	for i := 0; i < 2; i++ {
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	}
	var outcome appCommandImportWailsCopyOutcome
	select {
	case outcome = <-completed:
		*finished = true
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if outcome.err != nil || outcome.result == nil || !outcome.result.Success || outcome.result.Imported != 2 {
		t.Fatalf("ImportDataWithResolutions copy: result=%+v err=%v", outcome.result, outcome.err)
	}

	reported := appCommandImportWailsReportTargetIDs(t, outcome.result)
	if len(reported) != 2 {
		t.Fatalf("relatório de cópia não aponta UUIDs novos: %+v", reported)
	}
	if _, ok := reported[globalID]; ok {
		t.Fatalf("cópia global reutilizou UUID de origem: %s", globalID)
	}
	if _, ok := reported[workspaceID]; ok {
		t.Fatalf("cópia workspace reutilizou UUID de origem: %s", workspaceID)
	}
	var persisted []commandconfig.Layer
	if err := database.DB().Where("user_id = ?", a.currentUserID).Find(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 4 {
		t.Fatalf("camadas persistidas = %d, want originais+cópias", len(persisted))
	}
	for _, id := range []string{globalID, workspaceID} {
		if !slices.ContainsFunc(persisted, func(layer commandconfig.Layer) bool { return layer.ID == id && layer.UserID == a.currentUserID }) {
			t.Fatalf("origem não preservada no DB: %s", id)
		}
	}
	for id := range reported {
		if !slices.ContainsFunc(persisted, func(layer commandconfig.Layer) bool { return layer.ID == id && layer.UserID == a.currentUserID }) {
			t.Fatalf("cópia não persistida com owner derivado: %s", id)
		}
	}

	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	globalSnapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
	if err != nil || len(globalSnapshot.Layers) != 2 {
		t.Fatalf("publicação/persistência global: %+v err=%v", globalSnapshot, err)
	}
	workspaceSnapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &workspace})
	if err != nil || len(workspaceSnapshot.Layers) != 4 {
		t.Fatalf("publicação/persistência workspace: %+v err=%v", workspaceSnapshot, err)
	}
	if len(globalSnapshot.AutomationGrants) != 0 || len(globalSnapshot.ActivationClaims) != 0 || len(workspaceSnapshot.AutomationGrants) != 0 || len(workspaceSnapshot.ActivationClaims) != 0 {
		t.Fatal("roundtrip restaurou grants/claims")
	}
	if _, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID); err != nil {
		t.Fatalf("mapa publicado após roundtrip: %v", err)
	}
	// TODO: cobrir foreign/missing UUID, workspace sem IncludeWorkspace,
	// payload sensível/misto e sessão revogada/OS bloqueado sem confirmação.
	// TODO: adicionar a matriz de seleção explícita por subconjunto dentro do
	// escopo do owner, mantendo a seleção global+workspace deste roundtrip.
}

func commandExportDesktopTwoLayerFixture(t *testing.T) (*App, context.Context, string, string, string) {
	t.Helper()
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	current, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	globalID, workspaceID := appCommandPortabilityUUID(t), appCommandPortabilityUUID(t)
	workspace := current.WorkspaceID
	for _, layer := range []commandconfig.Layer{
		{ID: globalID, UserID: a.currentUserID, Name: "selection global", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now},
		{ID: workspaceID, UserID: a.currentUserID, WorkspaceID: &workspace, Name: "selection workspace", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.DB().Create(&layer).Error; err != nil {
			t.Fatalf("camada fixture: %v", err)
		}
	}
	a.wireExportImport()
	return a, ctx, globalID, workspaceID, workspace
}

func TestCommandExportDesktopSelectionScopesAndInvalidReferences(t *testing.T) {
	tests := []struct {
		name    string
		request func(globalID, workspaceID string) portability.ExportRequest
		want    int
	}{
		{"global default", func(_, _ string) portability.ExportRequest {
			return portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true}
		}, 1},
		{"todos os workspaces autorizados", func(_, _ string) portability.ExportRequest {
			return portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeWorkspace: true}
		}, 2},
		{"subconjunto explícito", func(globalID, _ string) portability.ExportRequest {
			return portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: []string{globalID}}
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, globalID, workspaceID, _ := commandExportDesktopTwoLayerFixture(t)
			raw, err := a.exportImportAPI.ExportData(tc.request(globalID, workspaceID))
			if err != nil {
				t.Fatalf("ExportData: %v", err)
			}
			layers, err := portability.ParseCommandImportEnvelope([]byte(raw))
			if err != nil || len(layers) != tc.want {
				t.Fatalf("seleção exportada=%d err=%v, want %d", len(layers), err, tc.want)
			}
			if tc.want == 1 && layers[0].ID != globalID {
				t.Fatalf("seleção global retornou ID=%q, want %q", layers[0].ID, globalID)
			}
		})
	}
}

func TestCommandExportDesktopRejectsForeignMissingAndWorkspaceWithoutInclude(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, a *App, globalID, workspaceID string) string
	}{
		{"UUID estrangeiro", func(t *testing.T, a *App, _, _ string) string {
			id := appCommandPortabilityUUID(t)
			layer := commandconfig.Layer{ID: id, UserID: appCommandPortabilityUUID(t), Name: "foreign", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := database.DB().Create(&layer).Error; err != nil {
				t.Fatal(err)
			}
			return id
		}},
		{"UUID ausente", func(t *testing.T, _ *App, _, _ string) string { return appCommandPortabilityUUID(t) }},
		{"workspace sem IncludeWorkspace", func(_ *testing.T, _ *App, _, workspaceID string) string { return workspaceID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, globalID, workspaceID, _ := commandExportDesktopTwoLayerFixture(t)
			id := tc.prepare(t, a, globalID, workspaceID)
			raw, err := a.exportImportAPI.ExportData(portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: []string{id}})
			if err == nil || raw != "" {
				t.Fatalf("referência inválida virou export: raw=%q err=%v", raw, err)
			}
		})
	}
}

func TestCommandExportDesktopRejectsRevokedSessionAndOSLock(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, a *App, ctx context.Context)
	}{
		{"sessão revogada", func(t *testing.T, a *App, ctx context.Context) {
			sessionID := a.commandProduct.Load().principal.SessionID
			if err := database.DB().Model(&database.Session{}).Where("id = ?", sessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"OS bloqueado", func(t *testing.T, a *App, ctx context.Context) {
			if err := a.commandHost.SetOSSessionState(ctx, true, true); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, ctx, _, _, _ := commandExportDesktopTwoLayerFixture(t)
			tc.mutate(t, a, ctx)
			raw, err := a.exportImportAPI.ExportData(portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true})
			if err == nil || raw != "" {
				t.Fatalf("segurança não recusou export: raw=%q err=%v", raw, err)
			}
		})
	}
}

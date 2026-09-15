package commandportability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func portabilityUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func portabilityLayer(t *testing.T) LayerExport {
	t.Helper()
	layerID, bindingID := portabilityUUID(t), portabilityUUID(t)
	command := "workspace.tab.new"
	return LayerExport{
		ID:                 layerID,
		Scope:              PortableScope{Kind: WorkspaceScope, WorkspaceID: "source-workspace"},
		Name:               "Atalhos",
		Description:        "portável",
		Enabled:            true,
		ResolutionPriority: 3,
		Bindings: []BindingExport{{
			ID:                 bindingID,
			LayerRefKind:       "user",
			LayerRef:           layerID,
			TriggerType:        "keyboard.local",
			TriggerSpec:        `{"version":1,"code":"KeyA","modifiers":[]}`,
			CommandID:          &command,
			Arguments:          `{}`,
			Condition:          `{"version":1,"clauses":[]}`,
			Effect:             "execute",
			Enabled:            true,
			ResolutionPriority: 2,
			ReviewStatus:       "active",
			Presentation:       `{}`,
		}},
	}
}

func portabilityRefs(t *testing.T) ReferencePort {
	return portabilityRefsWithSensitivePaths(t, nil)
}

func portabilityRefsWithSensitivePaths(t *testing.T, sensitivePaths []string) ReferencePort {
	t.Helper()
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Novo", Description: "Cria", Category: "Workspace"},
		"en":    {Name: "New", Description: "Create", Category: "Workspace"},
		"es":    {Name: "Nuevo", Description: "Crea", Category: "Workspace"},
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{
		Definition: commandcatalog.Definition{
			ID: "workspace.tab.new", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal}, Context: commandcatalog.ContextPolicy{None: true},
			Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
				"query":      {Type: commandcatalog.SchemaString, Optional: true},
				"token":      {Type: commandcatalog.SchemaString, Optional: true},
				"credential": {Type: commandcatalog.SchemaObject, Optional: true, Properties: map[string]commandcatalog.Schema{"kind": {Type: commandcatalog.SchemaString}, "pattern": {Type: commandcatalog.SchemaString}}, Required: []string{"kind", "pattern"}},
			}}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk:           commandcatalog.RiskLow,
			SensitivePaths: commandcatalog.SensitivePaths{Input: sensitivePaths},
			Persistence:    commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
			Scopes:         []commandcatalog.Scope{commandcatalog.ScopeGlobal}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/workspace/tab/new", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/workspace/tab/new", Classification: commandcatalog.HandlerInternal},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return ReferencePort{
		Catalog:   registry,
		Trigger:   func(_ context.Context, triggerType, _ string) (string, error) { return triggerType + ":KeyA", nil },
		Workspace: func(_ context.Context, id string) (string, error) { return id, nil },
	}
}

func TestFromSnapshotNaoExportaOwnerOuMetadadosDeExecucao(t *testing.T) {
	user := portabilityUUID(t)
	layerID, bindingID := portabilityUUID(t), portabilityUUID(t)
	command := "workspace.tab.new"
	snapshot := commandconfig.Snapshot{
		Scope: commandconfig.Scope{UserID: user},
		Layers: []commandconfig.Layer{{
			ID:                 layerID,
			UserID:             user,
			Name:               "Atalhos",
			Description:        "descrição",
			Enabled:            true,
			Source:             "user",
			ResolutionPriority: 3,
		}},
		Bindings: []commandconfig.Binding{{
			ID:           bindingID,
			UserID:       user,
			LayerRefKind: "user",
			LayerRef:     layerID,
			TriggerType:  "keyboard.local",
			TriggerSpec:  `{}`,
			CommandID:    &command,
			Arguments:    `{}`,
			Condition:    `{}`,
			Effect:       "execute",
			Enabled:      true,
			Source:       "user",
			ReviewStatus: "active",
			Presentation: `{}`,
		}},
	}
	exported, err := FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw := exported[0]
	if raw.Scope.Kind != GlobalScope || raw.Name != "Atalhos" || len(raw.Bindings) != 1 {
		t.Fatalf("DTO incorreto: %+v", raw)
	}
	if strings.Contains(string(mustJSON(t, raw)), user) || strings.Contains(string(mustJSON(t, raw)), `"source"`) {
		t.Fatalf("owner/source vazaram para export: %s", mustJSON(t, raw))
	}
}

func TestFromSnapshotRoundTripRegrasGlobalEWorkspace(t *testing.T) {
	user := portabilityUUID(t)
	globalID, workspaceID := portabilityUUID(t), portabilityUUID(t)
	globalRuleID, workspaceRuleID := portabilityUUID(t), portabilityUUID(t)
	workspace := "source-workspace"
	command := "workspace.tab.new"
	snapshot := commandconfig.Snapshot{
		Scope: commandconfig.Scope{UserID: user, WorkspaceID: &workspace},
		Layers: []commandconfig.Layer{
			{ID: globalID, UserID: user, Name: "Global", Source: "user", Enabled: true},
			{ID: workspaceID, UserID: user, WorkspaceID: &workspace, Name: "Workspace", Source: "user", Enabled: true},
		},
		Bindings: []commandconfig.Binding{
			{ID: portabilityUUID(t), UserID: user, LayerRefKind: "user", LayerRef: globalID, TriggerType: "keyboard.local", TriggerSpec: `{}`, CommandID: &command, Arguments: `{}`, Condition: `{}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`},
			{ID: portabilityUUID(t), UserID: user, WorkspaceID: &workspace, LayerRefKind: "user", LayerRef: workspaceID, TriggerType: "keyboard.local", TriggerSpec: `{}`, CommandID: &command, Arguments: `{}`, Condition: `{}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`},
		},
		ActivationRules: []commandactivation.Rule{
			{ID: globalRuleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: globalID, RuleRefKind: commandactivation.UserRef, RuleRef: globalRuleID, Mode: commandactivation.ModeAlways, Condition: `{}`, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active"},
			{ID: workspaceRuleID, UserID: user, WorkspaceID: &workspace, LayerRefKind: commandactivation.UserRef, LayerRef: workspaceID, RuleRefKind: commandactivation.UserRef, RuleRef: workspaceRuleID, Mode: commandactivation.ModeAlways, Condition: `{}`, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active"},
		},
	}
	exported, err := FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 2 || len(exported[0].ActivationRules) != 1 || len(exported[1].ActivationRules) != 1 {
		t.Fatalf("regras global/workspace não exportadas: %+v", exported)
	}
	plan, err := PlanImport(context.Background(), exported, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{workspace: "destination-workspace"}}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, portabilityRefs(t))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := plan.Snapshot(user)
	if err != nil {
		t.Fatal(err)
	}
	var globalRules, workspaceRules int
	for _, rule := range restored.ActivationRules {
		if rule.WorkspaceID == nil {
			globalRules++
		} else if *rule.WorkspaceID == "destination-workspace" {
			workspaceRules++
		}
	}
	if globalRules != 1 || workspaceRules != 1 {
		t.Fatalf("round-trip de escopos perdeu regras: %+v", restored.ActivationRules)
	}
}

func TestFromSnapshotNaoIgnoraDeltaBuiltin(t *testing.T) {
	user, bindingID := portabilityUUID(t), portabilityUUID(t)
	snapshot := commandconfig.Snapshot{
		Scope:    commandconfig.Scope{UserID: user},
		Layers:   []commandconfig.Layer{{ID: portabilityUUID(t), UserID: user, Name: "Global", Source: "user"}},
		Bindings: []commandconfig.Binding{{ID: bindingID, UserID: user, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}},
	}
	if _, err := FromSnapshot(snapshot); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("delta builtin foi ignorado: %v", err)
	}
}

func TestPlanImportCopyRemapeiaWorkspaceBindingEDesabilitaEvento(t *testing.T) {
	layer := portabilityLayer(t)
	ruleID := portabilityUUID(t)
	layer.ActivationRules = []ActivationRuleExport{{ID: ruleID, LayerRefKind: "user", LayerRef: layer.ID, RuleRefKind: "user", RuleRef: ruleID, Mode: "event", Condition: `{}`, Lifecycle: "persistent", EventName: stringPtr("job.completed"), Enabled: true, ReviewStatus: "active"}}
	if err := layer.validate(); err != nil {
		t.Fatalf("fixture inválida: %v", err)
	}
	refs := portabilityRefs(t)
	plan, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{"source-workspace": "dest-workspace"}}, func(context.Context, string, string) (Ownership, error) {
		return AbsentOwner, nil
	}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Layers) != 1 || plan.Layers[0].TargetID == layer.ID || plan.Layers[0].TargetScope.WorkspaceID != "dest-workspace" || plan.Layers[0].Enabled != true {
		t.Fatalf("plano de cópia inválido: %+v", plan)
	}
	if plan.Layers[0].Layer.Bindings[0].LayerRef != plan.Layers[0].TargetID || plan.Layers[0].Layer.Bindings[0].ID == layer.Bindings[0].ID {
		t.Fatalf("referência/ID do binding não remapeados: %+v", plan.Layers[0].Layer.Bindings)
	}
	if plan.Layers[0].Layer.ActivationRules[0].ID == ruleID || plan.Layers[0].Layer.ActivationRules[0].RuleRef == ruleID || plan.Layers[0].Layer.ActivationRules[0].LayerRef != plan.Layers[0].TargetID {
		t.Fatalf("referência/ID da regra não remapeados: %+v", plan.Layers[0].Layer.ActivationRules)
	}
	if plan.Layers[0].Layer.ActivationRules[0].Enabled || len(plan.Warnings) != 1 || plan.Warnings[0].Code != "event_rule_disabled" {
		t.Fatalf("regra event-driven não foi desabilitada: %+v", plan)
	}
}

func TestPlanImportForeignOwnerNaoRevelaConteudo(t *testing.T) {
	layer := portabilityLayer(t)
	plan, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) {
		return ForeignUserOwner, nil
	}, portabilityRefs(t))
	if plan.Layers != nil || !errors.Is(err, ErrForeignOwner) || strings.Contains(err.Error(), layer.Name) || strings.Contains(err.Error(), layer.ID) {
		t.Fatalf("foreign_owner revelou conteúdo: plan=%+v err=%v", plan, err)
	}
}

func TestPlanImportExigePatternDeCredencialSemValorBruto(t *testing.T) {
	layer := portabilityLayer(t)
	layer.Bindings[0].Arguments = `{"credential":{"kind":"credential","pattern":"api.example"}}`
	var gotPattern string
	refs := portabilityRefs(t)
	refs.CredentialPattern = func(_ context.Context, pattern string) (CredentialStatus, error) {
		gotPattern = pattern
		return CredentialMissing, nil
	}
	plan, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs)
	if err != nil || len(plan.Warnings) != 1 || plan.Warnings[0].Code != "credential_missing" || gotPattern != "api.example" {
		t.Fatalf("pattern exato não foi resolvido: plan=%+v err=%v pattern=%q", plan, err, gotPattern)
	}
	layer.Bindings[0].Arguments = `{"token":"valor-real"}`
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, portabilityRefs(t)); !errors.Is(err, ErrSensitiveValue) {
		t.Fatalf("segredo bruto aceito: %v", err)
	}
}

func TestPlanImportAplicaSensitivePathDoCatalogo(t *testing.T) {
	layer := portabilityLayer(t)
	layer.Bindings[0].Arguments = `{"query":"valor-bruto"}`
	refs := portabilityRefsWithSensitivePaths(t, []string{"/query"})
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); !errors.Is(err, ErrSensitiveValue) {
		t.Fatalf("SensitivePaths do catálogo não bloqueou valor bruto: %v", err)
	}
}

func TestPlanImportNaoConfiaSoNoWorkspaceMap(t *testing.T) {
	layer := portabilityLayer(t)
	refs := portabilityRefs(t)
	refs.Workspace = nil
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{"source-workspace": "dest-workspace"}}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); !errors.Is(err, ErrWorkspaceResolution) {
		t.Fatalf("workspace map sem lookup autorizado foi aceito: %v", err)
	}
	refs.Workspace = func(_ context.Context, id string) (string, error) { return id, nil }
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{"source-workspace": "dest-workspace"}}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); err != nil {
		t.Fatalf("workspace autorizado foi rejeitado: %v", err)
	}
}

func TestPlanImportFalhaFechadoSemCatalogoOuAdapter(t *testing.T) {
	layer := portabilityLayer(t)
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, ReferencePort{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("planner aceitou portas ausentes: %v", err)
	}
}

func stringPtr(value string) *string { return &value }

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

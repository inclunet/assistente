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

func portabilityRefsWithBuiltin(t *testing.T) ReferencePort {
	t.Helper()
	refs := portabilityRefs(t)
	refs.BuiltinLayer = func(_ context.Context, id string) error {
		if id != "application.defaults" {
			return errors.New("camada builtin ausente")
		}
		return nil
	}
	refs.BuiltinDefault = func(_ context.Context, id string) error {
		if id != "builtin.tab.new" && id != "builtin.rule" {
			return errors.New("default builtin ausente")
		}
		return nil
	}
	refs.BuiltinRuleReference = func(_ context.Context, id string) error {
		if id != "builtin.rule" {
			return errors.New("regra builtin ausente")
		}
		return nil
	}
	return refs
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

func TestFromSnapshotRoundTripDeltaBuiltinSemCamadaUser(t *testing.T) {
	user, bindingID := portabilityUUID(t), portabilityUUID(t)
	layerID := portabilityUUID(t)
	snapshot := commandconfig.Snapshot{
		Scope:    commandconfig.Scope{UserID: user},
		Layers:   []commandconfig.Layer{{ID: layerID, UserID: user, Name: "Global", Source: "user"}},
		Bindings: []commandconfig.Binding{{ID: bindingID, UserID: user, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}},
	}
	exported, err := FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var deltaContainer LayerExport
	for _, candidate := range exported {
		if candidate.DeltaOnly {
			deltaContainer = candidate
		}
	}
	if len(exported) != 2 || !deltaContainer.DeltaOnly || len(deltaContainer.BuiltinDeltas) != 1 || deltaContainer.BuiltinDeltas[0].ID != bindingID {
		t.Fatalf("delta builtin não foi separado em contêiner explícito: %+v", exported)
	}
	plan, err := PlanImport(context.Background(), exported, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, portabilityRefsWithBuiltin(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := plan.Snapshot(user)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Layers) != 1 || len(got.Bindings) != 1 || got.Bindings[0].LayerRefKind != "builtin" || got.Bindings[0].LayerRef != "application.defaults" {
		t.Fatalf("round-trip builtin alterou escopo/ref: %+v", got)
	}
}

func TestPlanImportRejeitaContenedorDeltaAmbiguo(t *testing.T) {
	layer := portabilityLayer(t)
	layer.ID = ""
	layer.Name = "Atalhos"
	if _, err := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, portabilityRefs(t)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("camada sem ID foi reinterpretada como contêiner: %v", err)
	}
}

func TestFromSnapshotRoundTripDeltasBuiltinGlobalEWorkspace(t *testing.T) {
	user := portabilityUUID(t)
	workspace := "source-workspace"
	globalBinding, workspaceBinding := portabilityUUID(t), portabilityUUID(t)
	globalWorkspace, workspaceWorkspace := (*string)(nil), &workspace
	snapshot := commandconfig.Snapshot{Scope: commandconfig.Scope{UserID: user}, Bindings: []commandconfig.Binding{
		{ID: globalBinding, UserID: user, WorkspaceID: globalWorkspace, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp-global")},
		{ID: workspaceBinding, UserID: user, WorkspaceID: workspaceWorkspace, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp-workspace")},
	}}
	exported, err := FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 2 || !exported[0].DeltaOnly || !exported[1].DeltaOnly || exported[0].Scope.Kind != GlobalScope || exported[1].Scope.WorkspaceID != workspace {
		t.Fatalf("escopos builtin não foram agrupados de forma determinística: %+v", exported)
	}
	plan, err := PlanImport(context.Background(), exported, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{workspace: "destination-workspace"}}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, portabilityRefsWithBuiltin(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := plan.Snapshot(user)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Layers) != 0 || len(got.Bindings) != 2 {
		t.Fatalf("contêiner delta criou camada fake: %+v", got)
	}
	var globalCount, workspaceCount int
	for _, binding := range got.Bindings {
		if binding.ID == globalBinding || binding.ID == workspaceBinding {
			t.Fatalf("cópia reutilizou ID do delta: %+v", binding)
		}
		if binding.WorkspaceID == nil {
			globalCount++
		} else if *binding.WorkspaceID == "destination-workspace" {
			workspaceCount++
		}
	}
	if globalCount != 1 || workspaceCount != 1 {
		t.Fatalf("round-trip duplicou ou perdeu escopo global/workspace: %+v", got.Bindings)
	}
}

func TestPlanImportBuiltinDeltaUsaSensitivePathsDoCatalogo(t *testing.T) {
	command := "workspace.tab.new"
	delta := LayerExport{Scope: PortableScope{Kind: GlobalScope}, Enabled: true, DeltaOnly: true, BuiltinDeltas: []BindingExport{{
		ID: portabilityUUID(t), LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &command, Arguments: `{"query":"segredo"}`, Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp"),
	}}}
	refs := portabilityRefsWithSensitivePaths(t, []string{"/query"})
	refs.BuiltinLayer = func(context.Context, string) error { return nil }
	refs.BuiltinDefault = func(context.Context, string) error { return nil }
	if _, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); !errors.Is(err, ErrSensitiveValue) {
		t.Fatalf("SensitivePaths não protegeu delta builtin: %v", err)
	}
}

func TestPlanImportBuiltinDistingueCamadaDeDefault(t *testing.T) {
	command := "workspace.tab.new"
	delta := LayerExport{Scope: PortableScope{Kind: GlobalScope}, Enabled: true, DeltaOnly: true, BuiltinDeltas: []BindingExport{{
		ID: portabilityUUID(t), LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, CommandID: &command, Arguments: `{}`, Condition: `{}`, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp"),
	}}}
	ownership := func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }
	refs := portabilityRefsWithBuiltin(t)
	refs.BuiltinLayer = nil
	if _, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: KeepMode}, ownership, refs); !errors.Is(err, ErrMissingReference) {
		t.Fatalf("delta sem porta da camada builtin foi aceito: %v", err)
	}
	refs = portabilityRefsWithBuiltin(t)
	refs.BuiltinDefault = nil
	if _, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: KeepMode}, ownership, refs); !errors.Is(err, ErrMissingReference) {
		t.Fatalf("delta sem porta do default builtin foi aceito: %v", err)
	}
}

func TestPlanImportRegraUserEmCamadaBuiltinSemDefault(t *testing.T) {
	ruleID := portabilityUUID(t)
	rule := ActivationRuleExport{ID: ruleID, LayerRefKind: "builtin", LayerRef: "application.defaults", RuleRefKind: "user", RuleRef: ruleID, Mode: "always", Condition: `{}`, Lifecycle: "persistent", Enabled: true, ReviewStatus: "active"}
	delta := LayerExport{Scope: PortableScope{Kind: GlobalScope}, Enabled: true, DeltaOnly: true, BuiltinRuleDeltas: []ActivationRuleExport{rule}}
	refs := portabilityRefsWithBuiltin(t)
	plan, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: KeepMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs)
	if err != nil {
		t.Fatalf("regra user em camada builtin válida foi recusada: %v", err)
	}
	if len(plan.Layers) != 1 || len(plan.Layers[0].Layer.BuiltinRuleDeltas) != 1 || plan.Layers[0].Layer.BuiltinRuleDeltas[0].ReplacesDefaultID != nil {
		t.Fatalf("metadados de default foram exigidos ou alterados: %+v", plan)
	}
}

func TestPlanImportBuiltinRuleNaturalKeyUsaEscopoDestino(t *testing.T) {
	rule := func(id, workspace string) LayerExport {
		return LayerExport{Scope: PortableScope{Kind: WorkspaceScope, WorkspaceID: workspace}, Enabled: true, DeltaOnly: true, BuiltinRuleDeltas: []ActivationRuleExport{{
			ID: id, LayerRefKind: "builtin", LayerRef: "application.defaults", RuleRefKind: "builtin", RuleRef: "builtin.rule", Mode: "always", Condition: `{}`, Lifecycle: "persistent", Enabled: true, ReviewStatus: "active",
		}}}
	}
	layers := []LayerExport{rule(portabilityUUID(t), "source-a"), rule(portabilityUUID(t), "source-b")}
	refs := portabilityRefsWithBuiltin(t)
	refs.BuiltinRule = func(context.Context, string, string, string) (bool, error) { return false, nil }
	_, err := PlanImport(context.Background(), layers, PlanOptions{Mode: CopyMode, WorkspaceMap: map[string]string{"source-a": "destination", "source-b": "destination"}}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("natural key duplicada após remapeamento foi aceita: %v", err)
	}
}

func TestPlanImportCopyBuiltinRuleNaoDuplicaChaveNatural(t *testing.T) {
	rule := ActivationRuleExport{ID: portabilityUUID(t), LayerRefKind: "builtin", LayerRef: "application.defaults", RuleRefKind: "builtin", RuleRef: "builtin.rule", Mode: "always", Condition: `{}`, Lifecycle: "persistent", Enabled: true, ReviewStatus: "active", ReplacesDefaultID: stringPtr("builtin.rule"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}
	delta := LayerExport{Scope: PortableScope{Kind: GlobalScope}, Enabled: true, DeltaOnly: true, BuiltinRuleDeltas: []ActivationRuleExport{rule}}
	refs := portabilityRefsWithBuiltin(t)
	refs.BuiltinRule = func(context.Context, string, string, string) (bool, error) { return true, nil }
	if _, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: CopyMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); !errors.Is(err, ErrBuiltinRuleConflict) {
		t.Fatalf("cópia duplicada de regra builtin foi aceita: %v", err)
	}
	refs.BuiltinRule = nil
	if _, err := PlanImport(context.Background(), []LayerExport{delta}, PlanOptions{Mode: CopyMode}, func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs); !errors.Is(err, ErrMissingReference) {
		t.Fatalf("cópia de regra builtin sem lookup autoritativo foi aceita: %v", err)
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

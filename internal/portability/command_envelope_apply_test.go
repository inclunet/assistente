package portability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	cp "assistente/internal/commandportability"
	"gorm.io/gorm"
)

// SQLite, projetor, decisão e writer são reais. Sessão, autorização e
// presenter são test doubles; não se usa uma sessão pessoal do aplicativo.
func TestCommandEnvelopeRoundTripApply(t *testing.T) {
	for _, tc := range []struct {
		workspace string
		version   int
	}{{"", 1}, {"", 2}, {"workspace-opaco", 1}, {"workspace-opaco", 2}} {
		workspace := tc.workspace
		t.Run(fmt.Sprintf("scope=%s/version=%d", workspace, tc.version), func(t *testing.T) {
			ctx := context.Background()
			source := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
			destination := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
			var ws, targetWS *string
			targetWorkspace := ""
			if workspace != "" {
				ws = &workspace
				targetWorkspace = "workspace-destino-distinto"
				targetWS = &targetWorkspace
				for _, pair := range []struct {
					f  applyImportFixture
					ws *string
				}{{source, ws}, {destination, targetWS}} {
					f := pair.f
					if err := f.db.Create(&commandconfig.Generation{ID: applyImportUUID(t), UserID: f.user, WorkspaceID: pair.ws, Generation: 1}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := source.db.Model(&commandconfig.Layer{}).Where("id = ?", source.layer.ID).Update("workspace_id", workspace).Error; err != nil {
					t.Fatal(err)
				}
			}
			// O default é fornecido pelo host; somente o delta tem linha SQL.
			destination.projection.BuiltinLayers = []commandconfig.BuiltinLayer{{ID: "application.defaults", Active: true, Defaults: []commandbindings.Default{{Candidate: commandbindings.Candidate{ID: "builtin.tab.new", Trigger: "keyboard.local:KeyA", CommandID: applyImportCommand, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true}, Version: "current-version", Fingerprint: "current-fingerprint"}}}}
			defaultID, version, fingerprint := "builtin.tab.new", "old-version", "old-fingerprint"
			delta := commandconfig.Binding{ID: applyImportUUID(t), UserID: source.user, WorkspaceID: ws, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, Arguments: "{}", Condition: applyImportCondition, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "needs_review", Presentation: `{"version":1}`, ReplacesDefaultID: &defaultID, ReplacesDefaultVersion: &version, ReplacesDefaultFingerprint: &fingerprint}
			if err := source.db.Create(&delta).Error; err != nil {
				t.Fatal(err)
			}
			command := applyImportCommand
			executable := commandconfig.Binding{ID: applyImportUUID(t), UserID: source.user, WorkspaceID: ws, LayerRefKind: "user", LayerRef: source.layer.ID, TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, CommandID: &command, Arguments: `{"query":"consulta portátil"}`, Condition: applyImportCondition, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1}`}
			if err := source.db.Create(&executable).Error; err != nil {
				t.Fatal(err)
			}
			eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
			contextID, eventID, builtinID := applyImportUUID(t), applyImportUUID(t), applyImportUUID(t)
			ruleDefault := "builtin.rule.event"
			rules := []commandactivation.Rule{
				{ID: contextID, UserID: source.user, WorkspaceID: ws, LayerRefKind: commandactivation.UserRef, LayerRef: source.layer.ID, RuleRefKind: commandactivation.UserRef, RuleRef: contextID, Mode: commandactivation.ModeCondition, Condition: applyImportCondition, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active"},
				{ID: eventID, UserID: source.user, WorkspaceID: ws, LayerRefKind: commandactivation.UserRef, LayerRef: source.layer.ID, RuleRefKind: commandactivation.UserRef, RuleRef: eventID, Mode: commandactivation.ModeEvent, Condition: applyImportCondition, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, Enabled: true, Source: "user", ReviewStatus: "active"},
				{ID: builtinID, UserID: source.user, WorkspaceID: ws, LayerRefKind: commandactivation.BuiltinRef, LayerRef: "application.defaults", RuleRefKind: commandactivation.BuiltinRef, RuleRef: ruleDefault, Mode: commandactivation.ModeEvent, Condition: applyImportCondition, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, Enabled: true, Source: "user", ReviewStatus: "needs_review", ReplacesDefaultID: &ruleDefault, ReplacesDefaultVersion: &version, ReplacesDefaultFingerprint: &fingerprint},
			}
			if err := source.db.Create(&rules).Error; err != nil {
				t.Fatal(err)
			}
			if ws != nil {
				global := delta
				global.ID = applyImportUUID(t)
				global.WorkspaceID = nil
				if err := source.db.Create(&global).Error; err != nil {
					t.Fatal(err)
				}
				// Outro workspace possui referência indisponível. Ele não deve
				// ser consultado/validado como parte do export do escopo pedido.
				otherWorkspace := "workspace-fora-do-escopo"
				other := delta
				other.ID = applyImportUUID(t)
				other.WorkspaceID = &otherWorkspace
				other.LayerRef = "missing.defaults"
				if err := source.db.Create(&other).Error; err != nil {
					t.Fatal(err)
				}
			}
			refs := envelopeRefs(t, workspace)
			raw, err := ExportCommandEnvelope(ctx, source.db, commandconfig.Scope{UserID: source.user, WorkspaceID: ws}, refs)
			if err != nil {
				t.Fatal(err)
			}
			parsed, unsupported, err := parseExportFile(string(raw))
			if err != nil || len(unsupported) != 0 || parsed.Version != ExportVersion || len(parsed.Resources.CommandLayers) != 2 {
				t.Fatalf("envelope: %s error=%v", raw, err)
			}
			var exportedBindings, exportedRules, exportedDeltas, exportedRuleDeltas int
			for _, layer := range parsed.Resources.CommandLayers {
				exportedBindings += len(layer.Bindings)
				exportedRules += len(layer.ActivationRules)
				exportedDeltas += len(layer.BuiltinDeltas)
				exportedRuleDeltas += len(layer.BuiltinRuleDeltas)
			}
			if exportedBindings != 1 || exportedRules != 2 || exportedDeltas != 1 || exportedRuleDeltas != 1 {
				t.Fatalf("export incompleto: %s", raw)
			}
			parsed.Version = tc.version
			raw, err = json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}
			globalBefore, err := destination.store.Load(ctx, commandconfig.Scope{UserID: destination.user})
			if err != nil {
				t.Fatal(err)
			}
			// Nome distinto preserva também a camada preexistente do destino.
			options := cp.PlanOptions{Mode: cp.ReplaceMode, RenameByLayer: map[string]string{source.layer.ID: "restaurada"}}
			if ws != nil {
				options.WorkspaceMap = map[string]string{workspace: targetWorkspace}
			}
			refs = envelopeRefs(t, targetWorkspace)
			owner := func(_ context.Context, _ string, id string) (cp.Ownership, error) {
				var count int64
				if err := destination.db.Model(&commandconfig.Layer{}).Where("user_id = ? AND id = ?", destination.user, id).Count(&count).Error; err != nil {
					return "", err
				}
				if count == 0 {
					if err := destination.db.Model(&commandconfig.Binding{}).Where("user_id = ? AND id = ?", destination.user, id).Count(&count).Error; err != nil {
						return "", err
					}
				}
				if count > 0 {
					return cp.CurrentUserOwner, nil
				}
				if err := destination.db.Model(&commandactivation.Rule{}).Where("user_id = ? AND id = ?", destination.user, id).Count(&count).Error; err != nil {
					return "", err
				}
				if count > 0 {
					return cp.CurrentUserOwner, nil
				}
				return cp.AbsentOwner, nil
			}
			// Campos de concessão não pertencem ao DTO. Mesmo um arquivo que
			// reivindica grant para regra existente deve falhar antes da decisão.
			var contaminated map[string]any
			if err := json.Unmarshal(raw, &contaminated); err != nil {
				t.Fatal(err)
			}
			for _, entry := range contaminated["resources"].(map[string]any)["commandLayers"].([]any) {
				layer := entry.(map[string]any)
				if rules, ok := layer["activationRules"].([]any); ok {
					rules[0].(map[string]any)["automationGrantId"] = applyImportUUID(t)
				}
			}
			contaminatedRaw, err := json.Marshal(contaminated)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyCommandEnvelope(ctx, destination.service, "fixture-token", targetWS, contaminatedRaw, options, owner, refs); !errors.Is(err, cp.ErrInvalid) {
				t.Fatalf("grant do arquivo foi aceito: %v", err)
			}
			if targetWS != nil {
				denied := refs
				denied.Workspace = func(context.Context, string) (string, error) { return "", cp.ErrWorkspaceResolution }
				if _, err := ApplyCommandEnvelope(ctx, destination.service, "fixture-token", targetWS, raw, options, owner, denied); !errors.Is(err, cp.ErrWorkspaceResolution) {
					t.Fatalf("WorkspaceMap substituiu autorização: %v", err)
				}
			}
			if destination.presenter.calls != 0 {
				t.Fatal("payload/grant/workspace recusado abriu decisão")
			}
			if _, err := ApplyCommandEnvelope(ctx, destination.service, "fixture-token", targetWS, raw, options, owner, refs); err != nil {
				t.Fatal(err)
			}
			if destination.presenter.calls != 1 {
				t.Fatalf("decisões=%d", destination.presenter.calls)
			}
			persisted, err := destination.store.Load(ctx, commandconfig.Scope{UserID: destination.user, WorkspaceID: targetWS})
			if err != nil {
				t.Fatal(err)
			}
			if len(persisted.ActivationRules) != 3 || len(persisted.AutomationGrants) != 0 || len(persisted.ActivationClaims) != 0 {
				t.Fatalf("regras/efeitos importados incorretamente: %+v", persisted)
			}
			for _, rule := range persisted.ActivationRules {
				if rule.UserID != destination.user || !reflect.DeepEqual(rule.WorkspaceID, targetWS) {
					t.Fatalf("owner/escopo da regra: %+v", rule)
				}
				if rule.Mode == commandactivation.ModeEvent && (rule.Enabled || rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil) {
					t.Fatalf("evento importado habilitado/concedido: %+v", rule)
				}
			}
			if ws != nil {
				globalAfter, err := destination.store.Load(ctx, commandconfig.Scope{UserID: destination.user})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(globalBefore.Layers, globalAfter.Layers) || !reflect.DeepEqual(globalBefore.Bindings, globalAfter.Bindings) || !reflect.DeepEqual(globalBefore.Generations, globalAfter.Generations) {
					t.Fatal("import workspace alterou global")
				}
			}
			afterRaw, err := ExportCommandEnvelope(ctx, destination.db, commandconfig.Scope{UserID: destination.user, WorkspaceID: targetWS}, refs)
			if err != nil {
				t.Fatal(err)
			}
			after, _, err := parseExportFile(string(afterRaw))
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, actual := range after.Resources.CommandLayers {
				for _, expected := range parsed.Resources.CommandLayers {
					if actual.ID != expected.ID || actual.DeltaOnly != expected.DeltaOnly {
						continue
					}
					if !expected.DeltaOnly {
						expected.Name = "restaurada"
					}
					if targetWS != nil {
						expected.Scope.WorkspaceID = targetWorkspace
					}
					// Importação de configuração não transfere habilitação de eventos.
					for i := range expected.ActivationRules {
						if expected.ActivationRules[i].EventName != nil {
							expected.ActivationRules[i].Enabled = false
						}
					}
					for i := range expected.BuiltinRuleDeltas {
						if expected.BuiltinRuleDeltas[i].EventName != nil {
							expected.BuiltinRuleDeltas[i].Enabled = false
						}
					}
					if !reflect.DeepEqual(actual, expected) {
						t.Fatalf("roundtrip perdeu conteúdo: want=%+v got=%+v", expected, actual)
					}
					found++
				}
			}
			if found != 2 {
				t.Fatalf("camada ou delta não reexportado: %s", afterRaw)
			}
			options.Mode = cp.KeepMode
			if _, err := ApplyCommandEnvelope(ctx, destination.service, "fixture-token", targetWS, raw, options, owner, refs); !errors.Is(err, cp.ErrNoChanges) {
				t.Fatalf("reimport: %v", err)
			}
			if destination.presenter.calls != 1 {
				t.Fatal("no-op abriu decisão")
			}
		})
	}
}

func envelopeRefs(t *testing.T, workspace string) cp.ReferencePort {
	t.Helper()
	return cp.ReferencePort{Catalog: applyImportRegistry(t),
		Trigger: func(_ context.Context, typ, raw string) (string, error) {
			if typ != "keyboard.local" || raw != applyImportTrigger {
				return "", cp.ErrInvalid
			}
			return "keyboard.local:KeyA", nil
		},
		BuiltinLayer: func(_ context.Context, id string) error {
			if id != "application.defaults" {
				return cp.ErrMissingReference
			}
			return nil
		},
		BuiltinDefault: func(_ context.Context, id string) error {
			if id != "builtin.tab.new" && id != "builtin.rule.event" {
				return cp.ErrMissingReference
			}
			return nil
		},
		BuiltinRuleReference: func(_ context.Context, id string) error {
			if id != "builtin.rule.event" {
				return cp.ErrMissingReference
			}
			return nil
		},
		Workspace: func(_ context.Context, id string) (string, error) {
			if id == "" || id != workspace {
				return "", cp.ErrWorkspaceResolution
			}
			return id, nil
		},
	}
}

func TestCommandEnvelopeRejeitaVersaoEConteudoAntesDaDecisao(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	refs := envelopeRefs(t, "")
	raw, err := ExportCommandEnvelope(context.Background(), f.db, commandconfig.Scope{UserID: f.user}, refs)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0, -1, ExportVersion + 1} {
		var file ExportFile
		if err := json.Unmarshal(raw, &file); err != nil {
			t.Fatal(err)
		}
		file.Version = version
		bad, err := json.Marshal(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyCommandEnvelope(context.Background(), f.service, "token", nil, bad, cp.PlanOptions{Mode: cp.KeepMode}, func(context.Context, string, string) (cp.Ownership, error) { return cp.CurrentUserOwner, nil }, refs); err == nil {
			t.Fatalf("versão %d aceita", version)
		}
	}
	for _, payload := range []string{
		`{"version":2,"version":1,"resources":{}}`,
		`{"version":2,"resources":{"commandLayers":[],"grants":[{}]}}`,
		`{"version":2,"options":{"includeCredentials":true},"resources":{}}`,
	} {
		if _, err := ApplyCommandEnvelope(context.Background(), f.service, "token", nil, []byte(payload), cp.PlanOptions{Mode: cp.KeepMode}, nil, refs); err == nil {
			t.Fatalf("payload aceito: %s", payload)
		}
	}
	var file ExportFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	command := applyImportCommand
	file.Resources.CommandLayers[0].Bindings = []cp.BindingExport{{ID: applyImportUUID(t), LayerRefKind: "user", LayerRef: f.layer.ID, CommandID: &command, Arguments: `{"password":"fixture-only-secret"}`, TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, Condition: applyImportCondition, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}}
	bad, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyCommandEnvelope(context.Background(), f.service, "token", nil, bad, cp.PlanOptions{Mode: cp.ReplaceMode}, func(context.Context, string, string) (cp.Ownership, error) { return cp.CurrentUserOwner, nil }, refs); !errors.Is(err, cp.ErrSensitiveValue) {
		t.Fatalf("segredo bruto: %v", err)
	}
	if f.presenter.calls != 0 {
		t.Fatal("payload recusado abriu decisão")
	}
}

func TestCommandEnvelopeRecusaLegadoSensivelPeloCatalogo(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	refs := envelopeRefs(t, "")
	refs.Catalog = applyImportRegistry(t, "/query")
	command := applyImportCommand
	binding := cp.BindingExport{ID: applyImportUUID(t), LayerRefKind: "user", LayerRef: f.layer.ID, TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, CommandID: &command, Arguments: `{"query":"fixture-sensitive-value"}`, Condition: applyImportCondition, Effect: "execute", Enabled: true, ReviewStatus: "active", Presentation: `{"version":1}`}
	file := ExportFile{Version: ExportVersion, Resources: ExportResources{CommandLayers: []cp.LayerExport{{ID: f.layer.ID, Name: f.layer.Name, Enabled: true, Scope: cp.PortableScope{Kind: cp.GlobalScope}, Bindings: []cp.BindingExport{binding}}}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	owner := func(context.Context, string, string) (cp.Ownership, error) { return cp.CurrentUserOwner, nil }
	if _, err := ApplyCommandEnvelope(context.Background(), f.service, "token", nil, raw, cp.PlanOptions{Mode: cp.ReplaceMode}, owner, refs); !errors.Is(err, cp.ErrSensitiveValue) {
		t.Fatalf("import sensível: %v", err)
	}
	legacy := commandconfig.Binding{ID: binding.ID, UserID: f.user, LayerRefKind: binding.LayerRefKind, LayerRef: binding.LayerRef, TriggerType: binding.TriggerType, TriggerSpec: binding.TriggerSpec, CommandID: &command, Arguments: binding.Arguments, Condition: binding.Condition, Effect: binding.Effect, Enabled: true, Source: "user", ReviewStatus: "active", Presentation: binding.Presentation}
	if err := f.db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if raw, err := ExportCommandEnvelope(context.Background(), f.db, commandconfig.Scope{UserID: f.user}, refs); !errors.Is(err, cp.ErrSensitiveValue) || len(raw) != 0 {
		t.Fatalf("export legado deveria falhar sem conteúdo: bytes=%d err=%v", len(raw), err)
	}
	if f.presenter.calls != 0 {
		t.Fatal("segredo abriu decisão")
	}
}

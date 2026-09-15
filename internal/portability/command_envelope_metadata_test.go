package portability

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandconfig"
	cp "assistente/internal/commandportability"
	"gorm.io/gorm"
)

func TestCommandEnvelopeRecusaSegredoLegadoEmDocumentosDoBinding(t *testing.T) {
	for _, field := range []string{"condition", "presentation", "triggerSpec"} {
		for _, kind := range []string{"user", "builtin"} {
			for _, effect := range []string{"execute", "suppress"} {
				t.Run(field+"/"+kind+"/"+effect, func(t *testing.T) {
					ctx := context.Background()
					f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
					refs := envelopeRefs(t, "")
					triggerCalls := 0
					trustedTrigger := refs.Trigger
					refs.Trigger = func(ctx context.Context, typ, raw string) (string, error) {
						triggerCalls++
						return trustedTrigger(ctx, typ, raw)
					}
					command, defaultID, version, fingerprint := applyImportCommand, "builtin.tab.new", "1", "fingerprint"
					binding := cp.BindingExport{ID: applyImportUUID(t), LayerRefKind: kind, LayerRef: f.layer.ID, TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, CommandID: &command, Arguments: "{}", Condition: applyImportCondition, Presentation: `{"version":1}`, Effect: effect, Enabled: false, ReviewStatus: "active", ReplacesDefaultID: &defaultID, ReplacesDefaultVersion: &version, ReplacesDefaultFingerprint: &fingerprint}
					if effect == "suppress" {
						binding.CommandID = nil
					}
					layer := cp.LayerExport{ID: f.layer.ID, Name: f.layer.Name, Enabled: true, Scope: cp.PortableScope{Kind: cp.GlobalScope}}
					if kind == "builtin" {
						binding.LayerRef = "application.defaults"
						layer = cp.LayerExport{DeltaOnly: true, Enabled: true, Scope: cp.PortableScope{Kind: cp.GlobalScope}}
					}
					// O detector compartilhado deve percorrer objetos e arrays,
					// mesmo em bindings desabilitados e suppress sem argumentos.
					legacyDocument := `{"nested":[{"access_token":"fixture-only-secret"}]}`
					wantErr := cp.ErrSensitiveValue
					switch field {
					case "condition":
						binding.Condition = legacyDocument
					case "presentation":
						binding.Presentation = legacyDocument
					default:
						// TriggerSpec usa a gramática fechada do adapter registrado;
						// a porta também é obrigatória em export/suppress/desabilitado.
						binding.TriggerSpec = legacyDocument
						wantErr = cp.ErrInvalid
					}
					if kind == "builtin" {
						layer.BuiltinDeltas = []cp.BindingExport{binding}
					} else {
						layer.Bindings = []cp.BindingExport{binding}
					}
					raw, err := json.Marshal(ExportFile{Version: ExportVersion, Resources: ExportResources{CommandLayers: []cp.LayerExport{layer}}})
					if err != nil {
						t.Fatal(err)
					}
					owner := func(context.Context, string, string) (cp.Ownership, error) { return cp.CurrentUserOwner, nil }
					if _, err := ApplyCommandEnvelope(ctx, f.service, "token", nil, raw, cp.PlanOptions{Mode: cp.ReplaceMode}, owner, refs); !errors.Is(err, wantErr) {
						t.Fatalf("import %s: %v", field, err)
					}
					after, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
					if err != nil {
						t.Fatal(err)
					}
					if f.presenter.calls != 0 || !reflect.DeepEqual(after.Layers, f.before.Layers) || !reflect.DeepEqual(after.Bindings, f.before.Bindings) || !reflect.DeepEqual(after.Generations, f.before.Generations) {
						t.Fatal("segredo abriu decisão ou mudou estado")
					}
					legacy := commandconfig.Binding{ID: binding.ID, UserID: f.user, LayerRefKind: binding.LayerRefKind, LayerRef: binding.LayerRef, TriggerType: binding.TriggerType, TriggerSpec: binding.TriggerSpec, CommandID: binding.CommandID, Arguments: binding.Arguments, Condition: binding.Condition, Presentation: binding.Presentation, Effect: binding.Effect, Enabled: binding.Enabled, Source: "user", ReviewStatus: binding.ReviewStatus, ReplacesDefaultID: binding.ReplacesDefaultID, ReplacesDefaultVersion: binding.ReplacesDefaultVersion, ReplacesDefaultFingerprint: binding.ReplacesDefaultFingerprint}
					if err := f.db.Create(&legacy).Error; err != nil {
						t.Fatal(err)
					}
					if raw, err := ExportCommandEnvelope(ctx, f.db, commandconfig.Scope{UserID: f.user}, refs); !errors.Is(err, wantErr) || len(raw) != 0 {
						t.Fatalf("export vazou metadado legado: bytes=%d err=%v", len(raw), err)
					}
					if field == "triggerSpec" && triggerCalls != 2 {
						t.Fatalf("adapter deve validar import e export: calls=%d", triggerCalls)
					}
				})
			}
		}
	}
}

// Limite conhecido: Store.Load agrega global+workspace e valida o agregado.
// Sem uma porta de leitura exata no Store, não contornamos essa validação.
func TestCommandEnvelopeWorkspaceFalhaFechadoComGlobalEstruturalmenteInvalido(t *testing.T) {
	ctx := context.Background()
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	workspace := "workspace-opaco"
	if err := f.db.Model(&commandconfig.Layer{}).Where("id = ?", f.layer.ID).Update("workspace_id", workspace).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&commandconfig.Generation{ID: applyImportUUID(t), UserID: f.user, WorkspaceID: &workspace, Generation: 1}).Error; err != nil {
		t.Fatal(err)
	}
	refs := envelopeRefs(t, workspace)
	scope := commandconfig.Scope{UserID: f.user, WorkspaceID: &workspace}
	if _, err := ExportCommandEnvelope(ctx, f.db, scope, refs); err != nil {
		t.Fatalf("workspace válido: %v", err)
	}
	command := applyImportCommand
	orphan := commandconfig.Binding{ID: applyImportUUID(t), UserID: f.user, LayerRefKind: "user", LayerRef: applyImportUUID(t), TriggerType: "keyboard.local", TriggerSpec: applyImportTrigger, CommandID: &command, Arguments: "{}", Condition: applyImportCondition, Presentation: `{"version":1}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active"}
	if err := f.db.Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	if raw, err := ExportCommandEnvelope(ctx, f.db, scope, refs); !errors.Is(err, commandconfig.ErrInvalid) || len(raw) != 0 {
		t.Fatalf("global inválido não foi recusado: bytes=%d err=%v", len(raw), err)
	}
}

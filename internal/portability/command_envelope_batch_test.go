package portability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	cp "assistente/internal/commandportability"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestCommandEnvelopeBatchUsesRealDestinationReferencesAtomically(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit"
		if fail {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			injected := errors.New("last global hook")
			f := newApplyImportFixture(t, func(_ context.Context, _ *gorm.DB, diff commandconfig.MutationDiff) error {
				if fail && diff.Scope.WorkspaceID == nil {
					return injected
				}
				return nil
			})
			ctx := database.WithUserID(context.Background(), f.user)
			target := "destination-workspace"
			if err := f.store.EnsureScope(ctx, commandconfig.Scope{UserID: f.user, WorkspaceID: &target}); err != nil {
				t.Fatal(err)
			}
			globalID, localID := applyImportUUID(t), applyImportUUID(t)
			file := ExportFile{Version: ExportVersion, Resources: ExportResources{CommandLayers: []cp.LayerExport{
				{ID: globalID, Name: "global from envelope", Scope: cp.PortableScope{Kind: cp.GlobalScope}, Enabled: true},
				{ID: localID, Name: "workspace from envelope", Scope: cp.PortableScope{Kind: cp.WorkspaceScope, WorkspaceID: "source-workspace"}, Enabled: true},
			}}}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			refs := envelopeRefs(t, target)
			options := cp.PlanOptions{Mode: cp.ReplaceMode, WorkspaceMap: map[string]string{"source-workspace": target}, Name: cp.NewStoreLayerName(f.db)}
			result, err := ApplyCommandEnvelopeBatch(ctx, f.service, "token", raw, options, cp.NewStoreOwnership(f.db), refs)
			diffs := result.Diffs
			var count int64
			if e := f.db.Model(&commandconfig.Layer{}).Where("id IN ?", []string{globalID, localID}).Count(&count).Error; e != nil {
				t.Fatal(e)
			}
			if fail {
				if result.Report != nil {
					t.Fatal("rollback vazou relatório do plano não aplicado")
				}
				if !errors.Is(err, injected) || count != 0 || len(diffs) != 0 {
					t.Fatalf("rollback: rows=%d diffs=%d err=%v", count, len(diffs), err)
				}
				return
			}
			if err != nil || count != 2 || len(diffs) != 2 {
				t.Fatalf("commit: rows=%d diffs=%d err=%v", count, len(diffs), err)
			}
			if result.Report == nil || result.Report.NoChanges || len(result.Report.Layers) != 2 {
				t.Fatalf("relatório ausente no envelope: %+v", result.Report)
			}
			var local commandconfig.Layer
			if err := f.db.Where("id = ?", localID).Take(&local).Error; err != nil {
				t.Fatal(err)
			}
			if local.UserID != f.user || local.WorkspaceID == nil || *local.WorkspaceID != target {
				t.Fatal("owner/escopo portátil foi usado como autoridade")
			}
			options.Mode = cp.KeepMode
			if _, err := ApplyCommandEnvelopeBatch(ctx, f.service, "token", raw, options, cp.NewStoreOwnership(f.db), refs); !errors.Is(err, cp.ErrNoChanges) {
				t.Fatalf("Keep: %v", err)
			}
			file.Options.IncludeCredentials = true
			invalid, err := json.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyCommandEnvelopeBatch(ctx, f.service, "token", invalid, options, cp.NewStoreOwnership(f.db), refs); !errors.Is(err, cp.ErrUnsupported) {
				t.Fatalf("credenciais misturadas: %v", err)
			}
		})
	}
}

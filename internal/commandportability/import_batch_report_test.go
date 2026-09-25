package commandportability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestBatchReportMissingCredentialMatchesDisabledPersistence(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	if err := f.db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), f.user)
	refs := applyImportRefs(t, f)
	refs.CredentialPattern = NewCredentialPatternResolver(f.db)
	binding := applyImportBinding(t, f.layer.ID, applyImportCommand)
	binding.Arguments = `{"credential":{"kind":"credential","pattern":"private-pattern.example.test"}}`
	result, err := ApplyPlanImportBatch(ctx, f.service, "token", []LayerExport{applyImportLayer(f, "private layer name", &binding)},
		PlanOptions{Mode: ReplaceMode, Name: NewStoreLayerName(f.db)}, NewStoreOwnership(f.db), refs)
	if err != nil {
		t.Fatalf("import: %v (decisões=%d)", err, f.presenter.calls)
	}
	if result.Report == nil || len(result.Report.Warnings) != 1 || result.Report.Warnings[0].Code != "credential_missing" || result.Report.Warnings[0].Count != 1 {
		t.Fatalf("aviso ausente no relatório: %+v", result.Report)
	}
	after, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
	if err != nil || len(after.Bindings) != 1 || after.Bindings[0].Enabled {
		t.Fatalf("binding deve permanecer desabilitado: %+v err=%v", after.Bindings, err)
	}
	if len(result.Diffs) != 1 || len(result.Diffs[0].AfterLayers) != 1 || len(after.Layers) != 1 {
		t.Fatal("camada substituída ou diff ausente")
	}
	confirmed := result.Diffs[0].AfterLayers[0]
	if !after.Layers[0].UpdatedAt.Equal(confirmed.UpdatedAt) || !after.Layers[0].CreatedAt.Equal(confirmed.CreatedAt) || after.Layers[0].Name != confirmed.Name {
		t.Fatalf("gravação divergiu da camada confirmada: got=%+v want=%+v", after.Layers[0], confirmed)
	}
	raw, err := json.Marshal(result.Report)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-pattern", "private layer name", "arguments", f.user} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("relatório contém metadado privado %q", private)
		}
	}
}

package plugins

import (
	"context"
	"slices"
	"strings"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
)

func TestBuilderTransfersJobDefinitionAcrossAccounts(t *testing.T) {
	db := managedJobTestDB(t)
	source := database.WithUserID(t.Context(), "source")
	destination := database.WithUserID(t.Context(), "destination")
	store := NewInventory(db).WithJobRefresh(func(context.Context, string, commandsecurity.Admission) error { return nil })
	repo := jobs.NewDBRepository(db)
	const private = "private-value-for-source-only"
	original := &jobs.Job{ID: "reminder", Name: "Monthly review", Tool: "notify", Enabled: true, Inputs: map[string]any{"text": private, "options": map[string]any{"token": private}}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}}
	if err := repo.SaveJob(source, original); err != nil {
		t.Fatal(err)
	}
	names, err := store.BuilderJobNames(destination)
	if err != nil || len(names) != 0 {
		t.Fatal(names, err)
	}
	req := BuilderRequest{Name: "review-kit", Jobs: []string{original.ID}}
	if _, _, err := store.BuildPackage(destination, req); err == nil || err.Error() != "PLUGIN_BUILDER_JOB_UNAVAILABLE: reminder" {
		t.Fatalf("job indisponível na conta de destino: %v", err)
	}
	data, snap, err := store.BuildPackage(source, req)
	if err != nil {
		t.Fatal(err)
	}
	raw := snap.Files[nativeExtension+"/jobs/reminder.json"]
	if strings.Contains(string(raw), private) || strings.Contains(string(raw), original.DatabaseID) || strings.Contains(string(raw), "enabled") {
		t.Fatal("exportou dados da conta de origem")
	}
	components := DiscoverComponents(snap)
	if len(components) != 1 || !slices.Equal(components[0].JobRequiredInputs, []string{"options", "text"}) {
		t.Fatal(components)
	}
	archive, err := pluginpackage.OpenArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(fn func() error) error { return fn() }
	installed, err := store.InstallSource(destination, "review-kit", snap.Digest, "", admit, func() (*pluginpackage.LocalSnapshot, error) { return archive.Package("review-kit", ".") })
	if err != nil {
		t.Fatal(err)
	}
	slug, err := store.PrepareJob(destination, installed.ID, installed.Digest, "jobs/reminder", admit)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repo.GetJob(destination, slug)
	if err != nil {
		t.Fatal(err)
	}
	if target.Enabled || len(target.Inputs) > 0 {
		t.Fatal("job começou ativo ou trouxe valores privados")
	}
	target.Enabled = true
	if err := repo.SaveJob(destination, target); err == nil {
		t.Fatal("ativou sem configurar as entradas")
	}
	target.Inputs = map[string]any{"text": "  ", "options": false}
	if err := repo.SaveJob(destination, target); err == nil {
		t.Fatal("ativou com texto vazio")
	}
	target.Inputs = map[string]any{"text": "valor local", "options": false}
	if err := repo.SaveJob(destination, target); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckPluginJobActivation(destination, target); err != nil {
		t.Fatal(err)
	}
}

package plugins

import (
	"context"
	"gorm.io/gorm"
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
	admit := func(_ *gorm.DB, fn func() error) error { return fn() }
	installed, err := store.InstallSource(destination, "review-kit", snap.Digest, "", nil, admit, func() (*pluginpackage.LocalSnapshot, error) { return archive.Package("review-kit", ".") })
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

func TestBuilderPreservesDistinctJobsWithNonportableSlugs(t *testing.T) {
	db := managedJobTestDB(t)
	ctx := database.WithUserID(t.Context(), "source")
	store := NewInventory(db)
	repo := jobs.NewDBRepository(db)
	// A real portable slug can equal the fallback for another job.
	portable := builderJobKeys([]string{"con"})["con"]
	slugs := []string{portable, "con", strings.Repeat("a", 65), strings.Repeat("a", 66)}
	for _, slug := range slugs {
		job := &jobs.Job{ID: slug, Name: slug, Tool: "notify", Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}}
		if err := repo.SaveJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	names, err := store.BuilderJobNames(ctx)
	if err != nil || len(names) != len(slugs) {
		t.Fatal(names, err)
	}
	request := BuilderRequest{Name: "jobs", Jobs: names}
	_, snap, err := store.BuildPackage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	components := DiscoverComponents(snap)
	if len(components) != len(slugs) {
		t.Fatal(components)
	}
	seen := map[string]bool{}
	for _, component := range components {
		key := strings.TrimPrefix(component.Key, "jobs/")
		if component.Reason != "" || !validBuilderSkill(key) || seen[component.Name] || !slices.Contains(slugs, component.Name) || (component.Name == portable && key != portable) {
			t.Fatalf("job incorreto: %+v", component)
		}
		seen[component.Name] = true
	}
	slices.Reverse(request.Jobs)
	_, reversed, err := store.BuildPackage(ctx, request)
	if err != nil || snap.Digest != reversed.Digest {
		t.Fatalf("seleção em ordem inversa: %v", err)
	}
	request.Jobs = []string{"con", "con"}
	if _, _, err := store.BuildPackage(ctx, request); err == nil {
		t.Fatal("aceitou job repetido")
	}
}

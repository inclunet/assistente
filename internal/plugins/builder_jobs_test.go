package plugins

import (
	"context"
	"encoding/json"

	"reflect"
	"slices"
	"strings"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestBuilderTransfersJobDefinitionAcrossAccounts(t *testing.T) {
	db := managedJobTestDB(t)
	source := database.WithUserID(t.Context(), "source")
	destination := database.WithUserID(t.Context(), "destination")
	store := NewInventory(db).WithJobRefresh(func(context.Context, string, commandsecurity.Admission) error { return nil })
	repo := jobs.NewDBRepository(db)
	const literal = "revisar relatório"
	const reference = `{{ secret "mensageria" }}`
	original := &jobs.Job{ID: "reminder", Name: "Monthly review", Tool: "notify", Pipeline: "mensal", Output: jobs.OutputConfig{Map: map[string]string{"resumo": "text"}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorRetry, MaxRetries: 2}, Enabled: true, Inputs: map[string]any{"text": literal, "options": map[string]any{"token": reference}}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}}
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
	raw := snap.Files[nativeExtension+"/jobs/reminder.yaml"]
	if !strings.Contains(string(raw), literal) || !strings.Contains(string(raw), reference) || strings.Contains(string(raw), original.DatabaseID) {
		t.Fatal("exportou dados da conta de origem")
	}
	components := DiscoverComponents(snap)
	if len(components) != 1 || components[0].Tool != original.Tool {
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
	if _, err := store.PrepareJob(destination, installed.ID, installed.Digest, "jobs/reminder", admit); err == nil {
		t.Fatal("preparou com o pipeline disponível apenas na conta de origem")
	}
	var sourcePipeline database.JobPipeline
	if err := db.Where("user_id=? AND slug=?", "source", "mensal").Take(&sourcePipeline).Error; err != nil {
		t.Fatal(err)
	}
	localPipeline := database.JobPipeline{UserID: "destination", Slug: "mensal", Name: "Mensal local", Enabled: true}
	if err := db.Create(&localPipeline).Error; err != nil {
		t.Fatal(err)
	}
	owner := "destination"
	homonym := database.ToolCatalog{UserID: &owner, Name: "notify", Origin: "external", DisplayName: "Notificação externa", AvailabilityStatus: "available"}
	if err := db.Create(&homonym).Error; err != nil {
		t.Fatal(err)
	}
	unqualified := *original
	unqualified.ID = "ambiguous"
	unqualified.Enabled = false
	unqualified.ToolOrigin = ""
	if err := db.Transaction(func(tx *gorm.DB) error {
		return jobs.CreateDisabledPluginJobTx(destination, tx, &unqualified, installed.ID, installed.Digest, "jobs/ambiguous")
	}); err == nil {
		t.Fatal("aceitou ferramenta homônima sem origem")
	}
	slug, err := store.PrepareJob(destination, installed.ID, installed.Digest, "jobs/reminder", admit)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repo.GetJob(destination, slug)
	if err != nil {
		t.Fatal(err)
	}
	if target.Enabled || target.DatabaseID == original.DatabaseID || !reflect.DeepEqual(target.Inputs, original.Inputs) || !reflect.DeepEqual(target.Output.Map, original.Output.Map) || target.ErrorPolicy.MaxRetries != 2 {
		t.Fatal("a instalação deve preservar os argumentos e criar uma identidade local desativada")
	}
	var stored database.Job
	if err := db.Where("id=?", target.DatabaseID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if target.ToolOrigin != "builtin" || stored.ToolCatalogID == homonym.ID {
		t.Fatal("a preparação escolheu a ferramenta homônima")
	}
	if stored.PipelineID == nil || *stored.PipelineID != localPipeline.ID || localPipeline.ID == sourcePipeline.ID {
		t.Fatal("o vínculo deve usar o UUID do pipeline local")
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	target = new(jobs.Job)
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatal(err)
	}
	target.Enabled = true
	if err := repo.SaveJob(destination, target); err != nil {
		t.Fatal(err)
	}
	target, err = repo.GetJob(destination, slug)
	if err != nil || target.ToolOrigin != "builtin" {
		t.Fatal(target, err)
	}
	if err := repo.CheckPluginJobActivation(destination, target); err != nil {
		t.Fatal(err)
	}
	copySlug, err := store.CopyJob(destination, installed.ID, slug, uuid.NewString(), "Local", admit)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := repo.GetJob(destination, copySlug)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(copied)
	if err != nil {
		t.Fatal(err)
	}
	copied = new(jobs.Job)
	if err := json.Unmarshal(encoded, copied); err != nil {
		t.Fatal(err)
	}
	copied.Description = "Uma edição local"
	if err := repo.SaveJob(destination, copied); err != nil {
		t.Fatal(err)
	}
	var copyRow database.Job
	if err := db.Where("user_id=? AND slug=?", owner, copySlug).Take(&copyRow).Error; err != nil || copyRow.ToolCatalogID != stored.ToolCatalogID {
		t.Fatal(copyRow.ToolCatalogID, err)
	}
	if err := db.Model(&database.ToolCatalog{}).Where("id=?", stored.ToolCatalogID).Update("availability_status", "unavailable").Error; err != nil {
		t.Fatal(err)
	}
	target.Enabled = false
	if err := repo.SaveJob(destination, target); err != nil {
		t.Fatal(err)
	}
	target.Enabled = true
	if err := repo.SaveJob(destination, target); err == nil {
		t.Fatal("habilitou com ferramenta indisponível")
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

package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func managedJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "jobs.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err = db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginProfileActivation{}, &database.PluginSkillActivation{}, &database.PluginMCPBinding{}, &database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.Job{}, &database.JobPipeline{}, &database.JobTrigger{}, &database.Tag{}, &database.TagAssignment{}, &database.JobProfileGrant{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&database.ToolCatalog{Name: "notify", Origin: "builtin", AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestManagedJobRevisionAndRecovery(t *testing.T) {
	db := managedJobTestDB(t)
	var err error
	root := t.TempDir()
	dir := filepath.Join(root, nativeExtension, "jobs")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{filepath.Join(root, "plugin.json"): `{"$schema":"` + pluginpackage.ManifestSchema + `","name":"reminders","extensions":{"br.com.inclunet.assistente":{"version":1,"jobs":["review"]}}}`, filepath.Join(dir, "review.json"): `{"name":"Review","tool":"notify","triggers":[{"type":"manual"}]}`} {
		if err = os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	owner := database.WithUserID(t.Context(), "alice")
	other := database.WithUserID(t.Context(), "bob")
	unavailable := false
	refreshes := 0
	store := NewInventory(db).WithJobRefresh(func(context.Context, string) error {
		refreshes++
		if unavailable {
			return errors.New("offline")
		}
		return nil
	})
	snap, err := pluginpackage.InspectLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.InstallLocal(owner, root, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	admit := func(fn func() error) error { return fn() }
	prepare := func(ctx context.Context) (string, error) {
		return store.PrepareJob(ctx, row.ID, row.Digest, "jobs/review", admit)
	}
	if _, err = prepare(other); err == nil {
		t.Fatal("outra conta preparou o job")
	}
	slug, err := prepare(owner)
	if err != nil {
		t.Fatal(err)
	}
	again, err := prepare(owner)
	if err != nil || again != slug {
		t.Fatalf("repetição %q: %v", again, err)
	}
	repo := jobs.NewDBRepository(db)
	job, err := repo.GetJob(owner, slug)
	if err != nil {
		t.Fatal(err)
	}
	if job.Enabled || job.PluginDigest != row.Digest {
		t.Fatalf("preparação incorreta: %+v", job)
	}
	job.Enabled = true
	if err = repo.SaveJob(owner, job); err != nil {
		t.Fatal(err)
	}
	if err = repo.CheckPluginJobActivation(owner, job); err != nil {
		t.Fatal(err)
	}
	unavailable = true
	if err = store.ChangeInstallation(owner, row.ID, row.Digest, "uninstall", admit); !errors.Is(err, ErrJobSyncPending) {
		t.Fatalf("falha posterior ao commit: %v", err)
	}
	if err = repo.CheckPluginJobActivation(owner, job); !errors.Is(err, jobs.ErrPluginJobInactive) {
		t.Fatalf("revisão removida: %v", err)
	}
	unavailable = false
	before := refreshes
	if err = store.SyncPendingJobs(other); err != nil || refreshes != before {
		t.Fatalf("reparo cruzou contas: %v", err)
	}
	if err = store.SyncPendingJobs(owner); err != nil {
		t.Fatal(err)
	}
	if err = store.SyncPendingJobs(owner); err != nil || refreshes != before+1 {
		t.Fatalf("reparo duplicado: %d, %v", refreshes, err)
	}
}

func TestCopyManagedJobAfterRevocation(t *testing.T) {
	db := managedJobTestDB(t)
	owner := database.WithUserID(t.Context(), "ana")
	other := database.WithUserID(t.Context(), "bob")
	source := &jobs.Job{RequiredInputs: []string{"options"}, ID: "lembrete", Name: "Lembrete", Tool: "notify",
		Inputs: map[string]any{"text": "edição local"}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}}
	if err := jobs.CreateDisabledPluginJobTx(owner, db, source, "origem", "revisao", "jobs/lembrete"); err != nil {
		t.Fatal(err)
	}
	if _, err := revokeJobs(db, "ana", "origem"); err != nil {
		t.Fatal(err)
	}
	unavailable := true
	store := NewInventory(db).WithJobRefresh(func(context.Context, string) error {
		if unavailable {
			return errors.New("offline")
		}
		return nil
	})
	admit := func(f func() error) error { return f() }
	const request = "ed2712e4-28c5-4c7b-90b9-8c2705aa9e02"
	copyJob := func(ctx context.Context) (string, error) {
		return store.CopyJob(ctx, "origem", source.ID, request, "Cópia", admit)
	}
	if _, err := copyJob(other); err == nil {
		t.Fatal("cópia entre contas")
	}
	copySlug, err := copyJob(owner)
	if !errors.Is(err, ErrJobSyncPending) {
		t.Fatalf("falha após commit: %v", err)
	}
	repo := jobs.NewDBRepository(db)
	copied, err := repo.GetJob(owner, copySlug)
	if err != nil {
		t.Fatal(err)
	}
	original, err := repo.GetJob(owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Enabled || copied.DatabaseID == original.DatabaseID || copied.PluginInstallationID != "" ||
		copied.Inputs["text"] != "edição local" || len(copied.Triggers) != 1 || copied.Name != "Cópia" {
		t.Fatalf("cópia inesperada: %+v", copied)
	}
	if len(copied.RequiredInputs) != 1 || copied.RequiredInputs[0] != "options" {
		t.Fatalf("requisitos da cópia: %v", copied.RequiredInputs)
	}
	copied.Enabled = true
	if err := repo.SaveJob(owner, copied); err == nil || !strings.Contains(err.Error(), "PLUGIN_JOB_INPUT_REQUIRED") {
		t.Fatalf("habilitação sem options: %v", err)
	}
	copied.Inputs["options"] = false
	copied.Name = "Nome editado"
	if err := repo.SaveJob(owner, copied); err != nil {
		t.Fatal(err)
	}
	unavailable = false
	repeated, err := copyJob(owner)
	if err != nil || repeated != copySlug {
		t.Fatalf("repetição: %s, %v", repeated, err)
	}
	copied, err = repo.GetJob(owner, copySlug)
	if err != nil || copied.Name != "Nome editado" {
		t.Fatalf("edição perdida: %+v, %v", copied, err)
	}
	if _, err := store.CopyJob(owner, "outra", source.ID, request, "Cópia", admit); err == nil {
		t.Fatal("origem incorreta aceita")
	}
}

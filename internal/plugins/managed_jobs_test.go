package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestManagedJobRevisionAndRecovery(t *testing.T) {
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
	store := NewInventory(db).WithJobRefresh(func(context.Context, string, commandsecurity.Admission) error {
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
	admit := func(_ *gorm.DB, fn func() error) error { return fn() }
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
	if err = store.SyncPendingJobs(other, admit); err != nil || refreshes != before {
		t.Fatalf("reparo cruzou contas: %v", err)
	}
	if err = store.SyncPendingJobs(owner, admit); err != nil {
		t.Fatal(err)
	}
	if err = store.SyncPendingJobs(owner, admit); err != nil || refreshes != before+1 {
		t.Fatalf("reparo duplicado: %d, %v", refreshes, err)
	}
}

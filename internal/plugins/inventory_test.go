package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestLocalInventoryPublishesReviewedRevisionPerAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "inventory.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}); err != nil {
		t.Fatal(err)
	}
	store := NewInventory(db)
	pkg := t.TempDir()
	const manifest = `{"$schema":"` + pluginpackage.ManifestSchema + `","name":"example","version":"1"}`
	if err := os.WriteFile(filepath.Join(pkg, "plugin.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	seed, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	alice := database.WithUserID(context.Background(), "alice")
	bob := database.WithUserID(context.Background(), "bob")
	if _, err := store.InstallLocal(context.Background(), seed.Origin, seed.Digest, ""); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatal(err)
	}
	type publication struct {
		row database.PluginInstallation
		err error
	}
	start := make(chan struct{})
	results := make(chan publication, 4)
	for range 4 {
		go func() {
			<-start
			row, err := store.InstallLocal(alice, seed.Origin, seed.Digest, "")
			results <- publication{row, err}
		}()
	}
	close(start)
	var first database.PluginInstallation
	for range 4 {
		result := <-results
		if result.err != nil || (first.ID != "" && first.ID != result.row.ID) {
			t.Fatalf("concurrent publication: %+v", result)
		}
		first = result.row
	}
	second, err := store.InstallLocal(alice, seed.Origin, seed.Digest, "")
	if err != nil || first.ID != second.ID {
		t.Fatal(second, err)
	}
	unseen, err := store.List(bob)
	if err != nil || len(unseen) != 0 {
		t.Fatal(unseen, err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "README.md"), []byte("v2"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(alice, seed.Origin, seed.Digest, first.Digest); !errors.Is(err, ErrPackageChanged) {
		t.Fatal(err)
	}
	update, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(alice, seed.Origin, update.Digest, "stale"); !errors.Is(err, ErrInstallationChanged) {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_revision BEFORE INSERT ON plugin_revisions BEGIN SELECT RAISE(ABORT, 'simulated interruption'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(alice, seed.Origin, update.Digest, seed.Digest); err == nil {
		t.Fatal("expected publication failure")
	}
	rows, err := store.List(alice)
	if err != nil || len(rows) != 1 || rows[0].Digest != seed.Digest {
		t.Fatal(rows, err)
	}
	if err := db.Exec("DROP TRIGGER reject_revision").Error; err != nil {
		t.Fatal(err)
	}
	denied := errors.New("sessão revogada antes do commit")
	if _, err := store.InstallReviewed(alice, seed.Origin, update.Digest, seed.Digest, func(func() error) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	rows, err = store.List(alice)
	if err != nil || len(rows) != 1 || rows[0].Digest != seed.Digest {
		t.Fatalf("publicação recusada alterou a revisão: %v %v", rows, err)
	}
	updated, err := store.InstallLocal(alice, seed.Origin, update.Digest, seed.Digest)
	if err != nil || updated.Digest != update.Digest {
		t.Fatal(updated, err)
	}
	var revisions []database.PluginRevision
	if err := db.Where("installation_id = ?", first.ID).Find(&revisions).Error; err != nil || len(revisions) != 2 {
		t.Fatal(revisions, err)
	}
	for _, revision := range revisions {
		var files map[string][]byte
		if err := json.Unmarshal(revision.Files, &files); err != nil {
			t.Fatal(err)
		}
		if string(files["plugin.json"]) != manifest {
			t.Fatal("lost manifest")
		}
		if revision.Digest == seed.Digest && files["README.md"] != nil {
			t.Fatal("prior revision changed")
		}
	}
}

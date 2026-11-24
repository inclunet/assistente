package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestPluginPreviewUsesSelectedDirectoryAndSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"$schema":"`+pluginpackage.ManifestSchema+`","name":"local"}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "plugins.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}); err != nil {
		t.Fatal(err)
	}
	inventory := plugins.NewInventory(db)
	api := NewPlugins()
	active := true
	missing := errors.New("stale")
	ctx := database.WithUserID(context.Background(), "owner")
	session := stubSession{ctx: ctx}
	AttachPlugins(api, session, inventory, func() (string, error) { return dir, nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(_ *gorm.DB, handoff func() error) error {
			if !active {
				return missing
			}
			return handoff()
		}, nil
	})
	// Hold the old picker until the next page has received its own preview.
	var first atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	api.choose = func() (string, error) {
		if !first.Swap(true) {
			close(entered)
			<-release
		}
		return dir, nil
	}
	old := make(chan error, 1)
	go func() { _, err := api.PreviewLocal(); old <- err }()
	<-entered
	current, err := api.PreviewLocal()
	close(release)
	if oldErr := <-old; oldErr == nil || err != nil {
		t.Fatalf("old error %v, current error %v", oldErr, err)
	}
	api.mu.Lock()
	currentSelection := api.selection
	api.mu.Unlock()
	if currentSelection == nil || currentSelection.preview.Token != current.Token {
		t.Fatal("late preview replaced the current selection")
	}
	preview, err := api.PreviewLocal()
	if err != nil || preview.Name != "local" || len(preview.Files) != 1 {
		t.Fatal(preview, err)
	}
	if _, err := api.InstallReviewed("foreign-token"); err == nil {
		t.Fatal("accepted foreign token")
	}
	if err := api.DiscardPreview(preview.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(preview.Token); err == nil {
		t.Fatal("installed discarded selection")
	}
	preview, err = api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	active = false
	if _, err := api.InstallReviewed(preview.Token); !errors.Is(err, missing) {
		t.Fatal(err)
	}
	active = true
	rows, err := api.ListInstalled()
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	p, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(p.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(p.Token); err == nil {
		t.Fatal("replayed selection")
	}
	rows, err = api.ListInstalled()
	if err != nil || len(rows) != 1 || rows[0].Digest != p.Digest {
		t.Fatal(rows, err)
	}
}

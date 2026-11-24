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
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.MCPServerLog{}, &database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	var syncErr error
	inventory := plugins.NewInventory(db).WithMCPSync(func(context.Context) error { return syncErr })
	api := NewPlugins()
	active := true
	missing := errors.New("stale")
	ctx := database.WithUserID(context.Background(), "owner")
	session := stubSession{ctx: ctx}
	var notifications int
	AttachPlugins(api, session, inventory, func() (string, error) { return dir, nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(_ *gorm.DB, handoff func() error) error {
			if !active {
				return missing
			}
			return handoff()
		}, nil
	}, func() { notifications++ })
	var pickerCalls atomic.Int32
	api.choose = func() (string, error) {
		pickerCalls.Add(1)
		return dir, nil
	}
	// Hold the old admission until the next page has received its own preview.
	var first atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	capture := api.capture
	api.capture = func(ctx context.Context) (commandsecurity.Admission, error) {
		if !first.Swap(true) {
			close(entered)
			<-release
		}
		return capture(ctx)
	}
	old := make(chan error, 1)
	go func() { _, err := api.PreviewLocal(); old <- err }()
	<-entered
	current, err := api.PreviewLocal()
	close(release)
	if oldErr := <-old; !errors.Is(oldErr, errPreviewSuperseded) || err != nil || pickerCalls.Load() != 1 {
		t.Fatalf("old error %v, current error %v, picker calls %d", oldErr, err, pickerCalls.Load())
	}
	api.mu.Lock()
	currentSelection := api.selection
	api.mu.Unlock()
	if currentSelection == nil || currentSelection.preview.Token != current.Token {
		t.Fatal("late preview replaced the current selection")
	}
	// A picker that was already open when superseded must not inspect its result.
	pickerEntered, pickerRelease := make(chan struct{}), make(chan struct{})
	first.Store(false)
	api.capture = capture
	api.choose = func() (string, error) {
		if !first.Swap(true) {
			close(pickerEntered)
			<-pickerRelease
			return filepath.Join(dir, "removed"), nil
		}
		return dir, nil
	}
	go func() { _, err := api.PreviewLocal(); old <- err }()
	<-pickerEntered
	_, err = api.PreviewLocal()
	close(pickerRelease)
	if oldErr := <-old; !errors.Is(oldErr, errPreviewSuperseded) || err != nil {
		t.Fatalf("superseded picker: %v, current: %v", oldErr, err)
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
	if notifications != 0 {
		t.Fatalf("unexpected catalog event before any activation: %d", notifications)
	}
	skillDir := filepath.Join(dir, "skills", "note")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: note\ndescription: Instruções\n---\nTexto"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"sample":{"type":"sse","url":"https://example.org/mcp"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := api.InstallReviewed(updated.Token)
	if err != nil || notifications != 1 {
		t.Fatal(err, notifications)
	}
	if err := api.SetSkillActive(installed.ID, p.Digest, "skills/note", true); err == nil || notifications != 1 {
		t.Fatal(err, notifications)
	}
	for index, enable := range []bool{true, false} {
		if err := api.SetSkillActive(installed.ID, installed.Digest, "skills/note", enable); err != nil || notifications != index+2 {
			t.Fatal(err, notifications)
		}
	}
	if err := api.RegisterMCP(installed.ID, installed.Digest, "mcp/sample"); err != nil || notifications != 3 {
		t.Fatal(err, notifications)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.MCPServer{}).Where("name = ?", "sample").Update("name", "Renamed").Error; err != nil {
		t.Fatal(err)
	}
	edited, err := api.PreviewLocal()
	if err != nil || len(edited.EditedMCPServers) != 1 || edited.EditedMCPServers[0] != "sample (Renamed)" {
		t.Fatal(edited, err)
	}
	if err := db.Model(&database.MCPServer{}).Where("name = ?", "Renamed").Update("name", "sample").Error; err != nil {
		t.Fatal(err)
	}
	revised, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	if len(revised.AffectedMCPServers) != 1 || revised.AffectedMCPServers[0] != "sample" {
		t.Fatalf("missing impact: %+v", revised)
	}
	if err := api.RemoveMCP(installed.ID, installed.Digest, "mcp/sample", true); err != nil {
		t.Fatal(err)
	}
	if err := api.RegisterMCP(installed.ID, installed.Digest, "mcp/sample"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(revised.Token); !errors.Is(err, plugins.ErrInstallationChanged) {
		t.Fatalf("changed identity: %v", err)
	}
	revised, err = api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	syncErr = errors.New("runtime offline")
	if _, err := api.InstallReviewed(revised.Token); !errors.Is(err, plugins.ErrMCPSyncPending) || notifications != 4 {
		t.Fatal(err, notifications)
	}

}

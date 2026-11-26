package plugins

import (
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
	"assistente/internal/sqlite"
	"context"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledSkillActivationAndSlashIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "components.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginAgentActivation{}, &database.PluginProfileActivation{}, &database.Job{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	store := NewInventory(db)
	if err := root.WriteFile("plugin.json", []byte(`{"name":"tools","$schema":"`+pluginpackage.ManifestSchema+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"note", "restricted", "with-shell"} {
		if err := root.MkdirAll("skills/"+name, 0700); err != nil {
			t.Fatal(err)
		}
		settings := ""
		if name == "restricted" {
			settings = "allowed-tools: Read\n"
		}
		raw := "---\nname: " + name + "\ndescription: Example instruction skill\n" + settings + "---\nReviewed instructions."
		if name == "note" {
			raw += "\n![diagrama](fig.png)\n\n~~~text\n!exemplo\n~~~"
		}
		if name == "with-shell" {
			raw += "\n!echo checked"
		}
		if err := root.WriteFile("skills/"+name+"/SKILL.md", []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := pluginpackage.InspectLocal(root.Name())
	if err != nil {
		t.Fatal(err)
	}
	alice := database.WithUserID(context.Background(), "alice")
	bob := database.WithUserID(context.Background(), "bob")
	row, err := store.InstallLocal(alice, snap.Origin, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, commit func() error) error { return commit() }
	deny := func(_ *gorm.DB, _ func() error) error { return database.ErrUserScopeRequired }
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/note", true, deny); err != database.ErrUserScopeRequired {
		t.Fatal(err)
	}
	if rows, err := store.ActiveSkills(alice); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	if err := store.SetSkillActive(bob, row.ID, row.Digest, "skills/note", true, allow); err == nil {
		t.Fatal("foreign activation")
	}
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/with-shell", true, allow); err == nil {
		t.Fatal("activated inline shell as plain instructions")
	}
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/restricted", true, allow); err == nil {
		t.Fatal("ignored a tool restriction")
	}
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/note", true, allow); err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManagerWithInstalledSkills(store.ActiveSkills)
	live, err := store.ActiveSkills(alice)
	if err != nil || len(live) != 1 {
		t.Fatal(live, err)
	}
	catalog, err := skills.ListForContext(alice, manager)
	if err != nil {
		t.Fatal(err)
	}
	policy := skills.ResolveSelectionPolicy(catalog, nil, false, false)
	result, found, err := skills.InvokeContext(alice, "/"+live[0].Slug, manager, "conv", policy)
	if err != nil || !found || !strings.Contains(result.Content, "Reviewed instructions") {
		t.Fatal(result, found, err)
	}
	foreign, err := store.ActiveSkills(bob)
	if err != nil || len(foreign) != 0 {
		t.Fatal(foreign, err)
	}
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/note", false, allow); err != nil {
		t.Fatal(err)
	}
	if active, err := store.ActiveSkills(alice); err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
	if err := store.SetSkillActive(alice, row.ID, row.Digest, "skills/note", true, allow); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("README.md", []byte("change"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := pluginpackage.InspectLocal(root.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(alice, changed.Origin, changed.Digest, row.Digest); err != nil {
		t.Fatal(err)
	}
	if active, err := store.ActiveSkills(alice); err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}

	if err := store.ChangeInstallation(bob, row.ID, changed.Digest, "disable", allow); err == nil {
		t.Fatal("foreign account disabled package")
	}
	if err := store.ChangeInstallation(alice, row.ID, row.Digest, "uninstall", allow); !errors.Is(err, ErrInstallationChanged) {
		t.Fatal(err)
	}
	if err := store.SetSkillActive(alice, row.ID, changed.Digest, "skills/note", true, allow); err != nil {
		t.Fatal(err)
	}
	revoked := errors.New("revoked")
	if err := store.ChangeInstallation(alice, row.ID, changed.Digest, "disable", func(*gorm.DB, func() error) error { return revoked }); !errors.Is(err, revoked) {
		t.Fatal(err)
	}
	if active, err := store.ActiveSkills(alice); err != nil || len(active) != 1 {
		t.Fatal(active, err)
	}
	if err := store.ChangeInstallation(alice, row.ID, changed.Digest, "disable", allow); err != nil {
		t.Fatal(err)
	}
	if active, err := store.ActiveSkills(alice); err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
	if err := store.SetSkillActive(alice, row.ID, changed.Digest, "skills/note", true, allow); !errors.Is(err, ErrInstallationDisabled) {
		t.Fatal(err)
	}
	if err := store.ChangeInstallation(alice, row.ID, changed.Digest, "enable", allow); err != nil {
		t.Fatal(err)
	}
	if active, err := store.ActiveSkills(alice); err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
	if err := store.SetSkillActive(alice, row.ID, changed.Digest, "skills/note", true, allow); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.ChangeInstallation(alice, row.ID, changed.Digest, "uninstall", allow); err != nil {
			t.Fatal(err)
		}
	}
	if rows, err := store.List(alice); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	var count int64
	if err := db.Model(&database.PluginRevision{}).Where("installation_id=?", row.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if _, err := root.Stat("plugin.json"); err != nil {
		t.Fatal(err)
	}
}

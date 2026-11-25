package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"assistente/internal/skills"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestBuilderSaveRechecksRevisionAndSessionBeforePublishing(t *testing.T) {
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "builder.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}); err != nil {
		t.Fatal(err)
	}
	list := database.TaskList{UserID: "author", Title: "Release"}
	if err := db.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.TaskListWorkflow{TaskListID: list.ID, Statuses: `[{"id":1,"label":"New"}]`, AllowedTransitions: `{}`, InitialStatusID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "author")
	valid := true
	api := NewPlugins()
	AttachPlugins(api, stubSession{ctx: ctx}, plugins.NewInventory(db), func() (string, error) { return "", nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(_ *gorm.DB, fn func() error) error {
			if !valid {
				return errors.New("session changed")
			}
			return fn()
		}, nil
	}, nil)
	req := plugins.BuilderRequest{Name: "release", TaskLists: []plugins.BuilderTaskList{{ID: list.ID, Key: "plan"}}}
	first, err := api.PreviewBuilder(req)
	if err != nil || len(first.Contents) != 2 {
		t.Fatal(first, err)
	}
	if err := db.Model(&list).Update("title", "Launch").Error; err != nil {
		t.Fatal(err)
	}
	AttachPluginSaveDialog(api, func(ports.SaveFileOptions) (string, error) { t.Fatal("stale preview opened picker"); return "", nil })
	if _, err := api.SaveBuilder(req, first.Digest); !errors.Is(err, plugins.ErrPackageChanged) {
		t.Fatal(err)
	}
	current, err := api.PreviewBuilder(req)
	if err != nil {
		t.Fatal(err)
	}
	AttachPluginSaveDialog(api, func(ports.SaveFileOptions) (string, error) { return "", nil })
	if saved, err := api.SaveBuilder(req, current.Digest); err != nil || saved {
		t.Fatal(saved, err)
	}
	AttachPluginSaveDialog(api, func(ports.SaveFileOptions) (string, error) { return filepath.Join(dir, "absent", "output.zip"), nil })
	if saved, err := api.SaveBuilder(req, current.Digest); saved || !errors.Is(err, errBuilderDestination) {
		t.Fatal(saved, err)
	}
	target := filepath.Join(dir, "review.zip")
	if err := os.WriteFile(target, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	AttachPluginSaveDialog(api, func(ports.SaveFileOptions) (string, error) { valid = false; return target, nil })
	if saved, err := api.SaveBuilder(req, current.Digest); err == nil || saved {
		t.Fatal(saved, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "previous" {
		t.Fatal("changed destination", err)
	}
	valid = true
	AttachPluginSaveDialog(api, func(ports.SaveFileOptions) (string, error) { return target, nil })
	if saved, err := api.SaveBuilder(req, current.Digest); err != nil || !saved {
		t.Fatal(saved, err)
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := pluginpackage.OpenArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := archive.Package("review", ".")
	if err != nil || snap.Digest != current.Digest {
		t.Fatal(snap, err)
	}
}

// A discovery warning can cross the bridge only while the caller's session is admitted.
func TestBuilderSkillNamesPublishesPartialCatalogOnlyToAdmittedSession(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "author")
	source := plugins.NewInventory(nil).WithSkillDefinitions(func() (skills.PortableDefinitions, error) {
		return skills.PortableDefinitions{"notes": nil}, os.ErrPermission
	})
	api := NewPlugins()
	var deny error
	AttachPlugins(api, stubSession{ctx: ctx}, source, func() (string, error) { return "", nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(_ *gorm.DB, fn func() error) error {
			if deny != nil {
				return deny
			}
			return fn()
		}, nil
	}, nil)
	catalog, err := api.BuilderSkillNames()
	if err != nil || catalog == nil || !catalog.Partial || len(catalog.Names) != 1 || catalog.Names[0] != "notes" {
		t.Fatal(catalog, err)
	}
	deny = errors.New("session expired")
	catalog, err = api.BuilderSkillNames()
	if !errors.Is(err, deny) || catalog != nil {
		t.Fatal(catalog, err)
	}
}

package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestTemplateListSurvivesUpdateAndUninstall(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "templates.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	var notified []string
	store := NewInventory(db).WithListCreated(func(ctx context.Context, list *database.TaskList, _ commandsecurity.Admission) {
		if list.TaskCount != 1 || list.Workflow == nil {
			t.Fatal(list)
		}
		notified = append(notified, list.ID)
	})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, nativeExtension, "tasklists"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(file, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("plugin.json", `{"$schema":"`+pluginpackage.ManifestSchema+`","name":"planner","extensions":{"br.com.inclunet.assistente":{"version":1,"tasklists":["release"]}}}`)
	const template = `{"title":"Release","workflow":{"statuses":[{"id":1,"label":"Open"},{"id":2,"label":"Done"}],"initialStatusId":1,"allowedTransitions":{"1":[2]}},"tasks":[{"title":"Check"}]}`
	write(nativeExtension+"/tasklists/release.json", template)
	ctx := database.WithUserID(context.Background(), "reader")
	install := func(previous string) database.PluginInstallation {
		t.Helper()
		snap, err := pluginpackage.InspectLocal(dir)
		if err != nil {
			t.Fatal(err)
		}
		row, err := store.InstallLocal(ctx, dir, snap.Digest, previous)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	row := install("")
	admit := commandsecurity.Admission(func(fn func() error) error { return fn() })
	request := uuid.NewString()
	create := func(ctx context.Context) (string, error) {
		return store.CreateListFromTemplate(ctx, row.ID, row.Digest, "tasklists/release", request, admit)
	}
	if _, err := create(database.WithUserID(context.Background(), "other")); err == nil {
		t.Fatal("foreign template available")
	}
	listID, err := create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n < database.MaxTaskLists; n++ {
		if err := db.Create(&database.TaskList{UserID: "reader", Title: "Existing"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if second, err := create(ctx); err != nil || second != listID {
		t.Fatal(second, err)
	}
	request = uuid.NewString()
	if _, err := create(ctx); err == nil {
		t.Fatal("account quota ignored")
	}
	if len(notified) != 1 || notified[0] != listID {
		t.Fatal("unexpected creation events", notified)
	}
	var task database.Task
	if err := db.Where("task_list_id=?", listID).Take(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&task).Update("status_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	note := database.TaskNote{TaskID: task.ID, UserID: "reader", Content: "personal note"}
	if err := db.Create(&note).Error; err != nil {
		t.Fatal(err)
	}
	write("README.md", "new revision")
	row = install(row.Digest)
	if err := store.ChangeInstallation(ctx, row.ID, row.Digest, "uninstall", admit); err != nil {
		t.Fatal(err)
	}
	if _, err := create(ctx); err == nil {
		t.Fatal("removed template remained available")
	}
	var saved database.Task
	if err := db.Where("id=?", task.ID).Take(&saved).Error; err != nil || saved.StatusID != 2 {
		t.Fatal(saved, err)
	}
	var preserved database.TaskNote
	if err := db.Where("id=?", note.ID).Take(&preserved).Error; err != nil || preserved.Content != "personal note" {
		t.Fatal(preserved, err)
	}
}

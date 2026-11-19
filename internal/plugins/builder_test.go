package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
	"assistente/internal/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestBuilderExportsOnlySelectedDefinitionsToAnotherAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "builder.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginProfileActivation{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	list := database.TaskList{UserID: "author", Title: "Release"}
	if err := db.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	workflow := database.TaskListWorkflow{TaskListID: list.ID, Statuses: `[{"id":1,"label":"Ready"},{"id":2,"label":"Done"}]`, AllowedTransitions: `{"1":[2]}`, InitialStatusID: 1}
	if err := db.Create(&workflow).Error; err != nil {
		t.Fatal(err)
	}
	task := database.Task{TaskListID: list.ID, Title: "Publish release", StatusID: 2, AssigneeName: "Private Assignee"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.TaskNote{TaskID: task.ID, UserID: "author", Content: "Private note"}).Error; err != nil {
		t.Fatal(err)
	}
	skill, err := skills.Compose(&skills.SkillMetadata{Name: "notes", Version: "1.0.0", Description: "Summarize"}, "Read the list.")
	if err != nil {
		t.Fatal(err)
	}
	store := NewInventory(db).WithSkillDefinition(func(key string) ([]byte, error) {
		if key != "notes" {
			return nil, os.ErrNotExist
		}
		return []byte(skill), nil
	}, nil)
	ctx := database.WithUserID(context.Background(), "author")
	reader := database.WithUserID(context.Background(), "reader")
	req := BuilderRequest{Name: "release-kit", Version: "1", TaskLists: []BuilderTaskList{{ID: list.ID, Key: "release"}}}
	restricted := NewInventory(db).WithSkillDefinition(func(string) ([]byte, error) {
		return []byte("---\nname: notes\nversion: 1.0.0\ndescription: Summarize\nallowed-tools: Read\n---\nRead"), nil
	}, nil)
	if _, _, err := restricted.BuildPackage(ctx, BuilderRequest{Name: "notes", Skills: []string{"notes"}}); err == nil {
		t.Fatal("lost tool restriction")
	}
	for _, name := range []string{"con", "NUL.txt", "LPT2"} {
		invalid := req
		invalid.Name = name
		if _, _, err := store.BuildPackage(ctx, invalid); err == nil {
			t.Fatal("reserved archive name accepted", name)
		}
	}
	invalid := req
	invalid.TaskLists = []BuilderTaskList{{ID: list.ID, Key: "aux"}}
	if _, _, err := store.BuildPackage(ctx, invalid); err == nil {
		t.Fatal("reserved resource name accepted")
	}
	if _, _, err := store.BuildPackage(reader, req); err == nil {
		t.Fatal("foreign source used")
	}
	_, first, err := store.BuildPackage(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	file := nativeExtension + "/tasklists/release.json"
	if strings.Contains(string(first.Files[file]), "Publish release") {
		t.Fatal("tasks included without selection")
	}
	req.TaskLists[0].IncludeTasks = true
	req.Skills = []string{"notes"}
	data, snap, err := store.BuildPackage(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if string(snap.Files["skills/notes/SKILL.md"]) != skill {
		t.Fatal("skill definition changed")
	}
	if snap.Digest == first.Digest {
		t.Fatal("missing digest change")
	}
	for _, private := range []string{list.ID, task.ID, "Private Assignee", "Private note", "statusId", "assigneeName"} {
		if strings.Contains(string(snap.Files[file]), private) {
			t.Fatal("private field included", private)
		}
	}
	archive, err := pluginpackage.OpenArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	inspect := func() (*pluginpackage.LocalSnapshot, error) { return archive.Package("release-kit", ".") }
	admit := commandsecurity.Admission(func(fn func() error) error { return fn() })
	installed, err := store.InstallSource(reader, "release-kit", snap.Digest, "", admit, inspect)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateListFromTemplate(reader, installed.ID, installed.Digest, "tasklists/release", uuid.NewString(), admit)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSkillActive(reader, installed.ID, installed.Digest, "skills/notes", true); err != nil {
		t.Fatal(err)
	}
	active, err := store.ActiveSkills(reader)
	if err != nil || len(active) != 1 || active[0].Content != "Read the list." {
		t.Fatal(active, err)
	}
	var received database.Task
	if err := db.Where("task_list_id=?", id).Take(&received).Error; err != nil || received.StatusID != 1 || received.Title != task.Title {
		t.Fatal(received, err)
	}
}

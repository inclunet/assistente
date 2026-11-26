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
	"assistente/internal/profiles"
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
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.Job{}, &database.PluginProfileActivation{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.PluginListReceipt{}, &database.MCPServer{}, &database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	list := database.TaskList{UserID: "author", Title: "Release", Slug: "releases"}
	if err := db.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	workflow := database.TaskListWorkflow{TaskListID: list.ID, Statuses: `[{"id":1,"label":"Ready"},{"id":2,"label":"Done"}]`, AllowedTransitions: `{"1":[2]}`, InitialStatusID: 1}
	if err := db.Create(&workflow).Error; err != nil {
		t.Fatal(err)
	}
	task := database.Task{TaskListID: list.ID, Title: "Publish release", Code: "release", Link: "https://example.org/release", StatusID: 2, AssigneeName: "Private Assignee"}
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
	store := NewInventory(db).WithSkillDefinitions(func() (skills.PortableDefinitions, error) {
		return skills.PortableDefinitions{"notes": func() ([]byte, error) { return []byte(skill), nil }}, os.ErrPermission
	})

	ctx := database.WithUserID(context.Background(), "author")
	reader := database.WithUserID(context.Background(), "reader")
	req := BuilderRequest{Name: "release-kit", Version: "1", TaskLists: []BuilderTaskList{{ID: list.ID, Key: "release"}}}
	restricted := NewInventory(db).WithSkillDefinitions(func() (skills.PortableDefinitions, error) {
		return skills.PortableDefinitions{"notes": func() ([]byte, error) {
			return []byte("---\nname: notes\nversion: 1.0.0\ndescription: Summarize\nallowed-tools: Read\n---\nRead"), nil
		}}, nil
	})
	if _, _, err := restricted.BuildPackage(ctx, BuilderRequest{Name: "notes", Skills: []string{"notes"}}); err == nil || !strings.Contains(err.Error(), "PLUGIN_BUILDER_SKILL_UNAVAILABLE:notes") {
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
	req.Profiles = []string{"writer"}
	store = store.WithProfileDefinitions(func() (profiles.PortableDefinitions, error) {
		return profiles.PortableDefinitions{"writer": func() ([]byte, error) { return []byte(`{"name":"Writer","chat":{"llm_provider":"$default","response_timeout":60,"disable_tools":true,"max_tokens":350}}`), nil }}, nil
	})
	if _, _, err := store.BuildPackage(ctx, BuilderRequest{Name: "writer", Profiles: []string{"gone"}}); err == nil || err.Error() != "PLUGIN_BUILDER_PROFILE_UNAVAILABLE:gone" {
		t.Fatal("missing profile", err)
	}
	_, single, err := store.BuildPackage(ctx, BuilderRequest{Name: "writer", Profiles: []string{"writer"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(single.Manifest.Extensions[nativeExtension]), "tasklists") {
		t.Fatal("unselected component category present")
	}
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
	if !strings.Contains(string(snap.Files[file]), "https://example.org/release") {
		t.Fatal("o link da tarefa inicial não foi preservado")
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
	admit := commandsecurity.Admission(func(_ *gorm.DB, fn func() error) error { return fn() })
	installed, err := store.InstallSource(reader, "release-kit", snap.Digest, "", nil, admit, inspect)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateListFromTemplate(reader, installed.ID, installed.Digest, "tasklists/release", uuid.NewString(), admit)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSkillActive(reader, installed.ID, installed.Digest, "skills/notes", true, admit); err != nil {
		t.Fatal(err)
	}
	if err := store.SetProfileActive(reader, installed.ID, installed.Digest, "profiles/writer", true, admit); err != nil {
		t.Fatal(err)
	}
	entries, err := store.ListPluginProfiles(reader)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	profile, err := store.GetPluginProfile(reader, entries[0].Slug)
	if err != nil || profile.Name != "Writer" || *profile.Chat.MaxTokens != 350 || !profile.Chat.DisableTools {
		t.Fatal(profile, err)
	}
	active, err := store.ActiveSkills(reader)
	if err != nil || len(active) != 1 || active[0].Content != "Read the list." {
		t.Fatal(active, err)
	}
	var receivedList database.TaskList
	if err := db.Where("id=? AND user_id=? AND slug=?", id, "reader", "releases").Take(&receivedList).Error; err != nil {
		t.Fatal(err)
	}
	var received database.Task
	if err := db.Where("task_list_id=?", id).Take(&received).Error; err != nil || received.StatusID != 1 || received.Title != task.Title || received.Code != task.Code || received.Link != task.Link {
		t.Fatal(received, err)
	}
}

func TestBuilderOffersPortableSkillNames(t *testing.T) {
	inventory := NewInventory(nil).WithSkillDefinitions(func() (skills.PortableDefinitions, error) {
		return skills.PortableDefinitions{"notes": nil, "my_skill": nil, "con": nil}, os.ErrPermission
	})
	catalog := inventory.BuilderSkillNames()
	if !catalog.Partial || len(catalog.Names) != 1 || catalog.Names[0] != "notes" {
		t.Fatal(catalog)
	}
}

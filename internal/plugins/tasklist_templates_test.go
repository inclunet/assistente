package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestTemplateListSurvivesUpdateAndUninstall(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(database.SQLiteDSN(filepath.Join(t.TempDir(), "templates.db"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginMCPBinding{}, &database.PluginListReceipt{}, &database.MCPServer{}, &database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
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
	admit := commandsecurity.Admission(func(_ *gorm.DB, fn func() error) error { return fn() })
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
	original := request
	request = uuid.NewString()
	deletedID, err := create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Select("Tasks", "Workflow").Delete(&database.TaskList{UUIDModel: database.UUIDModel{ID: deletedID}}).Error; err != nil {
		t.Fatal(err)
	}
	if replay, err := create(ctx); err != nil || replay != deletedID {
		t.Fatal(replay, err)
	}
	var recreated int64
	if err := db.Model(&database.TaskList{}).Where("id=?", deletedID).Count(&recreated).Error; err != nil || recreated != 0 {
		t.Fatal(recreated, err)
	}
	request = original
	for n := 1; n < database.MaxTaskLists; n++ {
		if err := db.Create(&database.TaskList{UserID: "reader", Title: "Existing"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	request = strings.ToUpper(request)
	if second, err := create(ctx); err != nil || second != listID {
		t.Fatal(second, err)
	}
	request = uuid.NewString()
	if _, err := create(ctx); !errors.Is(err, database.ErrTaskListCapacity) {
		t.Fatal("account quota:", err)
	}
	if len(notified) != 2 || notified[0] != listID || notified[1] != deletedID {
		t.Fatal("unexpected creation events", notified)
	}
	var task database.Task
	if err := db.Where("task_list_id=?", listID).Take(&task).Error; err != nil {
		t.Fatal(err)
	}
	var wf database.TaskListWorkflow
	if err := db.Where("task_list_id=?", listID).Take(&wf).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{listID, wf.ID, task.ID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.Version() != 7 {
			t.Fatal("expected UUIDv7", id, err)
		}
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
	var receipts int64
	if err := db.Model(&database.PluginListReceipt{}).Where("installation_id=?", row.ID).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("receipts after uninstall", receipts, err)
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

func TestTaskListTemplateDiagnostics(t *testing.T) {
	const entry = nativeExtension + "/tasklists/release.json"
	snap := pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Extensions: map[string]json.RawMessage{
		nativeExtension: json.RawMessage(`{"version":1,"tasklists":["release"]}`),
	}}}
	for _, tc := range []struct{ name, content, reason string }{
		{"missing", "", "missingTemplate"},
		{"invalid JSON", "{", "invalidTemplate"},
		{"unnamed status", `{"title":"Release","workflow":{"statuses":[{"id":1,"label":" "},{"id":2,"label":"Done"}],"initialStatusId":1,"allowedTransitions":{"1":[2]}}}`, "invalidTemplate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.content != "" {
				snap.Files = map[string][]byte{entry: []byte(tc.content)}
			}
			got := nativeComponents(&snap)
			if len(got) != 1 || got[0].Reason != tc.reason || got[0].Entry != entry {
				t.Fatal(got)
			}
		})
	}
	names := make([]string, 2049)
	for i := range names {
		names[i] = fmt.Sprintf("template-%d", i)
	}
	for _, tc := range []struct {
		name            string
		lists, profiles int
		valid           bool
	}{
		{"lists only", 2049, 0, false},
		{"profiles only", 0, 2049, false},
		{"mixed overflow", 1024, 1025, false},
		{"mixed at limit", 1024, 1024, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(nativeResources{Version: 1, TaskLists: names[:tc.lists], Profiles: names[:tc.profiles]})
			if err != nil {
				t.Fatal(err)
			}
			snap.Manifest.Extensions[nativeExtension] = raw
			got := nativeComponents(&snap)
			if tc.valid {
				if len(got) != tc.lists+tc.profiles {
					t.Fatalf("got %d components at limit", len(got))
				}
			} else if len(got) != 1 || got[0].Kind != "invalidNative" {
				t.Fatalf("excessive declarations expanded to %d components", len(got))
			}
		})
	}
}

package tasklist

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

// TestParseExternalUpdatedAt_JiraOffset garante que external_updated_at aceita o
// formato ISO-8601 do Jira com offset ±HHMM (sem dois-pontos), além de RFC3339.
func TestParseExternalUpdatedAt_JiraOffset(t *testing.T) {
	valid := []string{
		"2026-05-25T15:35:53.521-0300",  // Jira com ms
		"2026-05-25T15:35:53-0300",      // Jira sem ms
		"2026-05-25T15:35:53.521-03:00", // RFC3339 (com :)
		"2026-05-25T15:35:53Z",          // UTC
		"2026-05-25",                    // só data
	}
	for _, in := range valid {
		tm, err := parseExternalUpdatedAt(in)
		if err != nil {
			t.Fatalf("parseExternalUpdatedAt(%q) erro inesperado: %v", in, err)
		}
		if tm == nil {
			t.Fatalf("parseExternalUpdatedAt(%q) retornou nil sem erro", in)
		}
	}

	// String vazia continua sendo "sem valor" (nil, nil), não erro.
	if tm, err := parseExternalUpdatedAt("   "); err != nil || tm != nil {
		t.Fatalf("vazio deveria dar (nil,nil), got tm=%v err=%v", tm, err)
	}

	// Lixo continua sendo rejeitado.
	if _, err := parseExternalUpdatedAt("not-a-time"); err == nil {
		t.Fatalf("esperava erro para timestamp inválido")
	}
}

func TestTaskNoteParameters_ListContractAndUUIDs(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(NewTaskNote(nil).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties inválido: %#v", schema["properties"])
	}
	for _, field := range []string{"task_list_id", "note_id"} {
		property, ok := properties[field].(map[string]any)
		if !ok || property["type"] != "string" {
			t.Fatalf("%s deve declarar UUID string, got %#v", field, properties[field])
		}
	}
	if _, ok := properties["list"]; !ok {
		t.Fatal("schema deve expor discriminador list")
	}
	for _, field := range []string{"limit", "cursor", "sort"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("schema não contém %s", field)
		}
	}
	if _, required := schema["required"]; required {
		t.Fatal("content não pode ser obrigatório no schema para permitir list=true")
	}
}

func TestTaskNote_ListPagedByTaskSourceAndType(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("Fila", defaultStatuses())
	task := mgr.addTask(list.ID, "Chamado", 1)
	task.Code = "ISSUE-1"
	customer := database.TaskNoteCustomer
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tiedNoteContent := make(map[string]string)
	for index, externalID := range []string{"comment-a", "comment-b", "comment-c"} {
		note, _, err := mgr.UpsertTaskNoteByExternal(context.Background(), database.UpsertTaskNoteByExternalParams{
			TaskID:           task.ID,
			Type:             &customer,
			Content:          externalID,
			ExternalSource:   "jira",
			ExternalID:       externalID,
			ExternalParentID: "thread-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if index < 2 {
			tiedNoteContent[note.ID] = externalID
		}
		if err := mgr.db.Model(&database.TaskNote{}).Where("id = ?", note.ID).
			Updates(map[string]any{"created_at": base.Add(time.Duration(index/2) * time.Minute)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mgr.CreateTaskNote(context.Background(), task.ID, database.TaskNoteInternal, "local", "", ""); err != nil {
		t.Fatal(err)
	}
	mgr.refreshSnapshots()

	tool := NewTaskNote(mgr)
	first, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"list":               true,
		"task_code":          "ISSUE-1",
		"task_list_id":       "  " + list.ID + "  ",
		"source":             "jira",
		"type":               2,
		"external_parent_id": "thread-1",
		"limit":              2,
		"sort":               "created_at:asc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if first.IsError || !first.Structured {
		t.Fatalf("unexpected first page result: %+v", first)
	}
	var firstBody struct {
		Notes []struct {
			ID         string `json:"id"`
			Content    string `json:"content"`
			TaskID     string `json:"task_id"`
			Type       int    `json:"type"`
			Source     string `json:"source"`
			ExternalID string `json:"external_id"`
		} `json:"notes"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal([]byte(first.Content), &firstBody); err != nil {
		t.Fatal(err)
	}
	if len(firstBody.Notes) != 2 || !firstBody.HasMore || firstBody.NextCursor == "" {
		t.Fatalf("unexpected first page body: %+v", firstBody)
	}
	tiedIDs := make([]string, 0, len(tiedNoteContent))
	for id := range tiedNoteContent {
		tiedIDs = append(tiedIDs, id)
	}
	sort.Strings(tiedIDs)
	if firstBody.Notes[0].ID != tiedIDs[0] || firstBody.Notes[1].ID != tiedIDs[1] ||
		firstBody.Notes[0].Content != tiedNoteContent[tiedIDs[0]] ||
		firstBody.Notes[1].Content != tiedNoteContent[tiedIDs[1]] {
		t.Fatalf("created_at+id tie-breaker is unstable: %+v", firstBody.Notes)
	}

	second, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"list":               true,
		"task_code":          "ISSUE-1",
		"task_list_id":       list.ID,
		"source":             "jira",
		"type":               2,
		"external_parent_id": "thread-1",
		"limit":              2,
		"sort":               "created_at:asc",
		"cursor":             firstBody.NextCursor,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var secondBody struct {
		Notes      []map[string]any `json:"notes"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal([]byte(second.Content), &secondBody); err != nil {
		t.Fatal(err)
	}
	if len(secondBody.Notes) != 1 || secondBody.HasMore || secondBody.NextCursor != nil {
		t.Fatalf("unexpected second page: %+v", secondBody)
	}

	mismatch, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"list": true, "task_id": task.ID, "source": "jira", "type": 3,
		"external_parent_id": "thread-1", "limit": 2, "sort": "created_at:asc",
		"cursor": firstBody.NextCursor,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !mismatch.IsError || !strings.Contains(mismatch.Content, "não corresponde") {
		t.Fatalf("expected cursor binding error, got %+v", mismatch)
	}
}

func TestTaskNote_ListValidationAndNullFilter(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("Fila", defaultStatuses())
	task := mgr.addTask(list.ID, "Chamado", 1)
	if _, err := mgr.CreateTaskNote(context.Background(), task.ID, database.TaskNoteInternal, "local", "", ""); err != nil {
		t.Fatal(err)
	}
	tool := NewTaskNote(mgr)

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "cursor inválido", args: map[string]any{"list": true, "cursor": "bad"}, want: "cursor inválido"},
		{name: "cursor vazio", args: map[string]any{"list": true, "cursor": "  "}, want: "cursor must be a non-empty"},
		{name: "sort vazio", args: map[string]any{"list": true, "sort": "  "}, want: "sort must be created_at"},
		{name: "limit inválido", args: map[string]any{"list": true, "limit": 101}, want: "limit must be between"},
		{name: "campo de escrita", args: map[string]any{"list": true, "content": "x"}, want: "content cannot be combined"},
		{name: "paginação sem list", args: map[string]any{"limit": 2, "content": "x"}, want: "require list=true"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), mustMarshal(t, test.args))
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(result.Content, test.want) {
				t.Fatalf("want error containing %q, got %+v", test.want, result)
			}
		})
	}

	local, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"list": true, "source": nil, "external_id": nil, "external_parent_id": nil,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if local.IsError || !local.Structured {
		t.Fatalf("null filters should be valid: %+v", local)
	}
	var body struct {
		Notes []map[string]any `json:"notes"`
	}
	if err := json.Unmarshal([]byte(local.Content), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notes) != 1 || body.Notes[0]["content"] != "local" {
		t.Fatalf("null filters returned unexpected notes: %+v", body.Notes)
	}
}

func TestTaskNote_LegacyWriteStillWorksWithoutList(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("Fila", defaultStatuses())
	task := mgr.addTask(list.ID, "Chamado", 1)
	result, err := NewTaskNote(mgr).Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    1,
		"content": "comportamento legado",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "Note added") {
		t.Fatalf("legacy write regressed: %+v", result)
	}
	notes, err := mgr.GetTaskNotes(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Content != "comportamento legado" {
		t.Fatalf("legacy note was not persisted: %+v", notes)
	}
}

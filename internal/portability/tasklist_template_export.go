package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// ExportTaskListTemplate reads publishable columns in the caller's read
// transaction. Notes, assignments, progress, grants and work history never
// enter the builder's in-memory package.
func ExportTaskListTemplate(ctx context.Context, tx *gorm.DB, id string, initialTasks bool) (TaskListExport, error) {
	var template TaskListExport
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return template, err
	}
	var list database.TaskList
	if err := tx.Select("id", "slug", "title", "description", "preferred_view_mode", "validation_policy").Where("id=? AND user_id=?", id, owner).Preload("Workflow").Take(&list).Error; err != nil {
		return template, err
	}
	if list.Workflow == nil {
		return template, errors.New("missing workflow")
	}
	w, err := exportTaskListWorkflow(*list.Workflow)
	if err != nil {
		return template, err
	}
	w.ID, w.TaskListID = "", ""
	template = TaskListExport{Title: list.Title, Slug: list.Slug, Description: list.Description, PreferredViewMode: list.PreferredViewMode, ValidationPolicy: list.ValidationPolicy, Workflow: w}
	if !initialTasks {
		return template, nil
	}
	var rows []database.Task
	if err := tx.Select("id", "parent_id", "title", "description", "link", "code").Where("task_list_id=?", list.ID).Order("`order`, created_at, id").Limit(1001).Find(&rows).Error; err != nil {
		return template, err
	}
	if len(rows) > 1000 {
		return template, errors.New("too many initial tasks")
	}
	children := map[string][]database.Task{}
	for _, row := range rows {
		parent := ""
		if row.ParentID != nil {
			parent = *row.ParentID
		}
		children[parent] = append(children[parent], row)
	}
	count := 0
	var descend func(string, int) ([]TaskExport, error)
	descend = func(parent string, depth int) ([]TaskExport, error) {
		var items []TaskExport
		for _, row := range children[parent] {
			if depth > 16 {
				return nil, errors.New("too many task levels")
			}
			sub, err := descend(row.ID, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, TaskExport{Title: row.Title, Description: row.Description, Link: row.Link, Code: row.Code, Children: sub})
			count++
		}
		return items, nil
	}
	template.Tasks, err = descend("", 1)
	if err == nil && count != len(rows) {
		err = errors.New("invalid task hierarchy")
	}
	return template, err
}

// MarshalTemplate omite os campos obrigatórios de estado de uma instância
// que o encoder de backups grava mesmo quando estão zerados.
func (list TaskListExport) MarshalTemplate() ([]byte, error) {
	data, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	delete(object, "createdAt")
	var omitProgress func(map[string]any)
	omitProgress = func(parent map[string]any) {
		for _, field := range []string{"tasks", "children"} {
			children, _ := parent[field].([]any)
			for _, item := range children {
				task := item.(map[string]any)
				delete(task, "createdAt")
				delete(task, "statusId")
				delete(task, "order")
				omitProgress(task)
			}
		}
	}
	omitProgress(object)
	return json.MarshalIndent(object, "", "  ")
}

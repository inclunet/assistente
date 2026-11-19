package portability

import (
	"context"
	"errors"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// ExportTaskListTemplate reads publishable columns in the caller's read
// transaction. Notes, assignments, progress, grants and work history never
// enter the builder's in-memory package.
func ExportTaskListTemplate(ctx context.Context, tx *gorm.DB, id string, initialTasks bool) (TaskListTemplate, error) {
	var template TaskListTemplate
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return template, err
	}
	var list database.TaskList
	if err := tx.Select("id", "title", "description").Where("id=? AND user_id=?", id, owner).Preload("Workflow").Take(&list).Error; err != nil {
		return template, err
	}
	if list.Workflow == nil {
		return template, errors.New("missing workflow")
	}
	w, err := exportTaskListWorkflow(*list.Workflow)
	if err != nil {
		return template, err
	}
	template = TaskListTemplate{Title: list.Title, Description: list.Description, Workflow: TemplateWorkflow{Statuses: w.Statuses, InitialStatusID: w.InitialStatusID, AllowedTransitions: w.AllowedTransitions}}
	if !initialTasks {
		return template, nil
	}
	var rows []database.Task
	if err := tx.Select("id", "parent_id", "title", "description").Where("task_list_id=?", list.ID).Order("`order`, created_at, id").Limit(1001).Find(&rows).Error; err != nil {
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
	var descend func(string, int) ([]TemplateTask, error)
	descend = func(parent string, depth int) ([]TemplateTask, error) {
		var items []TemplateTask
		for _, row := range children[parent] {
			if depth > 16 {
				return nil, errors.New("too many task levels")
			}
			sub, err := descend(row.ID, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, TemplateTask{Title: row.Title, Description: row.Description, Children: sub})
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

package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ReadTaskListTemplate(raw []byte) (*TaskListExport, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("invalid tasklist template size")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var template TaskListExport
	if err := dec.Decode(&template); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF || strings.TrimSpace(template.Title) == "" {
		return nil, errors.New("invalid tasklist template")
	}
	if err := database.ValidateTaskListSlugFormat(template.Slug); err != nil {
		return nil, err
	}
	if _, err := database.ParseTaskListValidationPolicyJSON(template.ValidationPolicy); err != nil {
		return nil, err
	}
	if template.ID != "" || !template.CreatedAt.IsZero() {
		return nil, errors.New("template contains instance metadata")
	}
	if template.Workflow.ID != "" || template.Workflow.TaskListID != "" {
		return nil, errors.New("template workflow contains instance identifiers")
	}
	for _, status := range template.Workflow.Statuses {
		if strings.TrimSpace(status.Label) == "" {
			return nil, errors.New("empty template workflow status label")
		}
	}
	if _, _, err := validateImportedTaskListWorkflow(template.Workflow); err != nil {
		return nil, err
	}
	total := 0
	var validate func([]TaskExport, int) error
	validate = func(tasks []TaskExport, depth int) error {
		for _, task := range tasks {
			if task.ID != "" || task.TaskListID != "" || task.ParentID != "" || task.StatusID != 0 || task.Order != 0 || task.AssigneeID != "" || task.AssigneeName != "" || task.CreatorID != "" || task.CreatorName != "" || len(task.Notes) != 0 || task.CompletedAt != nil || !task.CreatedAt.IsZero() || task.DueDate != nil {
				return errors.New("template task contains instance data")
			}
			total++
			if depth > 16 || total > 1000 || strings.TrimSpace(task.Title) == "" {
				return errors.New("invalid template task")
			}
			if err := validate(task.Children, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validate(template.Tasks, 1); err != nil {
		return nil, err
	}
	return &template, nil
}

// InstantiateTemplate writes an independent list with fresh identifiers through the
// same transaction and workflow validation used by the portability importer.
func (t TaskListExport) InstantiateTemplate(ctx context.Context, tx *gorm.DB, id string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	out := TaskListExport{ID: id, Slug: t.Slug, Title: t.Title, Description: t.Description, PreferredViewMode: t.PreferredViewMode, ValidationPolicy: t.ValidationPolicy, Workflow: t.Workflow}
	if out.Slug != "" {
		var occupied []string
		if err := tx.WithContext(ctx).Model(&database.TaskList{}).Where("user_id = ? AND slug <> ''", userID).Pluck("slug", &occupied).Error; err != nil {
			return err
		}
		for copyNumber := 2; slices.Contains(occupied, out.Slug); copyNumber++ {
			suffix := "-" + strconv.Itoa(copyNumber)
			out.Slug = t.Slug[:min(len(t.Slug), 63-len(suffix))] + suffix
		}
	}
	workflowID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	out.Workflow.ID = workflowID.String()
	var tasks func([]TaskExport) ([]TaskExport, error)
	tasks = func(input []TaskExport) ([]TaskExport, error) {
		result := make([]TaskExport, 0, len(input))
		for order, item := range input {
			id, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			children, err := tasks(item.Children)
			if err != nil {
				return nil, err
			}
			result = append(result, TaskExport{ID: id.String(), Title: item.Title, Description: item.Description, Link: item.Link, Code: item.Code, Order: order, StatusID: t.Workflow.InitialStatusID, Children: children})
		}
		return result, nil
	}
	out.Tasks, err = tasks(t.Tasks)
	if err != nil {
		return err
	}
	return persistTaskList(ctx, tx, out, nil)
}

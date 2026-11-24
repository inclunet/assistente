package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TemplateTask struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Children    []TemplateTask `json:"children,omitempty"`
}

type TemplateWorkflow struct {
	Statuses           []TaskListWorkflowStatusExport `json:"statuses"`
	AllowedTransitions map[int][]int                  `json:"allowedTransitions"`
	InitialStatusID    int                            `json:"initialStatusId"`
}

// TaskListTemplate contains distributable definitions. Instance identifiers,
// assignments, notes and progress are intentionally absent from its schema.
type TaskListTemplate struct {
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	Workflow    TemplateWorkflow `json:"workflow"`
	Tasks       []TemplateTask   `json:"tasks,omitempty"`
}

func (t TaskListTemplate) workflowExport() TaskListWorkflowExport {
	return TaskListWorkflowExport{Statuses: t.Workflow.Statuses, AllowedTransitions: t.Workflow.AllowedTransitions, InitialStatusID: t.Workflow.InitialStatusID}
}

func ReadTaskListTemplate(raw []byte) (*TaskListTemplate, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("invalid tasklist template size")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var template TaskListTemplate
	if err := dec.Decode(&template); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF || strings.TrimSpace(template.Title) == "" {
		return nil, errors.New("invalid tasklist template")
	}
	for _, status := range template.Workflow.Statuses {
		if strings.TrimSpace(status.Label) == "" {
			return nil, errors.New("empty template workflow status label")
		}
	}
	if _, _, err := validateImportedTaskListWorkflow(template.workflowExport()); err != nil {
		return nil, err
	}
	total := 0
	var validate func([]TemplateTask, int) error
	validate = func(tasks []TemplateTask, depth int) error {
		for _, task := range tasks {
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

// Instantiate writes an independent list with fresh identifiers through the
// same transaction and workflow validation used by the portability importer.
func (t TaskListTemplate) Instantiate(ctx context.Context, tx *gorm.DB, id string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	out := TaskListExport{ID: id, Title: t.Title, Description: t.Description, Workflow: t.workflowExport()}
	workflowID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	out.Workflow.ID = workflowID.String()
	var tasks func([]TemplateTask) ([]TaskExport, error)
	tasks = func(input []TemplateTask) ([]TaskExport, error) {
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
			result = append(result, TaskExport{ID: id.String(), Title: item.Title, Description: item.Description, Order: order, StatusID: t.Workflow.InitialStatusID, Children: children})
		}
		return result, nil
	}
	out.Tasks, err = tasks(t.Tasks)
	if err != nil {
		return err
	}
	return persistTaskList(ctx, tx, out, nil)
}

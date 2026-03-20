package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type upsertTaskArgs struct {
	TaskListID  uint   `json:"task_list_id"`
	TaskID      *uint  `json:"task_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StatusID    *int   `json:"status_id,omitempty"`
	ParentID    *uint  `json:"parent_task_id,omitempty"`
}

type UpsertTaskTool struct {
	mgr TaskListManager
}

func NewUpsertTask(mgr TaskListManager) *UpsertTaskTool {
	return &UpsertTaskTool{mgr: mgr}
}

func (t *UpsertTaskTool) Name() string { return "upsert_task" }

func (t *UpsertTaskTool) Description() string {
	return "Creates or updates a task. If task_id is provided, updates the existing task; otherwise creates a new one. Use get_task_list first to see available status IDs for the workflow."
}

func (t *UpsertTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list this task belongs to"
			},
			"task_id": {
				"type": "integer",
				"description": "ID of existing task to update. Omit to create a new task"
			},
			"title": {
				"type": "string",
				"description": "Task title"
			},
			"description": {
				"type": "string",
				"description": "Task description (optional)"
			},
			"status_id": {
				"type": "integer",
				"description": "Workflow status ID. Must be a valid status in the task list's workflow. Omit when creating to use the initial status"
			},
			"parent_task_id": {
				"type": "integer",
				"description": "Parent task ID to create this as a subtask (optional)"
			}
		},
		"required": ["task_list_id", "title"],
		"additionalProperties": false
	}`)
}

func (t *UpsertTaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params upsertTaskArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskListID == 0 {
		return tools.ToolResult{Content: "task_list_id is required and must be > 0", IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "title cannot be empty", IsError: true}, nil
	}

	// Validate status_id if provided
	if params.StatusID != nil {
		if err := t.validateStatusID(params.TaskListID, *params.StatusID); err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
	}

	// UPDATE existing task
	if params.TaskID != nil {
		return t.updateTask(*params.TaskID, title, params.Description, params.StatusID)
	}

	// CREATE new task
	return t.createTask(params.TaskListID, title, params.Description, params.ParentID, params.StatusID)
}

func (t *UpsertTaskTool) validateStatusID(taskListID uint, statusID int) error {
	workflow, err := t.mgr.GetWorkflow(taskListID)
	if err != nil {
		return fmt.Errorf("could not load workflow for task list %d: %v", taskListID, err)
	}

	statuses, err := parseWorkflowStatuses(workflow)
	if err != nil {
		return fmt.Errorf("could not parse workflow statuses: %v", err)
	}

	for _, s := range statuses {
		if s.ID == statusID {
			return nil
		}
	}

	validLabels := make([]string, len(statuses))
	for i, s := range statuses {
		validLabels[i] = fmt.Sprintf("%d (%s)", s.ID, s.Label)
	}
	return fmt.Errorf("invalid status_id %d. Valid statuses: %s", statusID, strings.Join(validLabels, ", "))
}

func (t *UpsertTaskTool) updateTask(taskID uint, title, description string, statusID *int) (tools.ToolResult, error) {
	if err := t.mgr.UpdateTask(taskID, title, description); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating task %d: %v", taskID, err), IsError: true}, nil
	}

	if statusID != nil {
		if err := t.mgr.UpdateTaskStatus(taskID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task updated but status change failed: %v", err), IsError: true}, nil
		}
	}

	task, err := t.mgr.GetTask(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task updated (id=%d) but could not fetch result: %v", taskID, err)}, nil
	}

	resultJSON, _ := json.Marshal(map[string]any{
		"id":        task.ID,
		"title":     task.Title,
		"status_id": task.StatusID,
		"action":    "updated",
	})
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task updated:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": "updated"},
	}, nil
}

func (t *UpsertTaskTool) createTask(taskListID uint, title, description string, parentID *uint, statusID *int) (tools.ToolResult, error) {
	task, err := t.mgr.CreateTask(taskListID, title, description, parentID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task: %v", err), IsError: true}, nil
	}

	// If a specific status was requested (different from initial), update it
	if statusID != nil && *statusID != task.StatusID {
		if err := t.mgr.UpdateTaskStatus(task.ID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task created (id=%d) but status change failed: %v", task.ID, err), IsError: true}, nil
		}
		task.StatusID = *statusID
	}

	resultJSON, _ := json.Marshal(map[string]any{
		"id":        task.ID,
		"title":     task.Title,
		"status_id": task.StatusID,
		"action":    "created",
	})
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task created:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": "created"},
	}, nil
}

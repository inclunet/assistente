package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type upsertTaskArgs struct {
	TaskListID   uint    `json:"task_list_id"`
	TaskID       *uint   `json:"task_id,omitempty"`
	Title        string  `json:"title"`
	Description  string  `json:"description,omitempty"`
	Code         string  `json:"code,omitempty"`
	Link         string  `json:"link,omitempty"`
	StatusID     *int    `json:"status_id,omitempty"`
	ParentID     *uint   `json:"parent_task_id,omitempty"`
	AssigneeName *string `json:"assignee_name,omitempty"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
	CreatorName  *string `json:"creator_name,omitempty"`
	CreatorID    *string `json:"creator_id,omitempty"`
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
			},
			"code": {
				"type": "string",
				"description": "External identifier or ticket code (e.g. FSD-12345, JIRA-999). Optional"
			},
			"link": {
				"type": "string",
				"description": "URL or deep link associated with this task (e.g. assistente://conversation/open?id=123 or https://jira.example.com/browse/FSD-123). Optional"
			},
			"assignee_name": {
				"type": "string",
				"description": "Display name of the person currently working on this task. Set to empty string to clear. Optional"
			},
			"assignee_id": {
				"type": "string",
				"description": "Stable identifier for the assignee (e.g. email, UUID, external account ID). Set to empty string to clear. Optional"
			},
			"creator_name": {
				"type": "string",
				"description": "Display name of the person who created/originated this task. Set to empty string to clear. Optional"
			},
			"creator_id": {
				"type": "string",
				"description": "Stable identifier for the creator (e.g. email, UUID, external account ID). Set to empty string to clear. Optional"
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

	// UPDATE existing task by ID
	if params.TaskID != nil {
		return t.updateTask(*params.TaskID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID)
	}

	// DEDUP by code: if code is provided, look for existing task in the same list
	if code := strings.TrimSpace(params.Code); code != "" {
		existing, err := t.mgr.FindTaskByCode(params.TaskListID, code)
		if err == nil && existing != nil {
			return t.updateTask(existing.ID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID)
		}
	}

	// CREATE new task
	return t.createTask(params.TaskListID, title, params.Description, params.Code, params.Link, params.ParentID, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID)
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

func (t *UpsertTaskTool) updateTask(taskID uint, title, description, code, link string, statusID *int, assigneeName, assigneeID, creatorName, creatorID *string) (tools.ToolResult, error) {
	oldTask, err := t.mgr.GetTask(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%d): %v", taskID, err), IsError: true}, nil
	}

	aName := derefOrKeep(assigneeName, oldTask, func(t *database.Task) string { return t.AssigneeName })
	aID := derefOrKeep(assigneeID, oldTask, func(t *database.Task) string { return t.AssigneeID })
	cName := derefOrKeep(creatorName, oldTask, func(t *database.Task) string { return t.CreatorName })
	cID := derefOrKeep(creatorID, oldTask, func(t *database.Task) string { return t.CreatorID })

	needsStatusChange := statusID != nil && *statusID != oldTask.StatusID

	fieldsChanged := title != oldTask.Title ||
		description != oldTask.Description ||
		code != oldTask.Code ||
		link != oldTask.Link ||
		aName != oldTask.AssigneeName ||
		aID != oldTask.AssigneeID ||
		cName != oldTask.CreatorName ||
		cID != oldTask.CreatorID

	if !fieldsChanged && !needsStatusChange {
		resultJSON, _ := json.Marshal(t.taskResultMap(oldTask, "noop"))
		return tools.ToolResult{
			Content:  fmt.Sprintf("Task unchanged:\n%s", string(resultJSON)),
			Metadata: map[string]any{"task_id": oldTask.ID, "action": "noop"},
		}, nil
	}

	if fieldsChanged {
		if err := t.mgr.UpdateTaskFull(taskID, title, description, code, link, aName, aID, cName, cID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error updating task %d: %v", taskID, err), IsError: true}, nil
		}
		t.emitAssigneeChangeNote(oldTask, aName, taskID)
	}

	if needsStatusChange {
		if err := t.mgr.UpdateTaskStatus(taskID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task updated but status change failed: %v", err), IsError: true}, nil
		}
	}

	task, err := t.mgr.GetTask(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task updated (id=%d) but could not fetch result: %v", taskID, err)}, nil
	}

	resultJSON, _ := json.Marshal(t.taskResultMap(task, "updated"))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task updated:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": "updated"},
	}, nil
}

func (t *UpsertTaskTool) createTask(taskListID uint, title, description, code, link string, parentID *uint, statusID *int, assigneeName, assigneeID, creatorName, creatorID *string) (tools.ToolResult, error) {
	aName := derefOr(assigneeName, "")
	aID := derefOr(assigneeID, "")
	cName := derefOr(creatorName, "")
	cID := derefOr(creatorID, "")

	var task *database.Task
	var err error
	if aName != "" || aID != "" || cName != "" || cID != "" {
		task, err = t.mgr.CreateTaskFull(taskListID, title, description, code, link, aName, aID, cName, cID, parentID)
	} else {
		task, err = t.mgr.CreateTask(taskListID, title, description, code, link, parentID)
	}
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task: %v", err), IsError: true}, nil
	}

	if statusID != nil && *statusID != task.StatusID {
		if err := t.mgr.UpdateTaskStatus(task.ID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task created (id=%d) but status change failed: %v", task.ID, err), IsError: true}, nil
		}
		task.StatusID = *statusID
	}

	resultJSON, _ := json.Marshal(t.taskResultMap(task, "created"))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task created:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": "created"},
	}, nil
}

func (t *UpsertTaskTool) taskResultMap(task *database.Task, action string) map[string]any {
	result := map[string]any{
		"id":        task.ID,
		"title":     task.Title,
		"status_id": task.StatusID,
		"action":    action,
	}
	if task.AssigneeName != "" {
		result["assignee_name"] = task.AssigneeName
	}
	if task.AssigneeID != "" {
		result["assignee_id"] = task.AssigneeID
	}
	if task.CreatorName != "" {
		result["creator_name"] = task.CreatorName
	}
	if task.CreatorID != "" {
		result["creator_id"] = task.CreatorID
	}
	return result
}

func derefOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

func derefOrKeep(p *string, task *database.Task, getter func(*database.Task) string) string {
	if p != nil {
		return *p
	}
	if task != nil {
		return getter(task)
	}
	return ""
}

func (t *UpsertTaskTool) emitAssigneeChangeNote(oldTask *database.Task, newAssigneeName string, taskID uint) {
	if oldTask == nil {
		return
	}
	oldName := oldTask.AssigneeName
	if oldName == newAssigneeName {
		return
	}

	var content string
	switch {
	case oldName == "" && newAssigneeName != "":
		content = fmt.Sprintf("Assignee set to %s", newAssigneeName)
	case oldName != "" && newAssigneeName == "":
		content = fmt.Sprintf("Assignee removed (was %s)", oldName)
	default:
		content = fmt.Sprintf("Assignee changed from %s to %s", oldName, newAssigneeName)
	}

	_, _ = t.mgr.CreateTaskNote(taskID, database.TaskNoteSystem, content, "system", "")
}

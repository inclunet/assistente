package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type taskArgs struct {
	TaskListID   uint    `json:"task_list_id,omitempty"`
	TaskID       *uint   `json:"task_id,omitempty"`
	Delete       bool    `json:"delete,omitempty"`
	Duplicate    bool    `json:"duplicate,omitempty"`
	Title        string  `json:"title,omitempty"`
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

type TaskTool struct {
	mgr TaskListManager
}

func NewTask(mgr TaskListManager) *TaskTool {
	return &TaskTool{mgr: mgr}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return `Full CRUD for tasks. With task_id only → read (returns details, subtasks, and notes). Without task_id + task_list_id + title → create (dedup by code). With task_id + title → update. With task_id + different task_list_id → move. With task_id + duplicate → copy (inherits description/link/assignee/creator; code NOT copied). With task_id + delete → permanently removes task and subtasks. Use task_list tool to see available status IDs.`
}

func (t *TaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list. Required for create. When updating, a different value moves the task to that list"
			},
			"task_id": {
				"type": "integer",
				"description": "ID of existing task. Alone → read details+notes. With title → update. With duplicate → copy. With delete → remove"
			},
			"delete": {
				"type": "boolean",
				"description": "When true, permanently deletes the task referenced by task_id and all its subtasks. Requires task_id. Cannot be combined with duplicate"
			},
			"duplicate": {
				"type": "boolean",
				"description": "When true, creates a copy of the task referenced by task_id. Inherits description, link, assignee, and creator. Code is NOT copied. Requires task_id"
			},
			"title": {
				"type": "string",
				"description": "Task title. Required for create, update, and duplicate"
			},
			"description": {
				"type": "string",
				"description": "Task description (optional)"
			},
			"status_id": {
				"type": "integer",
				"description": "Workflow status ID. Must be valid in the task list's workflow. Omit when creating to use initial status"
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
				"description": "URL or deep link associated with this task. Optional"
			},
			"assignee_name": {
				"type": "string",
				"description": "Display name of the assignee. Set to empty string to clear. Optional"
			},
			"assignee_id": {
				"type": "string",
				"description": "Stable identifier for the assignee. Set to empty string to clear. Optional"
			},
			"creator_name": {
				"type": "string",
				"description": "Display name of the creator. Set to empty string to clear. Optional"
			},
			"creator_id": {
				"type": "string",
				"description": "Stable identifier for the creator. Set to empty string to clear. Optional"
			}
		},
		"additionalProperties": false
	}`)
}

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params taskArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.Delete && params.Duplicate {
		return tools.ToolResult{Content: "delete and duplicate cannot be used together", IsError: true}, nil
	}

	if params.Delete {
		if params.TaskID == nil {
			return tools.ToolResult{Content: "delete requires task_id to reference the task to remove", IsError: true}, nil
		}
		return t.deleteTask(*params.TaskID)
	}

	isWrite := strings.TrimSpace(params.Title) != "" || params.Duplicate || params.StatusID != nil ||
		params.Description != "" || params.Code != "" || params.Link != "" ||
		params.AssigneeName != nil || params.AssigneeID != nil ||
		params.CreatorName != nil || params.CreatorID != nil || params.ParentID != nil

	// READ mode: task_id only, no write params
	if params.TaskID != nil && !isWrite {
		if *params.TaskID == 0 {
			return tools.ToolResult{Content: "task_id must be > 0", IsError: true}, nil
		}
		return t.readTask(*params.TaskID)
	}

	// WRITE modes below
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "title is required for create, update, and duplicate operations", IsError: true}, nil
	}

	if params.Duplicate && params.TaskID == nil {
		return tools.ToolResult{Content: "duplicate requires task_id to reference the source task", IsError: true}, nil
	}

	if params.TaskListID == 0 && params.TaskID == nil {
		return tools.ToolResult{Content: "task_list_id is required when creating a new task", IsError: true}, nil
	}

	if params.StatusID != nil {
		listID := params.TaskListID
		if listID == 0 && params.TaskID != nil {
			task, err := t.mgr.GetTask(*params.TaskID)
			if err == nil {
				listID = task.TaskListID
			}
		}
		if listID != 0 {
			if err := t.validateStatusID(listID, *params.StatusID); err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
		}
	}

	if params.TaskID != nil {
		if params.Duplicate {
			return t.duplicateTask(params.TaskListID, *params.TaskID, title, params.Description, params.Code, params.Link, params.ParentID, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID)
		}

		moved := false
		if params.TaskListID != 0 {
			var err error
			moved, err = t.moveIfNeeded(*params.TaskID, params.TaskListID)
			if err != nil {
				return tools.ToolResult{Content: fmt.Sprintf("Error moving task %d to list %d: %v", *params.TaskID, params.TaskListID, err), IsError: true}, nil
			}
		}
		return t.updateTask(*params.TaskID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, moved)
	}

	if code := strings.TrimSpace(params.Code); code != "" {
		existing, err := t.mgr.FindTaskByCode(params.TaskListID, code)
		if err == nil && existing != nil {
			return t.updateTask(existing.ID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, false)
		}
	}

	return t.createTask(params.TaskListID, title, params.Description, params.Code, params.Link, params.ParentID, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID)
}

// ==================== Read ====================

func (t *TaskTool) readTask(taskID uint) (tools.ToolResult, error) {
	task, err := t.mgr.GetTask(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%d): %v", taskID, err), IsError: true}, nil
	}

	notes, err := t.mgr.GetTaskNotes(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error fetching notes for task %d: %v", taskID, err), IsError: true}, nil
	}

	typeLabels := map[int]string{1: "internal", 2: "customer", 3: "agent", 4: "system"}

	response := map[string]any{
		"id":           task.ID,
		"task_list_id": task.TaskListID,
		"title":        task.Title,
		"status_id":    task.StatusID,
	}
	if task.Description != "" {
		response["description"] = task.Description
	}
	if task.Code != "" {
		response["code"] = task.Code
	}
	if task.Link != "" {
		response["link"] = task.Link
	}
	if task.ParentID != nil {
		response["parent_id"] = *task.ParentID
	}
	if task.AssigneeName != "" {
		response["assignee_name"] = task.AssigneeName
	}
	if task.AssigneeID != "" {
		response["assignee_id"] = task.AssigneeID
	}
	if task.CreatorName != "" {
		response["creator_name"] = task.CreatorName
	}
	if task.CreatorID != "" {
		response["creator_id"] = task.CreatorID
	}

	if len(task.Subtasks) > 0 {
		subtaskIDs := make([]uint, len(task.Subtasks))
		for i, s := range task.Subtasks {
			subtaskIDs[i] = s.ID
		}
		response["subtask_ids"] = subtaskIDs
	}

	if len(notes) > 0 {
		notesList := make([]map[string]any, len(notes))
		for i, note := range notes {
			n := map[string]any{
				"id":      note.ID,
				"type":    typeLabels[int(note.Type)],
				"content": note.Content,
				"date":    note.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			if note.AuthorName != "" {
				n["author"] = note.AuthorName
			}
			if note.ExternalSource != "" {
				n["source"] = note.ExternalSource
			}
			if note.ExternalID != "" {
				n["external_id"] = note.ExternalID
			}
			if note.ExternalParentID != "" {
				n["external_parent_id"] = note.ExternalParentID
			}
			if note.ExternalUpdatedAt != nil {
				n["external_updated_at"] = note.ExternalUpdatedAt.Format(time.RFC3339)
			}
			notesList[i] = n
		}
		response["notes"] = notesList
	}

	resultJSON, _ := json.Marshal(response)
	return tools.ToolResult{
		Content:  string(resultJSON),
		Metadata: map[string]any{"task_id": taskID, "note_count": len(notes)},
	}, nil
}

// ==================== Write ====================

func (t *TaskTool) deleteTask(taskID uint) (tools.ToolResult, error) {
	task, err := t.mgr.GetTask(taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%d): %v", taskID, err), IsError: true}, nil
	}

	if err := t.mgr.DeleteTask(taskID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error deleting task %d: %v", taskID, err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content:  fmt.Sprintf("Task deleted: '%s' (id=%d)", task.Title, task.ID),
		Metadata: map[string]any{"task_id": taskID, "action": "deleted"},
	}, nil
}

func (t *TaskTool) moveIfNeeded(taskID uint, targetListID uint) (bool, error) {
	task, err := t.mgr.GetTask(taskID)
	if err != nil {
		return false, fmt.Errorf("task not found (id=%d): %v", taskID, err)
	}
	if task.TaskListID == targetListID {
		return false, nil
	}
	if _, err := t.mgr.MoveTaskToList(taskID, targetListID); err != nil {
		return false, err
	}
	return true, nil
}

func (t *TaskTool) validateStatusID(taskListID uint, statusID int) error {
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

func (t *TaskTool) updateTask(taskID uint, title, description, code, link string, statusID *int, assigneeName, assigneeID, creatorName, creatorID *string, moved bool) (tools.ToolResult, error) {
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

	if !fieldsChanged && !needsStatusChange && !moved {
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

	action := "updated"
	if moved {
		action = "moved"
	}
	resultJSON, _ := json.Marshal(t.taskResultMap(task, action))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task %s:\n%s", action, string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": action},
	}, nil
}

func (t *TaskTool) duplicateTask(taskListID uint, sourceID uint, title, description, code, link string, parentID *uint, statusID *int, assigneeName, assigneeID, creatorName, creatorID *string) (tools.ToolResult, error) {
	source, err := t.mgr.GetTask(sourceID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Source task not found (id=%d): %v", sourceID, err), IsError: true}, nil
	}

	if description == "" {
		description = source.Description
	}
	if link == "" {
		link = source.Link
	}

	effectiveListID := taskListID
	if effectiveListID == 0 {
		effectiveListID = source.TaskListID
	}

	aName := derefOr(assigneeName, source.AssigneeName)
	aID := derefOr(assigneeID, source.AssigneeID)
	cName := derefOr(creatorName, source.CreatorName)
	cID := derefOr(creatorID, source.CreatorID)

	task, err := t.mgr.CreateTaskFull(effectiveListID, title, description, code, link, aName, aID, cName, cID, parentID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error duplicating task: %v", err), IsError: true}, nil
	}

	if statusID != nil && *statusID != task.StatusID {
		if err := t.mgr.UpdateTaskStatus(task.ID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task duplicated (id=%d) but status change failed: %v", task.ID, err), IsError: true}, nil
		}
		task.StatusID = *statusID
	}

	resultJSON, _ := json.Marshal(t.taskResultMap(task, "duplicated"))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task duplicated (from id=%d):\n%s", sourceID, string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "source_task_id": sourceID, "action": "duplicated"},
	}, nil
}

func (t *TaskTool) createTask(taskListID uint, title, description, code, link string, parentID *uint, statusID *int, assigneeName, assigneeID, creatorName, creatorID *string) (tools.ToolResult, error) {
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

// ==================== Helpers ====================

func (t *TaskTool) taskResultMap(task *database.Task, action string) map[string]any {
	result := map[string]any{
		"id":           task.ID,
		"task_list_id": task.TaskListID,
		"title":        task.Title,
		"status_id":    task.StatusID,
		"action":       action,
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

func (t *TaskTool) emitAssigneeChangeNote(oldTask *database.Task, newAssigneeName string, taskID uint) {
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

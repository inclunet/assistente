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
	TaskListID   string  `json:"task_list_id,omitempty"`
	TaskListSlug string  `json:"task_list_slug,omitempty"`
	TaskID       *string `json:"task_id,omitempty"`
	Delete       bool    `json:"delete,omitempty"`
	Duplicate    bool    `json:"duplicate,omitempty"`
	Title        string  `json:"title,omitempty"`
	Description  string  `json:"description,omitempty"`
	Code         string  `json:"code,omitempty"`
	Link         string  `json:"link,omitempty"`
	StatusID     *int    `json:"status_id,omitempty"`
	ParentID     *string `json:"parent_task_id,omitempty"`
	AssigneeName *string `json:"assignee_name,omitempty"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
	CreatorName  *string `json:"creator_name,omitempty"`
	CreatorID    *string `json:"creator_id,omitempty"`
	// ConversationID vincula a task a uma conversa. nil = não altera; "" = limpa
	// o vínculo; valor = vincula. Use get_conversation_info para obter o id.
	ConversationID *string `json:"conversation_id,omitempty"`
}

type TaskTool struct {
	mgr TaskListManager
}

func NewTask(mgr TaskListManager) *TaskTool {
	return &TaskTool{mgr: mgr}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return `Full CRUD for tasks. Read/delete/duplicate source: task_id and/or (task_list_id/task_list_slug + code). With task_id and code only (no new title semantics): code must match that task. With list+code only: finds the task in that list. With task_id only, list ref is not used to locate the task — on update/duplicate, task_list_id/slug is the destination list for move/copy. Create: task_list_id and/or task_list_slug + title; optional code (dedup updates existing task with that code). Update by task_id + title: code field is the new task code, not for resolution. Duplicate by task_id: optional code sets the new copy's code. Use task_list for status IDs.`
}

func (t *TaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list. For create, use with task_list_slug or alone. When updating, with task_list_slug must match the same list; a different resolved list moves the task"
			},
			"task_list_slug": {
				"type": "string",
				"description": "Stable slug of the task list (lowercase). For create, use with task_list_id or alone. If both id and slug are sent, they must refer to the same list"
			},
			"task_id": {
				"type": "integer",
				"description": "Numeric task id. For read/delete/duplicate/update, use alone or with code and/or list ref for consistency checks, or omit when using task_list_id/slug + code to identify the task"
			},
			"delete": {
				"type": "boolean",
				"description": "Permanently deletes the task and subtasks. Identify the task with task_id and/or (task_list_id or task_list_slug + code). Cannot be combined with duplicate"
			},
			"duplicate": {
				"type": "boolean",
				"description": "Copies the task (code not copied). Identify source with task_id and/or list+code. Optional task_list_id/slug when source was identified by task_id — destination list for the copy"
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
				"description": "Task code within the list (ticket id). Optional on create; with task_list_id/slug can identify the task for read/update/delete/duplicate without task_id; with task_id must match that task"
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
			},
			"conversation_id": {
				"type": "string",
				"description": "Links this task to a conversation (1 conversation : N tasks). Use the id returned by get_conversation_info (e.g. the current chat) to bind the task to that conversation. Set to empty string to clear the link. Omit to leave unchanged"
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

	listIP := uintPtrIfPositive(params.TaskListID)
	tidPtr := taskIDPtrForResolve(params.TaskID)
	codeTrim := strings.TrimSpace(params.Code)

	isWrite := strings.TrimSpace(params.Title) != "" || params.Duplicate || params.StatusID != nil ||
		params.Description != "" || params.Link != "" ||
		params.AssigneeName != nil || params.AssigneeID != nil ||
		params.CreatorName != nil || params.CreatorID != nil || params.ParentID != nil ||
		params.ConversationID != nil

	if params.Delete {
		resolvedID, err := t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, tidPtr, params.Code)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		return t.deleteTask(ctx, resolvedID)
	}

	if !isWrite {
		resolvedID, err := t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, tidPtr, params.Code)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		return t.readTask(ctx, resolvedID)
	}

	// Caso especial: o único campo de escrita é conversation_id. Vincular/
	// desvincular uma task existente a uma conversa não deve exigir title
	// (diferente de create/update/duplicate). Resolve a task e aplica o vínculo.
	convOnly := params.ConversationID != nil &&
		strings.TrimSpace(params.Title) == "" && !params.Duplicate && params.StatusID == nil &&
		params.Description == "" && params.Link == "" &&
		params.AssigneeName == nil && params.AssigneeID == nil &&
		params.CreatorName == nil && params.CreatorID == nil && params.ParentID == nil
	if convOnly {
		resolvedID, err := t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, tidPtr, params.Code)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		if err := t.applyTaskConversation(ctx, resolvedID, params.ConversationID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error linking task %s to conversation: %v", resolvedID, err), IsError: true}, nil
		}
		return t.readTask(ctx, resolvedID)
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "title is required for create, update, and duplicate operations", IsError: true}, nil
	}

	if params.Duplicate {
		resolveCode := strings.TrimSpace(params.Code)
		if tidPtr != nil {
			resolveCode = ""
		}
		srcID, err := t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, tidPtr, resolveCode)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		tgt := ""
		if tidPtr != nil && (params.TaskListID != "" || strings.TrimSpace(params.TaskListSlug) != "") {
			ip := uintPtrIfPositive(params.TaskListID)
			targetListID, err := t.mgr.ResolveTaskListRef(ctx, ip, params.TaskListSlug)
			if err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
			tgt = targetListID
		}
		newTaskCode := ""
		if tidPtr != nil {
			newTaskCode = strings.TrimSpace(params.Code)
		}
		return t.duplicateTask(ctx, tgt, srcID, title, params.Description, newTaskCode, params.Link, params.ParentID, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, params.ConversationID)
	}

	if tidPtr != nil {
		// code no corpo é o novo valor do campo; não usar para resolver identidade quando já há task_id
		resolvedID, err := t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, tidPtr, "")
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}

		if params.StatusID != nil {
			listForStatus, err := t.listIDForStatusValidation(ctx, resolvedID, params, tidPtr)
			if err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
			if err := t.validateStatusID(ctx, listForStatus, *params.StatusID); err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
		}

		moved := false
		if params.TaskListID != "" || strings.TrimSpace(params.TaskListSlug) != "" {
			ip := uintPtrIfPositive(params.TaskListID)
			targetListID, err := t.mgr.ResolveTaskListRef(ctx, ip, params.TaskListSlug)
			if err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
			moved, err = t.moveIfNeeded(ctx, resolvedID, targetListID)
			if err != nil {
				return tools.ToolResult{Content: fmt.Sprintf("Error moving task %s to list %s: %v", resolvedID, targetListID, err), IsError: true}, nil
			}
		}
		return t.updateTask(ctx, resolvedID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, params.ConversationID, moved)
	}

	createListID, err := t.mgr.ResolveTaskListRef(ctx, listIP, params.TaskListSlug)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if params.StatusID != nil {
		if err := t.validateStatusID(ctx, createListID, *params.StatusID); err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
	}

	if codeTrim != "" {
		existing, err := t.mgr.FindTaskByCode(ctx, createListID, codeTrim)
		if err == nil && existing != nil {
			return t.updateTask(ctx, existing.ID, title, params.Description, params.Code, params.Link, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, params.ConversationID, false)
		}
	}

	return t.createTask(ctx, createListID, title, params.Description, params.Code, params.Link, params.ParentID, params.StatusID, params.AssigneeName, params.AssigneeID, params.CreatorName, params.CreatorID, params.ConversationID)
}

func (t *TaskTool) listIDForStatusValidation(ctx context.Context, resolvedTaskID string, p taskArgs, tidPtr *string) (string, error) {
	if tidPtr != nil && (p.TaskListID != "" || strings.TrimSpace(p.TaskListSlug) != "") {
		ip := uintPtrIfPositive(p.TaskListID)
		return t.mgr.ResolveTaskListRef(ctx, ip, p.TaskListSlug)
	}
	task, err := t.mgr.GetTask(ctx, resolvedTaskID)
	if err != nil {
		return "", err
	}
	return task.TaskListID, nil
}

// ==================== Read ====================

func (t *TaskTool) readTask(ctx context.Context, taskID string) (tools.ToolResult, error) {
	task, err := t.mgr.GetTask(ctx, taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%s): %v", taskID, err), IsError: true}, nil
	}

	notes, err := t.mgr.GetTaskNotes(ctx, taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error fetching notes for task %s: %v", taskID, err), IsError: true}, nil
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
	if task.ConversationID != nil && *task.ConversationID != "" {
		response["conversation_id"] = *task.ConversationID
	}

	if len(task.Subtasks) > 0 {
		subtaskIDs := make([]string, len(task.Subtasks))
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

func (t *TaskTool) deleteTask(ctx context.Context, taskID string) (tools.ToolResult, error) {
	task, err := t.mgr.GetTask(ctx, taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%s): %v", taskID, err), IsError: true}, nil
	}

	if err := t.mgr.DeleteTask(ctx, taskID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error deleting task %s: %v", taskID, err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content:  fmt.Sprintf("Task deleted: '%s' (id=%s)", task.Title, task.ID),
		Metadata: map[string]any{"task_id": taskID, "action": "deleted"},
	}, nil
}

func (t *TaskTool) moveIfNeeded(ctx context.Context, taskID string, targetListID string) (bool, error) {
	task, err := t.mgr.GetTask(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("task not found (id=%s): %v", taskID, err)
	}
	if task.TaskListID == targetListID {
		return false, nil
	}
	if _, err := t.mgr.MoveTaskToList(ctx, taskID, targetListID); err != nil {
		return false, err
	}
	return true, nil
}

func (t *TaskTool) validateStatusID(ctx context.Context, taskListID string, statusID int) error {
	workflow, err := t.mgr.GetWorkflow(ctx, taskListID)
	if err != nil {
		return fmt.Errorf("could not load workflow for task list %s: %v", taskListID, err)
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

func (t *TaskTool) updateTask(ctx context.Context, taskID string, title, description, code, link string, statusID *int, assigneeName, assigneeID, creatorName, creatorID, conversationID *string, moved bool) (tools.ToolResult, error) {
	oldTask, err := t.mgr.GetTask(ctx, taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%s): %v", taskID, err), IsError: true}, nil
	}

	aName := derefOrKeep(assigneeName, oldTask, func(t *database.Task) string { return t.AssigneeName })
	aID := derefOrKeep(assigneeID, oldTask, func(t *database.Task) string { return t.AssigneeID })
	cName := derefOrKeep(creatorName, oldTask, func(t *database.Task) string { return t.CreatorName })
	cID := derefOrKeep(creatorID, oldTask, func(t *database.Task) string { return t.CreatorID })

	needsStatusChange := statusID != nil && *statusID != oldTask.StatusID
	curConv := ""
	if oldTask.ConversationID != nil {
		curConv = *oldTask.ConversationID
	}
	convChanged := conversationID != nil && strings.TrimSpace(*conversationID) != curConv

	fieldsChanged := title != oldTask.Title ||
		description != oldTask.Description ||
		code != oldTask.Code ||
		link != oldTask.Link ||
		aName != oldTask.AssigneeName ||
		aID != oldTask.AssigneeID ||
		cName != oldTask.CreatorName ||
		cID != oldTask.CreatorID

	if !fieldsChanged && !needsStatusChange && !moved && !convChanged {
		resultJSON, _ := json.Marshal(t.taskResultMap(oldTask, "noop"))
		return tools.ToolResult{
			Content:  fmt.Sprintf("Task unchanged:\n%s", string(resultJSON)),
			Metadata: map[string]any{"task_id": oldTask.ID, "action": "noop"},
		}, nil
	}

	if fieldsChanged {
		if err := t.mgr.UpdateTaskFull(ctx, taskID, title, description, code, link, aName, aID, cName, cID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error updating task %s: %v", taskID, err), IsError: true}, nil
		}
		t.emitAssigneeChangeNote(ctx, oldTask, aName, taskID)
	}

	if convChanged {
		if err := t.applyTaskConversation(ctx, taskID, conversationID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task updated but conversation link failed: %v", err), IsError: true}, nil
		}
	}

	if needsStatusChange {
		if err := t.mgr.UpdateTaskStatus(ctx, taskID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task updated but status change failed: %v", err), IsError: true}, nil
		}
	}

	task, err := t.mgr.GetTask(ctx, taskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task updated (id=%s) but could not fetch result: %v", taskID, err)}, nil
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

func (t *TaskTool) duplicateTask(ctx context.Context, taskListID string, sourceID string, title, description, code, link string, parentID *string, statusID *int, assigneeName, assigneeID, creatorName, creatorID, conversationID *string) (tools.ToolResult, error) {
	source, err := t.mgr.GetTask(ctx, sourceID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Source task not found (id=%s): %v", sourceID, err), IsError: true}, nil
	}

	if description == "" {
		description = source.Description
	}
	if link == "" {
		link = source.Link
	}

	effectiveListID := taskListID
	if effectiveListID == "" {
		effectiveListID = source.TaskListID
	}

	aName := derefOr(assigneeName, source.AssigneeName)
	aID := derefOr(assigneeID, source.AssigneeID)
	cName := derefOr(creatorName, source.CreatorName)
	cID := derefOr(creatorID, source.CreatorID)

	task, err := t.mgr.CreateTaskFull(ctx, effectiveListID, title, description, code, link, aName, aID, cName, cID, parentID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error duplicating task: %v", err), IsError: true}, nil
	}

	// Por padrão a cópia herda a conversa da origem (como description/link);
	// um conversation_id explícito sobrescreve (inclusive "" para não vincular).
	effectiveConv := conversationID
	if effectiveConv == nil {
		effectiveConv = source.ConversationID
	}
	if err := t.applyTaskConversation(ctx, task.ID, effectiveConv); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task duplicated (id=%s) but conversation link failed: %v", task.ID, err), IsError: true}, nil
	}

	if statusID != nil && *statusID != task.StatusID {
		if err := t.mgr.UpdateTaskStatus(ctx, task.ID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task duplicated (id=%s) but status change failed: %v", task.ID, err), IsError: true}, nil
		}
		task.StatusID = *statusID
	}

	if updated, e := t.mgr.GetTask(ctx, task.ID); e == nil && updated != nil {
		task = updated
	}
	resultJSON, _ := json.Marshal(t.taskResultMap(task, "duplicated"))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task duplicated (from id=%s):\n%s", sourceID, string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "source_task_id": sourceID, "action": "duplicated"},
	}, nil
}

func (t *TaskTool) createTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string, statusID *int, assigneeName, assigneeID, creatorName, creatorID, conversationID *string) (tools.ToolResult, error) {
	aName := derefOr(assigneeName, "")
	aID := derefOr(assigneeID, "")
	cName := derefOr(creatorName, "")
	cID := derefOr(creatorID, "")

	var task *database.Task
	var err error
	if aName != "" || aID != "" || cName != "" || cID != "" {
		task, err = t.mgr.CreateTaskFull(ctx, taskListID, title, description, code, link, aName, aID, cName, cID, parentID)
	} else {
		task, err = t.mgr.CreateTask(ctx, taskListID, title, description, code, link, parentID)
	}
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task: %v", err), IsError: true}, nil
	}

	if err := t.applyTaskConversation(ctx, task.ID, conversationID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task created (id=%s) but conversation link failed: %v", task.ID, err), IsError: true}, nil
	}

	if statusID != nil && *statusID != task.StatusID {
		if err := t.mgr.UpdateTaskStatus(ctx, task.ID, *statusID); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task created (id=%s) but status change failed: %v", task.ID, err), IsError: true}, nil
		}
		task.StatusID = *statusID
	}

	if updated, e := t.mgr.GetTask(ctx, task.ID); e == nil && updated != nil {
		task = updated
	}
	resultJSON, _ := json.Marshal(t.taskResultMap(task, "created"))
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task created:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_id": task.ID, "action": "created"},
	}, nil
}

// ==================== Helpers ====================

// applyTaskConversation aplica o vínculo com conversa quando conversation_id foi
// enviado. nil = não altera; "" = limpa o vínculo (NULL); valor = vincula.
func (t *TaskTool) applyTaskConversation(ctx context.Context, taskID string, conversationID *string) error {
	if conversationID == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*conversationID)
	var ptr *string
	if trimmed != "" {
		ptr = &trimmed
	}
	return t.mgr.SetTaskConversation(ctx, taskID, ptr)
}

func (t *TaskTool) taskResultMap(task *database.Task, action string) map[string]any {
	result := map[string]any{
		"id":           task.ID,
		"task_list_id": task.TaskListID,
		"title":        task.Title,
		"status_id":    task.StatusID,
		"action":       action,
	}
	if task.ConversationID != nil && *task.ConversationID != "" {
		result["conversation_id"] = *task.ConversationID
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

func uintPtrIfPositive(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func taskIDPtrForResolve(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	v := *p
	return &v
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

func (t *TaskTool) emitAssigneeChangeNote(ctx context.Context, oldTask *database.Task, newAssigneeName string, taskID string) {
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

	_, _ = t.mgr.CreateTaskNote(ctx, taskID, database.TaskNoteSystem, content, "system", "")
}

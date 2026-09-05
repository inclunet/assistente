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

type taskNoteArgs struct {
	TaskListID        string  `json:"task_list_id,omitempty"`
	TaskListSlug      string  `json:"task_list_slug,omitempty"`
	TaskID            *string `json:"task_id,omitempty"`
	TaskCode          string  `json:"task_code,omitempty"`
	Code              string  `json:"code,omitempty"`
	NoteID            *string `json:"note_id,omitempty"`
	Type              *int    `json:"type,omitempty"`
	Content           string  `json:"content"`
	AuthorName        string  `json:"author_name,omitempty"`
	AuthorID          string  `json:"author_id,omitempty"`
	Source            string  `json:"source,omitempty"`
	ExternalID        string  `json:"external_id,omitempty"`
	ExternalParentID  string  `json:"external_parent_id,omitempty"`
	ExternalUpdatedAt string  `json:"external_updated_at,omitempty"`
}

type TaskNoteTool struct {
	mgr TaskListManager
}

func NewTaskNote(mgr TaskListManager) *TaskNoteTool {
	return &TaskNoteTool{mgr: mgr}
}

func (t *TaskNoteTool) Name() string { return "task_note" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *TaskNoteTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"}
}

func (t *TaskNoteTool) Description() string {
	return `Create or update a persistent note/comment attached to one task; this tool does not change the task's core fields or delete notes.
Use when: recording internal context, a customer response, an agent action, a system event, or synchronizing an external comment. For sync, source + external_id performs an idempotent upsert.
Do not use: use task for title, status, assignee, hierarchy, or deletion; task_list for the container/workflow; memory only for durable knowledge that should outlive task tracking.
Persistence, risk, and cost: notes persist in the database and note events may trigger jobs. Avoid secrets and redundant transcripts; long notes increase later task-read output. Updating by note_id replaces its content.
Resolution: task_id wins; task_code resolves Task.Code across lists and can be scoped by task_list_id/slug; list + code follows task resolution. note_id updates an existing note. Manual create requires type: 1 internal, 2 customer, 3 agent, 4 system.
Examples: add {"task_id":"<id>","type":1,"content":"Waiting for approval"}; sync {"task_code":"FSD-123","source":"jira","external_id":"comment-9","type":2,"content":"Approved"}.`
}

func (t *TaskNoteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "Task list id; use with task_list_slug and/or code to identify the task without task_id"
			},
			"task_list_slug": {
				"type": "string",
				"description": "Task list slug; with code identifies the task when task_id is omitted"
			},
			"task_id": {
				"type": "integer",
				"description": "Numeric task id. Optional if task_code or task_list_id/slug + code identify the task. Takes precedence over task_code when both are set"
			},
			"task_code": {
				"type": "string",
				"description": "Task.Code (e.g. FSD-12345) to find the task without numeric task_id. Optional task_list_id or task_list_slug disambiguates if the same code exists in multiple lists"
			},
			"code": {
				"type": "string",
				"description": "Task code within a specific list when resolving with task_list_id/slug (not the same field as task_code)"
			},
			"note_id": {
				"type": "integer",
				"description": "ID of an existing note to update. Omit to create or upsert"
			},
			"type": {
				"type": "integer",
				"description": "Note type: 1=internal, 2=customer, 3=agent, 4=system. Required for manual create; required on first external upsert when the note does not exist yet",
				"enum": [1, 2, 3, 4]
			},
			"content": {
				"type": "string",
				"description": "Note content (supports markdown)"
			},
			"author_name": {
				"type": "string",
				"description": "Display name of the note author (optional)"
			},
			"author_id": {
				"type": "string",
				"description": "Stable identifier for the author (optional)"
			},
			"source": {
				"type": "string",
				"description": "External system key for idempotent sync (e.g. jira). Use together with external_id"
			},
			"external_id": {
				"type": "string",
				"description": "Stable remote identifier (e.g. Jira comment id). Use together with source"
			},
			"external_parent_id": {
				"type": "string",
				"description": "Optional remote parent reference (e.g. issue key FSD-123)"
			},
			"external_updated_at": {
				"type": "string",
				"description": "Optional RFC3339 timestamp of last remote update"
			}
		},
		"required": ["content"],
		"additionalProperties": false
	}`)
}

// resolveTaskID resolves which task the note targets. Priority: task_id (with optional consistency checks) > task_code > task_list + code.
func (t *TaskNoteTool) resolveTaskID(ctx context.Context, params taskNoteArgs) (string, error) {
	listIP := uintPtrIfPositive(params.TaskListID)
	tidPtr := taskIDPtrForResolve(params.TaskID)
	taskCodeTrim := strings.TrimSpace(params.TaskCode)
	codeTrim := strings.TrimSpace(params.Code)

	if tidPtr != nil {
		task, err := t.mgr.GetTask(ctx, *tidPtr)
		if err != nil {
			return "", fmt.Errorf("task_id %s não encontrado: %w", *tidPtr, err)
		}
		if taskCodeTrim != "" && strings.TrimSpace(task.Code) != taskCodeTrim {
			return "", fmt.Errorf("task_id %s tem task_code %q, que não coincide com %q", task.ID, task.Code, taskCodeTrim)
		}
		if codeTrim != "" && task.Code != codeTrim {
			return "", fmt.Errorf("task_id %s e code %q não correspondem à mesma task", *tidPtr, codeTrim)
		}
		hasListRef := listIP != nil || strings.TrimSpace(params.TaskListSlug) != ""
		if hasListRef {
			listID, err := t.mgr.ResolveTaskListRef(ctx, listIP, params.TaskListSlug)
			if err != nil {
				return "", err
			}
			if task.TaskListID != listID {
				return "", fmt.Errorf("task_id %s e lista referenciada não correspondem à mesma task", *tidPtr)
			}
		}
		return task.ID, nil
	}

	if taskCodeTrim != "" {
		var scope *string
		if listIP != nil || strings.TrimSpace(params.TaskListSlug) != "" {
			lid, err := t.mgr.ResolveTaskListRef(ctx, listIP, params.TaskListSlug)
			if err != nil {
				return "", err
			}
			scope = &lid
		}
		return t.mgr.ResolveTaskIDByTaskCode(ctx, scope, taskCodeTrim)
	}

	return t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, nil, codeTrim)
}

func (t *TaskNoteTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params taskNoteArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	content := strings.TrimSpace(params.Content)
	if content == "" {
		return tools.ToolResult{Content: "content cannot be empty", IsError: true}, nil
	}

	listIP := uintPtrIfPositive(params.TaskListID)
	tidPtr := taskIDPtrForResolve(params.TaskID)
	taskCodeTrim := strings.TrimSpace(params.TaskCode)
	codeTrim := strings.TrimSpace(params.Code)

	if params.NoteID != nil {
		note, err := t.mgr.GetTaskNote(ctx, *params.NoteID)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Note not found (id=%s): %v", *params.NoteID, err), IsError: true}, nil
		}
		var taskID string
		if tidPtr != nil || taskCodeTrim != "" || codeTrim != "" || listIP != nil || strings.TrimSpace(params.TaskListSlug) != "" {
			resolved, err := t.resolveTaskID(ctx, params)
			if err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
			if resolved != note.TaskID {
				return tools.ToolResult{Content: fmt.Sprintf("note_id %s belongs to task %s, which does not match the resolved task %s", *params.NoteID, note.TaskID, resolved), IsError: true}, nil
			}
			taskID = resolved
		} else {
			taskID = note.TaskID
		}
		return t.updateNote(ctx, *params.NoteID, taskID, content)
	}

	resolvedTaskID, err := t.resolveTaskID(ctx, params)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	src := strings.TrimSpace(params.Source)
	extID := strings.TrimSpace(params.ExternalID)
	if src != "" || extID != "" {
		if src == "" || extID == "" {
			return tools.ToolResult{Content: "for external idempotent upsert, both source and external_id are required", IsError: true}, nil
		}
		return t.upsertExternalNote(ctx, resolvedTaskID, content, params.AuthorName, params.AuthorID, src, extID, strings.TrimSpace(params.ExternalParentID), params.ExternalUpdatedAt, params.Type)
	}

	if params.Type == nil {
		return tools.ToolResult{Content: "type is required when creating a new note without external source + external_id", IsError: true}, nil
	}
	return t.createNote(ctx, resolvedTaskID, *params.Type, content, params.AuthorName, params.AuthorID)
}

func (t *TaskNoteTool) upsertExternalNote(ctx context.Context, taskID string, content, authorName, authorID, source, externalID, externalParentID, externalUpdatedAtRaw string, typeArg *int) (tools.ToolResult, error) {
	var extTime *time.Time
	if strings.TrimSpace(externalUpdatedAtRaw) != "" {
		ts, err := parseExternalUpdatedAt(externalUpdatedAtRaw)
		if err != nil {
			return tools.ToolResult{Content: "invalid external_updated_at: " + err.Error(), IsError: true}, nil
		}
		extTime = ts
	}

	var typePtr *database.TaskNoteType
	if typeArg != nil {
		nt := int(*typeArg)
		if nt < 1 || nt > 4 {
			return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
		}
		tv := database.TaskNoteType(nt)
		typePtr = &tv
	}

	note, created, err := t.mgr.UpsertTaskNoteByExternal(ctx, database.UpsertTaskNoteByExternalParams{
		TaskID:            taskID,
		Type:              typePtr,
		Content:           content,
		AuthorName:        authorName,
		AuthorID:          authorID,
		ExternalSource:    source,
		ExternalID:        externalID,
		ExternalParentID:  externalParentID,
		ExternalUpdatedAt: extTime,
	})
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error upserting external note on task %s: %v", taskID, err), IsError: true}, nil
	}

	action := "updated"
	if created {
		action = "created"
	}
	typeLabels := map[int]string{1: "internal note", 2: "customer response", 3: "agent action", 4: "system event"}
	typeLabel := typeLabels[int(note.Type)]
	if typeLabel == "" {
		typeLabel = fmt.Sprintf("type %d", note.Type)
	}

	resultMap := map[string]any{
		"id":          note.ID,
		"task_id":     note.TaskID,
		"type":        typeLabel,
		"action":      action,
		"source":      source,
		"external_id": externalID,
	}
	if note.AuthorName != "" {
		resultMap["author_name"] = note.AuthorName
	}
	if note.AuthorID != "" {
		resultMap["author_id"] = note.AuthorID
	}
	if note.ExternalParentID != "" {
		resultMap["external_parent_id"] = note.ExternalParentID
	}
	if note.ExternalUpdatedAt != nil {
		resultMap["external_updated_at"] = note.ExternalUpdatedAt.Format(time.RFC3339)
	}

	resultJSON, _ := json.Marshal(resultMap)
	md := map[string]any{"note_id": note.ID, "task_id": taskID, "action": action, "source": source, "external_id": externalID}
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note %s on task %s (%s):\n%s", action, taskID, typeLabel, string(resultJSON)),
		Metadata: md,
	}, nil
}

func parseExternalUpdatedAt(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999-0700", // Jira (ISO-8601, offset ±HHMM sem dois-pontos), fração opcional
		"2006-01-02T15:04:05-0700",           // Jira sem fração
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		} else {
			lastErr = err
		}
	}
	return nil, fmt.Errorf("could not parse timestamp: %v", lastErr)
}

func (t *TaskNoteTool) createNote(ctx context.Context, taskID string, noteType int, content, authorName, authorID string) (tools.ToolResult, error) {
	if noteType < 1 || noteType > 4 {
		return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
	}

	authorName = strings.TrimSpace(authorName)
	authorID = strings.TrimSpace(authorID)

	note, err := t.mgr.CreateTaskNote(ctx, taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating note on task %s: %v", taskID, err), IsError: true}, nil
	}

	typeLabels := map[int]string{1: "internal note", 2: "customer response", 3: "agent action", 4: "system event"}

	resultMap := map[string]any{
		"id":      note.ID,
		"task_id": note.TaskID,
		"type":    typeLabels[noteType],
		"action":  "created",
	}
	if note.AuthorName != "" {
		resultMap["author_name"] = note.AuthorName
	}
	if note.AuthorID != "" {
		resultMap["author_id"] = note.AuthorID
	}
	resultJSON, _ := json.Marshal(resultMap)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note added to task %s (%s):\n%s", taskID, typeLabels[noteType], string(resultJSON)),
		Metadata: map[string]any{"note_id": note.ID, "task_id": taskID, "action": "created"},
	}, nil
}

func (t *TaskNoteTool) updateNote(ctx context.Context, noteID string, taskID string, content string) (tools.ToolResult, error) {
	existing, err := t.mgr.GetTaskNote(ctx, noteID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Note not found (id=%s): %v", noteID, err), IsError: true}, nil
	}

	if existing.TaskID != taskID {
		return tools.ToolResult{Content: fmt.Sprintf("Note %s does not belong to task %s", noteID, taskID), IsError: true}, nil
	}

	if err := t.mgr.UpdateTaskNote(ctx, noteID, content); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating note %s: %v", noteID, err), IsError: true}, nil
	}

	resultMap := map[string]any{
		"id":      noteID,
		"task_id": taskID,
		"action":  "updated",
	}
	resultJSON, _ := json.Marshal(resultMap)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note updated:\n%s", string(resultJSON)),
		Metadata: map[string]any{"note_id": noteID, "task_id": taskID, "action": "updated"},
	}, nil
}

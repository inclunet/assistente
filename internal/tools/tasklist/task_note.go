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
	TaskID             uint   `json:"task_id"`
	NoteID             *uint  `json:"note_id,omitempty"`
	Type               *int   `json:"type,omitempty"`
	Content            string `json:"content"`
	AuthorName         string `json:"author_name,omitempty"`
	AuthorID           string `json:"author_id,omitempty"`
	Source             string `json:"source,omitempty"`
	ExternalID         string `json:"external_id,omitempty"`
	ExternalParentID   string `json:"external_parent_id,omitempty"`
	ExternalUpdatedAt  string `json:"external_updated_at,omitempty"`
}

type TaskNoteTool struct {
	mgr TaskListManager
}

func NewTaskNote(mgr TaskListManager) *TaskNoteTool {
	return &TaskNoteTool{mgr: mgr}
}

func (t *TaskNoteTool) Name() string { return "task_note" }

func (t *TaskNoteTool) Description() string {
	return "Creates or updates a note on a task. Modes: (1) note_id → update content for that note. (2) source + external_id + task_id → idempotent upsert for synced comments (Jira, etc.): creates once, then updates on repeat. (3) otherwise create manual note (requires type). Use task tool with task_id to read notes. Types: 1=internal, 2=customer, 3=agent, 4=system."
}

func (t *TaskNoteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "integer",
				"description": "ID of the task this note belongs to"
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
		"required": ["task_id", "content"],
		"additionalProperties": false
	}`)
}

func (t *TaskNoteTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params taskNoteArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskID == 0 {
		return tools.ToolResult{Content: "task_id is required and must be > 0", IsError: true}, nil
	}

	content := strings.TrimSpace(params.Content)
	if content == "" {
		return tools.ToolResult{Content: "content cannot be empty", IsError: true}, nil
	}

	if params.NoteID != nil {
		return t.updateNote(*params.NoteID, params.TaskID, content)
	}

	src := strings.TrimSpace(params.Source)
	extID := strings.TrimSpace(params.ExternalID)
	if src != "" || extID != "" {
		if src == "" || extID == "" {
			return tools.ToolResult{Content: "for external idempotent upsert, both source and external_id are required", IsError: true}, nil
		}
		return t.upsertExternalNote(params.TaskID, content, params.AuthorName, params.AuthorID, src, extID, strings.TrimSpace(params.ExternalParentID), params.ExternalUpdatedAt, params.Type)
	}

	if params.Type == nil {
		return tools.ToolResult{Content: "type is required when creating a new note without external source + external_id", IsError: true}, nil
	}
	return t.createNote(params.TaskID, *params.Type, content, params.AuthorName, params.AuthorID)
}

func (t *TaskNoteTool) upsertExternalNote(taskID uint, content, authorName, authorID, source, externalID, externalParentID, externalUpdatedAtRaw string, typeArg *int) (tools.ToolResult, error) {
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

	note, created, err := t.mgr.UpsertTaskNoteByExternal(database.UpsertTaskNoteByExternalParams{
		TaskID:             taskID,
		Type:               typePtr,
		Content:            content,
		AuthorName:         authorName,
		AuthorID:           authorID,
		ExternalSource:     source,
		ExternalID:         externalID,
		ExternalParentID:   externalParentID,
		ExternalUpdatedAt:  extTime,
	})
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error upserting external note on task %d: %v", taskID, err), IsError: true}, nil
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
		"id":           note.ID,
		"task_id":      note.TaskID,
		"type":         typeLabel,
		"action":       action,
		"source":       source,
		"external_id":  externalID,
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
		Content:  fmt.Sprintf("Note %s on task %d (%s):\n%s", action, taskID, typeLabel, string(resultJSON)),
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

func (t *TaskNoteTool) createNote(taskID uint, noteType int, content, authorName, authorID string) (tools.ToolResult, error) {
	if noteType < 1 || noteType > 4 {
		return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
	}

	authorName = strings.TrimSpace(authorName)
	authorID = strings.TrimSpace(authorID)

	note, err := t.mgr.CreateTaskNote(taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating note on task %d: %v", taskID, err), IsError: true}, nil
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
		Content:  fmt.Sprintf("Note added to task %d (%s):\n%s", taskID, typeLabels[noteType], string(resultJSON)),
		Metadata: map[string]any{"note_id": note.ID, "task_id": taskID, "action": "created"},
	}, nil
}

func (t *TaskNoteTool) updateNote(noteID uint, taskID uint, content string) (tools.ToolResult, error) {
	existing, err := t.mgr.GetTaskNote(noteID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Note not found (id=%d): %v", noteID, err), IsError: true}, nil
	}

	if existing.TaskID != taskID {
		return tools.ToolResult{Content: fmt.Sprintf("Note %d does not belong to task %d", noteID, taskID), IsError: true}, nil
	}

	if err := t.mgr.UpdateTaskNote(noteID, content); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating note %d: %v", noteID, err), IsError: true}, nil
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

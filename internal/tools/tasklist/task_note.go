package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type taskNoteArgs struct {
	TaskID     uint   `json:"task_id"`
	NoteID     *uint  `json:"note_id,omitempty"`
	Type       *int   `json:"type,omitempty"`
	Content    string `json:"content"`
	AuthorName string `json:"author_name,omitempty"`
	AuthorID   string `json:"author_id,omitempty"`
}

type TaskNoteTool struct {
	mgr TaskListManager
}

func NewTaskNote(mgr TaskListManager) *TaskNoteTool {
	return &TaskNoteTool{mgr: mgr}
}

func (t *TaskNoteTool) Name() string { return "task_note" }

func (t *TaskNoteTool) Description() string {
	return "Creates or updates a note on a task. Without note_id → creates a new note (requires type). With note_id → updates the note content. Use task tool with task_id to read existing notes. Note types: 1=internal, 2=customer response, 3=agent action, 4=system event."
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
				"description": "ID of an existing note to update. Omit to create a new note"
			},
			"type": {
				"type": "integer",
				"description": "Note type: 1=internal note, 2=customer response, 3=agent action, 4=system event. Required when creating a new note",
				"enum": [1, 2, 3, 4]
			},
			"content": {
				"type": "string",
				"description": "Note content (supports markdown)"
			},
			"author_name": {
				"type": "string",
				"description": "Display name of the note author (optional, for creating)"
			},
			"author_id": {
				"type": "string",
				"description": "Stable identifier for the author (optional, for creating)"
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

	if params.Type == nil {
		return tools.ToolResult{Content: "type is required when creating a new note", IsError: true}, nil
	}
	return t.createNote(params.TaskID, *params.Type, content, params.AuthorName, params.AuthorID)
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
		Metadata: map[string]any{"note_id": note.ID, "task_id": note.TaskID, "action": "created"},
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

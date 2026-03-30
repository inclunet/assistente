package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type getTaskNotesArgs struct {
	TaskID uint `json:"task_id"`
}

type GetTaskNotesTool struct {
	mgr TaskListManager
}

func NewGetTaskNotes(mgr TaskListManager) *GetTaskNotesTool {
	return &GetTaskNotesTool{mgr: mgr}
}

func (t *GetTaskNotesTool) Name() string { return "get_task_notes" }

func (t *GetTaskNotesTool) Description() string {
	return "Returns all notes and interactions for a specific task, ordered chronologically. Each note has a type (internal/customer/agent/system), content, author, and timestamp."
}

func (t *GetTaskNotesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "integer",
				"description": "ID of the task to get notes for"
			}
		},
		"required": ["task_id"],
		"additionalProperties": false
	}`)
}

func (t *GetTaskNotesTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params getTaskNotesArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskID == 0 {
		return tools.ToolResult{Content: "task_id is required and must be > 0", IsError: true}, nil
	}

	// Verify task exists
	task, err := t.mgr.GetTask(params.TaskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%d): %v", params.TaskID, err), IsError: true}, nil
	}

	notes, err := t.mgr.GetTaskNotes(params.TaskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error fetching notes for task %d: %v", params.TaskID, err), IsError: true}, nil
	}

	if len(notes) == 0 {
		return tools.ToolResult{
			Content:  fmt.Sprintf("No notes found for task '%s' (id=%d)", task.Title, task.ID),
			Metadata: map[string]any{"task_id": params.TaskID, "count": 0},
		}, nil
	}

	typeLabels := map[int]string{1: "internal", 2: "customer", 3: "agent", 4: "system"}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task '%s' (id=%d) — %d note(s):\n\n", task.Title, task.ID, len(notes)))

	for i, note := range notes {
		typeLabel := typeLabels[int(note.Type)]
		if typeLabel == "" {
			typeLabel = "unknown"
		}
		sb.WriteString(fmt.Sprintf("--- Note %d [%s] ---\n", i+1, typeLabel))
		if note.AuthorName != "" {
			sb.WriteString(fmt.Sprintf("Author: %s\n", note.AuthorName))
		}
		sb.WriteString(fmt.Sprintf("Date: %s\n", note.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString(note.Content)
		sb.WriteString("\n\n")
	}

	return tools.ToolResult{
		Content:  sb.String(),
		Metadata: map[string]any{"task_id": params.TaskID, "count": len(notes)},
	}, nil
}

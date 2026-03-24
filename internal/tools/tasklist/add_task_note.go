package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type addTaskNoteArgs struct {
	TaskID  uint   `json:"task_id"`
	Type    int    `json:"type"`
	Content string `json:"content"`
	Author  string `json:"author,omitempty"`
}

type AddTaskNoteTool struct {
	mgr TaskListManager
}

func NewAddTaskNote(mgr TaskListManager) *AddTaskNoteTool {
	return &AddTaskNoteTool{mgr: mgr}
}

func (t *AddTaskNoteTool) Name() string { return "add_task_note" }

func (t *AddTaskNoteTool) Description() string {
	return "Adds a note or interaction to a task. Use this to record internal notes, customer responses, agent actions, or system events on a task."
}

func (t *AddTaskNoteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "integer",
				"description": "ID of the task to add a note to"
			},
			"type": {
				"type": "integer",
				"description": "Note type: 1=internal note, 2=customer response, 3=agent action, 4=system event",
				"enum": [1, 2, 3, 4]
			},
			"content": {
				"type": "string",
				"description": "Note content (supports markdown)"
			},
			"author": {
				"type": "string",
				"description": "Author name or identifier (optional)"
			}
		},
		"required": ["task_id", "type", "content"],
		"additionalProperties": false
	}`)
}

func (t *AddTaskNoteTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params addTaskNoteArgs
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

	if params.Type < 1 || params.Type > 4 {
		return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
	}

	note, err := t.mgr.CreateTaskNote(params.TaskID, database.TaskNoteType(params.Type), content, strings.TrimSpace(params.Author))
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating note on task %d: %v", params.TaskID, err), IsError: true}, nil
	}

	typeLabels := map[int]string{1: "internal note", 2: "customer response", 3: "agent action", 4: "system event"}

	resultJSON, _ := json.Marshal(map[string]any{
		"id":      note.ID,
		"task_id": note.TaskID,
		"type":    typeLabels[params.Type],
		"author":  note.Author,
		"action":  "created",
	})
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note added to task %d (%s):\n%s", params.TaskID, typeLabels[params.Type], string(resultJSON)),
		Metadata: map[string]any{"note_id": note.ID, "task_id": note.TaskID, "action": "created"},
	}, nil
}

package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/tools"
)

type deleteTaskArgs struct {
	TaskID uint `json:"task_id"`
}

type DeleteTaskTool struct {
	mgr TaskListManager
}

func NewDeleteTask(mgr TaskListManager) *DeleteTaskTool {
	return &DeleteTaskTool{mgr: mgr}
}

func (t *DeleteTaskTool) Name() string { return "delete_task" }

func (t *DeleteTaskTool) Description() string {
	return "Deletes a task and all its subtasks permanently."
}

func (t *DeleteTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "integer",
				"description": "ID of the task to delete"
			}
		},
		"required": ["task_id"],
		"additionalProperties": false
	}`)
}

func (t *DeleteTaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params deleteTaskArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskID == 0 {
		return tools.ToolResult{Content: "task_id is required and must be > 0", IsError: true}, nil
	}

	// Verify task exists before deleting
	task, err := t.mgr.GetTask(params.TaskID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task not found (id=%d): %v", params.TaskID, err), IsError: true}, nil
	}

	if err := t.mgr.DeleteTask(params.TaskID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error deleting task %d: %v", params.TaskID, err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content:  fmt.Sprintf("Task deleted: '%s' (id=%d)", task.Title, task.ID),
		Metadata: map[string]any{"task_id": params.TaskID, "deleted": true},
	}, nil
}

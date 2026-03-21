package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type bulkTaskItem struct {
	TaskListID  uint   `json:"task_list_id"`
	TaskID      *uint  `json:"task_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StatusID    *int   `json:"status_id,omitempty"`
	ParentID    *uint  `json:"parent_task_id,omitempty"`
}

type bulkUpsertTasksArgs struct {
	Tasks []bulkTaskItem `json:"tasks"`
}

type BulkUpsertTasksTool struct {
	mgr TaskListManager
}

func NewBulkUpsertTasks(mgr TaskListManager) *BulkUpsertTasksTool {
	return &BulkUpsertTasksTool{mgr: mgr}
}

func (t *BulkUpsertTasksTool) Name() string { return "bulk_upsert_tasks" }

func (t *BulkUpsertTasksTool) Description() string {
	return "Creates or updates multiple tasks in a single call. Each item in the tasks array follows the same rules as upsert_task. Useful for initializing a task list with multiple tasks at once."
}

func (t *BulkUpsertTasksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tasks": {
				"type": "array",
				"description": "Array of tasks to create or update",
				"items": {
					"type": "object",
					"properties": {
						"task_list_id": {
							"type": "integer",
							"description": "ID of the task list"
						},
						"task_id": {
							"type": "integer",
							"description": "ID of existing task to update. Omit to create"
						},
						"title": {
							"type": "string",
							"description": "Task title"
						},
						"description": {
							"type": "string",
							"description": "Task description"
						},
						"status_id": {
							"type": "integer",
							"description": "Workflow status ID"
						},
						"parent_task_id": {
							"type": "integer",
							"description": "Parent task ID for subtasks"
						}
					},
					"required": ["task_list_id", "title"]
				}
			}
		},
		"required": ["tasks"],
		"additionalProperties": false
	}`)
}

func (t *BulkUpsertTasksTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params bulkUpsertTasksArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if len(params.Tasks) == 0 {
		return tools.ToolResult{Content: "tasks array cannot be empty", IsError: true}, nil
	}

	upsertTool := &UpsertTaskTool{mgr: t.mgr}

	successCount := 0
	var errors []string

	for i, item := range params.Tasks {
		taskArgs := upsertTaskArgs{
			TaskListID:  item.TaskListID,
			TaskID:      item.TaskID,
			Title:       item.Title,
			Description: item.Description,
			StatusID:    item.StatusID,
			ParentID:    item.ParentID,
		}
		argsJSON, _ := json.Marshal(taskArgs)

		result, err := upsertTool.Execute(ctx, argsJSON)
		if err != nil {
			errors = append(errors, fmt.Sprintf("task[%d]: internal error: %v", i, err))
			continue
		}
		if result.IsError {
			errors = append(errors, fmt.Sprintf("task[%d] '%s': %s", i, item.Title, result.Content))
			continue
		}
		successCount++
	}

	response := map[string]any{
		"success_count": successCount,
		"failed_count":  len(errors),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	resultJSON, _ := json.Marshal(response)

	content := fmt.Sprintf("Bulk upsert complete: %d succeeded, %d failed", successCount, len(errors))
	if len(errors) > 0 {
		content += "\nErrors:\n- " + strings.Join(errors, "\n- ")
	}

	return tools.ToolResult{
		Content:  content,
		Metadata: map[string]any{"success_count": successCount, "failed_count": len(errors), "details": string(resultJSON)},
	}, nil
}

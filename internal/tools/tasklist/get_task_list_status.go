package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/tools"
)

type getTaskListStatusArgs struct {
	TaskListID uint `json:"task_list_id"`
}

type GetTaskListStatusTool struct {
	mgr TaskListManager
}

func NewGetTaskListStatus(mgr TaskListManager) *GetTaskListStatusTool {
	return &GetTaskListStatusTool{mgr: mgr}
}

func (t *GetTaskListStatusTool) Name() string { return "get_task_list_status" }

func (t *GetTaskListStatusTool) Description() string {
	return "Returns a summary of task counts per status for a task list, useful for quick progress checks without fetching all task details."
}

func (t *GetTaskListStatusTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list to get status for"
			}
		},
		"required": ["task_list_id"],
		"additionalProperties": false
	}`)
}

func (t *GetTaskListStatusTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params getTaskListStatusArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskListID == 0 {
		return tools.ToolResult{Content: "task_list_id is required and must be > 0", IsError: true}, nil
	}

	// Verify task list exists and get workflow
	taskList, err := t.mgr.GetTaskList(params.TaskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%d): %v", params.TaskListID, err), IsError: true}, nil
	}

	stats, err := t.mgr.GetTaskListStats(params.TaskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error getting stats: %v", err), IsError: true}, nil
	}

	// Enrich with status labels
	response := map[string]any{
		"task_list_id":    params.TaskListID,
		"task_list_title": taskList.Title,
		"total":           stats["total"],
	}

	if taskList.Workflow != nil {
		statuses, err := parseWorkflowStatuses(taskList.Workflow)
		if err == nil {
			byStatus, _ := stats["byStatus"].(map[string]int64)
			statusCounts := make([]map[string]any, 0, len(statuses))
			for _, s := range statuses {
				count := int64(0)
				if byStatus != nil {
					count = byStatus[fmt.Sprintf("%d", s.ID)]
				}
				statusCounts = append(statusCounts, map[string]any{
					"status_id": s.ID,
					"label":     s.Label,
					"count":     count,
				})
			}
			response["statuses"] = statusCounts
		}
	}

	resultJSON, _ := json.Marshal(response)
	return tools.ToolResult{
		Content:  string(resultJSON),
		Metadata: map[string]any{"task_list_id": params.TaskListID},
	}, nil
}

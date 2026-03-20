package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type getTaskListArgs struct {
	TaskListID uint `json:"task_list_id"`
}

type GetTaskListTool struct {
	mgr TaskListManager
}

func NewGetTaskList(mgr TaskListManager) *GetTaskListTool {
	return &GetTaskListTool{mgr: mgr}
}

func (t *GetTaskListTool) Name() string { return "get_task_list" }

func (t *GetTaskListTool) Description() string {
	return "Returns a task list with all its tasks and subtasks organized hierarchically, including workflow status definitions."
}

func (t *GetTaskListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list to retrieve"
			}
		},
		"required": ["task_list_id"],
		"additionalProperties": false
	}`)
}

func (t *GetTaskListTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params getTaskListArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	if params.TaskListID == 0 {
		return tools.ToolResult{Content: "task_list_id is required and must be > 0", IsError: true}, nil
	}

	taskList, err := t.mgr.GetTaskList(params.TaskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%d): %v", params.TaskListID, err), IsError: true}, nil
	}

	// Build a clean response with workflow info
	type statusInfo struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
	}
	type taskInfo struct {
		ID          uint       `json:"id"`
		Title       string     `json:"title"`
		Description string     `json:"description,omitempty"`
		StatusID    int        `json:"status_id"`
		ParentID    *uint      `json:"parent_id,omitempty"`
		Subtasks    []taskInfo `json:"subtasks,omitempty"`
	}

	var convertTasks func(tasks []database.Task) []taskInfo
	convertTasks = func(tasks []database.Task) []taskInfo {
		result := make([]taskInfo, len(tasks))
		for i, task := range tasks {
			result[i] = taskInfo{
				ID:          task.ID,
				Title:       task.Title,
				Description: task.Description,
				StatusID:    task.StatusID,
				ParentID:    task.ParentID,
			}
			if len(task.Subtasks) > 0 {
				result[i].Subtasks = convertTasks(task.Subtasks)
			}
		}
		return result
	}

	response := map[string]any{
		"id":    taskList.ID,
		"title": taskList.Title,
		"tasks": convertTasks(taskList.Tasks),
	}

	// Include workflow statuses for context
	if taskList.Workflow != nil {
		statuses, err := parseWorkflowStatuses(taskList.Workflow)
		if err == nil {
			statusList := make([]statusInfo, len(statuses))
			for i, s := range statuses {
				statusList[i] = statusInfo{ID: s.ID, Label: s.Label}
			}
			response["workflow_statuses"] = statusList
			response["initial_status_id"] = taskList.Workflow.InitialStatusID
		}
	}

	resultJSON, _ := json.Marshal(response)
	return tools.ToolResult{
		Content:  string(resultJSON),
		Metadata: map[string]any{"task_list_id": taskList.ID},
	}, nil
}

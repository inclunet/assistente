package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/tools"
)

type ListTaskListsTool struct {
	mgr TaskListManager
}

func NewListTaskLists(mgr TaskListManager) *ListTaskListsTool {
	return &ListTaskListsTool{mgr: mgr}
}

func (t *ListTaskListsTool) Name() string { return "list_task_lists" }

func (t *ListTaskListsTool) Description() string {
	return "Lists all existing task lists with their IDs, titles, and task counts."
}

func (t *ListTaskListsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`)
}

func (t *ListTaskListsTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	taskLists, err := t.mgr.GetAllTaskLists()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing task lists: %v", err), IsError: true}, nil
	}

	if len(taskLists) == 0 {
		return tools.ToolResult{Content: "No task lists found."}, nil
	}

	type taskListSummary struct {
		ID        uint   `json:"id"`
		Title     string `json:"title"`
		TaskCount int    `json:"task_count"`
	}

	summaries := make([]taskListSummary, len(taskLists))
	for i, tl := range taskLists {
		summaries[i] = taskListSummary{
			ID:        tl.ID,
			Title:     tl.Title,
			TaskCount: len(tl.Tasks),
		}
	}

	resultJSON, _ := json.Marshal(summaries)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Found %d task list(s):\n%s", len(summaries), string(resultJSON)),
		Metadata: map[string]any{"count": len(summaries)},
	}, nil
}

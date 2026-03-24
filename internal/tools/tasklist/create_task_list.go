package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type createTaskListArgs struct {
	Title            string `json:"title"`
	WorkflowTemplate string `json:"workflow_template,omitempty"`
}

type CreateTaskListTool struct {
	mgr TaskListManager
}

func NewCreateTaskList(mgr TaskListManager) *CreateTaskListTool {
	return &CreateTaskListTool{mgr: mgr}
}

func (t *CreateTaskListTool) Name() string { return "create_task_list" }

func (t *CreateTaskListTool) Description() string {
	return "Creates a new task list with an optional workflow template. Use 'simple' for a basic To Do/Done workflow, or 'kanban' (default) for To Do/In Progress/Done."
}

func (t *CreateTaskListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "Title for the new task list"
			},
			"workflow_template": {
				"type": "string",
				"enum": ["simple", "kanban"],
				"description": "Workflow template: 'simple' (To Do, Done) or 'kanban' (To Do, In Progress, Done). Defaults to 'kanban'"
			}
		},
		"required": ["title"],
		"additionalProperties": false
	}`)
}

func (t *CreateTaskListTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params createTaskListArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "Title cannot be empty", IsError: true}, nil
	}

	var template *database.TaskListWorkflow
	switch params.WorkflowTemplate {
	case "simple":
		template = simpleWorkflowTemplate()
	case "kanban", "":
		// kanban is default — uses nil (database creates default kanban workflow)
	default:
		return tools.ToolResult{
			Content: fmt.Sprintf("Invalid workflow_template '%s'. Valid values: 'simple', 'kanban'", params.WorkflowTemplate),
			IsError: true,
		}, nil
	}

	taskList, err := t.mgr.CreateTaskList(title, "", template)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task list: %v", err), IsError: true}, nil
	}

	result := map[string]any{
		"id":    taskList.ID,
		"title": taskList.Title,
	}
	if taskList.Workflow != nil {
		result["workflow_initial_status_id"] = taskList.Workflow.InitialStatusID
	}

	resultJSON, _ := json.Marshal(result)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task list created successfully:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_list_id": taskList.ID},
	}, nil
}

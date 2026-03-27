package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type workflowStatusArg struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

type workflowArg struct {
	Statuses           []workflowStatusArg `json:"statuses"`
	AllowedTransitions map[int][]int       `json:"allowed_transitions"`
	InitialStatusID    int                 `json:"initial_status_id"`
	StatusMigration    map[int]int         `json:"status_migration,omitempty"`
}

type upsertTaskListArgs struct {
	TaskListID        *uint        `json:"task_list_id,omitempty"`
	Title             string       `json:"title"`
	Description       string       `json:"description,omitempty"`
	PreferredViewMode string       `json:"preferred_view_mode,omitempty"`
	Workflow          *workflowArg `json:"workflow,omitempty"`
}

type UpsertTaskListTool struct {
	mgr TaskListManager
}

func NewUpsertTaskList(mgr TaskListManager) *UpsertTaskListTool {
	return &UpsertTaskListTool{mgr: mgr}
}

func (t *UpsertTaskListTool) Name() string { return "upsert_task_list" }

func (t *UpsertTaskListTool) Description() string {
	return `Creates or updates a task list, including its workflow (statuses and transitions). If task_list_id is provided, updates the existing list; otherwise creates a new one. Use this to set up custom workflows with specific statuses, transitions, and initial status. When updating a workflow that has tasks using status IDs that will be removed, provide status_migration to remap them.`
}

func (t *UpsertTaskListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of existing task list to update. Omit to create a new one"
			},
			"title": {
				"type": "string",
				"description": "Title for the task list"
			},
			"description": {
				"type": "string",
				"description": "Description for the task list (optional)"
			},
			"preferred_view_mode": {
				"type": "string",
				"enum": ["list", "kanban"],
				"description": "View mode: 'list' or 'kanban'. Defaults to 'list' for new lists"
			},
			"workflow": {
				"type": "object",
				"description": "Custom workflow definition. If omitted on create, a default kanban workflow is used. If omitted on update, the existing workflow is preserved",
				"properties": {
					"statuses": {
						"type": "array",
						"description": "Array of workflow statuses. Each status must have a unique integer 'id' (stable across updates) and a 'label'",
						"items": {
							"type": "object",
							"properties": {
								"id": {
									"type": "integer",
									"description": "Unique numeric ID for this status (must be > 0, stable across updates)"
								},
								"label": {
									"type": "string",
									"description": "Display label for this status"
								},
								"color": {
									"type": "string",
									"description": "CSS color for this status (optional, e.g. 'var(--color-warning)' or '#ff0000')"
								},
								"icon": {
									"type": "string",
									"description": "Icon/emoji for this status (optional, e.g. '⌛')"
								}
							},
							"required": ["id", "label"]
						}
					},
					"allowed_transitions": {
						"type": "object",
						"description": "Map of status_id to array of status_ids it can transition to. E.g. {\"1\": [2, 3], \"2\": [3]}",
						"additionalProperties": {
							"type": "array",
							"items": {"type": "integer"}
						}
					},
					"initial_status_id": {
						"type": "integer",
						"description": "ID of the status assigned to new tasks by default. Must be one of the status IDs"
					},
					"status_migration": {
						"type": "object",
						"description": "When removing statuses that have existing tasks, map old_status_id to new_status_id to migrate those tasks. E.g. {\"4\": 1} moves tasks from removed status 4 to status 1",
						"additionalProperties": {"type": "integer"}
					}
				},
				"required": ["statuses", "allowed_transitions", "initial_status_id"]
			}
		},
		"required": ["title"],
		"additionalProperties": false
	}`)
}

func (t *UpsertTaskListTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params upsertTaskListArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "title cannot be empty", IsError: true}, nil
	}

	if params.TaskListID != nil {
		return t.updateTaskList(*params.TaskListID, title, params.Description, params.PreferredViewMode, params.Workflow)
	}
	return t.createTaskList(title, params.Description, params.PreferredViewMode, params.Workflow)
}

func (t *UpsertTaskListTool) createTaskList(title, description, viewMode string, wf *workflowArg) (tools.ToolResult, error) {
	var template *database.TaskListWorkflow
	if wf != nil {
		tpl, err := t.buildWorkflowTemplate(wf)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		template = tpl
	}

	taskList, err := t.mgr.CreateTaskList(title, description, template)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task list: %v", err), IsError: true}, nil
	}

	if viewMode == "kanban" || viewMode == "list" {
		_ = t.mgr.UpdateTaskListFull(taskList.ID, title, description, viewMode)
	}

	result := t.buildResult(taskList, "created")
	resultJSON, _ := json.Marshal(result)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task list created:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_list_id": taskList.ID, "action": "created"},
	}, nil
}

func (t *UpsertTaskListTool) updateTaskList(id uint, title, description, viewMode string, wf *workflowArg) (tools.ToolResult, error) {
	existing, err := t.mgr.GetTaskList(id)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%d): %v", id, err), IsError: true}, nil
	}

	if err := t.mgr.UpdateTaskListFull(id, title, description, viewMode); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating task list: %v", err), IsError: true}, nil
	}

	if wf != nil {
		statuses := make([]database.TaskListWorkflowStatus, len(wf.Statuses))
		for i, s := range wf.Statuses {
			statuses[i] = database.TaskListWorkflowStatus{
				ID:    s.ID,
				Order: i,
				Label: s.Label,
				Color: withDefault(s.Color, "var(--accent)"),
				Icon:  withDefault(s.Icon, "⬜"),
			}
		}

		transitions := database.TaskListWorkflowTransitions(wf.AllowedTransitions)

		var migration map[int]int
		if wf.StatusMigration != nil && len(wf.StatusMigration) > 0 {
			migration = wf.StatusMigration
		}

		if err := t.mgr.UpdateWorkflowFull(id, statuses, transitions, wf.InitialStatusID, migration); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task list metadata updated but workflow update failed: %v", err), IsError: true}, nil
		}
	}

	updated, err := t.mgr.GetTaskList(id)
	if err != nil {
		updated = existing
	}

	result := t.buildResult(updated, "updated")
	resultJSON, _ := json.Marshal(result)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task list updated:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_list_id": id, "action": "updated"},
	}, nil
}

func (t *UpsertTaskListTool) buildWorkflowTemplate(wf *workflowArg) (*database.TaskListWorkflow, error) {
	if len(wf.Statuses) == 0 {
		return nil, fmt.Errorf("workflow.statuses cannot be empty")
	}

	statuses := make([]database.TaskListWorkflowStatus, len(wf.Statuses))
	idSet := make(map[int]bool, len(wf.Statuses))
	for i, s := range wf.Statuses {
		if s.ID <= 0 {
			return nil, fmt.Errorf("status id must be > 0, got %d", s.ID)
		}
		if idSet[s.ID] {
			return nil, fmt.Errorf("duplicate status id: %d", s.ID)
		}
		idSet[s.ID] = true
		statuses[i] = database.TaskListWorkflowStatus{
			ID:    s.ID,
			Order: i,
			Label: s.Label,
			Color: withDefault(s.Color, "var(--accent)"),
			Icon:  withDefault(s.Icon, "⬜"),
		}
	}

	if !idSet[wf.InitialStatusID] {
		return nil, fmt.Errorf("initial_status_id %d not found in statuses", wf.InitialStatusID)
	}

	for fromID, toIDs := range wf.AllowedTransitions {
		if !idSet[fromID] {
			return nil, fmt.Errorf("allowed_transitions references unknown source status: %d", fromID)
		}
		for _, toID := range toIDs {
			if !idSet[toID] {
				return nil, fmt.Errorf("allowed_transitions from %d references unknown target status: %d", fromID, toID)
			}
		}
	}

	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(wf.AllowedTransitions)

	return &database.TaskListWorkflow{
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    wf.InitialStatusID,
	}, nil
}

func (t *UpsertTaskListTool) buildResult(tl *database.TaskList, action string) map[string]any {
	result := map[string]any{
		"id":     tl.ID,
		"title":  tl.Title,
		"action": action,
	}
	if tl.Workflow != nil {
		statuses, err := parseWorkflowStatuses(tl.Workflow)
		if err == nil {
			statusSummary := make([]map[string]any, len(statuses))
			for i, s := range statuses {
				statusSummary[i] = map[string]any{"id": s.ID, "label": s.Label}
			}
			result["workflow_statuses"] = statusSummary
			result["initial_status_id"] = tl.Workflow.InitialStatusID
		}
	}
	return result
}

func withDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

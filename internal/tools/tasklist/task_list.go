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

type taskListArgs struct {
	TaskListID        *string         `json:"task_list_id,omitempty"`
	TaskListSlug      string          `json:"task_list_slug,omitempty"`
	Slug              *string         `json:"slug,omitempty"`
	Duplicate         bool            `json:"duplicate,omitempty"`
	SummaryOnly       bool            `json:"summary_only,omitempty"`
	Title             string          `json:"title,omitempty"`
	Description       string          `json:"description,omitempty"`
	PreferredViewMode string          `json:"preferred_view_mode,omitempty"`
	Workflow          *workflowArg    `json:"workflow,omitempty"`
	ValidationPolicy  json.RawMessage `json:"validation_policy,omitempty"`
	CustomActions     json.RawMessage `json:"custom_actions,omitempty"`
}

type TaskListTool struct {
	mgr TaskListManager
}

func NewTaskList(mgr TaskListManager) *TaskListTool {
	return &TaskListTool{mgr: mgr}
}

func (t *TaskListTool) Name() string { return "task_list" }

func (t *TaskListTool) Description() string {
	return `Full CRUD for task lists. Without params → lists all. Identify a list by task_list_id and/or task_list_slug (at least one for read/update/duplicate/summary); if both are sent, they must refer to the same list. With id or slug only → full details. With summary_only → lightweight status counts. Optional slug (create/duplicate: initial slug for the new list; update: set or clear — use empty string to remove slug). With title and no existing list reference → create. With id or slug → update (title may be omitted to keep current). With duplicate + title → copy (tasks NOT copied). validation_policy: task_code_regex, allowed_note_sources, note_external_id_regex, note_external_parent_id_regex; {} clears. custom_actions: optional per-list context-menu items/buttons (publish a domain event and/or open a link); [] clears, omit to keep. Workflow updates with removed statuses need status_migration.`
}

func (t *TaskListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_list_id": {
				"type": "integer",
				"description": "ID of the task list. Omit to list all (read) or create (write). With task_list_slug, both must match the same list. With only id or slug → read details; with title → update; with duplicate → copy"
			},
			"task_list_slug": {
				"type": "string",
				"description": "Stable slug of the list (lowercase). Use instead of or with task_list_id to identify the list; if both are sent, they must refer to the same list"
			},
			"slug": {
				"type": "string",
				"description": "On create/duplicate: optional slug for the new list. On update: set slug; send empty string to clear. Omit to leave slug unchanged on update"
			},
			"summary_only": {
				"type": "boolean",
				"description": "When true, returns only task counts per status (lightweight). Requires task_list_id or task_list_slug"
			},
			"duplicate": {
				"type": "boolean",
				"description": "When true, copies the referenced list (by id and/or slug). Inherits description, view mode, workflow, validation_policy unless overridden. Tasks are NOT copied. Requires title for the new list"
			},
			"title": {
				"type": "string",
				"description": "Title for the task list. Required for create, update, and duplicate"
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
			"validation_policy": {
				"type": "object",
				"description": "Optional per-list validation rules (JSON). Omit to leave unchanged on update. Use {} to clear. task_code_regex: Go regexp for task code when non-empty. allowed_note_sources: non-empty array restricts external note source (case-insensitive). note_external_id_regex / note_external_parent_id_regex: optional Go regexes for synced notes",
				"properties": {
					"task_code_regex": {"type": "string"},
					"allowed_note_sources": {
						"type": "array",
						"items": {"type": "string"}
					},
					"note_external_id_regex": {"type": "string"},
					"note_external_parent_id_regex": {"type": "string"}
				},
				"additionalProperties": false
			},
			"custom_actions": {
				"type": "array",
				"description": "Optional per-list custom actions (context-menu items / buttons). Omit to leave unchanged on update. Send [] to clear all. Each action publishes a domain event and/or opens a link. Templating: link, payload_template and when are Go templates rendered with the card as root '.task' (e.g. .task.code, .task.title, .task.link, .task.task_list_id, .task.task_list_slug) plus .now; use the json function for safe values, e.g. {\"code\": {{ json .task.code }}}",
				"items": {
					"type": "object",
					"properties": {
						"id": {
							"type": "string",
							"description": "Stable slug identifying the action. No spaces/whitespace or path separators (/ \\). Must be unique within the list"
						},
						"label": {
							"type": "string",
							"description": "Text shown on the menu item / button"
						},
						"icon": {
							"type": "string",
							"description": "Optional emoji/icon, e.g. '🔄'"
						},
						"surfaces": {
							"type": "array",
							"description": "Where the action appears. Defaults to card_menu when omitted",
							"items": {
								"type": "string",
								"enum": ["card_menu", "card_detail", "board_menu"]
							}
						},
						"event": {
							"type": "string",
							"description": "Domain event name to publish when triggered (no whitespace). Required unless 'link' is set"
						},
						"payload_template": {
							"type": "string",
							"description": "Go template rendered to a JSON object used as the event payload. Only applies when 'event' is set"
						},
						"link": {
							"type": "string",
							"description": "Go template rendered to a deep link (assistente://...) or external URL (http/https) to open. Required unless 'event' is set"
						},
						"when": {
							"type": "string",
							"description": "Optional Go template controlling visibility; the action only shows when it renders to a truthy value"
						},
						"confirm": {
							"type": "string",
							"description": "Optional confirmation text shown before running the action"
						},
						"danger": {
							"type": "boolean",
							"description": "When true, renders the action with a destructive/danger style"
						}
					},
					"required": ["id", "label"],
					"additionalProperties": false
				}
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
		"additionalProperties": false
	}`)
}

func (t *TaskListTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params taskListArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	hasValPolicy := rawJSONPresent(params.ValidationPolicy)
	hasCustomActions := rawJSONPresent(params.CustomActions)
	// task_list_slug e task_list_id são referências puras (modo leitura), nunca
	// gatilho de escrita: passar só a referência deve retornar os detalhes da
	// lista, não cair no update (que sobrescreveria description/view_mode com
	// vazio). A escrita é disparada por campos reais (title, description, slug
	// a definir, workflow, validation_policy, custom_actions, duplicate, ...).
	isWrite := strings.TrimSpace(params.Title) != "" || params.Workflow != nil || params.Duplicate || strings.TrimSpace(params.Description) != "" || params.PreferredViewMode != "" || hasValPolicy || hasCustomActions || params.Slug != nil

	idPtr := taskListIDPtrForResolve(params.TaskListID)
	slugRef := strings.TrimSpace(params.TaskListSlug)
	hasListRef := idPtr != nil || slugRef != ""

	if params.SummaryOnly && !hasListRef {
		return tools.ToolResult{Content: "summary_only requires task_list_id or task_list_slug", IsError: true}, nil
	}

	if params.Duplicate && !hasListRef {
		return tools.ToolResult{Content: "duplicate requires task_list_id or task_list_slug to reference the source list", IsError: true}, nil
	}

	// READ modes
	if !isWrite {
		if params.TaskListID != nil && *params.TaskListID == "" && slugRef == "" {
			return tools.ToolResult{Content: "task_list_id must be a non-empty string", IsError: true}, nil
		}
		if !hasListRef {
			return t.listAll(ctx)
		}
		resolved, err := t.mgr.ResolveTaskListRef(ctx, idPtr, slugRef)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		if params.SummaryOnly {
			return t.statusSummary(ctx, resolved)
		}
		return t.fullDetails(ctx, resolved)
	}

	// WRITE modes
	if hasListRef {
		resolved, err := t.mgr.ResolveTaskListRef(ctx, idPtr, slugRef)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		if params.Duplicate {
			title := strings.TrimSpace(params.Title)
			if title == "" {
				return tools.ToolResult{Content: "duplicate requires a non-empty title for the new list", IsError: true}, nil
			}
			newSlug := ""
			if params.Slug != nil {
				newSlug = *params.Slug
			}
			return t.duplicateTaskList(ctx, resolved, title, params.Description, params.PreferredViewMode, params.Workflow, params.ValidationPolicy, params.CustomActions, newSlug)
		}
		title := strings.TrimSpace(params.Title)
		if title == "" {
			ex, err := t.mgr.GetTaskList(ctx, resolved)
			if err != nil {
				return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%s): %v", resolved, err), IsError: true}, nil
			}
			title = ex.Title
		}
		if title == "" {
			return tools.ToolResult{Content: "title is required for create; for update the list must have a stored title", IsError: true}, nil
		}
		return t.updateTaskList(ctx, resolved, title, params.Description, params.PreferredViewMode, params.Workflow, params.ValidationPolicy, params.CustomActions, params.Slug)
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "title is required to create a new task list", IsError: true}, nil
	}
	initialSlug := ""
	if params.Slug != nil {
		initialSlug = *params.Slug
	}
	return t.createTaskList(ctx, title, params.Description, params.PreferredViewMode, params.Workflow, params.ValidationPolicy, params.CustomActions, initialSlug)
}

// ==================== Read Operations ====================

func (t *TaskListTool) listAll(ctx context.Context) (tools.ToolResult, error) {
	taskLists, err := t.mgr.GetAllTaskLists(ctx)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing task lists: %v", err), IsError: true}, nil
	}

	if len(taskLists) == 0 {
		return tools.ToolResult{Content: "No task lists found."}, nil
	}

	type taskListSummary struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Slug      string `json:"slug,omitempty"`
		TaskCount int    `json:"task_count"`
	}

	summaries := make([]taskListSummary, len(taskLists))
	for i, tl := range taskLists {
		summaries[i] = taskListSummary{
			ID:        tl.ID,
			Title:     tl.Title,
			Slug:      tl.Slug,
			TaskCount: len(tl.Tasks),
		}
	}

	resultJSON, _ := json.Marshal(summaries)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Found %d task list(s):\n%s", len(summaries), string(resultJSON)),
		Metadata: map[string]any{"count": len(summaries)},
	}, nil
}

func (t *TaskListTool) statusSummary(ctx context.Context, taskListID string) (tools.ToolResult, error) {
	taskList, err := t.mgr.GetTaskList(ctx, taskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%s): %v", taskListID, err), IsError: true}, nil
	}

	stats, err := t.mgr.GetTaskListStats(ctx, taskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error getting stats: %v", err), IsError: true}, nil
	}

	response := map[string]any{
		"task_list_id":    taskListID,
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
		Metadata: map[string]any{"task_list_id": taskListID},
	}, nil
}

func (t *TaskListTool) fullDetails(ctx context.Context, taskListID string) (tools.ToolResult, error) {
	taskList, err := t.mgr.GetTaskList(ctx, taskListID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%s): %v", taskListID, err), IsError: true}, nil
	}

	type statusInfo struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
	}
	type taskInfo struct {
		ID          string     `json:"id"`
		Title       string     `json:"title"`
		Description string     `json:"description,omitempty"`
		StatusID    int        `json:"status_id"`
		ParentID    *string    `json:"parent_id,omitempty"`
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
	if taskList.Slug != "" {
		response["slug"] = taskList.Slug
	}
	if vp := validationPolicyToMap(taskList.ValidationPolicy); vp != nil {
		response["validation_policy"] = vp
	}
	if ca := customActionsToList(taskList.CustomActions); ca != nil {
		response["custom_actions"] = ca
	}

	if taskList.Workflow != nil {
		statuses, err := parseWorkflowStatuses(taskList.Workflow)
		if err == nil {
			statusList := make([]statusInfo, len(statuses))
			for i, s := range statuses {
				statusList[i] = statusInfo{ID: s.ID, Label: s.Label}
			}
			response["workflow_statuses"] = statusList
			response["initial_status_id"] = taskList.Workflow.InitialStatusID

			stats, statsErr := t.mgr.GetTaskListStats(ctx, taskListID)
			if statsErr == nil {
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
				response["summary"] = map[string]any{
					"total":    stats["total"],
					"statuses": statusCounts,
				}
			}
		}
	}

	resultJSON, _ := json.Marshal(response)
	return tools.ToolResult{
		Content:  string(resultJSON),
		Metadata: map[string]any{"task_list_id": taskList.ID},
	}, nil
}

// ==================== Write Operations ====================

func (t *TaskListTool) createTaskList(ctx context.Context, title, description, viewMode string, wf *workflowArg, policyRaw, customActionsRaw json.RawMessage, initialSlug string) (tools.ToolResult, error) {
	var template *database.TaskListWorkflow
	if wf != nil {
		tpl, err := t.buildWorkflowTemplate(wf)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		template = tpl
	}

	taskList, err := t.mgr.CreateTaskList(ctx, title, description, template, initialSlug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating task list: %v", err), IsError: true}, nil
	}

	if viewMode == "kanban" || viewMode == "list" {
		_ = t.mgr.UpdateTaskListFull(ctx, taskList.ID, title, description, viewMode, nil)
	}

	if msg, err := t.applyValidationPolicy(ctx, taskList.ID, policyRaw); err != nil {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	if msg, err := t.applyCustomActions(ctx, taskList.ID, customActionsRaw); err != nil {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	updated, err := t.mgr.GetTaskList(ctx, taskList.ID)
	if err != nil {
		updated = taskList
	}
	result := t.buildResult(updated, "created")
	resultJSON, _ := json.Marshal(result)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task list created:\n%s", string(resultJSON)),
		Metadata: map[string]any{"task_list_id": taskList.ID, "action": "created"},
	}, nil
}

func (t *TaskListTool) duplicateTaskList(ctx context.Context, sourceID string, title, description, viewMode string, wfOverride *workflowArg, policyRaw, customActionsRaw json.RawMessage, newListSlug string) (tools.ToolResult, error) {
	source, err := t.mgr.GetTaskList(ctx, sourceID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Source task list not found (id=%s): %v", sourceID, err), IsError: true}, nil
	}

	if description == "" {
		description = source.Description
	}

	var template *database.TaskListWorkflow
	if wfOverride != nil {
		tpl, err := t.buildWorkflowTemplate(wfOverride)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		template = tpl
	} else if source.Workflow != nil {
		template = &database.TaskListWorkflow{
			Statuses:           source.Workflow.Statuses,
			AllowedTransitions: source.Workflow.AllowedTransitions,
			InitialStatusID:    source.Workflow.InitialStatusID,
		}
	}

	newList, err := t.mgr.CreateTaskList(ctx, title, description, template, newListSlug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error duplicating task list: %v", err), IsError: true}, nil
	}

	effectiveViewMode := viewMode
	if effectiveViewMode == "" {
		effectiveViewMode = source.PreferredViewMode
	}
	if effectiveViewMode == "kanban" || effectiveViewMode == "list" {
		_ = t.mgr.UpdateTaskListFull(ctx, newList.ID, title, description, effectiveViewMode, nil)
	}

	if len(policyRaw) > 0 && strings.TrimSpace(string(policyRaw)) != "" && strings.TrimSpace(string(policyRaw)) != "null" {
		if msg, err := t.applyValidationPolicy(ctx, newList.ID, policyRaw); err != nil {
			return tools.ToolResult{Content: msg, IsError: true}, nil
		}
	} else if strings.TrimSpace(source.ValidationPolicy) != "" {
		if err := t.mgr.SetTaskListValidationPolicy(ctx, newList.ID, source.ValidationPolicy); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error copying validation policy: %v", err), IsError: true}, nil
		}
	}

	if rawJSONPresent(customActionsRaw) {
		if msg, err := t.applyCustomActions(ctx, newList.ID, customActionsRaw); err != nil {
			return tools.ToolResult{Content: msg, IsError: true}, nil
		}
	} else if strings.TrimSpace(source.CustomActions) != "" {
		if err := t.mgr.SetTaskListCustomActions(ctx, newList.ID, source.CustomActions); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error copying custom actions: %v", err), IsError: true}, nil
		}
	}

	updated, err := t.mgr.GetTaskList(ctx, newList.ID)
	if err != nil {
		updated = newList
	}

	result := t.buildResult(updated, "duplicated")
	result["source_task_list_id"] = sourceID
	resultJSON, _ := json.Marshal(result)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Task list duplicated (from id=%s):\n%s", sourceID, string(resultJSON)),
		Metadata: map[string]any{"task_list_id": newList.ID, "source_task_list_id": sourceID, "action": "duplicated"},
	}, nil
}

func (t *TaskListTool) updateTaskList(ctx context.Context, id string, title, description, viewMode string, wf *workflowArg, policyRaw, customActionsRaw json.RawMessage, slugUpdate *string) (tools.ToolResult, error) {
	existing, err := t.mgr.GetTaskList(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Task list not found (id=%s): %v", id, err), IsError: true}, nil
	}

	if err := t.mgr.UpdateTaskListFull(ctx, id, title, description, viewMode, slugUpdate); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating task list: %v", err), IsError: true}, nil
	}

	if msg, err := t.applyValidationPolicy(ctx, id, policyRaw); err != nil {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	if msg, err := t.applyCustomActions(ctx, id, customActionsRaw); err != nil {
		return tools.ToolResult{Content: msg, IsError: true}, nil
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
		if len(wf.StatusMigration) > 0 {
			migration = wf.StatusMigration
		}

		if err := t.mgr.UpdateWorkflowFull(ctx, id, statuses, transitions, wf.InitialStatusID, migration); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Task list metadata updated but workflow update failed: %v", err), IsError: true}, nil
		}
	}

	updated, err := t.mgr.GetTaskList(ctx, id)
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

// ==================== Helpers ====================

func (t *TaskListTool) buildWorkflowTemplate(wf *workflowArg) (*database.TaskListWorkflow, error) {
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

func (t *TaskListTool) buildResult(tl *database.TaskList, action string) map[string]any {
	result := map[string]any{
		"id":     tl.ID,
		"title":  tl.Title,
		"action": action,
	}
	if tl.Slug != "" {
		result["slug"] = tl.Slug
	}
	if vp := validationPolicyToMap(tl.ValidationPolicy); vp != nil {
		result["validation_policy"] = vp
	}
	if ca := customActionsToList(tl.CustomActions); ca != nil {
		result["custom_actions"] = ca
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

func validationPolicyToMap(raw string) map[string]any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{"_parse_error": true, "raw": s}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

// customActionsToList interpreta o JSON armazenado em custom_actions e devolve
// a lista de ações como []map[string]any para echo nas respostas. Retorna nil
// quando não há ações configuradas ou o JSON é inválido.
func customActionsToList(raw string) []map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	ca, err := database.ParseTaskListCustomActionsJSON(raw)
	if err != nil || ca == nil || len(ca.Actions) == 0 {
		return nil
	}
	actionsJSON, err := json.Marshal(ca.Actions)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(actionsJSON, &out); err != nil {
		return nil
	}
	return out
}

// applyValidationPolicy aplica validation_policy quando o JSON foi enviado (inclui {} para limpar).
func (t *TaskListTool) applyValidationPolicy(ctx context.Context, taskListID string, policyRaw json.RawMessage) (errMsg string, err error) {
	if !rawJSONPresent(policyRaw) {
		return "", nil
	}
	s := strings.TrimSpace(string(policyRaw))
	if s == "{}" {
		if e := t.mgr.SetTaskListValidationPolicy(ctx, taskListID, ""); e != nil {
			return fmt.Sprintf("Error clearing validation_policy: %v", e), e
		}
		return "", nil
	}
	if _, e := database.ParseTaskListValidationPolicyJSON(s); e != nil {
		return e.Error(), e
	}
	if e := t.mgr.SetTaskListValidationPolicy(ctx, taskListID, s); e != nil {
		return fmt.Sprintf("Error saving validation_policy: %v", e), e
	}
	return "", nil
}

// rawJSONPresent reporta se um campo json.RawMessage opcional foi efetivamente
// enviado (não vazio e diferente de "null"). Usado para distinguir "omitido"
// (mantém valor atual) de "enviado" (aplica/limpa).
func rawJSONPresent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null"
}

// applyCustomActions aplica custom_actions quando enviado. Aceita um array de
// ações (forma natural do schema) ou um objeto {"actions":[...]}. [] ou {} limpam.
func (t *TaskListTool) applyCustomActions(ctx context.Context, taskListID string, raw json.RawMessage) (errMsg string, err error) {
	if !rawJSONPresent(raw) {
		return "", nil
	}
	s := strings.TrimSpace(string(raw))
	if s == "[]" || s == "{}" {
		if e := t.mgr.SetTaskListCustomActions(ctx, taskListID, ""); e != nil {
			return fmt.Sprintf("Error clearing custom_actions: %v", e), e
		}
		return "", nil
	}
	// Aceita array (forma do schema) e o embrulha no formato armazenado
	// {"actions":[...]}; objeto já no formato esperado passa direto.
	normalized := s
	if strings.HasPrefix(s, "[") {
		normalized = `{"actions":` + s + `}`
	}
	if _, e := database.ParseTaskListCustomActionsJSON(normalized); e != nil {
		return e.Error(), e
	}
	if e := t.mgr.SetTaskListCustomActions(ctx, taskListID, normalized); e != nil {
		return fmt.Sprintf("Error saving custom_actions: %v", e), e
	}
	return "", nil
}

func taskListIDPtrForResolve(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	v := *p
	return &v
}

package tasklist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"assistente/internal/database"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

const (
	planTaskCodePrefix = "plan:"
	maxPlanItems       = 100
	maxPlanItemIDBytes = 123
)

var updatePlanLocks = struct {
	sync.Mutex
	entries map[string]*updatePlanLockEntry
}{entries: make(map[string]*updatePlanLockEntry)}

type updatePlanLockEntry struct {
	mu   sync.Mutex
	refs int
}

type PlanManager interface {
	CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error)
	GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error)
	FindTaskListBySlug(ctx context.Context, slug string) (*database.TaskList, error)
	UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error
	SetTaskListConversation(ctx context.Context, id string, conversationID *string) error
	CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error)
	UpdateTask(ctx context.Context, id string, title, description, code, link string) error
	UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error
	PromoteTask(ctx context.Context, id string) error
	DeleteTask(ctx context.Context, id string) error
	GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error)
	ReorderTasks(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error
}

type UpdatePlan struct {
	mgr PlanManager
}

func NewUpdatePlan(mgr PlanManager) *UpdatePlan {
	return &UpdatePlan{mgr: mgr}
}

func (t *UpdatePlan) Name() string { return "update_plan" }

func (t *UpdatePlan) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{
		Category: "tasklist",
		Class:    "task_management",
		Package:  "tasks",
		Risk:     "write",
	}
}

func (t *UpdatePlan) Description() string {
	return "Creates or replaces the execution plan for the current conversation. " +
		"Send the complete ordered list on every call. Reuse stable item ids and update statuses as work advances. " +
		"At most one item may be in_progress. The plan is persisted in the Task List Manager; " +
		"use task_list, task, and task_note only for advanced task-management operations."
}

func (t *UpdatePlan) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "Short title for this conversation's plan, in the user's language"
			},
			"explanation": {
				"type": "string",
				"description": "Optional short explanation of what changed and why; omit to preserve the previous explanation"
			},
			"plan": {
				"type": "array",
				"description": "Complete ordered plan snapshot. Items omitted from this array are removed",
				"maxItems": 100,
				"items": {
					"type": "object",
					"properties": {
						"id": {
							"type": "string",
							"description": "Stable item id, reused across updates"
						},
						"step": {
							"type": "string",
							"description": "Concise description of the step"
						},
						"status": {
							"type": "string",
							"enum": ["pending", "in_progress", "completed"]
						}
					},
					"required": ["id", "step", "status"],
					"additionalProperties": false
				}
			}
		},
		"required": ["title", "plan"],
		"additionalProperties": false
	}`)
}

type updatePlanArgs struct {
	Title       string            `json:"title"`
	Explanation *string           `json:"explanation,omitempty"`
	Plan        *[]updatePlanItem `json:"plan"`
}

type updatePlanItem struct {
	ID     string `json:"id"`
	Step   string `json:"step"`
	Status string `json:"status"`
}

type updatePlanResponse struct {
	Status      string           `json:"status"`
	Updated     bool             `json:"updated"`
	PlanID      string           `json:"plan_id,omitempty"`
	Title       string           `json:"title,omitempty"`
	Explanation string           `json:"explanation,omitempty"`
	Plan        []updatePlanItem `json:"plan"`
	Counts      map[string]int   `json:"counts,omitempty"`
	Error       *updatePlanError `json:"error,omitempty"`
}

type updatePlanError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (t *UpdatePlan) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	if t.mgr == nil {
		return updatePlanErrorResult("", "unavailable", "Task List Manager is unavailable."), nil
	}

	var params updatePlanArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return updatePlanErrorResult("", "invalid_arguments", "Could not parse arguments: "+err.Error()), nil
	}
	if err := validateUpdatePlanArgs(&params); err != nil {
		return updatePlanErrorResult("", "invalid_plan", err.Error()), nil
	}

	invocation, ok := invocationctx.Get(ctx)
	conversationID := strings.TrimSpace(invocation.ConversationID)
	if !ok || conversationID == "" {
		return updatePlanErrorResult("", "conversation_required", "update_plan requires a current conversation context."), nil
	}

	unlock := lockUpdatePlan(conversationID)
	defer unlock()

	plan := *params.Plan
	taskList, metadataChanged, err := t.ensurePlanTaskList(ctx, conversationID, params)
	if err != nil {
		return updatePlanErrorResult("", "plan_unavailable", err.Error()), nil
	}
	if err := t.ensurePlanWorkflow(ctx, taskList.ID); err != nil {
		return updatePlanErrorResult(taskList.ID, "incompatible_workflow", err.Error()), nil
	}
	planChanged, err := t.reconcilePlan(ctx, taskList, plan)
	if err != nil {
		return updatePlanErrorResult(taskList.ID, "reconcile_failed", err.Error()), nil
	}

	explanation := taskList.Description
	if params.Explanation != nil {
		explanation = strings.TrimSpace(*params.Explanation)
	}
	counts := map[string]int{"pending": 0, "in_progress": 0, "completed": 0}
	for _, item := range plan {
		counts[item.Status]++
	}

	return updatePlanResult(updatePlanResponse{
		Status:      "ok",
		Updated:     metadataChanged || planChanged,
		PlanID:      taskList.ID,
		Title:       params.Title,
		Explanation: explanation,
		Plan:        plan,
		Counts:      counts,
	}, false, map[string]any{
		"plan_id":        taskList.ID,
		"item_count":     len(plan),
		"in_progress":    counts["in_progress"],
		"completed":      counts["completed"],
		"conversation":   conversationID,
		"task_list_slug": taskList.Slug,
	}), nil
}

func validateUpdatePlanArgs(params *updatePlanArgs) error {
	params.Title = strings.TrimSpace(params.Title)
	if params.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if params.Plan == nil {
		return fmt.Errorf("plan is required")
	}
	if len(*params.Plan) > maxPlanItems {
		return fmt.Errorf("plan may contain at most %d items", maxPlanItems)
	}

	seen := make(map[string]struct{}, len(*params.Plan))
	inProgress := 0
	for index := range *params.Plan {
		item := &(*params.Plan)[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Step = strings.TrimSpace(item.Step)
		item.Status = strings.TrimSpace(item.Status)
		if item.ID == "" {
			return fmt.Errorf("plan item %d has an empty id", index+1)
		}
		if len([]byte(item.ID)) > maxPlanItemIDBytes {
			return fmt.Errorf("plan item %q id exceeds %d bytes", item.ID, maxPlanItemIDBytes)
		}
		if item.Step == "" {
			return fmt.Errorf("plan item %q has an empty step", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("plan item id %q is duplicated", item.ID)
		}
		seen[item.ID] = struct{}{}
		switch item.Status {
		case "pending", "completed":
		case "in_progress":
			inProgress++
		default:
			return fmt.Errorf("plan item %q has invalid status %q", item.ID, item.Status)
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("plan may contain at most one in_progress item")
	}
	return nil
}

func (t *UpdatePlan) ensurePlanTaskList(ctx context.Context, conversationID string, params updatePlanArgs) (*database.TaskList, bool, error) {
	slug := planSlug(conversationID)
	plan, err := t.mgr.FindTaskListBySlug(ctx, slug)
	if err != nil {
		return nil, false, fmt.Errorf("find conversation plan: %w", err)
	}
	changed := false

	description := ""
	if params.Explanation != nil {
		description = strings.TrimSpace(*params.Explanation)
	}
	if plan == nil {
		plan, err = t.mgr.CreateTaskList(ctx, params.Title, description, nil, slug)
		if err != nil {
			return nil, false, fmt.Errorf("create plan: %w", err)
		}
		changed = true
	}

	storedConversationID := ""
	if plan.ConversationID != nil {
		storedConversationID = strings.TrimSpace(*plan.ConversationID)
	}
	if storedConversationID != "" && storedConversationID != conversationID {
		return nil, false, fmt.Errorf("reserved plan slug belongs to another conversation")
	}
	if plan.ConversationID == nil || *plan.ConversationID != conversationID {
		if err := t.mgr.SetTaskListConversation(ctx, plan.ID, &conversationID); err != nil {
			return nil, false, fmt.Errorf("link plan to conversation: %w", err)
		}
		plan.ConversationID = &conversationID
		changed = true
	}

	currentDescription := plan.Description
	if params.Explanation != nil {
		currentDescription = description
	}
	if plan.Title != params.Title || plan.Description != currentDescription || plan.PreferredViewMode != "list" {
		if err := t.mgr.UpdateTaskListFull(ctx, plan.ID, params.Title, currentDescription, "list", nil); err != nil {
			return nil, false, fmt.Errorf("update plan metadata: %w", err)
		}
		plan.Title = params.Title
		plan.Description = currentDescription
		plan.PreferredViewMode = "list"
		changed = true
	}

	full, err := t.mgr.GetTaskListWithHierarchy(ctx, plan.ID)
	if err != nil {
		return nil, false, fmt.Errorf("load plan: %w", err)
	}
	return full, changed, nil
}

func (t *UpdatePlan) ensurePlanWorkflow(ctx context.Context, taskListID string) error {
	workflow, err := t.mgr.GetWorkflow(ctx, taskListID)
	if err != nil {
		return fmt.Errorf("load plan workflow: %w", err)
	}
	var statuses []database.TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
		return fmt.Errorf("decode plan workflow: %w", err)
	}
	found := map[int]bool{}
	for _, status := range statuses {
		found[status.ID] = true
	}
	for _, required := range []int{1, 2, 3} {
		if !found[required] {
			return fmt.Errorf("plan workflow must retain status ids 1, 2, and 3")
		}
	}
	return nil
}

func (t *UpdatePlan) reconcilePlan(ctx context.Context, taskList *database.TaskList, desired []updatePlanItem) (bool, error) {
	allTasks := flattenPlanTasks(taskList.Tasks)
	existingByCode := make(map[string]database.Task, len(allTasks))
	tasksByID := make(map[string]database.Task, len(allTasks))
	rootTasks := make(map[string]database.Task, len(taskList.Tasks))
	for _, task := range taskList.Tasks {
		rootTasks[task.ID] = task
	}
	for _, task := range allTasks {
		tasksByID[task.ID] = task
		if !strings.HasPrefix(task.Code, planTaskCodePrefix) {
			continue
		}
		if _, duplicate := existingByCode[task.Code]; duplicate {
			return false, fmt.Errorf("plan contains duplicate persisted item id %q", strings.TrimPrefix(task.Code, planTaskCodePrefix))
		}
		existingByCode[task.Code] = task
	}

	desiredCodes := make(map[string]struct{}, len(desired))
	for _, item := range desired {
		desiredCodes[planTaskCodePrefix+item.ID] = struct{}{}
	}
	deleteIDs := make(map[string]struct{})
	for code, task := range existingByCode {
		if _, keep := desiredCodes[code]; !keep {
			deleteIDs[task.ID] = struct{}{}
		}
	}

	changed := false
	for _, task := range allTasks {
		if task.ParentID == nil {
			continue
		}
		_, managedAndDesired := desiredCodes[task.Code]
		if !managedAndDesired && !hasDeletedAncestor(task, tasksByID, deleteIDs) {
			continue
		}
		if _, deleting := deleteIDs[task.ID]; deleting {
			continue
		}
		if err := t.mgr.PromoteTask(ctx, task.ID); err != nil {
			return false, fmt.Errorf("promote preserved plan descendant %q: %w", task.ID, err)
		}
		task.ParentID = nil
		tasksByID[task.ID] = task
		rootTasks[task.ID] = task
		if strings.HasPrefix(task.Code, planTaskCodePrefix) {
			existingByCode[task.Code] = task
		}
		changed = true
	}

	managedOrderByStatus := map[int][]string{1: {}, 2: {}, 3: {}}
	for _, item := range desired {
		code := planTaskCodePrefix + item.ID
		statusID := planStatusID(item.Status)

		task, exists := existingByCode[code]
		if !exists {
			created, err := t.mgr.CreateTask(ctx, taskList.ID, item.Step, "", code, "", nil)
			if err != nil {
				return false, fmt.Errorf("create plan item %q: %w", item.ID, err)
			}
			task = *created
			rootTasks[task.ID] = task
			changed = true
		} else if task.Title != item.Step {
			if err := t.mgr.UpdateTask(ctx, task.ID, item.Step, task.Description, task.Code, task.Link); err != nil {
				return false, fmt.Errorf("update plan item %q: %w", item.ID, err)
			}
			task.Title = item.Step
			changed = true
		}
		if task.StatusID != statusID {
			if err := t.mgr.UpdateTaskStatus(ctx, task.ID, statusID); err != nil {
				return false, fmt.Errorf("update status of plan item %q: %w", item.ID, err)
			}
			task.StatusID = statusID
			if _, root := rootTasks[task.ID]; root {
				rootTasks[task.ID] = task
			}
			changed = true
		}
		if task.ParentID == nil {
			managedOrderByStatus[statusID] = append(managedOrderByStatus[statusID], task.ID)
		}
	}

	toDelete := make([]database.Task, 0, len(deleteIDs))
	depths := planTaskDepths(taskList.Tasks)
	for _, task := range existingByCode {
		if _, remove := deleteIDs[task.ID]; remove {
			toDelete = append(toDelete, task)
		}
	}
	sort.Slice(toDelete, func(i, j int) bool {
		if depths[toDelete[i].ID] == depths[toDelete[j].ID] {
			return toDelete[i].ID < toDelete[j].ID
		}
		return depths[toDelete[i].ID] > depths[toDelete[j].ID]
	})
	for _, task := range toDelete {
		if err := t.mgr.DeleteTask(ctx, task.ID); err != nil {
			return false, fmt.Errorf("remove plan item %q: %w", strings.TrimPrefix(task.Code, planTaskCodePrefix), err)
		}
		delete(rootTasks, task.ID)
		changed = true
	}

	currentByStatus := orderedRootTaskIDsByStatus(rootTasks)
	for statusID, managedIDs := range managedOrderByStatus {
		orderedIDs := append([]string(nil), managedIDs...)
		for _, id := range currentByStatus[statusID] {
			if !strings.HasPrefix(rootTasks[id].Code, planTaskCodePrefix) {
				orderedIDs = append(orderedIDs, id)
			}
		}
		if len(orderedIDs) == 0 {
			continue
		}
		if slices.Equal(currentByStatus[statusID], orderedIDs) {
			continue
		}
		if err := t.mgr.ReorderTasks(ctx, taskList.ID, statusID, orderedIDs); err != nil {
			return false, fmt.Errorf("reorder plan items in status %d: %w", statusID, err)
		}
		changed = true
	}
	return changed, nil
}

func flattenPlanTasks(tasks []database.Task) []database.Task {
	result := make([]database.Task, 0, len(tasks))
	var appendTasks func([]database.Task)
	appendTasks = func(items []database.Task) {
		for _, task := range items {
			result = append(result, task)
			appendTasks(task.Subtasks)
		}
	}
	appendTasks(tasks)
	return result
}

func planTaskDepths(tasks []database.Task) map[string]int {
	result := make(map[string]int)
	var walk func([]database.Task, int)
	walk = func(items []database.Task, depth int) {
		for _, task := range items {
			result[task.ID] = depth
			walk(task.Subtasks, depth+1)
		}
	}
	walk(tasks, 0)
	return result
}

func hasDeletedAncestor(task database.Task, tasksByID map[string]database.Task, deleteIDs map[string]struct{}) bool {
	parentID := task.ParentID
	seen := make(map[string]struct{})
	for parentID != nil && *parentID != "" {
		if _, deleted := deleteIDs[*parentID]; deleted {
			return true
		}
		if _, cycle := seen[*parentID]; cycle {
			return false
		}
		seen[*parentID] = struct{}{}
		parent, ok := tasksByID[*parentID]
		if !ok {
			return false
		}
		parentID = parent.ParentID
	}
	return false
}

func orderedRootTaskIDsByStatus(tasks map[string]database.Task) map[int][]string {
	byStatus := map[int][]database.Task{1: {}, 2: {}, 3: {}}
	for _, task := range tasks {
		byStatus[task.StatusID] = append(byStatus[task.StatusID], task)
	}
	result := map[int][]string{1: {}, 2: {}, 3: {}}
	for statusID, items := range byStatus {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Order == items[j].Order {
				return items[i].ID < items[j].ID
			}
			return items[i].Order < items[j].Order
		})
		for _, task := range items {
			result[statusID] = append(result[statusID], task.ID)
		}
	}
	return result
}

func planStatusID(status string) int {
	switch status {
	case "in_progress":
		return 2
	case "completed":
		return 3
	default:
		return 1
	}
}

func planSlug(conversationID string) string {
	sum := sha256.Sum256([]byte(conversationID))
	return "assistente-plan-" + hex.EncodeToString(sum[:16])
}

func lockUpdatePlan(key string) func() {
	updatePlanLocks.Lock()
	entry := updatePlanLocks.entries[key]
	if entry == nil {
		entry = &updatePlanLockEntry{}
		updatePlanLocks.entries[key] = entry
	}
	entry.refs++
	updatePlanLocks.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		updatePlanLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(updatePlanLocks.entries, key)
		}
		updatePlanLocks.Unlock()
	}
}

func updatePlanErrorResult(planID, code, message string) tools.ToolResult {
	return updatePlanResult(updatePlanResponse{
		Status:  "error",
		Updated: false,
		PlanID:  planID,
		Error:   &updatePlanError{Code: code, Message: message},
	}, true, map[string]any{
		"plan_id": planID,
		"code":    code,
	})
}

func updatePlanResult(response updatePlanResponse, isError bool, metadata map[string]any) tools.ToolResult {
	encoded, err := json.Marshal(response)
	if err != nil {
		return tools.ToolResult{
			Content: "Failed to serialize update_plan result: " + err.Error(),
			IsError: true,
		}
	}
	return tools.ToolResult{
		Content:    string(encoded),
		IsError:    isError,
		Metadata:   metadata,
		Structured: true,
	}
}

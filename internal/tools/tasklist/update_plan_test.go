package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
)

type fakePlanManager struct {
	lists       map[string]*database.TaskList
	workflows   map[string]*database.TaskListWorkflow
	nextList    int
	nextTask    int
	mutations   []string
	reorderings map[int][]string
	failCreate  string
}

func newFakePlanManager() *fakePlanManager {
	return &fakePlanManager{
		lists:       make(map[string]*database.TaskList),
		workflows:   make(map[string]*database.TaskListWorkflow),
		reorderings: make(map[int][]string),
	}
}

func (m *fakePlanManager) CreateTaskList(_ context.Context, title, description string, _ *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	for _, list := range m.lists {
		if list.Slug == slug {
			return nil, fmt.Errorf("slug already exists")
		}
	}
	m.nextList++
	id := fmt.Sprintf("list-%d", m.nextList)
	list := &database.TaskList{
		UUIDModel:         database.UUIDModel{ID: id},
		Title:             title,
		Description:       description,
		Slug:              slug,
		PreferredViewMode: "list",
		Tasks:             []database.Task{},
	}
	statuses, _ := json.Marshal([]database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "A Fazer"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	})
	m.lists[id] = list
	m.workflows[id] = &database.TaskListWorkflow{
		TaskListID:      id,
		Statuses:        string(statuses),
		InitialStatusID: 1,
	}
	m.mutations = append(m.mutations, "list:create")
	return cloneTaskList(list), nil
}

func (m *fakePlanManager) GetTaskList(_ context.Context, id string) (*database.TaskList, error) {
	list := m.lists[id]
	if list == nil {
		return nil, fmt.Errorf("list not found")
	}
	return cloneTaskList(list), nil
}

func (m *fakePlanManager) GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error) {
	return m.GetTaskList(ctx, id)
}

func (m *fakePlanManager) FindTaskListBySlug(_ context.Context, slug string) (*database.TaskList, error) {
	for _, list := range m.lists {
		if list.Slug == slug {
			return cloneTaskList(list), nil
		}
	}
	return nil, nil
}

func (m *fakePlanManager) GetAllTaskLists(context.Context) ([]database.TaskList, error) {
	result := make([]database.TaskList, 0, len(m.lists))
	for _, list := range m.lists {
		result = append(result, *cloneTaskList(list))
	}
	return result, nil
}

func (m *fakePlanManager) GetTaskListsByConversation(_ context.Context, conversationID string) ([]database.TaskList, error) {
	result := make([]database.TaskList, 0)
	for _, list := range m.lists {
		if list.ConversationID != nil && *list.ConversationID == conversationID {
			result = append(result, *cloneTaskList(list))
		}
	}
	return result, nil
}

func (m *fakePlanManager) UpdateTaskListFull(_ context.Context, id, title, description, preferredViewMode string, _ *string) error {
	list := m.lists[id]
	if list == nil {
		return fmt.Errorf("list not found")
	}
	list.Title = title
	list.Description = description
	list.PreferredViewMode = preferredViewMode
	m.mutations = append(m.mutations, "list:update")
	return nil
}

func (m *fakePlanManager) SetTaskListConversation(_ context.Context, id string, conversationID *string) error {
	list := m.lists[id]
	if list == nil {
		return fmt.Errorf("list not found")
	}
	list.ConversationID = conversationID
	m.mutations = append(m.mutations, "list:link")
	return nil
}

func (m *fakePlanManager) CreateTask(_ context.Context, taskListID, title, description, code, link string, parentID *string) (*database.Task, error) {
	list := m.lists[taskListID]
	if list == nil {
		return nil, fmt.Errorf("list not found")
	}
	if m.failCreate == code {
		m.failCreate = ""
		return nil, fmt.Errorf("falha transitória")
	}
	m.nextTask++
	order := len(list.Tasks)
	task := database.Task{
		UUIDModel:   database.UUIDModel{ID: fmt.Sprintf("task-%d", m.nextTask)},
		TaskListID:  taskListID,
		Title:       title,
		Description: description,
		Code:        code,
		Link:        link,
		StatusID:    1,
		ParentID:    parentID,
		Order:       order,
	}
	list.Tasks = append(list.Tasks, task)
	m.mutations = append(m.mutations, "task:create")
	return cloneTask(&task), nil
}

func (m *fakePlanManager) UpdateTask(_ context.Context, id, title, description, code, link string) error {
	task := m.findTask(id)
	if task == nil {
		return fmt.Errorf("task not found")
	}
	task.Title = title
	task.Description = description
	task.Code = code
	task.Link = link
	m.mutations = append(m.mutations, "task:update")
	return nil
}

func (m *fakePlanManager) UpdateTaskStatus(_ context.Context, id string, newStatusID int) error {
	task := m.findTask(id)
	if task == nil {
		return fmt.Errorf("task not found")
	}
	task.StatusID = newStatusID
	m.mutations = append(m.mutations, "task:status")
	return nil
}

func (m *fakePlanManager) DeleteTask(_ context.Context, id string) error {
	for _, list := range m.lists {
		if _, removed := removeFakeTask(&list.Tasks, id); removed {
			m.mutations = append(m.mutations, "task:delete")
			return nil
		}
	}
	return fmt.Errorf("task not found")
}

func (m *fakePlanManager) PromoteTask(_ context.Context, id string) error {
	for _, list := range m.lists {
		task, removed := removeFakeTask(&list.Tasks, id)
		if !removed {
			continue
		}
		task.ParentID = nil
		list.Tasks = append(list.Tasks, *task)
		m.mutations = append(m.mutations, "task:promote")
		return nil
	}
	return fmt.Errorf("task not found")
}

func (m *fakePlanManager) GetWorkflow(_ context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	workflow := m.workflows[taskListID]
	if workflow == nil {
		return nil, fmt.Errorf("workflow not found")
	}
	copy := *workflow
	return &copy, nil
}

func (m *fakePlanManager) ReorderTasks(_ context.Context, _ string, statusID int, orderedIDs []string) error {
	m.reorderings[statusID] = append([]string(nil), orderedIDs...)
	for order, id := range orderedIDs {
		task := m.findTask(id)
		if task == nil || task.StatusID != statusID {
			return fmt.Errorf("task inválida na reordenação")
		}
		task.Order = order
	}
	m.mutations = append(m.mutations, "task:reorder")
	return nil
}

func (m *fakePlanManager) findTask(id string) *database.Task {
	for _, list := range m.lists {
		if task := findFakeTask(list.Tasks, id); task != nil {
			return task
		}
	}
	return nil
}

func findFakeTask(tasks []database.Task, id string) *database.Task {
	for index := range tasks {
		if tasks[index].ID == id {
			return &tasks[index]
		}
		if task := findFakeTask(tasks[index].Subtasks, id); task != nil {
			return task
		}
	}
	return nil
}

func removeFakeTask(tasks *[]database.Task, id string) (*database.Task, bool) {
	for index := range *tasks {
		if (*tasks)[index].ID == id {
			task := (*tasks)[index]
			*tasks = append((*tasks)[:index], (*tasks)[index+1:]...)
			return &task, true
		}
		if task, removed := removeFakeTask(&(*tasks)[index].Subtasks, id); removed {
			return task, true
		}
	}
	return nil, false
}

func cloneTaskList(list *database.TaskList) *database.TaskList {
	copy := *list
	copy.Tasks = cloneFakeTasks(list.Tasks)
	return &copy
}

func cloneFakeTasks(tasks []database.Task) []database.Task {
	result := make([]database.Task, len(tasks))
	for index := range tasks {
		result[index] = tasks[index]
		result[index].Subtasks = cloneFakeTasks(tasks[index].Subtasks)
	}
	return result
}

func cloneTask(task *database.Task) *database.Task {
	copy := *task
	return &copy
}

func planContext(conversationID string) context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: conversationID,
	})
}

func executePlan(t *testing.T, tool *UpdatePlan, ctx context.Context, args map[string]any) updatePlanResponse {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}
	var response updatePlanResponse
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("resultado não é JSON estruturado: %v (%s)", err, result.Content)
	}
	if result.IsError {
		t.Fatalf("update_plan falhou: %#v", response.Error)
	}
	if !result.Structured {
		t.Fatal("resultado deveria ser estruturado")
	}
	return response
}

func TestUpdatePlanContract(t *testing.T) {
	tool := NewUpdatePlan(nil)
	if tool.Name() != "update_plan" {
		t.Fatalf("nome inesperado: %q", tool.Name())
	}
	if metadata := tool.CatalogMetadata(); metadata.Category != "tasklist" || metadata.Class != "task_management" || metadata.Risk != "write" {
		t.Fatalf("metadados inesperados: %#v", metadata)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("schema inválido: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	plan := properties["plan"].(map[string]any)
	if plan["maxItems"] != float64(maxPlanItems) {
		t.Fatalf("maxItems inesperado: %#v", plan["maxItems"])
	}
	status := plan["items"].(map[string]any)["properties"].(map[string]any)["status"].(map[string]any)
	if got := status["enum"].([]any); len(got) != 3 {
		t.Fatalf("enum de status inesperado: %#v", got)
	}
}

func TestUpdatePlanRequiresConversationBeforeMutation(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"Plano","plan":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !result.Structured || !strings.Contains(result.Content, `"code":"conversation_required"`) {
		t.Fatalf("resultado inesperado: %#v", result)
	}
	if len(mgr.mutations) != 0 {
		t.Fatalf("não deveria mutar sem conversa: %#v", mgr.mutations)
	}
}

func TestUpdatePlanRequiresPlanFieldBeforeMutation(t *testing.T) {
	mgr := newFakePlanManager()
	result, err := NewUpdatePlan(mgr).Execute(planContext("conv-missing-plan"), json.RawMessage(`{"title":"Plano"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, `"code":"invalid_plan"`) {
		t.Fatalf("resultado inesperado: %#v", result)
	}
	if len(mgr.mutations) != 0 {
		t.Fatalf("não deveria mutar sem plan: %#v", mgr.mutations)
	}
}

func TestUpdatePlanCreatesLinkedPersistentPlan(t *testing.T) {
	mgr := newFakePlanManager()
	response := executePlan(t, NewUpdatePlan(mgr), planContext("conv-1"), map[string]any{
		"title":       "Entregar recurso",
		"explanation": "Plano inicial",
		"plan": []map[string]any{
			{"id": "inspect", "step": "Inspecionar o código", "status": "completed"},
			{"id": "implement", "step": "Implementar", "status": "in_progress"},
			{"id": "verify", "step": "Validar", "status": "pending"},
		},
	})

	if response.PlanID == "" || response.Counts["completed"] != 1 || response.Counts["in_progress"] != 1 {
		t.Fatalf("resposta inesperada: %#v", response)
	}
	list := mgr.lists[response.PlanID]
	if list == nil || list.ConversationID == nil || *list.ConversationID != "conv-1" {
		t.Fatalf("plano não foi vinculado: %#v", list)
	}
	if list.Slug != planSlug("conv-1") || len(list.Tasks) != 3 {
		t.Fatalf("lista persistida inesperada: %#v", list)
	}
	statusByCode := map[string]int{}
	for _, task := range list.Tasks {
		statusByCode[task.Code] = task.StatusID
	}
	if statusByCode["plan:inspect"] != 3 || statusByCode["plan:implement"] != 2 || statusByCode["plan:verify"] != 1 {
		t.Fatalf("status persistidos inesperados: %#v", statusByCode)
	}
}

func TestUpdatePlanReconcilesSnapshotAndPreservesForeignTasks(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-2"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "in_progress"},
			{"id": "two", "step": "Segundo", "status": "pending"},
		},
	})
	list := mgr.lists[first.PlanID]
	originalID := list.Tasks[0].ID
	foreign, err := mgr.CreateTask(context.Background(), list.ID, "Manual", "", "manual", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	response := executePlan(t, tool, planContext("conv-2"), map[string]any{
		"title":       "Plano revisado",
		"explanation": "Escopo ajustado",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro revisado", "status": "completed"},
			{"id": "three", "step": "Terceiro", "status": "in_progress"},
		},
	})
	if response.PlanID != first.PlanID {
		t.Fatalf("deveria reutilizar plano: %q != %q", response.PlanID, first.PlanID)
	}

	list = mgr.lists[first.PlanID]
	codes := make([]string, 0, len(list.Tasks))
	for _, task := range list.Tasks {
		codes = append(codes, task.Code)
		if task.Code == "plan:one" && (task.ID != originalID || task.Title != "Primeiro revisado" || task.StatusID != 3) {
			t.Fatalf("item estável não foi atualizado in place: %#v", task)
		}
	}
	sort.Strings(codes)
	want := []string{"manual", "plan:one", "plan:three"}
	if fmt.Sprint(codes) != fmt.Sprint(want) {
		t.Fatalf("codes = %#v, esperado %#v", codes, want)
	}
	if mgr.findTask(foreign.ID) == nil {
		t.Fatal("task externa ao plano não deveria ser removida")
	}
}

func TestUpdatePlanRejectsInvalidSnapshotBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		plan []map[string]any
	}{
		{
			name: "ids duplicados",
			plan: []map[string]any{
				{"id": "same", "step": "A", "status": "pending"},
				{"id": "same", "step": "B", "status": "completed"},
			},
		},
		{
			name: "dois em progresso",
			plan: []map[string]any{
				{"id": "a", "step": "A", "status": "in_progress"},
				{"id": "b", "step": "B", "status": "in_progress"},
			},
		},
		{
			name: "status inválido",
			plan: []map[string]any{
				{"id": "a", "step": "A", "status": "blocked"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newFakePlanManager()
			encoded, _ := json.Marshal(map[string]any{"title": "Plano", "plan": tc.plan})
			result, err := NewUpdatePlan(mgr).Execute(planContext("conv-invalid"), encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(result.Content, `"code":"invalid_plan"`) {
				t.Fatalf("resultado inesperado: %#v", result)
			}
			if len(mgr.mutations) != 0 {
				t.Fatalf("snapshot inválido não deveria mutar: %#v", mgr.mutations)
			}
		})
	}
}

func TestUpdatePlanScopesPlansByConversation(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-a"), map[string]any{
		"title": "Plano A",
		"plan":  []map[string]any{},
	})
	second := executePlan(t, tool, planContext("conv-b"), map[string]any{
		"title": "Plano B",
		"plan":  []map[string]any{},
	})
	if first.PlanID == second.PlanID || len(mgr.lists) != 2 {
		t.Fatalf("conversas deveriam ter planos distintos: %#v %#v", first, second)
	}
}

func TestUpdatePlanIdenticalSnapshotDoesNotEmitMutations(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	args := map[string]any{
		"title": "Plano estável",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "in_progress"},
			{"id": "two", "step": "Segundo", "status": "pending"},
		},
	}
	executePlan(t, tool, planContext("conv-idempotent"), args)
	mutations := len(mgr.mutations)
	response := executePlan(t, tool, planContext("conv-idempotent"), args)
	if len(mgr.mutations) != mutations {
		t.Fatalf("snapshot idêntico emitiu mutações: antes=%d depois=%d (%#v)", mutations, len(mgr.mutations), mgr.mutations)
	}
	if response.Updated {
		t.Fatal("snapshot idêntico não deveria informar updated=true")
	}
}

func TestUpdatePlanReorderIncludesManualTasks(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-order"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "pending"},
		},
	})
	manual, err := mgr.CreateTask(context.Background(), first.PlanID, "Manual", "", "manual", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	executePlan(t, tool, planContext("conv-order"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "two", "step": "Segundo", "status": "pending"},
			{"id": "one", "step": "Primeiro", "status": "pending"},
		},
	})
	got := mgr.reorderings[1]
	if len(got) != 3 || got[2] != manual.ID {
		t.Fatalf("reordenação deve incluir card manual ao final: %#v", got)
	}
}

func TestUpdatePlanFindsManagedSubtaskWithoutDuplicating(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-subtask"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "pending"},
		},
	})
	list := mgr.lists[first.PlanID]
	managed := list.Tasks[0]
	parent, err := mgr.CreateTask(context.Background(), list.ID, "Avô manual", "", "manual-grandparent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := mgr.CreateTask(context.Background(), list.ID, "Pai manual", "", "manual-parent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	grandparentID := parent.ID
	child.ParentID = &grandparentID
	parentID := child.ID
	managed.ParentID = &parentID
	list.Tasks = []database.Task{*parent}
	list.Tasks[0].Subtasks = []database.Task{*child}
	list.Tasks[0].Subtasks[0].Subtasks = []database.Task{managed}
	createdBefore := mgr.nextTask

	executePlan(t, tool, planContext("conv-subtask"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "pending"},
		},
	})
	if mgr.nextTask != createdBefore {
		t.Fatalf("subtask gerenciada foi duplicada: antes=%d depois=%d", createdBefore, mgr.nextTask)
	}
	promoted := mgr.findTask(managed.ID)
	if promoted == nil || promoted.ParentID != nil {
		t.Fatalf("item gerenciado profundo deveria voltar à raiz: %#v", promoted)
	}
}

func TestUpdatePlanEmptySnapshotRemainsExplicitInResponse(t *testing.T) {
	mgr := newFakePlanManager()
	result, err := NewUpdatePlan(mgr).Execute(planContext("conv-empty"), json.RawMessage(`{"title":"Plano","plan":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, `"plan":[]`) {
		t.Fatalf("snapshot vazio deve permanecer explícito: %s", result.Content)
	}
}

func TestUpdatePlanRetryConvergesAfterPartialFailure(t *testing.T) {
	mgr := newFakePlanManager()
	mgr.failCreate = "plan:two"
	tool := NewUpdatePlan(mgr)
	args := map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "one", "step": "Primeiro", "status": "completed"},
			{"id": "two", "step": "Segundo", "status": "in_progress"},
		},
	}
	encoded, _ := json.Marshal(args)
	failed, err := tool.Execute(planContext("conv-retry"), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !failed.IsError || !strings.Contains(failed.Content, `"code":"reconcile_failed"`) {
		t.Fatalf("falha parcial inesperada: %#v", failed)
	}

	response := executePlan(t, tool, planContext("conv-retry"), args)
	list := mgr.lists[response.PlanID]
	if len(list.Tasks) != 2 {
		t.Fatalf("retry não convergiu: %#v", list.Tasks)
	}
	statuses := map[string]int{}
	for _, task := range list.Tasks {
		statuses[task.Code] = task.StatusID
	}
	if statuses["plan:one"] != 3 || statuses["plan:two"] != 2 {
		t.Fatalf("status após retry inesperados: %#v", statuses)
	}
}

func TestUpdatePlanRejectsIncompatibleWorkflow(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-workflow"), map[string]any{
		"title": "Plano",
		"plan":  []map[string]any{},
	})
	statuses, _ := json.Marshal([]database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Aberto"},
		{ID: 2, Order: 1, Label: "Fechado"},
	})
	mgr.workflows[first.PlanID].Statuses = string(statuses)

	result, err := tool.Execute(planContext("conv-workflow"), json.RawMessage(`{"title":"Plano","plan":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, `"code":"incompatible_workflow"`) {
		t.Fatalf("workflow incompatível deveria falhar fechado: %#v", result)
	}
}

func TestUpdatePlanRejectsReservedSlugFromOtherConversation(t *testing.T) {
	mgr := newFakePlanManager()
	list, err := mgr.CreateTaskList(context.Background(), "Conflito", "", nil, planSlug("conv-owner"))
	if err != nil {
		t.Fatal(err)
	}
	other := "conv-other"
	mgr.lists[list.ID].ConversationID = &other

	result, err := NewUpdatePlan(mgr).Execute(planContext("conv-owner"), json.RawMessage(`{"title":"Plano","plan":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, `"code":"plan_unavailable"`) {
		t.Fatalf("slug reservado de outra conversa deveria falhar: %#v", result)
	}
}

func TestUpdatePlanPreservesDesiredChildWhenParentIsOmitted(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-parent-delete"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "parent", "step": "Pai", "status": "pending"},
			{"id": "child", "step": "Filho", "status": "pending"},
		},
	})
	list := mgr.lists[first.PlanID]
	parent := list.Tasks[0]
	child := list.Tasks[1]
	parentID := parent.ID
	child.ParentID = &parentID
	parent.Subtasks = []database.Task{child}
	list.Tasks = []database.Task{parent}

	executePlan(t, tool, planContext("conv-parent-delete"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "child", "step": "Filho", "status": "pending"},
		},
	})
	if mgr.findTask(parent.ID) != nil {
		t.Fatal("pai omitido deveria ser removido")
	}
	preserved := mgr.findTask(child.ID)
	if preserved == nil || preserved.ParentID != nil {
		t.Fatalf("filho desejado deveria ser preservado na raiz: %#v", preserved)
	}
}

func TestUpdatePlanDeletesOmittedHierarchyDeepestFirst(t *testing.T) {
	mgr := newFakePlanManager()
	tool := NewUpdatePlan(mgr)
	first := executePlan(t, tool, planContext("conv-tree-delete"), map[string]any{
		"title": "Plano",
		"plan": []map[string]any{
			{"id": "parent", "step": "Pai", "status": "pending"},
			{"id": "child", "step": "Filho", "status": "pending"},
		},
	})
	list := mgr.lists[first.PlanID]
	parent := list.Tasks[0]
	child := list.Tasks[1]
	parentID := parent.ID
	child.ParentID = &parentID
	parent.Subtasks = []database.Task{child}
	list.Tasks = []database.Task{parent}

	executePlan(t, tool, planContext("conv-tree-delete"), map[string]any{
		"title": "Plano",
		"plan":  []map[string]any{},
	})
	if mgr.findTask(parent.ID) != nil || mgr.findTask(child.ID) != nil {
		t.Fatalf("hierarquia omitida deveria ser removida: %#v", list.Tasks)
	}
}

package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

// ==================== Fake Manager ====================

type fakeTaskListManager struct {
	taskLists      map[string]*database.TaskList
	tasks          map[string]*database.Task
	workflows      map[string]*database.TaskListWorkflow
	notes          map[string][]database.TaskNote
	extNoteIndex   map[string]string
	nextListID     int
	nextTaskID     int
	nextNoteID     int
	createListErr  error
	getListErr     error
	getAllErr      error
	statsErr       error
	createTaskErr  error
	getTaskErr     error
	updateTaskErr  error
	updateStatErr  error
	deleteTaskErr  error
	getWorkflowErr error
	createNoteErr  error
	getNotesErr    error
}

func newFakeManager() *fakeTaskListManager {
	return &fakeTaskListManager{
		taskLists:    make(map[string]*database.TaskList),
		tasks:        make(map[string]*database.Task),
		workflows:    make(map[string]*database.TaskListWorkflow),
		notes:        make(map[string][]database.TaskNote),
		extNoteIndex: make(map[string]string),
		nextListID:   1,
		nextTaskID:   1,
		nextNoteID:   1,
	}
}

func (f *fakeTaskListManager) addTaskList(title string, statuses []database.TaskListWorkflowStatus) *database.TaskList {
	id := fmt.Sprintf("%d", f.nextListID)
	f.nextListID++

	statusesJSON, _ := json.Marshal(statuses)
	transitions := database.TaskListWorkflowTransitions{1: {2, 3}, 2: {1, 3}, 3: {1, 2}}
	transitionsJSON, _ := json.Marshal(transitions)

	wf := &database.TaskListWorkflow{
		TaskListID:         id,
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    1,
	}
	wf.ID = id
	f.workflows[id] = wf

	tl := &database.TaskList{
		Title:    title,
		Workflow: wf,
	}
	tl.ID = id
	f.taskLists[id] = tl
	return tl
}

func defaultStatuses() []database.TaskListWorkflowStatus {
	return []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
		{ID: 2, Order: 1, Label: "Em Progresso", Color: "var(--color-info)", Icon: "▶️"},
		{ID: 3, Order: 2, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
	}
}

func (f *fakeTaskListManager) addTaskListWithTransitions(title string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions) *database.TaskList {
	id := fmt.Sprintf("%d", f.nextListID)
	f.nextListID++

	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)

	wf := &database.TaskListWorkflow{
		TaskListID:         id,
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    1,
	}
	wf.ID = id
	f.workflows[id] = wf

	tl := &database.TaskList{
		Title:    title,
		Workflow: wf,
	}
	tl.ID = id
	f.taskLists[id] = tl
	return tl
}

func (f *fakeTaskListManager) addTask(taskListID string, title string, statusID int) *database.Task {
	id := fmt.Sprintf("%d", f.nextTaskID)
	f.nextTaskID++
	task := &database.Task{
		TaskListID: taskListID,
		Title:      title,
		StatusID:   statusID,
	}
	task.ID = id
	f.tasks[id] = task

	if tl, ok := f.taskLists[taskListID]; ok {
		tl.Tasks = append(tl.Tasks, *task)
	}
	return task
}

func (f *fakeTaskListManager) findTaskListIDBySlug(norm string) string {
	if norm == "" {
		return ""
	}
	for id, tl := range f.taskLists {
		if database.NormalizeTaskListSlug(tl.Slug) == norm {
			return id
		}
	}
	return ""
}

func (f *fakeTaskListManager) CreateTaskList(_ context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	if f.createListErr != nil {
		return nil, f.createListErr
	}
	s := database.NormalizeTaskListSlug(slug)
	if err := database.ValidateTaskListSlugFormat(s); err != nil {
		return nil, err
	}
	if s != "" {
		if oid := f.findTaskListIDBySlug(s); oid != "" {
			return nil, fmt.Errorf("slug %q já está em uso por outra lista", s)
		}
	}
	tl := f.addTaskList(title, defaultStatuses())
	tl.Description = description
	tl.Slug = s
	return tl, nil
}

func (f *fakeTaskListManager) GetTaskList(_ context.Context, id string) (*database.TaskList, error) {
	if f.getListErr != nil {
		return nil, f.getListErr
	}
	tl, ok := f.taskLists[id]
	if !ok {
		return nil, fmt.Errorf("task list not found: %s", id)
	}
	return tl, nil
}

func (f *fakeTaskListManager) GetAllTaskLists(_ context.Context) ([]database.TaskList, error) {
	if f.getAllErr != nil {
		return nil, f.getAllErr
	}
	result := make([]database.TaskList, 0, len(f.taskLists))
	for _, tl := range f.taskLists {
		result = append(result, *tl)
	}
	return result, nil
}

func (f *fakeTaskListManager) GetTaskListStats(_ context.Context, taskListID string) (map[string]interface{}, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	tl, ok := f.taskLists[taskListID]
	if !ok {
		return nil, fmt.Errorf("task list not found: %s", taskListID)
	}
	byStatus := make(map[string]int64)
	for _, task := range tl.Tasks {
		byStatus[fmt.Sprintf("%d", task.StatusID)]++
	}
	return map[string]interface{}{
		"total":    int64(len(tl.Tasks)),
		"byStatus": byStatus,
	}, nil
}

func (f *fakeTaskListManager) CreateTask(_ context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	if f.createTaskErr != nil {
		return nil, f.createTaskErr
	}
	if _, ok := f.taskLists[taskListID]; !ok {
		return nil, fmt.Errorf("task list not found: %s", taskListID)
	}
	pol, _ := fakeListPolicy(f, taskListID)
	if err := database.ValidateTaskCodeAgainstPolicy(code, pol); err != nil {
		return nil, err
	}
	wf := f.workflows[taskListID]
	task := f.addTask(taskListID, title, wf.InitialStatusID)
	task.Description = description
	task.Code = code
	task.Link = link
	task.ParentID = parentID
	return task, nil
}

func (f *fakeTaskListManager) CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	task, err := f.CreateTask(ctx, taskListID, title, description, code, link, parentID)
	if err != nil {
		return nil, err
	}
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	task.CreatorName = creatorName
	task.CreatorID = creatorID
	return task, nil
}

func (f *fakeTaskListManager) GetTask(_ context.Context, id string) (*database.Task, error) {
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, nil
}

func (f *fakeTaskListManager) FindTaskByCode(_ context.Context, taskListID string, code string) (*database.Task, error) {
	for _, task := range f.tasks {
		if task.TaskListID == taskListID && task.Code == code {
			return task, nil
		}
	}
	return nil, nil
}

func (f *fakeTaskListManager) UpdateTask(_ context.Context, id string, title, description, code, link string) error {
	if f.updateTaskErr != nil {
		return f.updateTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	pol, _ := fakeListPolicy(f, task.TaskListID)
	if err := database.ValidateTaskCodeAgainstPolicy(code, pol); err != nil {
		return err
	}
	task.Title = title
	task.Description = description
	task.Code = code
	task.Link = link
	return nil
}

func (f *fakeTaskListManager) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if err := f.UpdateTask(ctx, id, title, description, code, link); err != nil {
		return err
	}
	task := f.tasks[id]
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	task.CreatorName = creatorName
	task.CreatorID = creatorID
	return nil
}

func (f *fakeTaskListManager) UpdateTaskAssignee(_ context.Context, id string, assigneeName, assigneeID string) error {
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	return nil
}

func (f *fakeTaskListManager) SetTaskConversation(_ context.Context, id string, conversationID *string) error {
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	task.ConversationID = conversationID
	return nil
}

func (f *fakeTaskListManager) UpdateTaskStatus(_ context.Context, id string, newStatusID int) error {
	if f.updateStatErr != nil {
		return f.updateStatErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	wf, ok := f.workflows[task.TaskListID]
	if !ok {
		return fmt.Errorf("workflow not found for task list: %s", task.TaskListID)
	}

	if task.StatusID == newStatusID {
		return nil
	}

	var statuses []database.TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(wf.Statuses), &statuses); err != nil {
		return err
	}

	toExists := false
	for _, s := range statuses {
		if s.ID == newStatusID {
			toExists = true
			break
		}
	}
	if !toExists {
		labels := make([]string, len(statuses))
		for i, s := range statuses {
			labels[i] = fmt.Sprintf("%d (%s)", s.ID, s.Label)
		}
		return fmt.Errorf("status destino %d não existe no workflow. Status válidos: %s",
			newStatusID, strings.Join(labels, ", "))
	}

	var transitions database.TaskListWorkflowTransitions
	if err := json.Unmarshal([]byte(wf.AllowedTransitions), &transitions); err != nil {
		return err
	}

	allowedStatuses, fromExists := transitions[task.StatusID]
	if !fromExists {
		task.StatusID = newStatusID
		return nil
	}

	for _, sid := range allowedStatuses {
		if sid == newStatusID {
			task.StatusID = newStatusID
			return nil
		}
	}

	return fmt.Errorf("transição de status %d para %d não é permitida pelo workflow", task.StatusID, newStatusID)
}

func (f *fakeTaskListManager) DeleteTask(_ context.Context, id string) error {
	if f.deleteTaskErr != nil {
		return f.deleteTaskErr
	}
	if _, ok := f.tasks[id]; !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	delete(f.tasks, id)
	return nil
}

func (f *fakeTaskListManager) MoveTaskToList(_ context.Context, taskID string, targetTaskListID string) (*database.Task, error) {
	task, ok := f.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.TaskListID != targetTaskListID {
		pol, _ := fakeListPolicy(f, targetTaskListID)
		if err := database.ValidateTaskCodeAgainstPolicy(task.Code, pol); err != nil {
			return nil, err
		}
	}
	wf, ok := f.workflows[targetTaskListID]
	if !ok {
		return nil, fmt.Errorf("workflow not found for task list: %s", targetTaskListID)
	}
	task.TaskListID = targetTaskListID
	task.StatusID = wf.InitialStatusID
	task.ParentID = nil
	return task, nil
}

func (f *fakeTaskListManager) GetWorkflow(_ context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	if f.getWorkflowErr != nil {
		return nil, f.getWorkflowErr
	}
	wf, ok := f.workflows[taskListID]
	if !ok {
		return nil, fmt.Errorf("workflow not found for task list: %s", taskListID)
	}
	return wf, nil
}

func (f *fakeTaskListManager) CreateTaskNote(_ context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	if f.createNoteErr != nil {
		return nil, f.createNoteErr
	}
	if _, ok := f.tasks[taskID]; !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	id := fmt.Sprintf("%d", f.nextNoteID)
	f.nextNoteID++
	note := database.TaskNote{
		TaskID:     taskID,
		Type:       noteType,
		Content:    content,
		AuthorName: authorName,
		AuthorID:   authorID,
	}
	note.ID = id
	f.notes[taskID] = append(f.notes[taskID], note)
	sl := f.notes[taskID]
	return &sl[len(sl)-1], nil
}

func (f *fakeTaskListManager) UpsertTaskNoteByExternal(_ context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	if f.createNoteErr != nil {
		return nil, false, f.createNoteErr
	}
	task, ok := f.tasks[p.TaskID]
	if !ok {
		return nil, false, fmt.Errorf("task not found: %s", p.TaskID)
	}
	src := strings.TrimSpace(p.ExternalSource)
	ext := strings.TrimSpace(p.ExternalID)
	if src == "" || ext == "" {
		return nil, false, fmt.Errorf("external_source e external_id são obrigatórios")
	}
	pol, _ := fakeListPolicy(f, task.TaskListID)
	if err := database.ValidateExternalNoteAgainstPolicy(src, ext, strings.TrimSpace(p.ExternalParentID), pol); err != nil {
		return nil, false, err
	}
	key := src + "\x00" + ext

	if noteID, ok := f.extNoteIndex[key]; ok {
		for tid, notes := range f.notes {
			for i := range notes {
				if notes[i].ID != noteID {
					continue
				}
				if tid != p.TaskID {
					return nil, false, fmt.Errorf("nota com source=%q external_id=%q já existe na task %s; recusado vincular à task %s", src, ext, tid, p.TaskID)
				}
				n := &f.notes[tid][i]
				n.Content = p.Content
				n.AuthorName = strings.TrimSpace(p.AuthorName)
				n.AuthorID = strings.TrimSpace(p.AuthorID)
				n.ExternalSource = src
				n.ExternalID = ext
				n.ExternalParentID = strings.TrimSpace(p.ExternalParentID)
				n.ExternalUpdatedAt = p.ExternalUpdatedAt
				if p.Type != nil {
					n.Type = *p.Type
				}
				return n, false, nil
			}
		}
	}

	if p.Type == nil {
		return nil, false, fmt.Errorf("type é obrigatório ao criar nota externa nova")
	}

	id := fmt.Sprintf("%d", f.nextNoteID)
	f.nextNoteID++
	note := database.TaskNote{
		TaskID:            p.TaskID,
		Type:              *p.Type,
		Content:           p.Content,
		AuthorName:        strings.TrimSpace(p.AuthorName),
		AuthorID:          strings.TrimSpace(p.AuthorID),
		ExternalSource:    src,
		ExternalID:        ext,
		ExternalParentID:  strings.TrimSpace(p.ExternalParentID),
		ExternalUpdatedAt: p.ExternalUpdatedAt,
	}
	note.ID = id
	f.notes[p.TaskID] = append(f.notes[p.TaskID], note)
	f.extNoteIndex[key] = id
	sl := f.notes[p.TaskID]
	return &sl[len(sl)-1], true, nil
}

func (f *fakeTaskListManager) UpdateTaskNote(_ context.Context, noteID string, content string) error {
	for taskID, notes := range f.notes {
		for i, n := range notes {
			if n.ID == noteID {
				f.notes[taskID][i].Content = content
				return nil
			}
		}
	}
	return fmt.Errorf("note not found: %s", noteID)
}

func (f *fakeTaskListManager) GetTaskNotes(_ context.Context, taskID string) ([]database.TaskNote, error) {
	if f.getNotesErr != nil {
		return nil, f.getNotesErr
	}
	return f.notes[taskID], nil
}

func (f *fakeTaskListManager) GetTaskNote(_ context.Context, noteID string) (*database.TaskNote, error) {
	for _, notes := range f.notes {
		for i, n := range notes {
			if n.ID == noteID {
				return &notes[i], nil
			}
		}
	}
	return nil, fmt.Errorf("note not found: %s", noteID)
}

func (f *fakeTaskListManager) UpdateTaskListFull(_ context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	tl, ok := f.taskLists[id]
	if !ok {
		return fmt.Errorf("task list not found: %s", id)
	}
	tl.Title = title
	tl.Description = description
	if preferredViewMode == "list" || preferredViewMode == "kanban" {
		tl.PreferredViewMode = preferredViewMode
	}
	if slug != nil {
		s := database.NormalizeTaskListSlug(*slug)
		if err := database.ValidateTaskListSlugFormat(s); err != nil {
			return err
		}
		if s != "" {
			if oid := f.findTaskListIDBySlug(s); oid != "" && oid != id {
				return fmt.Errorf("slug %q já está em uso por outra lista", s)
			}
		}
		tl.Slug = s
	}
	return nil
}

func (f *fakeTaskListManager) SetTaskListConversation(_ context.Context, id string, conversationID *string) error {
	tl, ok := f.taskLists[id]
	if !ok {
		return fmt.Errorf("task list not found: %s", id)
	}
	tl.ConversationID = conversationID
	return nil
}

func (f *fakeTaskListManager) ResolveTaskListRef(_ context.Context, taskListID *string, taskListSlug string) (string, error) {
	var idVal string
	if taskListID != nil {
		idVal = *taskListID
	}
	s := database.NormalizeTaskListSlug(taskListSlug)
	hasID := idVal != ""
	hasSlug := s != ""
	if !hasID && !hasSlug {
		return "", fmt.Errorf("informe task_list_id ou task_list_slug")
	}
	if hasID && !hasSlug {
		if _, ok := f.taskLists[idVal]; !ok {
			return "", fmt.Errorf("task_list_id %s não encontrado", idVal)
		}
		return idVal, nil
	}
	if !hasID && hasSlug {
		id := f.findTaskListIDBySlug(s)
		if id == "" {
			return "", fmt.Errorf("task_list_slug %q não encontrado", strings.TrimSpace(taskListSlug))
		}
		return id, nil
	}
	if _, ok := f.taskLists[idVal]; !ok {
		return "", fmt.Errorf("task_list_id %s não encontrado", idVal)
	}
	sid := f.findTaskListIDBySlug(s)
	if sid == "" {
		return "", fmt.Errorf("task_list_slug %q não encontrado", strings.TrimSpace(taskListSlug))
	}
	if idVal != sid {
		return "", fmt.Errorf("task_list_id %s e task_list_slug %q referem listas diferentes", idVal, strings.TrimSpace(taskListSlug))
	}
	return idVal, nil
}

func (f *fakeTaskListManager) ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	codeTrim := strings.TrimSpace(code)
	var idVal string
	if taskID != nil {
		idVal = *taskID
	}
	hasID := idVal != ""
	hasCode := codeTrim != ""
	listPtr := taskListID
	if listPtr != nil && *listPtr == "" {
		listPtr = nil
	}
	hasListRef := listPtr != nil || strings.TrimSpace(taskListSlug) != ""

	if !hasID && !hasCode {
		return "", fmt.Errorf("informe task_id ou code")
	}
	if hasCode && !hasID && !hasListRef {
		return "", fmt.Errorf("com code é necessário task_list_id ou task_list_slug")
	}

	if hasID && !hasCode {
		task, ok := f.tasks[idVal]
		if !ok {
			return "", fmt.Errorf("task_id %s não encontrado", idVal)
		}
		return task.ID, nil
	}

	if !hasID && hasCode {
		listID, err := f.ResolveTaskListRef(ctx, listPtr, taskListSlug)
		if err != nil {
			return "", err
		}
		for _, task := range f.tasks {
			if task.TaskListID == listID && task.Code == codeTrim {
				return task.ID, nil
			}
		}
		return "", fmt.Errorf("nenhuma task com code %q na lista", codeTrim)
	}

	task, ok := f.tasks[idVal]
	if !ok {
		return "", fmt.Errorf("task_id %s não encontrado", idVal)
	}
	if task.Code != codeTrim {
		return "", fmt.Errorf("task_id %s e code %q não correspondem à mesma task", idVal, codeTrim)
	}
	if hasListRef {
		listID, err := f.ResolveTaskListRef(ctx, listPtr, taskListSlug)
		if err != nil {
			return "", err
		}
		if task.TaskListID != listID {
			return "", fmt.Errorf("task_id %s e lista referenciada não correspondem à mesma task", idVal)
		}
	}
	return task.ID, nil
}

func (f *fakeTaskListManager) ResolveTaskIDByTaskCode(_ context.Context, taskListID *string, taskCode string) (string, error) {
	codeTrim := strings.TrimSpace(taskCode)
	if codeTrim == "" {
		return "", fmt.Errorf("task_code não pode ser vazio")
	}
	var matches []string
	for _, task := range f.tasks {
		if task.Code != codeTrim {
			continue
		}
		if taskListID != nil && *taskListID != "" && task.TaskListID != *taskListID {
			continue
		}
		matches = append(matches, task.ID)
	}
	switch len(matches) {
	case 0:
		if taskListID != nil && *taskListID != "" {
			return "", fmt.Errorf("nenhuma task com task_code %q na lista %s", codeTrim, *taskListID)
		}
		return "", fmt.Errorf("nenhuma task com task_code %q", codeTrim)
	case 1:
		return matches[0], nil
	default:
		if taskListID != nil && *taskListID != "" {
			return "", fmt.Errorf("múltiplas tasks com task_code %q na lista %s", codeTrim, *taskListID)
		}
		return "", fmt.Errorf("várias tasks com task_code %q; informe task_list_id ou task_list_slug para restringir à lista", codeTrim)
	}
}

func (f *fakeTaskListManager) SetTaskListValidationPolicy(_ context.Context, id string, policyJSON string) error {
	tl, ok := f.taskLists[id]
	if !ok {
		return fmt.Errorf("task list not found: %s", id)
	}
	s := strings.TrimSpace(policyJSON)
	if s != "" {
		if _, err := database.ParseTaskListValidationPolicyJSON(s); err != nil {
			return err
		}
	}
	tl.ValidationPolicy = s
	return nil
}

func (f *fakeTaskListManager) GetTaskListCustomActions(_ context.Context, id string) (*database.TaskListCustomActions, error) {
	tl, ok := f.taskLists[id]
	if !ok {
		return nil, fmt.Errorf("task list not found: %s", id)
	}
	return database.ParseTaskListCustomActionsJSON(tl.CustomActions)
}

func (f *fakeTaskListManager) SetTaskListCustomActions(_ context.Context, id string, actionsJSON string) error {
	tl, ok := f.taskLists[id]
	if !ok {
		return fmt.Errorf("task list not found: %s", id)
	}
	s := strings.TrimSpace(actionsJSON)
	if s != "" {
		if _, err := database.ParseTaskListCustomActionsJSON(s); err != nil {
			return err
		}
	}
	tl.CustomActions = s
	return nil
}

func fakeListPolicy(f *fakeTaskListManager, taskListID string) (*database.TaskListValidationPolicy, error) {
	tl, ok := f.taskLists[taskListID]
	if !ok {
		return nil, fmt.Errorf("task list not found: %s", taskListID)
	}
	return database.ParseTaskListValidationPolicyJSON(tl.ValidationPolicy)
}

func (f *fakeTaskListManager) UpdateWorkflowFull(_ context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	wf, ok := f.workflows[taskListID]
	if !ok {
		return fmt.Errorf("workflow not found for task list: %s", taskListID)
	}

	statusIDs := make(map[int]bool, len(statuses))
	for _, s := range statuses {
		statusIDs[s.ID] = true
	}

	if !statusIDs[initialStatusID] {
		return fmt.Errorf("initial_status_id %d não existe nos statuses fornecidos", initialStatusID)
	}

	for fromID, toIDs := range transitions {
		if !statusIDs[fromID] {
			return fmt.Errorf("transição referencia status inexistente: %d", fromID)
		}
		for _, toID := range toIDs {
			if !statusIDs[toID] {
				return fmt.Errorf("transição de %d referencia status inexistente: %d", fromID, toID)
			}
		}
	}

	// Check tasks using removed statuses
	for _, task := range f.tasks {
		if task.TaskListID != taskListID {
			continue
		}
		if statusIDs[task.StatusID] {
			continue
		}
		if statusMigration != nil {
			if newID, ok := statusMigration[task.StatusID]; ok {
				if statusIDs[newID] {
					task.StatusID = newID
					continue
				}
			}
		}
		return fmt.Errorf("status_id %d está em uso e não existe nos novos statuses", task.StatusID)
	}

	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)
	wf.Statuses = string(statusesJSON)
	wf.AllowedTransitions = string(transitionsJSON)
	wf.InitialStatusID = initialStatusID

	// Update tasklist workflow reference
	if tl, ok := f.taskLists[taskListID]; ok {
		tl.Workflow = wf
	}
	return nil
}

func (f *fakeTaskListManager) GetTaskCountsByStatus(_ context.Context, taskListID string) (map[int]int64, error) {
	counts := make(map[int]int64)
	for _, task := range f.tasks {
		if task.TaskListID == taskListID {
			counts[task.StatusID]++
		}
	}
	return counts, nil
}

// ==================== Helper ====================

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ==================== GetTaskList Tests (consolidated: list all, full details, summary) ====================

func TestGetTaskList_ParametersValidJSON(t *testing.T) {
	tool := NewTaskList(nil)
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() is not valid JSON: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["custom_actions"]; !ok {
		t.Fatalf("expected 'custom_actions' in tool parameters schema")
	}
}

func TestGetTaskList_Name(t *testing.T) {
	tool := NewTaskList(nil)
	if tool.Name() != "task_list" {
		t.Fatalf("expected 'task_list', got '%s'", tool.Name())
	}
}

func TestGetTaskList_ListAll_Empty(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No task lists") {
		t.Fatalf("expected 'No task lists', got: %s", result.Content)
	}
}

func TestGetTaskList_ListAll_WithItems(t *testing.T) {
	mgr := newFakeManager()
	mgr.addTaskList("List 1", defaultStatuses())
	mgr.addTaskList("List 2", defaultStatuses())
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2 task list") {
		t.Fatalf("expected 2 task lists, got: %s", result.Content)
	}
}

func TestGetTaskList_FullDetails(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test List", defaultStatuses())
	mgr.addTask(tl.ID, "Task 1", 1)
	mgr.addTask(tl.ID, "Task 2", 2)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": tl.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Test List") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "summary") {
		t.Fatalf("expected content to contain summary stats, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "\"total\"") {
		t.Fatalf("expected content to contain total count, got: %s", result.Content)
	}
}

func TestGetTaskList_NotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": "999"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task list")
	}
}

func TestTaskList_ConversationLink(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	// Cria lista já vinculada a uma conversa, com descrição.
	createRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":           "Linked List",
		"description":     "important desc",
		"conversation_id": "conv-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if createRes.IsError {
		t.Fatalf("unexpected error creating linked list: %s", createRes.Content)
	}
	if !strings.Contains(createRes.Content, "conv-1") {
		t.Fatalf("expected conversation_id in create output, got: %s", createRes.Content)
	}

	var created *database.TaskList
	for _, tl := range mgr.taskLists {
		if tl.Title == "Linked List" {
			created = tl
		}
	}
	if created == nil {
		t.Fatal("created list not found in fake manager")
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-1" {
		t.Fatalf("expected list linked to conv-1, got: %v", created.ConversationID)
	}

	// Update apenas-do-vínculo não pode sobrescrever title/description.
	updRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":    created.ID,
		"conversation_id": "conv-2",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updRes.IsError {
		t.Fatalf("unexpected error on conversation-only update: %s", updRes.Content)
	}
	if created.Title != "Linked List" || created.Description != "important desc" {
		t.Fatalf("conversation-only update should preserve title/description, got: %q / %q", created.Title, created.Description)
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-2" {
		t.Fatalf("expected list re-linked to conv-2, got: %v", created.ConversationID)
	}

	// Limpar o vínculo passando string vazia.
	clrRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":    created.ID,
		"conversation_id": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if clrRes.IsError {
		t.Fatalf("unexpected error clearing conversation link: %s", clrRes.Content)
	}
	if created.ConversationID != nil {
		t.Fatalf("expected conversation link cleared, got: %v", *created.ConversationID)
	}
}

func TestTaskList_DuplicateConversationInheritance(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)
	conv := "conv-src"
	src := mgr.addTaskList("Source", defaultStatuses())
	src.ConversationID = &conv

	// Duplicação sem conversation_id herda o vínculo da origem.
	if _, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": src.ID,
		"duplicate":    true,
		"title":        "Copy Inherits",
	})); err != nil {
		t.Fatal(err)
	}
	var inherited *database.TaskList
	for _, tl := range mgr.taskLists {
		if tl.Title == "Copy Inherits" {
			inherited = tl
		}
	}
	if inherited == nil {
		t.Fatal("inherited copy not found")
	}
	if inherited.ConversationID == nil || *inherited.ConversationID != conv {
		t.Fatalf("expected duplicate to inherit conv-src, got: %v", inherited.ConversationID)
	}

	// Duplicação com conversation_id explícito sobrescreve a herança.
	if _, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":    src.ID,
		"duplicate":       true,
		"title":           "Copy Override",
		"conversation_id": "conv-other",
	})); err != nil {
		t.Fatal(err)
	}
	var override *database.TaskList
	for _, tl := range mgr.taskLists {
		if tl.Title == "Copy Override" {
			override = tl
		}
	}
	if override == nil {
		t.Fatal("override copy not found")
	}
	if override.ConversationID == nil || *override.ConversationID != "conv-other" {
		t.Fatalf("expected duplicate to override with conv-other, got: %v", override.ConversationID)
	}
}

func TestGetTaskList_ZeroID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetTaskList_SummaryOnly(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Status Test", defaultStatuses())
	mgr.addTask(tl.ID, "Task 1", 1)
	mgr.addTask(tl.ID, "Task 2", 2)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"summary_only": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Status Test") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
}

func TestGetTaskList_SummaryOnly_NotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "999",
		"summary_only": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error")
	}
}

func TestGetTaskList_SummaryOnly_WithoutID_Error(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"summary_only": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when summary_only without task_list_id")
	}
	if !strings.Contains(result.Content, "summary_only requires task_list_id or task_list_slug") {
		t.Errorf("expected summary_only ref error, got: %s", result.Content)
	}
}

func TestTask_ReadNoNotes(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task 1") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "notes") {
		t.Fatalf("expected no notes key in output when empty, got: %s", result.Content)
	}
}

func TestTask_ReadWithNotes(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)

	mgr.CreateTaskNote(context.Background(), task.ID, 1, "First note", "Alice", "")     //nolint:errcheck
	mgr.CreateTaskNote(context.Background(), task.ID, 2, "Customer replied", "Bob", "") //nolint:errcheck

	tool := NewTask(mgr)
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task 1") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "First note") {
		t.Fatalf("expected 'First note' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Customer replied") {
		t.Fatalf("expected 'Customer replied' in content, got: %s", result.Content)
	}
}

func TestTask_ReadIncludesFields(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Detailed Task", 1)
	task.Description = "Some description"
	task.Code = "FSD-123"
	task.Link = "https://example.com"
	task.AssigneeName = "Alice"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	for _, expected := range []string{"Detailed Task", "Some description", "FSD-123", "https://example.com", "Alice"} {
		if !strings.Contains(result.Content, expected) {
			t.Errorf("expected '%s' in content, got: %s", expected, result.Content)
		}
	}
}

func TestTask_ConversationLink(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	// Cria task já vinculada a uma conversa.
	createRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":    tl.ID,
		"title":           "Linked task",
		"conversation_id": "conv-123",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if createRes.IsError {
		t.Fatalf("unexpected error creating linked task: %s", createRes.Content)
	}
	if !strings.Contains(createRes.Content, "conv-123") {
		t.Fatalf("expected conversation_id in create output, got: %s", createRes.Content)
	}

	var created *database.Task
	for _, tk := range mgr.tasks {
		if tk.Title == "Linked task" {
			created = tk
		}
	}
	if created == nil {
		t.Fatal("created task not found in fake manager")
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-123" {
		t.Fatalf("expected task linked to conv-123, got: %v", created.ConversationID)
	}

	// Atualização somente do vínculo (sem title) deve preservar os demais campos.
	updRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":         created.ID,
		"conversation_id": "conv-456",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updRes.IsError {
		t.Fatalf("unexpected error on conversation-only update: %s", updRes.Content)
	}
	if created.Title != "Linked task" {
		t.Fatalf("conversation-only update should preserve title, got: %q", created.Title)
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-456" {
		t.Fatalf("expected task re-linked to conv-456, got: %v", created.ConversationID)
	}

	// Limpar o vínculo passando string vazia.
	clrRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":         created.ID,
		"conversation_id": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if clrRes.IsError {
		t.Fatalf("unexpected error clearing conversation link: %s", clrRes.Content)
	}
	if created.ConversationID != nil {
		t.Fatalf("expected conversation link cleared, got: %v", *created.ConversationID)
	}
}

func TestTask_ReadNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": "999"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestTask_ReadZeroID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty task ID")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestTask_ReadByListSlugAndCode(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Bugs", defaultStatuses())
	tl.Slug = "bugs"
	task := mgr.addTask(tl.ID, "Fix it", 1)
	task.Code = "FSD-99"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
		"code":           "FSD-99",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Fix it") {
		t.Errorf("expected title in response, got: %s", result.Content)
	}
}

// ==================== UpsertTask Tests ====================

func TestUpsertTask_Name(t *testing.T) {
	tool := NewTask(nil)
	if tool.Name() != "task" {
		t.Fatalf("expected 'upsert_task', got '%s'", tool.Name())
	}
}

func TestUpsertTask_Create(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "New Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_Update(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Old Title", 1)
	tool := NewTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "New Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_InvalidStatus(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	statusID := 99
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid status_id")
	}
	if !strings.Contains(result.Content, "invalid status_id") {
		t.Fatalf("expected 'invalid status_id' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_EmptyTitle(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestUpsertTask_WithStatus(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "In Progress Task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

// ==================== Dedup by Code Tests ====================

func TestUpsertTask_DedupByCode_CreatesWhenNew(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Ticket Alpha",
		"code":         "PROJ-100",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result.Content)
	}
}

func TestUpsertTask_DedupByCode_UpdatesExisting(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewTask(mgr)

	// First call: creates
	result1, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Original Title",
		"code":         "PROJ-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result1.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result1.Content)
	}

	// Second call: same code, different title — should update
	result2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Updated Title",
		"code":         "PROJ-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result2.Content)
	}

	// Verify only one task exists with that code
	count := 0
	for _, task := range mgr.tasks {
		if task.Code == "PROJ-200" {
			count++
			if task.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got '%s'", task.Title)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 task with code PROJ-200, got %d", count)
	}
}

func TestUpsertTask_DedupByCode_SameCodeDifferentLists(t *testing.T) {
	mgr := newFakeManager()
	tl1 := mgr.addTaskList("List A", defaultStatuses())
	tl2 := mgr.addTaskList("List B", defaultStatuses())
	tool := NewTask(mgr)

	// Create in list A
	r1, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl1.ID,
		"title":        "Task in A",
		"code":         "PROJ-300",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r1.Content, "created") {
		t.Fatalf("expected 'created' in list A, got: %s", r1.Content)
	}

	// Create in list B with same code — should create (not update A's task)
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl2.ID,
		"title":        "Task in B",
		"code":         "PROJ-300",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r2.Content, "created") {
		t.Fatalf("expected 'created' in list B, got: %s", r2.Content)
	}

	// Verify two tasks exist with code PROJ-300 in different lists
	count := 0
	for _, task := range mgr.tasks {
		if task.Code == "PROJ-300" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 tasks with code PROJ-300 in different lists, got %d", count)
	}
}

func TestUpsertTask_DedupByCode_EmptyCodeAlwaysCreates(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("No Code", defaultStatuses())
	tool := NewTask(mgr)

	// Two creates without code — both should create new tasks
	r1, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task A",
	}))
	r2, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task B",
	}))

	if !strings.Contains(r1.Content, "created") {
		t.Fatalf("expected first 'created', got: %s", r1.Content)
	}
	if !strings.Contains(r2.Content, "created") {
		t.Fatalf("expected second 'created', got: %s", r2.Content)
	}
}

func TestUpsertTask_DedupByCode_TaskIdTakesPrecedence(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Precedence", defaultStatuses())
	tool := NewTask(mgr)

	// Create two tasks with different codes
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "First",
		"code":         "CODE-A",
	}))
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Second",
		"code":         "CODE-B",
	}))

	// Find task with CODE-A
	var taskA *database.Task
	for _, task := range mgr.tasks {
		if task.Code == "CODE-A" {
			taskA = task
			break
		}
	}
	if taskA == nil {
		t.Fatal("task with CODE-A not found")
	}

	// Update by task_id with a different code — task_id should take precedence
	taskID := taskA.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated by ID",
		"code":         "CODE-C",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result.Content)
	}
	if taskA.Title != "Updated by ID" {
		t.Errorf("expected title 'Updated by ID', got '%s'", taskA.Title)
	}
	if taskA.Code != "CODE-C" {
		t.Errorf("expected code 'CODE-C', got '%s'", taskA.Code)
	}
}

func TestUpsertTask_DedupByCode_UpdatesStatusToo(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Status", defaultStatuses())
	tool := NewTask(mgr)

	// Create task
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Work Item",
		"code":         "WI-500",
	}))

	// Update via code with new status
	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Work Item Updated",
		"code":         "WI-500",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result.Content)
	}

	// Verify status was updated
	for _, task := range mgr.tasks {
		if task.Code == "WI-500" {
			if task.StatusID != statusID {
				t.Errorf("expected status_id %d, got %d", statusID, task.StatusID)
			}
			break
		}
	}
}

// ==================== DeleteTask Tests ====================

func TestUpsertTask_DeleteSuccess(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Doomed Task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deleted") {
		t.Fatalf("expected 'deleted' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Doomed Task") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
}

func TestUpsertTask_DeleteNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "1",
		"task_id":      "999",
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestUpsertTask_DeleteWithoutTaskID_Error(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "1",
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when delete without task_id")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestUpsertTask_DeleteAndDuplicate_Error(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"delete":       true,
		"duplicate":    true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when delete and duplicate combined")
	}
	if !strings.Contains(result.Content, "cannot be used together") {
		t.Errorf("expected 'cannot be used together', got: %s", result.Content)
	}
}

// ==================== UpsertTaskNote Tests ====================

func TestUpsertTaskNote_Name(t *testing.T) {
	tool := NewTaskNote(nil)
	if tool.Name() != "task_note" {
		t.Fatalf("expected 'task_note', got '%s'", tool.Name())
	}
}

func TestUpsertTaskNote_CreateSuccess(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":     task.ID,
		"type":        1,
		"content":     "This is an internal note",
		"author_name": "Test User",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "internal note") {
		t.Fatalf("expected 'internal note' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_CreateWithListSlugAndCode(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Bugs", defaultStatuses())
	tl.Slug = "bugs"
	task := mgr.addTask(tl.ID, "Task", 1)
	task.Code = "J-1"
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
		"code":           "J-1",
		"type":           1,
		"content":        "via slug+code",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "via slug+code" {
		t.Fatalf("expected one note on task, got %+v", notes)
	}
}

func TestUpsertTaskNote_CreateCustomerType(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    2,
		"content": "Customer replied",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "customer response") {
		t.Fatalf("expected 'customer response' in content, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_UpdateSuccess(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)

	note, _ := mgr.CreateTaskNote(context.Background(), task.ID, 1, "Original content", "Alice", "")
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"note_id": note.ID,
		"content": "Updated content",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated' in content, got: %s", result.Content)
	}
	updated, _ := mgr.GetTaskNote(context.Background(), note.ID)
	if updated.Content != "Updated content" {
		t.Errorf("expected content 'Updated content', got '%s'", updated.Content)
	}
}

func TestUpsertTaskNote_UpdateNotFound(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"note_id": "999",
		"content": "Updated content",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent note")
	}
}

func TestUpsertTaskNote_UpdateWrongTask(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task1 := mgr.addTask(tl.ID, "Task 1", 1)
	task2 := mgr.addTask(tl.ID, "Task 2", 1)

	note, _ := mgr.CreateTaskNote(context.Background(), task1.ID, 1, "Note on task 1", "Alice", "")
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task2.ID,
		"note_id": note.ID,
		"content": "Trying to update via wrong task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when note does not belong to task")
	}
	if !strings.Contains(result.Content, "does not match") {
		t.Errorf("expected mismatch error, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_EmptyContent(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    1,
		"content": "  ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty content")
	}
}

func TestUpsertTaskNote_InvalidType(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    5,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid type")
	}
}

func TestUpsertTaskNote_ZeroTaskID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": "",
		"type":    1,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty task ID")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_TaskNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": "999",
		"type":    1,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestUpsertTaskNote_CreateWithoutType_Error(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"content": "note without type",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when type not provided for create")
	}
	if !strings.Contains(result.Content, "type is required") {
		t.Errorf("expected 'type is required', got: %s", result.Content)
	}
}

func TestUpsertTaskNote_ExternalIdempotentTwice(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	base := map[string]any{
		"task_id":             task.ID,
		"type":                2,
		"source":              "jira",
		"external_id":         "comment-98765",
		"external_parent_id":  "FSD-123",
		"author_name":         "Fulano",
		"author_id":           "abc",
		"content":             "Comentário vindo do Jira",
		"external_updated_at": "2026-04-08T12:00:00Z",
	}
	r1, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r1.IsError {
		t.Fatalf("first upsert: %v %s", err, r1.Content)
	}
	id1 := metadataNoteID(t, r1.Metadata)
	if id1 == "" {
		t.Fatalf("expected note_id in metadata: %#v", r1.Metadata)
	}
	if r1.Metadata["action"] != "created" {
		t.Fatalf("expected created, got %#v", r1.Metadata)
	}

	base["content"] = "Comentário vindo do Jira (editado)"
	r2, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r2.IsError {
		t.Fatalf("second upsert: %v %s", err, r2.Content)
	}
	id2 := metadataNoteID(t, r2.Metadata)
	if id1 != id2 {
		t.Fatalf("expected same note id, got %v then %v", id1, id2)
	}
	if r2.Metadata["action"] != "updated" {
		t.Fatalf("expected updated, got %#v", r2.Metadata)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "Comentário vindo do Jira (editado)" {
		t.Fatalf("content: %q", notes[0].Content)
	}
}

func TestUpsertTaskNote_ExternalUpdateWithoutType(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	_, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "type": 1, "content": "v1",
		"source": "jira", "external_id": "c1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "content": "v2",
		"source": "jira", "external_id": "c1",
	}))
	if err != nil || r2.IsError {
		t.Fatalf("update without type: %v %s", err, r2.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "v2" {
		t.Fatalf("notes: %+v", notes)
	}
	if notes[0].Type != database.TaskNoteInternal {
		t.Fatalf("type should stay internal, got %d", notes[0].Type)
	}
}

func TestUpsertTaskNote_ExternalRequiresBothKeys(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "type": 1, "content": "x",
		"source": "jira",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "both source and external_id") {
		t.Fatalf("expected both keys error, got: %s", r.Content)
	}
}

func TestUpsertTaskNote_ExternalConflictDifferentTask(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task1 := mgr.addTask(tl.ID, "T1", 1)
	task2 := mgr.addTask(tl.ID, "T2", 1)
	tool := NewTaskNote(mgr)

	_, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task1.ID, "type": 1, "content": "on t1",
		"source": "jira", "external_id": "same",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task2.ID, "type": 1, "content": "on t2",
		"source": "jira", "external_id": "same",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r2.IsError || !strings.Contains(r2.Content, "já existe") {
		t.Fatalf("expected conflict error, got: %s", r2.Content)
	}
}

func TestUpsertTaskNote_ByTaskCode_ManualCreate(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, err := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-12345", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "FSD-12345",
		"type":      2,
		"content":   "nota via code",
	}))
	if err != nil || r.IsError {
		t.Fatalf("execute: %v %s", err, r.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "nota via code" {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestUpsertTaskNote_ExternalByTaskCode_Idempotent(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, err := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-12345", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := NewTaskNote(mgr)

	base := map[string]any{
		"task_code":           "FSD-12345",
		"type":                2,
		"source":              "jira",
		"external_id":         "10001",
		"external_parent_id":  "FSD-12345",
		"external_updated_at": "2026-04-08T19:00:00Z",
		"author_name":         "Jane Doe",
		"content":             "Comentário sincronizado",
	}
	r1, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r1.IsError {
		t.Fatalf("first: %v %s", err, r1.Content)
	}
	id1 := metadataNoteID(t, r1.Metadata)
	base["content"] = "Comentário sincronizado (v2)"
	r2, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r2.IsError {
		t.Fatalf("second: %v %s", err, r2.Content)
	}
	id2 := metadataNoteID(t, r2.Metadata)
	if id1 != id2 {
		t.Fatalf("expected same note id")
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "Comentário sincronizado (v2)" {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestUpsertTaskNote_TaskCodeNotFound(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	_, _ = mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "OTHER", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "NOPE-1",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "nenhuma task com task_code") {
		t.Fatalf("expected not found: %s", r.Content)
	}
}

func TestUpsertTaskNote_TaskCodeAmbiguous(t *testing.T) {
	mgr := newFakeManager()
	tl1 := mgr.addTaskList("A", defaultStatuses())
	tl2 := mgr.addTaskList("B", defaultStatuses())
	_, _ = mgr.CreateTask(context.Background(), tl1.ID, "t", "", "SAME", "", nil)
	_, _ = mgr.CreateTask(context.Background(), tl2.ID, "t", "", "SAME", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "SAME",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "várias tasks com task_code") {
		t.Fatalf("expected ambiguous: %s", r.Content)
	}
}

func TestUpsertTaskNote_TaskCodeWithListSlug_Disambiguates(t *testing.T) {
	mgr := newFakeManager()
	tl1 := mgr.addTaskList("A", defaultStatuses())
	tl1.Slug = "lista-a"
	tl2 := mgr.addTaskList("B", defaultStatuses())
	tl2.Slug = "lista-b"
	taskB, _ := mgr.CreateTask(context.Background(), tl2.ID, "t", "", "KEY", "", nil)
	_, _ = mgr.CreateTask(context.Background(), tl1.ID, "t", "", "KEY", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "lista-b",
		"task_code":      "KEY",
		"type":           1,
		"content":        "scoped",
	}))
	if err != nil || r.IsError {
		t.Fatalf("execute: %v %s", err, r.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), taskB.ID)
	if len(notes) != 1 || notes[0].Content != "scoped" {
		t.Fatalf("expected note on task B, notes=%+v", notes)
	}
}

func TestUpsertTaskNote_TaskIDWithMismatchedTaskCode(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, _ := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-1", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":   task.ID,
		"task_code": "OTHER",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "não coincide") {
		t.Fatalf("expected mismatch: %s", r.Content)
	}
}

func TestTask_ReadNotesIncludeExternalFields(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	ts := time.Date(2026, 4, 8, 15, 30, 0, 0, time.UTC)
	_, _, _ = mgr.UpsertTaskNoteByExternal(context.Background(), database.UpsertTaskNoteByExternalParams{
		TaskID:            task.ID,
		Type:              ptrTaskNoteType(database.TaskNoteCustomer),
		Content:           "synced",
		ExternalSource:    "jira",
		ExternalID:        "c-1",
		ExternalParentID:  "FSD-9",
		ExternalUpdatedAt: &ts,
	})

	tool := NewTask(mgr)
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil || result.IsError {
		t.Fatalf("read: %v %s", err, result.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	rawNotes, ok := payload["notes"].([]any)
	if !ok || len(rawNotes) != 1 {
		t.Fatalf("notes: %#v", payload["notes"])
	}
	n := rawNotes[0].(map[string]any)
	if n["source"] != "jira" || n["external_id"] != "c-1" || n["external_parent_id"] != "FSD-9" {
		t.Fatalf("unexpected note fields: %#v", n)
	}
	if n["external_updated_at"] != ts.Format(time.RFC3339) {
		t.Fatalf("external_updated_at: %#v", n["external_updated_at"])
	}
}

func ptrTaskNoteType(t database.TaskNoteType) *database.TaskNoteType {
	return &t
}

func metadataNoteID(t *testing.T, md map[string]any) string {
	t.Helper()
	v, ok := md["note_id"]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		t.Fatalf("unexpected note_id type %T in metadata", v)
		return ""
	}
}

// ==================== UpsertTaskList Tests ====================

func TestUpsertTaskList_Name(t *testing.T) {
	tool := NewTaskList(nil)
	if tool.Name() != "task_list" {
		t.Fatalf("expected 'upsert_task_list', got '%s'", tool.Name())
	}
}

func TestUpsertTaskList_CreateSimple(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":       "My Project",
		"description": "A test project",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "My Project") {
		t.Fatalf("expected title in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_CreateWithCustomActions(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Suporte",
		"custom_actions": []map[string]any{
			{
				"id":       "refresh",
				"label":    "Atualizar",
				"icon":     "🔄",
				"surfaces": []string{"card_menu", "board_menu"},
				"event":    "tasklist.card.refresh",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	id, _ := result.Metadata["task_list_id"].(string)
	if id == "" {
		t.Fatalf("expected task_list_id in metadata, got: %#v", result.Metadata)
	}
	tl := mgr.taskLists[id]
	ca, err := database.ParseTaskListCustomActionsJSON(tl.CustomActions)
	if err != nil {
		t.Fatalf("stored custom_actions invalid: %v (raw=%s)", err, tl.CustomActions)
	}
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "refresh" {
		t.Fatalf("expected one action 'refresh', got: %#v", ca.Actions)
	}
	if !strings.Contains(result.Content, "custom_actions") {
		t.Fatalf("expected custom_actions echoed in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_UpdateCustomActionsClear(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.CustomActions = `{"actions":[{"id":"refresh","label":"Atualizar","event":"tasklist.card.refresh"}]}`
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":   tl.ID,
		"custom_actions": []map[string]any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.taskLists[tl.ID].CustomActions != "" {
		t.Fatalf("expected custom_actions cleared, got: %q", mgr.taskLists[tl.ID].CustomActions)
	}
}

func TestUpsertTaskList_CustomActionsInvalid(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Suporte",
		"custom_actions": []map[string]any{
			{"id": "bad id", "label": "X", "event": "tasklist.card.refresh"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for invalid action id, got: %s", result.Content)
	}
}

// TestUpsertTaskList_CustomActionsObjectRejected garante que custom_actions
// como objeto ({}) é rejeitado com erro, em vez de ser tratado como "limpar"
// (que mascararia um tipo errado como data-loss). Só [] limpa.
func TestUpsertTaskList_CustomActionsObjectRejected(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.CustomActions = `{"actions":[{"id":"refresh","label":"Atualizar","event":"tasklist.card.refresh"}]}`
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":   tl.ID,
		"custom_actions": map[string]any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for custom_actions as object {}, got: %s", result.Content)
	}
	if mgr.taskLists[tl.ID].CustomActions == "" {
		t.Fatalf("custom_actions should NOT be cleared by an invalid object {}")
	}
}

// TestGetTaskList_BySlugDoesNotMutate garante que uma chamada de leitura apenas
// com task_list_slug retorna os detalhes da lista SEM cair no caminho de
// escrita (que sobrescreveria description/view_mode com vazio). Regressão do
// bug em que task_list_slug sozinho era tratado como write.
func TestGetTaskList_BySlugDoesNotMutate(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.Slug = "bugs"
	tl.Description = "descrição importante"
	tl.PreferredViewMode = "kanban"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if action, ok := result.Metadata["action"]; ok {
		t.Fatalf("read-by-slug should not be a write, got action=%v", action)
	}
	got := mgr.taskLists[tl.ID]
	if got.Description != "descrição importante" {
		t.Fatalf("description was mutated by read-by-slug: %q", got.Description)
	}
	if got.PreferredViewMode != "kanban" {
		t.Fatalf("preferred_view_mode was mutated by read-by-slug: %q", got.PreferredViewMode)
	}
	if id, _ := result.Metadata["task_list_id"].(string); id != tl.ID {
		t.Fatalf("expected full details for %s, got metadata: %#v", tl.ID, result.Metadata)
	}
}

// TestCustomActionsToList_InvalidSurfacesError garante que JSON corrompido em
// custom_actions não some silenciosamente do echo: expõe um marcador de erro,
// coerente com validationPolicyToMap.
func TestCustomActionsToList_InvalidSurfacesError(t *testing.T) {
	// vazio -> omitido (nil)
	if out := customActionsToList("   "); out != nil {
		t.Fatalf("empty should be nil, got: %#v", out)
	}
	// lista vazia válida -> omitido (nil)
	if out := customActionsToList(`{"actions":[]}`); out != nil {
		t.Fatalf("empty actions should be nil, got: %#v", out)
	}
	// JSON inválido -> marcador de erro
	out := customActionsToList(`{"actions":[{"id":"x"`)
	if len(out) != 1 {
		t.Fatalf("invalid JSON should yield one marker entry, got: %#v", out)
	}
	if pe, _ := out[0]["_parse_error"].(bool); !pe {
		t.Fatalf("expected _parse_error marker, got: %#v", out[0])
	}
	if _, ok := out[0]["raw"]; !ok {
		t.Fatalf("expected raw in parse error marker, got: %#v", out[0])
	}
	// campo desconhecido (parser estrito) -> também é marcador de erro
	out2 := customActionsToList(`{"actions":[{"id":"x","label":"X","emits_event":"e"}]}`)
	if len(out2) != 1 {
		t.Fatalf("unknown field should yield marker, got: %#v", out2)
	}
	if pe, _ := out2[0]["_parse_error"].(bool); !pe {
		t.Fatalf("expected _parse_error marker for unknown field, got: %#v", out2[0])
	}
}

func TestUpsertTaskList_CreateWithCustomWorkflow(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Jira Mirror",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Aguardando Suporte", "color": "var(--color-warning)", "icon": "⏳"},
				{"id": 2, "label": "Aguardando Cliente", "color": "var(--color-info)", "icon": "👤"},
				{"id": 3, "label": "Resolvido", "color": "var(--color-success)", "icon": "✅"},
				{"id": 4, "label": "Concluído", "color": "var(--color-success)", "icon": "🏁"},
				{"id": 5, "label": "Cancelado", "color": "var(--color-danger)", "icon": "❌"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2, 3, 5},
				"2": []int{1, 3, 5},
				"3": []int{4, 1},
				"4": []int{},
				"5": []int{},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Jira Mirror") {
		t.Fatalf("expected title in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_CreateEmptyTitle(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":       "  ",
		"description": "forces write mode",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestUpsertTaskList_CreateInvalidWorkflow_DuplicateIDs(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Bad",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A"},
				{"id": 1, "label": "B"},
			},
			"allowed_transitions": map[string]any{"1": []int{}},
			"initial_status_id":   1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for duplicate status IDs")
	}
}

func TestUpsertTaskList_CreateInvalidWorkflow_BadInitialStatus(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Bad",
		"workflow": map[string]any{
			"statuses":            []map[string]any{{"id": 1, "label": "A"}},
			"allowed_transitions": map[string]any{"1": []int{}},
			"initial_status_id":   99,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid initial_status_id")
	}
}

func TestUpsertTaskList_UpdateMetadata(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Old Title", defaultStatuses())
	tool := NewTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "New Title",
		"description":  "Updated description",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated' in content, got: %s", result.Content)
	}
	if mgr.taskLists[tlID].Title != "New Title" {
		t.Errorf("expected title 'New Title', got '%s'", mgr.taskLists[tlID].Title)
	}
}

func TestUpsertTaskList_UpdateWorkflow(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	tool := NewTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Open"},
				{"id": 2, "label": "In Progress"},
				{"id": 3, "label": "Done"},
				{"id": 4, "label": "Cancelled"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2, 4},
				"2": []int{3, 4},
				"3": []int{},
				"4": []int{},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.workflows[tlID].InitialStatusID != 1 {
		t.Errorf("expected initial_status_id 1, got %d", mgr.workflows[tlID].InitialStatusID)
	}
}

func TestUpsertTaskList_UpdateWorkflow_RemoveStatusWithMigration(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A Fazer"},
				{"id": 3, "label": "Concluído"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{3},
				"3": []int{1},
			},
			"initial_status_id": 1,
			"status_migration":  map[string]any{"2": 1},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.TaskListID == tlID && task.StatusID == 2 {
			t.Error("expected task to be migrated from status 2")
		}
	}
}

func TestUpsertTaskList_UpdateWorkflow_RemoveStatusWithoutMigration_Fails(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A Fazer"},
				{"id": 3, "label": "Concluído"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{3},
				"3": []int{1},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when removing status that has tasks without migration")
	}
}

func TestUpsertTaskList_UpdateNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	id := "999"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": id,
		"title":        "Ghost",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task list")
	}
}

func TestUpsertTaskList_CreateWithViewMode(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":               "Kanban List",
		"preferred_view_mode": "kanban",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

// ==================== Duplicate TaskList Tests ====================

func TestUpsertTaskList_DuplicateBasic(t *testing.T) {
	mgr := newFakeManager()
	source := mgr.addTaskList("Source List", defaultStatuses())
	mgr.taskLists[source.ID].Description = "source description"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy of Source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected action duplicated, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Copy of Source") {
		t.Errorf("expected title in result, got: %s", result.Content)
	}

	// Verify a new list was created (not the source)
	found := false
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy of Source" {
			found = true
			if tl.Description != "source description" {
				t.Errorf("expected description inherited, got %q", tl.Description)
			}
			break
		}
	}
	if !found {
		t.Fatal("duplicate task list not found")
	}
}

func TestUpsertTaskList_DuplicateInheritsWorkflow(t *testing.T) {
	mgr := newFakeManager()
	customStatuses := []database.TaskListWorkflowStatus{
		{ID: 10, Order: 0, Label: "Novo"},
		{ID: 20, Order: 1, Label: "Andamento"},
		{ID: 30, Order: 2, Label: "Feito"},
	}
	source := mgr.addTaskList("Custom WF", customStatuses)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy WF",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	// Find the new list and verify its workflow matches source
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy WF" {
			if tl.Workflow == nil {
				t.Fatal("expected workflow on duplicated list")
			}
			if tl.Workflow.InitialStatusID != source.Workflow.InitialStatusID {
				t.Errorf("expected initial_status_id=%d, got %d", source.Workflow.InitialStatusID, tl.Workflow.InitialStatusID)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

func TestUpsertTaskList_DuplicateOverridesDescription(t *testing.T) {
	mgr := newFakeManager()
	source := mgr.addTaskList("Source", defaultStatuses())
	mgr.taskLists[source.ID].Description = "original desc"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy",
		"description":  "overridden desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy" {
			if tl.Description != "overridden desc" {
				t.Errorf("expected overridden description, got %q", tl.Description)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

func TestUpsertTaskList_DuplicateOverridesWorkflow(t *testing.T) {
	mgr := newFakeManager()
	source := mgr.addTaskList("Source", defaultStatuses())
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy Custom WF",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Open"},
				{"id": 2, "label": "Closed"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2},
				"2": []int{1},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with workflow override should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected duplicated, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateSourceNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	ghostID := "9999"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": ghostID,
		"duplicate":    true,
		"title":        "Ghost copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent source")
	}
	if !strings.Contains(result.Content, "não encontrado") {
		t.Errorf("expected not-found error, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateWithoutTaskListID_Error(t *testing.T) {
	mgr := newFakeManager()
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"duplicate": true,
		"title":     "No source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when duplicate is true without task_list_id")
	}
	if !strings.Contains(result.Content, "duplicate requires task_list_id or task_list_slug") {
		t.Errorf("expected duplicate ref error, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateDoesNotCopyTasks(t *testing.T) {
	mgr := newFakeManager()
	source := mgr.addTaskList("Source", defaultStatuses())
	mgr.addTask(source.ID, "Task 1", 1)
	mgr.addTask(source.ID, "Task 2", 2)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Empty Copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	// Find the new list and verify it has no tasks
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Empty Copy" {
			taskCount := 0
			for _, task := range mgr.tasks {
				if task.TaskListID == tl.ID {
					taskCount++
				}
			}
			if taskCount != 0 {
				t.Errorf("expected 0 tasks in duplicated list, got %d", taskCount)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

// ==================== Assignee Tests ====================

func TestUpsertTask_CreateWithAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	aName := "Alice"
	aID := "alice@example.com"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Assigned Task",
		"assignee_name": aName,
		"assignee_id":   aID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") {
		t.Fatalf("expected 'Alice' in result, got: %s", result.Content)
	}

	// Verify assignee was set
	for _, task := range mgr.tasks {
		if task.Title == "Assigned Task" {
			if task.AssigneeName != "Alice" {
				t.Errorf("expected assignee_name 'Alice', got '%s'", task.AssigneeName)
			}
			if task.AssigneeID != "alice@example.com" {
				t.Errorf("expected assignee_id 'alice@example.com', got '%s'", task.AssigneeID)
			}
			break
		}
	}
}

func TestUpsertTask_UpdateAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

	newName := "Bob"
	newID := "bob@example.com"
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"task_id":       taskID,
		"title":         "Task",
		"assignee_name": newName,
		"assignee_id":   newID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "Bob" {
		t.Errorf("expected assignee_name 'Bob', got '%s'", updated.AssigneeName)
	}
	if updated.AssigneeID != "bob@example.com" {
		t.Errorf("expected assignee_id 'bob@example.com', got '%s'", updated.AssigneeID)
	}
}

func TestUpsertTask_ClearAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

	empty := ""
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"task_id":       taskID,
		"title":         "Task",
		"assignee_name": empty,
		"assignee_id":   empty,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "" {
		t.Errorf("expected empty assignee_name, got '%s'", updated.AssigneeName)
	}
	if updated.AssigneeID != "" {
		t.Errorf("expected empty assignee_id, got '%s'", updated.AssigneeID)
	}
}

func TestUpsertTask_CreateWithoutAssignee_NoChange(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "No Assignee Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "No Assignee Task" {
			if task.AssigneeName != "" || task.AssigneeID != "" {
				t.Errorf("expected empty assignee fields for task without assignee params")
			}
			break
		}
	}
}

func TestUpsertTask_UpdatePreservesAssigneeWhenOmitted(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "Alice" {
		t.Errorf("expected assignee preserved as 'Alice', got '%s'", updated.AssigneeName)
	}
}

func TestUpsertTask_DedupByCode_WithAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	aName := "Alice"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Ticket",
		"code":          "FSD-100",
		"assignee_name": aName,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Update via code with new assignee
	newName := "Bob"
	result2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Ticket Updated",
		"code":          "FSD-100",
		"assignee_name": newName,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result2.Content)
	}

	for _, task := range mgr.tasks {
		if task.Code == "FSD-100" {
			if task.AssigneeName != "Bob" {
				t.Errorf("expected assignee_name 'Bob', got '%s'", task.AssigneeName)
			}
			break
		}
	}
}

// ==================== Creator Tests ====================

func TestUpsertTask_CreateWithCreator(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Ticket from external",
		"creator_name": "Fulano",
		"creator_id":   "user-123",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Fulano") {
		t.Errorf("expected 'Fulano' in result, got: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "Ticket from external" {
			if task.CreatorName != "Fulano" {
				t.Errorf("expected creator_name 'Fulano', got '%s'", task.CreatorName)
			}
			if task.CreatorID != "user-123" {
				t.Errorf("expected creator_id 'user-123', got '%s'", task.CreatorID)
			}
			break
		}
	}
}

func TestUpsertTask_CreateWithCreatorAndAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Full ticket",
		"creator_name":  "Fulano",
		"creator_id":    "user-123",
		"assignee_name": "Beltrano",
		"assignee_id":   "user-456",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "Full ticket" {
			if task.CreatorName != "Fulano" {
				t.Errorf("expected creator_name 'Fulano', got '%s'", task.CreatorName)
			}
			if task.AssigneeName != "Beltrano" {
				t.Errorf("expected assignee_name 'Beltrano', got '%s'", task.AssigneeName)
			}
			break
		}
	}
}

func TestUpsertTask_UpdatePreservesCreatorWhenOmitted(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Original"
	task.CreatorID = "orig-id"
	tool := NewTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.CreatorName != "Original" {
		t.Errorf("expected creator preserved as 'Original', got '%s'", updated.CreatorName)
	}
}

func TestUpsertTask_ClearCreator(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Someone"
	task.CreatorID = "some-id"
	tool := NewTask(mgr)

	empty := ""
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Task",
		"creator_name": empty,
		"creator_id":   empty,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.CreatorName != "" {
		t.Errorf("expected empty creator_name, got '%s'", updated.CreatorName)
	}
}

// ==================== UpsertTaskNote Author Tests ====================

func TestUpsertTaskNote_CreateWithStructuredAuthor(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":     task.ID,
		"type":        2,
		"content":     "Customer response here",
		"author_name": "Ciclano",
		"author_id":   "user-789",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	notes := mgr.notes[task.ID]
	if len(notes) == 0 {
		t.Fatal("expected at least one note")
	}
	note := notes[0]
	if note.AuthorName != "Ciclano" {
		t.Errorf("expected author_name 'Ciclano', got '%s'", note.AuthorName)
	}
	if note.AuthorID != "user-789" {
		t.Errorf("expected author_id 'user-789', got '%s'", note.AuthorID)
	}
}

func TestUpsertTaskNote_CreateNoAuthor(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    4,
		"content": "System event without author",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	notes := mgr.notes[task.ID]
	note := notes[0]
	if note.AuthorName != "" || note.AuthorID != "" {
		t.Errorf("expected author fields empty, got author_name=%q author_id=%q",
			note.AuthorName, note.AuthorID)
	}
}

// ==================== Idempotent upsert_task Tests ====================

func TestUpsertTask_SameStatus_DifferentDescription(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "old description"
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "new description from ADF",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail when status unchanged but description changed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected action=updated, got: %s", result.Content)
	}
	updated := mgr.tasks[task.ID]
	if updated.Description != "new description from ADF" {
		t.Errorf("expected new description, got: %s", updated.Description)
	}
	if updated.StatusID != 3 {
		t.Errorf("status should remain 3, got: %d", updated.StatusID)
	}
}

func TestUpsertTask_SameStatus_SameFields_Noop(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "same description"
	task.Code = "FSD-100"
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "same description",
		"code":         "FSD-100",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail for noop: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected action=noop, got: %s", result.Content)
	}
}

func TestUpsertTask_DifferentStatus_ValidTransition(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should succeed for valid status change: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected action=updated, got: %s", result.Content)
	}
	updated := mgr.tasks[task.ID]
	if updated.StatusID != 2 {
		t.Errorf("expected status 2, got: %d", updated.StatusID)
	}
}

func TestUpsertTask_DifferentStatus_InvalidTransition(t *testing.T) {
	mgr := newFakeManager()
	statuses := defaultStatuses()
	transitions := database.TaskListWorkflowTransitions{
		1: {2},
		2: {1, 3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for invalid transition, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "status change failed") {
		t.Errorf("expected status change failed message, got: %s", result.Content)
	}
}

func TestUpsertTask_Create_StillWorks(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Brand new task",
		"description":  "desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("create should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected action=created, got: %s", result.Content)
	}
}

func TestUpsertTask_DedupByCode_StillWorks(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	result1, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "First",
		"code":         "EXT-1",
	}))
	if result1.IsError {
		t.Fatalf("first create should succeed: %s", result1.Content)
	}
	if !strings.Contains(result1.Content, "created") {
		t.Errorf("expected created, got: %s", result1.Content)
	}

	result2, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Updated title",
		"code":         "EXT-1",
		"description":  "new desc",
	}))
	if result2.IsError {
		t.Fatalf("dedup update should succeed: %s", result2.Content)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Errorf("expected updated via dedup, got: %s", result2.Content)
	}
}

func TestUpsertTask_SameStatus_NoStatusID_UpdatesFields(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.Description = "old"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "new description",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected updated, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].Description != "new description" {
		t.Errorf("description not updated")
	}
}

// ==================== Workflow Transition Tests ====================

func TestUpsertTask_TransitionToTerminalStatus(t *testing.T) {
	mgr := newFakeManager()
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
		{ID: 4, Order: 3, Label: "Cancelado"},
		{ID: 5, Order: 4, Label: "Abandonado"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 4, 5},
		2: {1, 3, 4, 5},
	}
	tl := mgr.addTaskListWithTransitions("Jira-like", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 2
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("transition to terminal status should succeed: %s", result.Content)
	}
	if mgr.tasks[task.ID].StatusID != 3 {
		t.Errorf("expected status 3, got %d", mgr.tasks[task.ID].StatusID)
	}
}

func TestUpsertTask_TransitionFromZeroStatus(t *testing.T) {
	mgr := newFakeManager()
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 3},
		2: {1, 3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Orphan task", 0)
	task.StatusID = 0
	tool := NewTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Orphan task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("transition from status 0 (invalid) to valid status should succeed: %s", result.Content)
	}
	if mgr.tasks[task.ID].StatusID != 2 {
		t.Errorf("expected status 2, got %d", mgr.tasks[task.ID].StatusID)
	}
}

func TestUpsertTask_SameStatusNoop_NoUpdateTaskStatusCall(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 2
	tool := NewTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("same status should not fail: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected noop for same status and same fields, got: %s", result.Content)
	}
}

func TestUpsertTask_InvalidDestinationStatus(t *testing.T) {
	mgr := newFakeManager()
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Done"},
	}
	transitions := database.TaskListWorkflowTransitions{1: {2}}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

	statusID := 99
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent destination status")
	}
	if !strings.Contains(result.Content, "invalid status_id") {
		t.Errorf("expected 'invalid status_id' message, got: %s", result.Content)
	}
}

func TestUpsertTask_TransitionNotAllowedByWorkflow(t *testing.T) {
	mgr := newFakeManager()
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2},
		2: {3},
	}
	tl := mgr.addTaskListWithTransitions("Strict", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error: 1->3 is not allowed, only 1->2, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "status change failed") {
		t.Errorf("expected 'status change failed', got: %s", result.Content)
	}
}

func TestUpsertTask_CreateWithTerminalStatus(t *testing.T) {
	mgr := newFakeManager()
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 3},
		2: {3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Already done",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("creating task with terminal status should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected created, got: %s", result.Content)
	}
}

// ==================== Move Task Tests ====================

func TestUpsertTask_MoveToAnotherList(t *testing.T) {
	mgr := newFakeManager()
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "My task", 1)
	task.Description = "original desc"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "My task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "moved") {
		t.Errorf("expected action moved, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].TaskListID != listB.ID {
		t.Errorf("expected task_list_id=%s, got %s", listB.ID, mgr.tasks[task.ID].TaskListID)
	}
}

func TestUpsertTask_MoveSameList_Noop(t *testing.T) {
	mgr := newFakeManager()
	listA := mgr.addTaskList("List A", defaultStatuses())
	task := mgr.addTask(listA.ID, "My task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listA.ID,
		"task_id":      task.ID,
		"title":        "My task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("same-list update should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected noop (no move, same fields), got: %s", result.Content)
	}
}

func TestUpsertTask_MoveAndUpdateFields(t *testing.T) {
	mgr := newFakeManager()
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "Old title", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "New title",
		"description":  "new desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move+update should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "moved") {
		t.Errorf("expected action moved, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].Title != "New title" {
		t.Errorf("title not updated")
	}
	if mgr.tasks[task.ID].Description != "new desc" {
		t.Errorf("description not updated")
	}
}

func TestUpsertTask_MoveResetsStatus(t *testing.T) {
	mgr := newFakeManager()
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "Task", 1)
	task.StatusID = 2 // Em Progresso
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move should succeed: %s", result.Content)
	}
	// MoveTaskToList resets status to initial (1)
	if mgr.tasks[task.ID].StatusID != 1 {
		t.Errorf("expected status reset to 1 after move, got %d", mgr.tasks[task.ID].StatusID)
	}
}

// ==================== Duplicate Task Tests ====================

func TestUpsertTask_DuplicateSameList(t *testing.T) {
	mgr := newFakeManager()
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source task", 1)
	source.Description = "source desc"
	source.Link = "https://example.com"
	source.AssigneeName = "Alice"
	source.AssigneeID = "alice@example.com"
	source.CreatorName = "Bob"
	source.CreatorID = "bob@example.com"
	source.Code = "JIRA-123"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy of Source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected action duplicated, got: %s", result.Content)
	}

	// Find the new task (not source)
	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy of Source" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.TaskListID != list.ID {
		t.Errorf("expected task_list_id=%s, got %s", list.ID, newTask.TaskListID)
	}
	if newTask.Description != "source desc" {
		t.Errorf("expected description inherited, got %q", newTask.Description)
	}
	if newTask.Link != "https://example.com" {
		t.Errorf("expected link inherited, got %q", newTask.Link)
	}
	if newTask.AssigneeName != "Alice" {
		t.Errorf("expected assignee inherited, got %q", newTask.AssigneeName)
	}
	if newTask.CreatorName != "Bob" {
		t.Errorf("expected creator inherited, got %q", newTask.CreatorName)
	}
	// Code should NOT be inherited
	if newTask.Code != "" {
		t.Errorf("expected code NOT inherited, got %q", newTask.Code)
	}
}

func TestUpsertTask_DuplicateToAnotherList(t *testing.T) {
	mgr := newFakeManager()
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	source := mgr.addTask(listA.ID, "Source", 1)
	source.Description = "original"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("cross-list duplicate should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.TaskListID != listB.ID {
		t.Errorf("expected task_list_id=%s, got %s", listB.ID, newTask.TaskListID)
	}
	if newTask.Description != "original" {
		t.Errorf("expected description inherited, got %q", newTask.Description)
	}
}

func TestUpsertTask_DuplicateOverridesFields(t *testing.T) {
	mgr := newFakeManager()
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source", 1)
	source.Description = "source desc"
	source.Link = "https://old.com"
	source.AssigneeName = "Alice"
	tool := NewTask(mgr)

	newAssignee := "Charlie"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  list.ID,
		"task_id":       source.ID,
		"duplicate":     true,
		"title":         "Override copy",
		"description":   "new desc",
		"link":          "https://new.com",
		"assignee_name": newAssignee,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with overrides should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Override copy" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.Description != "new desc" {
		t.Errorf("expected overridden description, got %q", newTask.Description)
	}
	if newTask.Link != "https://new.com" {
		t.Errorf("expected overridden link, got %q", newTask.Link)
	}
	if newTask.AssigneeName != "Charlie" {
		t.Errorf("expected overridden assignee, got %q", newTask.AssigneeName)
	}
}

func TestUpsertTask_DuplicateWithCode(t *testing.T) {
	mgr := newFakeManager()
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source", 1)
	source.Code = "JIRA-100"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy with code",
		"code":         "JIRA-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with explicit code should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy with code" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.Code != "JIRA-200" {
		t.Errorf("expected explicit code JIRA-200, got %q", newTask.Code)
	}
}

func TestUpsertTask_DuplicateSourceNotFound(t *testing.T) {
	mgr := newFakeManager()
	list := mgr.addTaskList("List", defaultStatuses())
	tool := NewTask(mgr)

	ghostID := "9999"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      ghostID,
		"duplicate":    true,
		"title":        "Ghost copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent source task")
	}
	if !strings.Contains(result.Content, "não encontrado") {
		t.Errorf("expected not-found error, got: %s", result.Content)
	}
}

func TestUpsertTask_DuplicateWithoutTaskID_Error(t *testing.T) {
	mgr := newFakeManager()
	list := mgr.addTaskList("List", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"duplicate":    true,
		"title":        "No source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when duplicate is true without task_id")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

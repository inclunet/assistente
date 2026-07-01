package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const MaxTaskLists = 100

func taskQuery(ctx context.Context, base *gorm.DB) *gorm.DB {
	return ScopeByUser(ctx,
		base.WithContext(ctx).Joins("JOIN task_lists ON task_lists.id = tasks.task_list_id"),
		"task_lists.user_id",
	)
}

func taskNoteQuery(ctx context.Context, base *gorm.DB) *gorm.DB {
	return ScopeByUser(ctx,
		base.WithContext(ctx).
			Joins("JOIN tasks ON tasks.id = task_notes.task_id").
			Joins("JOIN task_lists ON task_lists.id = tasks.task_list_id"),
		"task_lists.user_id",
	)
}

func taskListWorkflowQuery(ctx context.Context, base *gorm.DB) *gorm.DB {
	return ScopeByUser(ctx,
		base.WithContext(ctx).Joins("JOIN task_lists ON task_lists.id = task_list_workflows.task_list_id"),
		"task_lists.user_id",
	)
}

// ==================== TaskList Operations ====================

// CreateTaskListWithContext cria uma nova tasklist pertencente ao usuário do
// contexto, com workflow padrão.
// templateWorkflow pode ser nil para usar workflow padrão (A Fazer, Em
// Progresso, Concluído). slug: opcional; normalizado e único quando não vazio.
func CreateTaskListWithContext(ctx context.Context, title, description string, templateWorkflow *TaskListWorkflow, slug string) (*TaskList, error) {
	// Valida limite
	var count int64
	if err := ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTaskLists {
		return nil, errors.New("limite de tasklists atingido")
	}

	taskList := &TaskList{
		Title:             title,
		Description:       description,
		PreferredViewMode: "list",
	}
	if userID, ok := UserIDFromContext(ctx); ok {
		taskList.UserID = userID
	}

	if err := db.WithContext(ctx).Create(taskList).Error; err != nil {
		return nil, err
	}

	// Cria workflow
	workflow, err := createWorkflowForTaskListWithContext(ctx, taskList.ID, templateWorkflow)
	if err != nil {
		return nil, err
	}

	taskList.Workflow = workflow

	if s := NormalizeTaskListSlug(slug); s != "" {
		if err := SetTaskListSlugWithContext(ctx, taskList.ID, s); err != nil {
			return nil, err
		}
		taskList.Slug = s
	}

	return taskList, nil
}

// GetTaskListWithContext retorna uma tasklist do usuário do contexto pelo ID,
// com workflow e tasks.
func GetTaskListWithContext(ctx context.Context, id string) (*TaskList, error) {
	var taskList TaskList
	err := WithSQLiteBusyRetry(ctx, "tasklist.get", func() error {
		return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Preload("Workflow").
			Preload("Tasks", func(db *gorm.DB) *gorm.DB {
				return db.Where("parent_id IS NULL").Order("`order` ASC")
			}).
			Preload("Tasks.Subtasks", func(db *gorm.DB) *gorm.DB {
				return db.Order("`order` ASC")
			}).
			First(&taskList, "id = ?", id).Error
	})

	return &taskList, err
}

// GetAllTaskListsWithContext retorna todas as tasklists do usuário do
// contexto, ordenadas por data de criação.
func GetAllTaskListsWithContext(ctx context.Context) ([]TaskList, error) {
	var taskLists []TaskList
	err := WithSQLiteBusyRetry(ctx, "tasklist.list_all", func() error {
		return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Preload("Workflow").
			Preload("Tasks", func(db *gorm.DB) *gorm.DB {
				return db.Where("parent_id IS NULL").Order("`order` ASC")
			}).
			Order("created_at DESC").
			Find(&taskLists).Error
	})
	return taskLists, err
}

// UpdateTaskListWithContext atualiza title e description de uma tasklist do
// usuário do contexto.
func UpdateTaskListWithContext(ctx context.Context, id string, title, description string) error {
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":       title,
			"description": description,
		}).Error
}

// SetTaskListViewModeWithContext define o modo de visualização (list ou
// kanban) de uma tasklist do usuário do contexto.
func SetTaskListViewModeWithContext(ctx context.Context, id string, viewMode string) error {
	if viewMode != "list" && viewMode != "kanban" {
		return errors.New("view mode inválido: use 'list' ou 'kanban'")
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", id).Update("preferred_view_mode", viewMode).Error
}

// CloneTaskListWithContext clona uma tasklist do usuário do contexto com seu
// workflow mas sem as tasks.
func CloneTaskListWithContext(ctx context.Context, id string, newTitle string) (*TaskList, error) {
	// Busca tasklist original
	original, err := GetTaskListWithContext(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cria nova tasklist
	cloned, err := CreateTaskListWithContext(ctx, newTitle, original.Description, original.Workflow, "")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(original.ValidationPolicy) != "" {
		if err := SetTaskListValidationPolicyWithContext(ctx, cloned.ID, original.ValidationPolicy); err != nil {
			return nil, err
		}
		cloned.ValidationPolicy = original.ValidationPolicy
	}

	if trimmed := strings.TrimSpace(original.CustomActions); trimmed != "" {
		if err := SetTaskListCustomActionsWithContext(ctx, cloned.ID, trimmed); err != nil {
			return nil, err
		}
		// Reflete o valor efetivamente persistido (SetTaskListCustomActionsWithContext
		// grava a versão com TrimSpace), evitando devolver ao caller um CustomActions
		// diferente do que ficou no banco.
		cloned.CustomActions = trimmed
	}

	return cloned, nil
}

// ClearTaskListWithContext remove todas as tasks de uma tasklist do usuário
// do contexto, mantendo a lista e o workflow.
func ClearTaskListWithContext(ctx context.Context, id string) error {
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.task_list_id = ?", id))
	// Deleta notas de todas as tasks da lista
	if err := db.WithContext(ctx).Where("task_id IN (?)", taskIDs).
		Delete(&TaskNote{}).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Where("id IN (?)", taskIDs).Delete(&Task{}).Error; err != nil {
		return err
	}
	return nil
}

// DeleteTaskListWithContext deleta uma tasklist do usuário do contexto e
// todas suas tasks.
func DeleteTaskListWithContext(ctx context.Context, id string) error {
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.task_list_id = ?", id))
	// Deleta notas de todas as tasks da lista
	if err := db.WithContext(ctx).Where("task_id IN (?)", taskIDs).
		Delete(&TaskNote{}).Error; err != nil {
		return err
	}

	// Deleta tasks
	if err := db.WithContext(ctx).Where("id IN (?)", taskIDs).Delete(&Task{}).Error; err != nil {
		return err
	}

	// Deleta workflow
	if err := db.WithContext(ctx).Where("task_list_id IN (?)",
		ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}).Select("id").Where("id = ?", id), "user_id"),
	).Delete(&TaskListWorkflow{}).Error; err != nil {
		return err
	}

	// Deleta tasklist
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Where("id = ?", id).Delete(&TaskList{}).Error
}

// ==================== Workflow Operations ====================

func createWorkflowForTaskListWithContext(ctx context.Context, taskListID string, template *TaskListWorkflow) (*TaskListWorkflow, error) {
	var workflow *TaskListWorkflow

	if template != nil {
		// Clona do template
		workflow = &TaskListWorkflow{
			TaskListID:         taskListID,
			Statuses:           template.Statuses,
			AllowedTransitions: template.AllowedTransitions,
			InitialStatusID:    template.InitialStatusID,
		}
	} else {
		// Cria workflow padrão (A Fazer, Em Progresso, Concluído)
		statuses := []TaskListWorkflowStatus{
			{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
			{ID: 2, Order: 1, Label: "Em Progresso", Color: "var(--color-info)", Icon: "▶️"},
			{ID: 3, Order: 2, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
		}
		statusesJSON, _ := json.Marshal(statuses)

		transitions := TaskListWorkflowTransitions{
			1: {2, 3},
			2: {1, 3},
			3: {1, 2},
		}
		transitionsJSON, _ := json.Marshal(transitions)

		workflow = &TaskListWorkflow{
			TaskListID:         taskListID,
			Statuses:           string(statusesJSON),
			AllowedTransitions: string(transitionsJSON),
			InitialStatusID:    1,
		}
	}

	if err := db.WithContext(ctx).Create(workflow).Error; err != nil {
		return nil, err
	}

	return workflow, nil
}

// GetWorkflowWithContext retorna o workflow de uma tasklist do usuário do
// contexto.
func GetWorkflowWithContext(ctx context.Context, taskListID string) (*TaskListWorkflow, error) {
	var workflow TaskListWorkflow
	err := WithSQLiteBusyRetry(ctx, "tasklist.workflow.get", func() error {
		return taskListWorkflowQuery(ctx, db.Model(&TaskListWorkflow{})).
			Where("task_list_workflows.task_list_id = ?", taskListID).
			First(&workflow).Error
	})
	return &workflow, err
}

// UpdateWorkflowWithContext atualiza statuses e/ou transitions de um workflow
// pertencente ao usuário do contexto.
func UpdateWorkflowWithContext(ctx context.Context, taskListID string, statuses []TaskListWorkflowStatus, transitions TaskListWorkflowTransitions) error {
	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)

	scopedListIDs := ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}).Select("id"), "user_id")
	return db.WithContext(ctx).Model(&TaskListWorkflow{}).
		Where("task_list_id = ?", taskListID).
		Where("task_list_id IN (?)", scopedListIDs).
		Updates(map[string]interface{}{
			"statuses":            string(statusesJSON),
			"allowed_transitions": string(transitionsJSON),
		}).Error
}

// UpdateWorkflowFullWithContext atualiza statuses, transitions e
// initial_status_id de um workflow pertencente ao usuário do contexto, com
// validação completa:
// - initial_status_id deve existir nos statuses
// - todas as transições devem referenciar status IDs válidos
// - status IDs em uso por tasks não podem ser removidos (a menos que status_migration mapeie-os)
// - status_migration pode ser nil se nenhum status será removido
func UpdateWorkflowFullWithContext(
	ctx context.Context,
	taskListID string,
	statuses []TaskListWorkflowStatus,
	transitions TaskListWorkflowTransitions,
	initialStatusID int,
	statusMigration map[int]int,
) error {
	if _, err := GetTaskListWithContext(ctx, taskListID); err != nil {
		return err
	}
	if len(statuses) == 0 {
		return errors.New("workflow deve ter pelo menos um status")
	}

	statusIDs := make(map[int]bool, len(statuses))
	for _, s := range statuses {
		if s.ID <= 0 {
			return fmt.Errorf("status ID deve ser > 0, encontrado: %d", s.ID)
		}
		if statusIDs[s.ID] {
			return fmt.Errorf("status ID duplicado: %d", s.ID)
		}
		statusIDs[s.ID] = true
	}

	if !statusIDs[initialStatusID] {
		return fmt.Errorf("initial_status_id %d não existe nos statuses fornecidos", initialStatusID)
	}

	for fromID, toIDs := range transitions {
		if !statusIDs[fromID] {
			return fmt.Errorf("transição referencia status de origem inexistente: %d", fromID)
		}
		for _, toID := range toIDs {
			if !statusIDs[toID] {
				return fmt.Errorf("transição de %d referencia status de destino inexistente: %d", fromID, toID)
			}
		}
	}

	for oldID, newID := range statusMigration {
		if statusIDs[oldID] {
			return fmt.Errorf("status_migration mapeia ID %d que ainda existe nos novos statuses", oldID)
		}
		if !statusIDs[newID] {
			return fmt.Errorf("status_migration mapeia para ID %d inexistente nos novos statuses", newID)
		}
	}

	counts, err := GetTaskCountsByStatusWithContext(ctx, taskListID)
	if err != nil {
		return fmt.Errorf("erro ao verificar tasks existentes: %w", err)
	}

	for usedStatusID, count := range counts {
		if count == 0 {
			continue
		}
		if statusIDs[usedStatusID] {
			continue
		}
		if statusMigration != nil {
			if _, ok := statusMigration[usedStatusID]; ok {
				continue
			}
		}
		return fmt.Errorf(
			"status_id %d está em uso por %d task(s) e não existe nos novos statuses; forneça status_migration para migrá-las",
			usedStatusID, count,
		)
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for oldID, newID := range statusMigration {
			taskIDs := taskQuery(ctx, tx.Model(&Task{}).Select("tasks.id").Where("tasks.task_list_id = ? AND tasks.status_id = ?", taskListID, oldID))
			if err := tx.Model(&Task{}).Where("id IN (?)", taskIDs).
				Update("status_id", newID).Error; err != nil {
				return fmt.Errorf("erro ao migrar tasks de status %d para %d: %w", oldID, newID, err)
			}
		}

		statusesJSON, _ := json.Marshal(statuses)
		transitionsJSON, _ := json.Marshal(transitions)

		scopedListIDs := ScopeByUser(ctx, tx.Model(&TaskList{}).Select("id"), "user_id")
		return tx.Model(&TaskListWorkflow{}).
			Where("task_list_id = ?", taskListID).
			Where("task_list_id IN (?)", scopedListIDs).
			Updates(map[string]interface{}{
				"statuses":            string(statusesJSON),
				"allowed_transitions": string(transitionsJSON),
				"initial_status_id":   initialStatusID,
			}).Error
	})
}

// GetTaskCountsByStatusWithContext retorna a contagem de tasks por status_id
// para uma tasklist do usuário do contexto.
func GetTaskCountsByStatusWithContext(ctx context.Context, taskListID string) (map[int]int64, error) {
	var counts []struct {
		StatusID int
		Count    int64
	}
	err := WithSQLiteBusyRetry(ctx, "tasklist.status_counts", func() error {
		return taskQuery(ctx, db.Model(&Task{})).
			Where("tasks.task_list_id = ?", taskListID).
			Group("tasks.status_id").
			Select("tasks.status_id, count(*) as count").
			Scan(&counts).Error
	})
	if err != nil {
		return nil, err
	}

	result := make(map[int]int64, len(counts))
	for _, c := range counts {
		result[c.StatusID] = c.Count
	}
	return result, nil
}

// UpdateTaskListFullWithContext atualiza title, description e
// preferred_view_mode de uma tasklist do usuário do contexto.
// slug: nil = não altera slug; ponteiro para string vazia = limpa slug;
// valor = define slug normalizado.
func UpdateTaskListFullWithContext(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	updates := map[string]interface{}{
		"title":       title,
		"description": description,
	}
	if preferredViewMode == "list" || preferredViewMode == "kanban" {
		updates["preferred_view_mode"] = preferredViewMode
	}
	if slug != nil {
		s := NormalizeTaskListSlug(*slug)
		if err := ValidateTaskListSlugFormat(s); err != nil {
			return err
		}
		if s != "" {
			taken, err := slugTakenByOtherThanWithContext(ctx, s, id)
			if err != nil {
				return err
			}
			if taken {
				return fmt.Errorf("slug %q já está em uso por outra lista", s)
			}
		}
		updates["slug"] = s
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", id).Updates(updates).Error
}

// SetTaskListConversationWithContext vincula (ou desvincula, com nil) uma
// tasklist do usuário do contexto a uma conversa. Não valida a existência da
// conversa: o vínculo é uma referência fraca (a conversa pode ser de outra
// origem/canal); o escopo por usuário já protege contra acesso indevido.
func SetTaskListConversationWithContext(ctx context.Context, id string, conversationID *string) error {
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").
		Where("id = ?", id).
		Update("conversation_id", conversationID).Error
}

// GetTaskListsByConversationIDWithContext retorna as tasklists do usuário do
// contexto vinculadas a uma conversa, com workflow e tasks raiz.
func GetTaskListsByConversationIDWithContext(ctx context.Context, conversationID string) ([]TaskList, error) {
	var taskLists []TaskList
	err := WithSQLiteBusyRetry(ctx, "tasklist.list_by_conversation", func() error {
		return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Preload("Workflow").
			Preload("Tasks", func(db *gorm.DB) *gorm.DB {
				return db.Where("parent_id IS NULL").Order("`order` ASC")
			}).
			Where("conversation_id = ?", conversationID).
			Order("created_at DESC").
			Find(&taskLists).Error
	})
	return taskLists, err
}

// ReorderWorkflowStatusesWithContext reordena os statuses do workflow do
// usuário do contexto, mantendo seus IDs e labels.
func ReorderWorkflowStatusesWithContext(ctx context.Context, taskListID string, statusOrder []int) error {
	// Busca workflow atual
	workflow, err := GetWorkflowWithContext(ctx, taskListID)
	if err != nil {
		return err
	}

	// Desserializa statuses
	var statuses []TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
		return err
	}

	// Valida que todos os status IDs estão presentes
	if len(statusOrder) != len(statuses) {
		return errors.New("número de statuses não corresponde")
	}

	// Reordena
	for i, statusID := range statusOrder {
		for j := range statuses {
			if statuses[j].ID == statusID {
				statuses[j].Order = i
				break
			}
		}
	}

	// Atualiza
	statusesJSON, _ := json.Marshal(statuses)
	scopedListIDs := ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}).Select("id"), "user_id")
	return db.WithContext(ctx).Model(&TaskListWorkflow{}).
		Where("task_list_id = ?", taskListID).
		Where("task_list_id IN (?)", scopedListIDs).
		Update("statuses", string(statusesJSON)).Error
}

// ValidateStatusTransitionWithContext valida se uma transição de status é
// permitida no workflow do usuário do contexto. Permite transição livre quando
// o status atual é inválido/vazio (0 ou ausente no workflow), desde que o
// status destino exista no workflow.
func ValidateStatusTransitionWithContext(ctx context.Context, taskListID string, fromStatusID, toStatusID int) error {
	if fromStatusID == toStatusID {
		return nil
	}

	workflow, err := GetWorkflowWithContext(ctx, taskListID)
	if err != nil {
		return err
	}

	// Desserializa statuses e transitions
	var statuses []TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
		return err
	}

	var transitions TaskListWorkflowTransitions
	if err := json.Unmarshal([]byte(workflow.AllowedTransitions), &transitions); err != nil {
		return err
	}

	// Valida que o status destino existe na lista de statuses do workflow
	toExists := false
	for _, s := range statuses {
		if s.ID == toStatusID {
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
			toStatusID, strings.Join(labels, ", "))
	}

	// Se o status atual não existe no workflow (task com status zerado/inválido),
	// permite a transição para qualquer status válido
	allowedStatuses, fromExists := transitions[fromStatusID]
	if !fromExists {
		return nil
	}

	for _, statusID := range allowedStatuses {
		if statusID == toStatusID {
			return nil
		}
	}

	return fmt.Errorf("transição de status %d para %d não é permitida pelo workflow", fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

// CreateTaskWithContext cria uma nova task em uma tasklist do usuário do
// contexto.
func CreateTaskWithContext(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*Task, error) {
	if _, err := GetTaskListWithContext(ctx, taskListID); err != nil {
		return nil, err
	}
	if parentID != nil {
		parent, err := GetTaskWithContext(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if parent.TaskListID != taskListID {
			return nil, fmt.Errorf("parent_id não pertence à tasklist solicitada")
		}
	}
	// Busca workflow para status inicial
	workflow, err := GetWorkflowWithContext(ctx, taskListID)
	if err != nil {
		return nil, err
	}

	// Calcula próxima ordem
	var maxOrder int
	query := taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", taskListID)
	if parentID != nil {
		query = query.Where("tasks.parent_id = ?", parentID)
	} else {
		query = query.Where("tasks.parent_id IS NULL")
	}
	query.Select(`COALESCE(MAX(tasks."order"), -1)`).Scan(&maxOrder)

	task := &Task{
		TaskListID:  taskListID,
		Title:       title,
		Description: description,
		Code:        code,
		Link:        link,
		StatusID:    workflow.InitialStatusID,
		ParentID:    parentID,
		Order:       maxOrder + 1,
	}

	if err := ValidateTaskCodeForTaskListWithContext(ctx, taskListID, code); err != nil {
		return nil, err
	}

	if err := db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// CreateTaskFullWithContext cria uma nova task em uma tasklist do usuário do
// contexto, com todos os campos, incluindo assignee e creator.
func CreateTaskFullWithContext(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*Task, error) {
	if _, err := GetTaskListWithContext(ctx, taskListID); err != nil {
		return nil, err
	}
	if parentID != nil {
		parent, err := GetTaskWithContext(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if parent.TaskListID != taskListID {
			return nil, fmt.Errorf("parent_id não pertence à tasklist solicitada")
		}
	}
	workflow, err := GetWorkflowWithContext(ctx, taskListID)
	if err != nil {
		return nil, err
	}

	var maxOrder int
	query := taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", taskListID)
	if parentID != nil {
		query = query.Where("tasks.parent_id = ?", parentID)
	} else {
		query = query.Where("tasks.parent_id IS NULL")
	}
	query.Select(`COALESCE(MAX(tasks."order"), -1)`).Scan(&maxOrder)

	task := &Task{
		TaskListID:   taskListID,
		Title:        title,
		Description:  description,
		Code:         code,
		Link:         link,
		AssigneeName: assigneeName,
		AssigneeID:   assigneeID,
		CreatorName:  creatorName,
		CreatorID:    creatorID,
		StatusID:     workflow.InitialStatusID,
		ParentID:     parentID,
		Order:        maxOrder + 1,
	}

	if err := ValidateTaskCodeForTaskListWithContext(ctx, taskListID, code); err != nil {
		return nil, err
	}

	if err := db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// FindTaskByCodeWithContext busca uma task pelo code dentro de uma tasklist
// do usuário do contexto. Retorna nil, nil se nao encontrar.
func FindTaskByCodeWithContext(ctx context.Context, taskListID string, code string) (*Task, error) {
	var task Task
	err := taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ? AND tasks.code = ?", taskListID, code).
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetTaskWithContext retorna uma task do usuário do contexto, incluindo
// subtasks.
func GetTaskWithContext(ctx context.Context, id string) (*Task, error) {
	var task Task
	err := taskQuery(ctx, db.Model(&Task{})).Preload("Subtasks", func(db *gorm.DB) *gorm.DB {
		return db.Order("`order` ASC")
	}).
		First(&task, "tasks.id = ?", id).Error
	return &task, err
}

// GetTasksByTaskListIDWithContext retorna todas as tasks principais de uma
// tasklist do usuário do contexto (sem subtasks nested).
func GetTasksByTaskListIDWithContext(ctx context.Context, taskListID string) ([]Task, error) {
	var tasks []Task
	err := taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ? AND tasks.parent_id IS NULL", taskListID).
		Order("tasks.`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// GetTasksByStatusWithContext retorna todas as tasks de um status específico
// dentro de uma tasklist do usuário do contexto.
func GetTasksByStatusWithContext(ctx context.Context, taskListID string, statusID int) ([]Task, error) {
	var tasks []Task
	err := taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ? AND tasks.status_id = ? AND tasks.parent_id IS NULL", taskListID, statusID).
		Order("tasks.`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// UpdateTaskWithContext atualiza title, description, code e link de uma task
// do usuário do contexto.
func UpdateTaskWithContext(ctx context.Context, id string, title, description, code, link string) error {
	task, err := GetTaskWithContext(ctx, id)
	if err != nil {
		return err
	}
	if err := ValidateTaskCodeForTaskListWithContext(ctx, task.TaskListID, code); err != nil {
		return err
	}
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).
		Where("id IN (?)", taskIDs).
		Updates(map[string]interface{}{
			"title":       title,
			"description": description,
			"code":        code,
			"link":        link,
		}).Error
}

// UpdateTaskFullWithContext atualiza todos os campos editáveis de uma task do
// usuário do contexto, incluindo assignee e creator.
func UpdateTaskFullWithContext(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	task, err := GetTaskWithContext(ctx, id)
	if err != nil {
		return err
	}
	if err := ValidateTaskCodeForTaskListWithContext(ctx, task.TaskListID, code); err != nil {
		return err
	}
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).
		Where("id IN (?)", taskIDs).
		Updates(map[string]interface{}{
			"title":         title,
			"description":   description,
			"code":          code,
			"link":          link,
			"assignee_name": assigneeName,
			"assignee_id":   assigneeID,
			"creator_name":  creatorName,
			"creator_id":    creatorID,
		}).Error
}

// SetTaskConversationWithContext vincula (ou desvincula, com nil) uma task do
// usuário do contexto a uma conversa. Usa o mesmo padrão de escopo via subquery
// de UpdateTaskWithContext para garantir que só tasks do usuário sejam afetadas.
func SetTaskConversationWithContext(ctx context.Context, id string, conversationID *string) error {
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).
		Where("id IN (?)", taskIDs).
		Update("conversation_id", conversationID).Error
}

// GetTasksByConversationIDWithContext retorna as tasks do usuário do contexto
// vinculadas a uma conversa (todas as tasks, inclusive subtasks), ordenadas.
func GetTasksByConversationIDWithContext(ctx context.Context, conversationID string) ([]Task, error) {
	var tasks []Task
	err := taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.conversation_id = ?", conversationID).
		Order("tasks.task_list_id ASC, tasks.`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// UpdateTaskAssigneeWithContext atualiza apenas o assignee de uma task do
// usuário do contexto.
func UpdateTaskAssigneeWithContext(ctx context.Context, id string, assigneeName, assigneeID string) error {
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).
		Where("id IN (?)", taskIDs).
		Updates(map[string]interface{}{
			"assignee_name": assigneeName,
			"assignee_id":   assigneeID,
		}).Error
}

// UpdateTaskStatusWithContext atualiza o status de uma task do usuário do
// contexto, com validação de transição.
func UpdateTaskStatusWithContext(ctx context.Context, id string, newStatusID int) error {
	// Busca task
	task, err := GetTaskWithContext(ctx, id)
	if err != nil {
		return err
	}

	// Valida transição
	if err := ValidateStatusTransitionWithContext(ctx, task.TaskListID, task.StatusID, newStatusID); err != nil {
		return err
	}

	// Atualiza status e data de conclusão se for o status final
	updates := map[string]interface{}{"status_id": newStatusID}

	// Se o novo status é o último (ordem máxima), marca como concluído
	workflow, _ := GetWorkflowWithContext(ctx, task.TaskListID)
	var statuses []TaskListWorkflowStatus
	_ = json.Unmarshal([]byte(workflow.Statuses), &statuses)

	for _, s := range statuses {
		if s.ID == newStatusID {
			// Se a label contém "Concluído" ou similar, marca CompletedAt
			if s.Label == "Concluído" {
				now := time.Now()
				updates["completed_at"] = now
			}
			break
		}
	}

	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Where("id IN (?)", taskIDs).Updates(updates).Error
}

// ReorderTasksWithContext reordena as tasks dentro de um status/parent
// pertencente ao usuário do contexto.
func ReorderTasksWithContext(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error {
	if _, err := GetTaskListWithContext(ctx, taskListID); err != nil {
		return err
	}
	for i, id := range orderedIDs {
		taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ? AND tasks.task_list_id = ? AND tasks.status_id = ?", id, taskListID, statusID))
		if err := db.WithContext(ctx).Model(&Task{}).
			Where("id = ? AND task_list_id = ? AND status_id = ?", id, taskListID, statusID).
			Where("id IN (?)", taskIDs).
			Update("order", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// PromoteTaskWithContext move uma subtask do usuário do contexto para uma
// task principal (remove parent).
func PromoteTaskWithContext(ctx context.Context, id string) error {
	task, err := GetTaskWithContext(ctx, id)
	if err != nil {
		return err
	}

	if task.ParentID == nil {
		return errors.New("task não é subtask")
	}

	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Where("id IN (?)", taskIDs).Update("parent_id", nil).Error
}

// DemoteTaskWithContext move uma task do usuário do contexto para ser subtask
// de outra (define parent).
func DemoteTaskWithContext(ctx context.Context, id string, parentID string) error {
	task, err := GetTaskWithContext(ctx, id)
	if err != nil {
		return err
	}
	parent, err := GetTaskWithContext(ctx, parentID)
	if err != nil {
		return err
	}
	if parent.TaskListID != task.TaskListID {
		return fmt.Errorf("parent_id não pertence à mesma tasklist")
	}
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Where("id IN (?)", taskIDs).Update("parent_id", parentID).Error
}

// MoveTaskToListWithContext move uma task do usuário do contexto (e suas
// subtasks) para outra tasklist do mesmo usuário. Reseta status para o
// inicial do workflow destino, limpa parent e recalcula order.
func MoveTaskToListWithContext(ctx context.Context, taskID string, targetTaskListID string) (*Task, error) {
	task, err := GetTaskWithContext(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task %s não encontrada: %w", taskID, err)
	}
	if _, err := GetTaskListWithContext(ctx, targetTaskListID); err != nil {
		return nil, err
	}

	if task.TaskListID == targetTaskListID {
		return task, nil
	}

	if err := ValidateTaskCodeForTaskListWithContext(ctx, targetTaskListID, task.Code); err != nil {
		return nil, err
	}

	workflow, err := GetWorkflowWithContext(ctx, targetTaskListID)
	if err != nil {
		return nil, fmt.Errorf("workflow da lista destino %s não encontrado: %w", targetTaskListID, err)
	}

	var maxOrder int
	taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ? AND tasks.parent_id IS NULL", targetTaskListID).
		Select(`COALESCE(MAX(tasks."order"), -1)`).
		Scan(&maxOrder)

	updates := map[string]interface{}{
		"task_list_id": targetTaskListID,
		"status_id":    workflow.InitialStatusID,
		"parent_id":    nil,
		"order":        maxOrder + 1,
		"completed_at": nil,
	}
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", taskID))
	if err := db.WithContext(ctx).Model(&Task{}).Where("id = ?", taskID).Where("id IN (?)", taskIDs).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Subtasks acompanham a task pai
	subtaskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.parent_id = ?", taskID))
	db.WithContext(ctx).Model(&Task{}).Where("id IN (?)", subtaskIDs).Updates(map[string]interface{}{
		"task_list_id": targetTaskListID,
		"status_id":    workflow.InitialStatusID,
		"completed_at": nil,
	})

	return GetTaskWithContext(ctx, taskID)
}

// GetSubtasksWithContext retorna todas as subtasks de uma task do usuário do
// contexto.
func GetSubtasksWithContext(ctx context.Context, parentID string) ([]Task, error) {
	var tasks []Task
	err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.parent_id = ?", parentID).
		Order("tasks.`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// DeleteTaskWithContext deleta uma task do usuário do contexto, suas notas e
// todas suas subtasks.
func DeleteTaskWithContext(ctx context.Context, id string) error {
	if _, err := GetTaskWithContext(ctx, id); err != nil {
		return err
	}

	// Deleta notas das subtasks e da task
	var subtaskIDs []string
	taskQuery(ctx, db.Model(&Task{})).Where("tasks.parent_id = ?", id).Pluck("tasks.id", &subtaskIDs)
	allIDs := append(subtaskIDs, id)
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.task_id IN ?", allIDs))
	if err := db.WithContext(ctx).Where("id IN (?)", noteIDs).Delete(&TaskNote{}).Error; err != nil {
		return err
	}

	// Deleta subtasks
	subtaskQuery := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.parent_id = ?", id))
	if err := db.WithContext(ctx).Where("id IN (?)", subtaskQuery).Delete(&Task{}).Error; err != nil {
		return err
	}

	// Deleta task
	taskIDs := taskQuery(ctx, db.Model(&Task{}).Select("tasks.id").Where("tasks.id = ?", id))
	return db.WithContext(ctx).Where("id IN (?)", taskIDs).Delete(&Task{}).Error
}

// ==================== TaskNote Operations ====================

// CreateTaskNoteWithContext cria uma nova nota/interação para uma task do
// usuário do contexto.
func CreateTaskNoteWithContext(ctx context.Context, taskID string, noteType TaskNoteType, content, authorName, authorID string) (*TaskNote, error) {
	if _, err := GetTaskWithContext(ctx, taskID); err != nil {
		return nil, fmt.Errorf("task %s não encontrada: %w", taskID, err)
	}
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	note := &TaskNote{
		UserID:     userID,
		TaskID:     taskID,
		Type:       noteType,
		Content:    content,
		AuthorName: authorName,
		AuthorID:   authorID,
	}

	if err := db.WithContext(ctx).Create(note).Error; err != nil {
		return nil, err
	}

	return note, nil
}

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint failed") ||
		strings.Contains(s, "constraint failed: UNIQUE") ||
		strings.Contains(s, "SQLITE_CONSTRAINT_UNIQUE")
}

// FindTaskNoteByExternalRefWithContext retorna a nota com a origem e ID
// externos informados pertencente ao usuário do contexto, ou nil se não
// existir.
func FindTaskNoteByExternalRefWithContext(ctx context.Context, externalSource, externalID string) (*TaskNote, error) {
	src := strings.TrimSpace(externalSource)
	ext := strings.TrimSpace(externalID)
	if src == "" || ext == "" {
		return nil, nil
	}
	var note TaskNote
	err := taskNoteQuery(ctx, db.Model(&TaskNote{})).
		Where("task_notes.external_source = ? AND task_notes.external_id = ?", src, ext).
		First(&note).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &note, nil
}

func applyTaskNoteExternalUpsertUpdatesWithContext(ctx context.Context, noteID string, p UpsertTaskNoteByExternalParams) error {
	updates := map[string]interface{}{
		"content":            p.Content,
		"author_name":        strings.TrimSpace(p.AuthorName),
		"author_id":          strings.TrimSpace(p.AuthorID),
		"external_parent_id": strings.TrimSpace(p.ExternalParentID),
		"external_source":    strings.TrimSpace(p.ExternalSource),
		"external_id":        strings.TrimSpace(p.ExternalID),
	}
	if p.ExternalUpdatedAt != nil {
		updates["external_updated_at"] = p.ExternalUpdatedAt
	}
	if p.Type != nil {
		updates["type"] = *p.Type
	}
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.id = ?", noteID))
	return db.WithContext(ctx).Model(&TaskNote{}).Where("id = ?", noteID).Where("id IN (?)", noteIDs).Updates(updates).Error
}

// UpsertTaskNoteByExternalWithContext cria ou atualiza uma nota de forma
// idempotente usando external_source + external_id, no escopo do usuário do
// contexto. O segundo retorno indica se a nota foi criada (true) ou apenas
// atualizada (false).
func UpsertTaskNoteByExternalWithContext(ctx context.Context, p UpsertTaskNoteByExternalParams) (*TaskNote, bool, error) {
	src := strings.TrimSpace(p.ExternalSource)
	ext := strings.TrimSpace(p.ExternalID)
	if src == "" || ext == "" {
		return nil, false, fmt.Errorf("external_source e external_id são obrigatórios para upsert externo")
	}

	task, err := GetTaskWithContext(ctx, p.TaskID)
	if err != nil {
		return nil, false, fmt.Errorf("task %s não encontrada: %w", p.TaskID, err)
	}

	if err := ValidateExternalNoteForTaskListWithContext(ctx, task.TaskListID, src, ext, strings.TrimSpace(p.ExternalParentID)); err != nil {
		return nil, false, err
	}

	existing, err := FindTaskNoteByExternalRefWithContext(ctx, src, ext)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.TaskID != p.TaskID {
			return nil, false, fmt.Errorf(
				"nota com source=%q external_id=%q já existe na task %s; recusado vincular à task %s",
				src, ext, existing.TaskID, p.TaskID,
			)
		}
		if p.Type == nil {
			updates := map[string]interface{}{
				"content":            p.Content,
				"author_name":        strings.TrimSpace(p.AuthorName),
				"author_id":          strings.TrimSpace(p.AuthorID),
				"external_parent_id": strings.TrimSpace(p.ExternalParentID),
			}
			if p.ExternalUpdatedAt != nil {
				updates["external_updated_at"] = p.ExternalUpdatedAt
			}
			return finishNoteUpdateWithContext(ctx, existing.ID, updates)
		}
		if err := applyTaskNoteExternalUpsertUpdatesWithContext(ctx, existing.ID, p); err != nil {
			return nil, false, err
		}
		out, err := GetTaskNoteWithContext(ctx, existing.ID)
		return out, false, err
	}

	if p.Type == nil {
		return nil, false, fmt.Errorf("type é obrigatório ao criar nota externa nova")
	}

	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, false, err
	}

	note := &TaskNote{
		UserID:            userID,
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

	if err := db.WithContext(ctx).Create(note).Error; err != nil {
		if !isSQLiteUniqueConstraintError(err) {
			return nil, false, err
		}
		// Corrida: outra goroutine criou a mesma referência — reconsultar e atualizar.
		again, e2 := FindTaskNoteByExternalRefWithContext(ctx, src, ext)
		if e2 != nil || again == nil {
			return nil, false, fmt.Errorf("criação conflitante e reconsulta falhou: %w", err)
		}
		if again.TaskID != p.TaskID {
			return nil, false, fmt.Errorf(
				"nota com source=%q external_id=%q já existe na task %s; recusado vincular à task %s",
				src, ext, again.TaskID, p.TaskID,
			)
		}
		if err := applyTaskNoteExternalUpsertUpdatesWithContext(ctx, again.ID, p); err != nil {
			return nil, false, err
		}
		out, err := GetTaskNoteWithContext(ctx, again.ID)
		return out, false, err
	}

	return note, true, nil
}

func finishNoteUpdateWithContext(ctx context.Context, noteID string, updates map[string]interface{}) (*TaskNote, bool, error) {
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.id = ?", noteID))
	if err := db.WithContext(ctx).Model(&TaskNote{}).Where("id = ?", noteID).Where("id IN (?)", noteIDs).Updates(updates).Error; err != nil {
		return nil, false, err
	}
	out, err := GetTaskNoteWithContext(ctx, noteID)
	return out, false, err
}

// GetTaskNoteWithContext retorna uma nota do usuário do contexto pelo ID.
func GetTaskNoteWithContext(ctx context.Context, noteID string) (*TaskNote, error) {
	var note TaskNote
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).First(&note, "task_notes.id = ?", noteID).Error; err != nil {
		return nil, fmt.Errorf("note %s não encontrada: %w", noteID, err)
	}
	return &note, nil
}

// GetTaskNotesWithContext retorna todas as notas de uma task do usuário do
// contexto, ordenadas cronologicamente.
func GetTaskNotesWithContext(ctx context.Context, taskID string) ([]TaskNote, error) {
	var notes []TaskNote
	err := taskNoteQuery(ctx, db.Model(&TaskNote{})).
		Where("task_notes.task_id = ?", taskID).
		Order("task_notes.created_at ASC").
		Find(&notes).Error
	return notes, err
}

// UpdateTaskNoteWithContext atualiza o conteúdo de uma nota existente do
// usuário do contexto.
func UpdateTaskNoteWithContext(ctx context.Context, noteID string, content string) error {
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.id = ?", noteID))
	return db.WithContext(ctx).Model(&TaskNote{}).
		Where("id = ?", noteID).
		Where("id IN (?)", noteIDs).
		Update("content", content).Error
}

// DeleteTaskNoteWithContext remove uma nota do usuário do contexto.
func DeleteTaskNoteWithContext(ctx context.Context, noteID string) error {
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.id = ?", noteID))
	return db.WithContext(ctx).Where("id IN (?)", noteIDs).Delete(&TaskNote{}).Error
}

// DeleteTaskNotesWithContext remove todas as notas de uma task do usuário do
// contexto (usado em cascata ao deletar tasks).
func DeleteTaskNotesWithContext(ctx context.Context, taskID string) error {
	noteIDs := taskNoteQuery(ctx, db.Model(&TaskNote{}).Select("task_notes.id").Where("task_notes.task_id = ?", taskID))
	return db.WithContext(ctx).Where("id IN (?)", noteIDs).Delete(&TaskNote{}).Error
}

// ==================== Utility Functions ====================

// GetTaskListStatsWithContext retorna estatísticas de uma tasklist do usuário
// do contexto (total, por status).
func GetTaskListStatsWithContext(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	if _, err := GetTaskListWithContext(ctx, taskListID); err != nil {
		return nil, err
	}
	var total int64
	byStatus := make(map[string]int64)

	// Total de tasks
	taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", taskListID).Count(&total)

	// Por status
	var counts []struct {
		StatusID int
		Count    int64
	}
	taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ?", taskListID).
		Group("tasks.status_id").
		Select("tasks.status_id, count(*) as count").
		Scan(&counts)

	for _, c := range counts {
		byStatus[fmt.Sprintf("%d", c.StatusID)] = c.Count
	}

	return map[string]interface{}{
		"total":    total,
		"byStatus": byStatus,
	}, nil
}

// GetTaskListWithHierarchyWithContext retorna uma tasklist do usuário do
// contexto com hierarquia completa de tasks.
func GetTaskListWithHierarchyWithContext(ctx context.Context, id string) (*TaskList, error) {
	taskList, err := GetTaskListWithContext(ctx, id)
	if err != nil {
		return nil, err
	}

	// Busca todas as tasks de uma vez
	var allTasks []Task
	if err := WithSQLiteBusyRetry(ctx, "tasklist.hierarchy.tasks", func() error {
		return taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", id).
			Order(`tasks."order" ASC, tasks.created_at ASC`).
			Find(&allTasks).Error
	}); err != nil {
		return nil, err
	}

	// Constrói hierarquia manualmente preservando profundidade arbitrária.
	taskMap := make(map[string]*Task)
	childrenByParentID := make(map[string][]*Task)
	rootTaskIDs := make([]string, 0)

	for i := range allTasks {
		allTasks[i].Subtasks = nil
		taskMap[allTasks[i].ID] = &allTasks[i]
	}

	for i := range allTasks {
		task := &allTasks[i]
		if task.ParentID != nil {
			childrenByParentID[*task.ParentID] = append(childrenByParentID[*task.ParentID], task)
			continue
		}
		rootTaskIDs = append(rootTaskIDs, task.ID)
	}

	var buildTaskTree func(task *Task) Task
	buildTaskTree = func(task *Task) Task {
		cloned := *task
		children := childrenByParentID[task.ID]
		cloned.Subtasks = make([]Task, 0, len(children))
		for _, child := range children {
			cloned.Subtasks = append(cloned.Subtasks, buildTaskTree(child))
		}
		return cloned
	}

	rootTasks := make([]Task, 0, len(rootTaskIDs))
	for _, rootID := range rootTaskIDs {
		if rootTask, ok := taskMap[rootID]; ok {
			rootTasks = append(rootTasks, buildTaskTree(rootTask))
		}
	}

	taskList.Tasks = rootTasks
	return taskList, nil
}

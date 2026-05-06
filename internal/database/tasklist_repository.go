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

// ==================== TaskList Operations ====================

// CreateTaskList cria uma nova tasklist com workflow padrão
// templateWorkflow pode ser nil para usar workflow padrão (A Fazer, Em Progresso, Concluído)
// slug: opcional; normalizado e único quando não vazio.
func CreateTaskList(title, description string, templateWorkflow *TaskListWorkflow, slug string) (*TaskList, error) {
	return CreateTaskListWithContext(context.Background(), title, description, templateWorkflow, slug)
}

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
	workflow, err := createWorkflowForTaskList(taskList.ID, templateWorkflow)
	if err != nil {
		return nil, err
	}

	taskList.Workflow = workflow

	if s := NormalizeTaskListSlug(slug); s != "" {
		if err := SetTaskListSlug(taskList.ID, s); err != nil {
			return nil, err
		}
		taskList.Slug = s
	}

	return taskList, nil
}

// GetTaskList retorna uma tasklist pelo ID com workflow e tasks
func GetTaskList(id string) (*TaskList, error) {
	return GetTaskListWithContext(context.Background(), id)
}

func GetTaskListWithContext(ctx context.Context, id string) (*TaskList, error) {
	var taskList TaskList
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Preload("Workflow").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Where("parent_id IS NULL").Order("`order` ASC")
		}).
		Preload("Tasks.Subtasks", func(db *gorm.DB) *gorm.DB {
			return db.Order("`order` ASC")
		}).
		First(&taskList, "id = ?", id).Error

	return &taskList, err
}

// GetAllTaskLists retorna todas as tasklists ordenadas por data de criação
func GetAllTaskLists() ([]TaskList, error) {
	return GetAllTaskListsWithContext(context.Background())
}

func GetAllTaskListsWithContext(ctx context.Context) ([]TaskList, error) {
	var taskLists []TaskList
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Preload("Workflow").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Where("parent_id IS NULL").Order("`order` ASC")
		}).
		Order("created_at DESC").
		Find(&taskLists).Error
	return taskLists, err
}

// UpdateTaskList atualiza title e description de uma tasklist
func UpdateTaskList(id string, title, description string) error {
	return UpdateTaskListWithContext(context.Background(), id, title, description)
}

func UpdateTaskListWithContext(ctx context.Context, id string, title, description string) error {
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":       title,
			"description": description,
		}).Error
}

// SetTaskListViewMode define o modo de visualização (list ou kanban)
func SetTaskListViewMode(id string, viewMode string) error {
	return SetTaskListViewModeWithContext(context.Background(), id, viewMode)
}

func SetTaskListViewModeWithContext(ctx context.Context, id string, viewMode string) error {
	if viewMode != "list" && viewMode != "kanban" {
		return errors.New("view mode inválido: use 'list' ou 'kanban'")
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", id).Update("preferred_view_mode", viewMode).Error
}

// CloneTaskList clona uma tasklist com seu workflow mas sem as tasks
func CloneTaskList(id string, newTitle string) (*TaskList, error) {
	// Busca tasklist original
	original, err := GetTaskList(id)
	if err != nil {
		return nil, err
	}

	// Cria nova tasklist
	cloned, err := CreateTaskList(newTitle, original.Description, original.Workflow, "")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(original.ValidationPolicy) != "" {
		if err := SetTaskListValidationPolicy(cloned.ID, original.ValidationPolicy); err != nil {
			return nil, err
		}
		cloned.ValidationPolicy = original.ValidationPolicy
	}

	return cloned, nil
}

// ClearTaskList remove todas as tasks de uma tasklist, mantendo a lista e o workflow
func ClearTaskList(id string) error {
	// Deleta notas de todas as tasks da lista
	if err := db.Where("task_id IN (?)", db.Model(&Task{}).Select("id").Where("task_list_id = ?", id)).
		Delete(&TaskNote{}).Error; err != nil {
		return err
	}
	if err := db.Where("task_list_id = ?", id).Delete(&Task{}).Error; err != nil {
		return err
	}
	return nil
}

// DeleteTaskList deleta uma tasklist e todas suas tasks
func DeleteTaskList(id string) error {
	// Deleta notas de todas as tasks da lista
	if err := db.Where("task_id IN (?)", db.Model(&Task{}).Select("id").Where("task_list_id = ?", id)).
		Delete(&TaskNote{}).Error; err != nil {
		return err
	}

	// Deleta tasks
	if err := db.Where("task_list_id = ?", id).Delete(&Task{}).Error; err != nil {
		return err
	}

	// Deleta workflow
	if err := db.Where("task_list_id = ?", id).Delete(&TaskListWorkflow{}).Error; err != nil {
		return err
	}

	// Deleta tasklist
	return db.Where("id = ?", id).Delete(&TaskList{}).Error
}

// ==================== Workflow Operations ====================

// createWorkflowForTaskList cria um workflow padrão ou a partir de um template
func createWorkflowForTaskList(taskListID string, template *TaskListWorkflow) (*TaskListWorkflow, error) {
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

	if err := db.Create(workflow).Error; err != nil {
		return nil, err
	}

	return workflow, nil
}

// GetWorkflow retorna o workflow de uma tasklist
func GetWorkflow(taskListID string) (*TaskListWorkflow, error) {
	var workflow TaskListWorkflow
	err := db.Where("task_list_id = ?", taskListID).First(&workflow).Error
	return &workflow, err
}

// UpdateWorkflow atualiza statuses e/ou transitions de um workflow
func UpdateWorkflow(taskListID string, statuses []TaskListWorkflowStatus, transitions TaskListWorkflowTransitions) error {
	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)

	return db.Model(&TaskListWorkflow{}).
		Where("task_list_id = ?", taskListID).
		Updates(map[string]interface{}{
			"statuses":            string(statusesJSON),
			"allowed_transitions": string(transitionsJSON),
		}).Error
}

// UpdateWorkflowFull atualiza statuses, transitions e initial_status_id de um workflow
// com validação completa:
// - initial_status_id deve existir nos statuses
// - todas as transições devem referenciar status IDs válidos
// - status IDs em uso por tasks não podem ser removidos (a menos que status_migration mapeie-os)
// - status_migration pode ser nil se nenhum status será removido
func UpdateWorkflowFull(
	taskListID string,
	statuses []TaskListWorkflowStatus,
	transitions TaskListWorkflowTransitions,
	initialStatusID int,
	statusMigration map[int]int,
) error {
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

	counts, err := GetTaskCountsByStatus(taskListID)
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

	return db.Transaction(func(tx *gorm.DB) error {
		for oldID, newID := range statusMigration {
			if err := tx.Model(&Task{}).
				Where("task_list_id = ? AND status_id = ?", taskListID, oldID).
				Update("status_id", newID).Error; err != nil {
				return fmt.Errorf("erro ao migrar tasks de status %d para %d: %w", oldID, newID, err)
			}
		}

		statusesJSON, _ := json.Marshal(statuses)
		transitionsJSON, _ := json.Marshal(transitions)

		return tx.Model(&TaskListWorkflow{}).
			Where("task_list_id = ?", taskListID).
			Updates(map[string]interface{}{
				"statuses":            string(statusesJSON),
				"allowed_transitions": string(transitionsJSON),
				"initial_status_id":   initialStatusID,
			}).Error
	})
}

// GetTaskCountsByStatus retorna a contagem de tasks por status_id para uma tasklist
func GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	var counts []struct {
		StatusID int
		Count    int64
	}
	err := db.Model(&Task{}).
		Where("task_list_id = ?", taskListID).
		Group("status_id").
		Select("status_id, count(*) as count").
		Scan(&counts).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]int64, len(counts))
	for _, c := range counts {
		result[c.StatusID] = c.Count
	}
	return result, nil
}

// UpdateTaskListFull atualiza title, description e preferred_view_mode de uma tasklist.
// slug: nil = não altera slug; ponteiro para string vazia = limpa slug; valor = define slug normalizado.
func UpdateTaskListFull(id string, title, description, preferredViewMode string, slug *string) error {
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
			taken, err := slugTakenByOtherThan(s, id)
			if err != nil {
				return err
			}
			if taken {
				return fmt.Errorf("slug %q já está em uso por outra lista", s)
			}
		}
		updates["slug"] = s
	}
	return db.Model(&TaskList{}).Where("id = ?", id).Updates(updates).Error
}

// ReorderWorkflowStatuses reordena os statuses mantendo seus IDs e labels
func ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	// Busca workflow atual
	workflow, err := GetWorkflow(taskListID)
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
	return db.Model(&TaskListWorkflow{}).
		Where("task_list_id = ?", taskListID).
		Update("statuses", string(statusesJSON)).Error
}

// ValidateStatusTransition valida se uma transição de status é permitida.
// Permite transição livre quando o status atual é inválido/vazio (0 ou ausente no workflow),
// desde que o status destino exista no workflow.
func ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	if fromStatusID == toStatusID {
		return nil
	}

	workflow, err := GetWorkflow(taskListID)
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

// CreateTask cria uma nova task em uma tasklist
func CreateTask(taskListID string, title, description, code, link string, parentID *string) (*Task, error) {
	// Busca workflow para status inicial
	workflow, err := GetWorkflow(taskListID)
	if err != nil {
		return nil, err
	}

	// Calcula próxima ordem
	var maxOrder int
	query := db.Model(&Task{}).Where("task_list_id = ?", taskListID)
	if parentID != nil {
		query = query.Where("parent_id = ?", parentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	query.Select(`COALESCE(MAX("order"), -1)`).Scan(&maxOrder)

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

	if err := ValidateTaskCodeForTaskList(taskListID, code); err != nil {
		return nil, err
	}

	if err := db.Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// CreateTaskFull cria uma nova task com todos os campos, incluindo assignee e creator
func CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*Task, error) {
	workflow, err := GetWorkflow(taskListID)
	if err != nil {
		return nil, err
	}

	var maxOrder int
	query := db.Model(&Task{}).Where("task_list_id = ?", taskListID)
	if parentID != nil {
		query = query.Where("parent_id = ?", parentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	query.Select(`COALESCE(MAX("order"), -1)`).Scan(&maxOrder)

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

	if err := ValidateTaskCodeForTaskList(taskListID, code); err != nil {
		return nil, err
	}

	if err := db.Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// FindTaskByCode busca uma task pelo code dentro de uma tasklist.
// Retorna nil, nil se nao encontrar.
func FindTaskByCode(taskListID string, code string) (*Task, error) {
	var task Task
	err := db.Where("task_list_id = ? AND code = ?", taskListID, code).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetTask retorna uma task com subtasks
func GetTask(id string) (*Task, error) {
	var task Task
	err := db.Preload("Subtasks", func(db *gorm.DB) *gorm.DB {
		return db.Order("`order` ASC")
	}).
		First(&task, "id = ?", id).Error
	return &task, err
}

// GetTasksByTaskListID retorna todas as tasks principais de uma tasklist (sem subtasks nested)
func GetTasksByTaskListID(taskListID string) ([]Task, error) {
	var tasks []Task
	err := db.Where("task_list_id = ? AND parent_id IS NULL", taskListID).
		Order("`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// GetTasksByStatus retorna todas as tasks de um status específico
func GetTasksByStatus(taskListID string, statusID int) ([]Task, error) {
	var tasks []Task
	err := db.Where("task_list_id = ? AND status_id = ? AND parent_id IS NULL", taskListID, statusID).
		Order("`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// UpdateTask atualiza title, description, code e link de uma task
func UpdateTask(id string, title, description, code, link string) error {
	var task Task
	if err := db.First(&task, "id = ?", id).Error; err != nil {
		return err
	}
	if err := ValidateTaskCodeForTaskList(task.TaskListID, code); err != nil {
		return err
	}
	return db.Model(&Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":       title,
			"description": description,
			"code":        code,
			"link":        link,
		}).Error
}

// UpdateTaskFull atualiza todos os campos editáveis de uma task, incluindo assignee e creator
func UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	var task Task
	if err := db.First(&task, "id = ?", id).Error; err != nil {
		return err
	}
	if err := ValidateTaskCodeForTaskList(task.TaskListID, code); err != nil {
		return err
	}
	return db.Model(&Task{}).
		Where("id = ?", id).
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

// UpdateTaskAssignee atualiza apenas o assignee de uma task
func UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	return db.Model(&Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"assignee_name": assigneeName,
			"assignee_id":   assigneeID,
		}).Error
}

// UpdateTaskStatus atualiza o status de uma task com validação de transição
func UpdateTaskStatus(id string, newStatusID int) error {
	// Busca task
	var task Task
	if err := db.First(&task, "id = ?", id).Error; err != nil {
		return err
	}

	// Valida transição
	if err := ValidateStatusTransition(task.TaskListID, task.StatusID, newStatusID); err != nil {
		return err
	}

	// Atualiza status e data de conclusão se for o status final
	updates := map[string]interface{}{"status_id": newStatusID}

	// Se o novo status é o último (ordem máxima), marca como concluído
	workflow, _ := GetWorkflow(task.TaskListID)
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

	return db.Model(&Task{}).Where("id = ?", id).Updates(updates).Error
}

// ReorderTasks reordena as tasks dentro de um status/parent
func ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	for i, id := range orderedIDs {
		if err := db.Model(&Task{}).
			Where("id = ? AND task_list_id = ? AND status_id = ?", id, taskListID, statusID).
			Update("order", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// PromoteTask move uma subtask para uma task principal (remove parent)
func PromoteTask(id string) error {
	var task Task
	if err := db.First(&task, "id = ?", id).Error; err != nil {
		return err
	}

	if task.ParentID == nil {
		return errors.New("task não é subtask")
	}

	return db.Model(&Task{}).Where("id = ?", id).Update("parent_id", nil).Error
}

// DemoteTask move uma task para ser subtask de outra (define parent)
func DemoteTask(id string, parentID string) error {
	return db.Model(&Task{}).Where("id = ?", id).Update("parent_id", parentID).Error
}

// MoveTaskToList move uma task (e suas subtasks) para outra tasklist.
// Reseta status para o inicial do workflow destino, limpa parent e recalcula order.
func MoveTaskToList(taskID string, targetTaskListID string) (*Task, error) {
	var task Task
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, fmt.Errorf("task %s não encontrada: %w", taskID, err)
	}

	if task.TaskListID == targetTaskListID {
		return &task, nil
	}

	if err := ValidateTaskCodeForTaskList(targetTaskListID, task.Code); err != nil {
		return nil, err
	}

	workflow, err := GetWorkflow(targetTaskListID)
	if err != nil {
		return nil, fmt.Errorf("workflow da lista destino %s não encontrado: %w", targetTaskListID, err)
	}

	var maxOrder int
	db.Model(&Task{}).
		Where("task_list_id = ? AND parent_id IS NULL", targetTaskListID).
		Select(`COALESCE(MAX("order"), -1)`).
		Scan(&maxOrder)

	updates := map[string]interface{}{
		"task_list_id": targetTaskListID,
		"status_id":    workflow.InitialStatusID,
		"parent_id":    nil,
		"order":        maxOrder + 1,
		"completed_at": nil,
	}
	if err := db.Model(&Task{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Subtasks acompanham a task pai
	db.Model(&Task{}).Where("parent_id = ?", taskID).Updates(map[string]interface{}{
		"task_list_id": targetTaskListID,
		"status_id":    workflow.InitialStatusID,
		"completed_at": nil,
	})

	return GetTask(taskID)
}

// GetSubtasks retorna todas as subtasks de uma task
func GetSubtasks(parentID string) ([]Task, error) {
	var tasks []Task
	err := db.Where("parent_id = ?", parentID).
		Order("`order` ASC").
		Find(&tasks).Error
	return tasks, err
}

// DeleteTask deleta uma task, suas notas e todas suas subtasks
func DeleteTask(id string) error {
	var task Task
	if err := db.First(&task, "id = ?", id).Error; err != nil {
		return err
	}

	// Deleta notas das subtasks e da task
	var subtaskIDs []string
	db.Model(&Task{}).Where("parent_id = ?", id).Pluck("id", &subtaskIDs)
	allIDs := append(subtaskIDs, id)
	if err := db.Where("task_id IN ?", allIDs).Delete(&TaskNote{}).Error; err != nil {
		return err
	}

	// Deleta subtasks
	if err := db.Where("parent_id = ?", id).Delete(&Task{}).Error; err != nil {
		return err
	}

	// Deleta task
	return db.Where("id = ?", id).Delete(&Task{}).Error
}

// ==================== TaskNote Operations ====================

// CreateTaskNote cria uma nova nota/interação para uma task.
func CreateTaskNote(taskID string, noteType TaskNoteType, content, authorName, authorID string) (*TaskNote, error) {
	var task Task
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, fmt.Errorf("task %s não encontrada: %w", taskID, err)
	}

	note := &TaskNote{
		TaskID:     taskID,
		Type:       noteType,
		Content:    content,
		AuthorName: authorName,
		AuthorID:   authorID,
	}

	if err := db.Create(note).Error; err != nil {
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

// FindTaskNoteByExternalRef retorna a nota com a origem e ID externos informados, ou nil se não existir.
func FindTaskNoteByExternalRef(externalSource, externalID string) (*TaskNote, error) {
	src := strings.TrimSpace(externalSource)
	ext := strings.TrimSpace(externalID)
	if src == "" || ext == "" {
		return nil, nil
	}
	var note TaskNote
	err := db.Where("external_source = ? AND external_id = ?", src, ext).First(&note).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &note, nil
}

func applyTaskNoteExternalUpsertUpdates(noteID string, p UpsertTaskNoteByExternalParams) error {
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
	return db.Model(&TaskNote{}).Where("id = ?", noteID).Updates(updates).Error
}

// UpsertTaskNoteByExternal cria ou atualiza uma nota de forma idempotente usando external_source + external_id.
// O segundo retorno indica se a nota foi criada (true) ou apenas atualizada (false).
func UpsertTaskNoteByExternal(p UpsertTaskNoteByExternalParams) (*TaskNote, bool, error) {
	src := strings.TrimSpace(p.ExternalSource)
	ext := strings.TrimSpace(p.ExternalID)
	if src == "" || ext == "" {
		return nil, false, fmt.Errorf("external_source e external_id são obrigatórios para upsert externo")
	}

	var task Task
	if err := db.First(&task, "id = ?", p.TaskID).Error; err != nil {
		return nil, false, fmt.Errorf("task %s não encontrada: %w", p.TaskID, err)
	}

	if err := ValidateExternalNoteForTaskList(task.TaskListID, src, ext, strings.TrimSpace(p.ExternalParentID)); err != nil {
		return nil, false, err
	}

	existing, err := FindTaskNoteByExternalRef(src, ext)
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
			return finishNoteUpdate(existing.ID, updates)
		}
		if err := applyTaskNoteExternalUpsertUpdates(existing.ID, p); err != nil {
			return nil, false, err
		}
		out, err := GetTaskNote(existing.ID)
		return out, false, err
	}

	if p.Type == nil {
		return nil, false, fmt.Errorf("type é obrigatório ao criar nota externa nova")
	}

	note := &TaskNote{
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

	if err := db.Create(note).Error; err != nil {
		if !isSQLiteUniqueConstraintError(err) {
			return nil, false, err
		}
		// Corrida: outra goroutine criou a mesma referência — reconsultar e atualizar.
		again, e2 := FindTaskNoteByExternalRef(src, ext)
		if e2 != nil || again == nil {
			return nil, false, fmt.Errorf("criação conflitante e reconsulta falhou: %w", err)
		}
		if again.TaskID != p.TaskID {
			return nil, false, fmt.Errorf(
				"nota com source=%q external_id=%q já existe na task %s; recusado vincular à task %s",
				src, ext, again.TaskID, p.TaskID,
			)
		}
		if err := applyTaskNoteExternalUpsertUpdates(again.ID, p); err != nil {
			return nil, false, err
		}
		out, err := GetTaskNote(again.ID)
		return out, false, err
	}

	return note, true, nil
}

func finishNoteUpdate(noteID string, updates map[string]interface{}) (*TaskNote, bool, error) {
	if err := db.Model(&TaskNote{}).Where("id = ?", noteID).Updates(updates).Error; err != nil {
		return nil, false, err
	}
	out, err := GetTaskNote(noteID)
	return out, false, err
}

// GetTaskNote retorna uma nota pelo ID
func GetTaskNote(noteID string) (*TaskNote, error) {
	var note TaskNote
	if err := db.First(&note, "id = ?", noteID).Error; err != nil {
		return nil, fmt.Errorf("note %s não encontrada: %w", noteID, err)
	}
	return &note, nil
}

// GetTaskNotes retorna todas as notas de uma task ordenadas cronologicamente
func GetTaskNotes(taskID string) ([]TaskNote, error) {
	var notes []TaskNote
	err := db.Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&notes).Error
	return notes, err
}

// UpdateTaskNote atualiza o conteúdo de uma nota existente
func UpdateTaskNote(noteID string, content string) error {
	return db.Model(&TaskNote{}).
		Where("id = ?", noteID).
		Update("content", content).Error
}

// DeleteTaskNote remove uma nota
func DeleteTaskNote(noteID string) error {
	return db.Where("id = ?", noteID).Delete(&TaskNote{}).Error
}

// DeleteTaskNotes remove todas as notas de uma task (usado em cascata ao deletar tasks)
func DeleteTaskNotes(taskID string) error {
	return db.Where("task_id = ?", taskID).Delete(&TaskNote{}).Error
}

// ==================== Utility Functions ====================

// GetTaskListStats retorna estatísticas de uma tasklist (total, por status)
func GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	var total int64
	byStatus := make(map[string]int64)

	// Total de tasks
	db.Model(&Task{}).Where("task_list_id = ?", taskListID).Count(&total)

	// Por status
	var counts []struct {
		StatusID int
		Count    int64
	}
	db.Model(&Task{}).
		Where("task_list_id = ?", taskListID).
		Group("status_id").
		Select("status_id, count(*) as count").
		Scan(&counts)

	for _, c := range counts {
		byStatus[fmt.Sprintf("%d", c.StatusID)] = c.Count
	}

	return map[string]interface{}{
		"total":    total,
		"byStatus": byStatus,
	}, nil
}

// GetTaskListWithHierarchy retorna uma tasklist com hierarquia completa de tasks
func GetTaskListWithHierarchy(id string) (*TaskList, error) {
	taskList, err := GetTaskList(id)
	if err != nil {
		return nil, err
	}

	// Busca todas as tasks de uma vez
	var allTasks []Task
	if err := db.Where("task_list_id = ?", id).
		Order(`"order" ASC, created_at ASC`).
		Find(&allTasks).Error; err != nil {
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

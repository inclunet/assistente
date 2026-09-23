package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrTaskListCommandStale indica que o alvo mudou desde o snapshot capturado
// para o comando. A comparação sempre acontece novamente dentro da transação
// final; este erro não deve ser convertido em sucesso pelo caller.
var ErrTaskListCommandStale = errors.New("tasklist changed before command commit")

// TaskListCommandMutationRequest é o conjunto selado de parâmetros de uma
// mutação de comando. O UserID não faz parte do request: ele vem do contexto
// autenticado e é validado por RequireUserID.
type TaskListCommandMutationRequest struct {
	Operation           string
	ID                  string
	Title               string
	Description         string
	ExpectedFingerprint string
}

// ReadTaskListCommandTargetWithContext devolve o payload de edição e seu
// fingerprint calculados no mesmo snapshot consistente de leitura.
//
// O fingerprint inclui metadados, workflow, tasks e notas. Isso mantém um
// único token válido para edição, clonagem e exclusão; em particular, uma
// exclusão não pode apagar uma lista depois que uma task/nota foi criada ou
// alterada. O owner é obrigatório e entra no material autenticado.
func ReadTaskListCommandTargetWithContext(ctx context.Context, id string) (*TaskList, string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, "", err
	}
	var target *TaskList
	var fingerprint string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		target, fingerprint, err = readTaskListCommandTargetTx(ctx, tx, id)
		return err
	})
	return target, fingerprint, err
}

// CommitTaskListCommandMutationWithContext compara e persiste uma mutação de
// tasklist em uma única transação BEGIN IMMEDIATE. A função não faz retry de
// SQLITE_BUSY: ela é o trecho final que deve ficar sob o guard de auth/
// workspace, portanto uma contenção retorna rapidamente ao caller.
func CommitTaskListCommandMutationWithContext(ctx context.Context, request TaskListCommandMutationRequest) (*TaskList, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	request.Operation = strings.ToLower(strings.TrimSpace(request.Operation))
	if !validTaskListCommandOperation(request.Operation) {
		return nil, fmt.Errorf("operação de tasklist inválida: %q", request.Operation)
	}
	if request.Operation == "create" {
		if request.ID != "" || request.ExpectedFingerprint != "" {
			return nil, ErrTaskListCommandStale
		}
	} else if request.ID == "" || request.ExpectedFingerprint == "" {
		return nil, ErrTaskListCommandStale
	}

	var result *TaskList
	err := withTaskListCommandTransaction(ctx, func(tx *gorm.DB) error {
		var err error
		switch request.Operation {
		case "create":
			result, err = createTaskListCommandTx(ctx, tx, request.Title, request.Description)
		case "update":
			result, err = updateTaskListCommandTx(ctx, tx, request)
		case "clone":
			result, err = cloneTaskListCommandTx(ctx, tx, request)
		case "clear":
			result, err = clearTaskListCommandTx(ctx, tx, request)
		case "delete":
			result, err = deleteTaskListCommandTx(ctx, tx, request)
		}
		return err
	})
	return result, err
}

func validTaskListCommandOperation(operation string) bool {
	switch operation {
	case "create", "update", "clone", "clear", "delete":
		return true
	default:
		return false
	}
}

func readTaskListCommandTargetTx(ctx context.Context, tx *gorm.DB, id string) (*TaskList, string, error) {
	ownerID, err := RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(id) == "" {
		return nil, "", gorm.ErrRecordNotFound
	}
	var target TaskList
	query := ScopeByUser(ctx, tx.WithContext(ctx).Model(&TaskList{}), "task_lists.user_id")
	if err := query.Where("task_lists.id = ?", id).First(&target).Error; err != nil {
		return nil, "", err
	}

	var workflow TaskListWorkflow
	if err := ScopeByUser(ctx, tx.WithContext(ctx).Model(&TaskListWorkflow{}).
		Joins("JOIN task_lists ON task_lists.id = task_list_workflows.task_list_id"), "task_lists.user_id").
		Where("task_list_workflows.task_list_id = ?", id).First(&workflow).Error; err != nil {
		return nil, "", err
	}
	target.Workflow = &workflow

	var rootCount int64
	if err := tx.Model(&Task{}).Where("task_list_id = ? AND parent_id IS NULL", id).Count(&rootCount).Error; err != nil {
		return nil, "", err
	}
	target.TaskCount = rootCount

	var tasks []Task
	if err := tx.Model(&Task{}).Where("task_list_id = ?", id).Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, "", err
	}
	var notes []TaskNote
	if err := tx.Model(&TaskNote{}).
		Where("task_id IN (?)", tx.Model(&Task{}).Select("id").Where("task_list_id = ?", id)).
		Order("id ASC").Find(&notes).Error; err != nil {
		return nil, "", err
	}

	fingerprint, err := taskListCommandFingerprint(ownerID, &target, &workflow, tasks, notes)
	if err != nil {
		return nil, "", err
	}
	// O payload de abertura é deliberadamente metadata-only. Tasks/notas foram
	// lidas no mesmo snapshot para formar o guard, mas não são necessárias no
	// formulário e não devem inflar o binding/UI.
	return &target, fingerprint, nil
}

func createTaskListCommandTx(ctx context.Context, tx *gorm.DB, title, description string) (*TaskList, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := tx.Model(&TaskList{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTaskLists {
		return nil, errors.New("limite de tasklists atingido")
	}
	list := &TaskList{UserID: userID, Title: title, Description: description, PreferredViewMode: "list"}
	if err := tx.Create(list).Error; err != nil {
		return nil, err
	}
	workflow, err := createWorkflowForTaskListWithDB(ctx, tx, list.ID, nil)
	if err != nil {
		return nil, err
	}
	list.Workflow = workflow
	return list, nil
}

func updateTaskListCommandTx(ctx context.Context, tx *gorm.DB, request TaskListCommandMutationRequest) (*TaskList, error) {
	current, fingerprint, err := readTaskListCommandTargetTx(ctx, tx, request.ID)
	if err != nil {
		return nil, err
	}
	if fingerprint != request.ExpectedFingerprint {
		return nil, ErrTaskListCommandStale
	}
	updatedAt := nextTaskListUpdatedAt(current.UpdatedAt)
	result := ScopeByUser(ctx, tx.WithContext(ctx).Model(&TaskList{}), "user_id").
		Where("id = ?", request.ID).
		Updates(map[string]interface{}{"title": request.Title, "description": request.Description, "updated_at": updatedAt})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	current.Title = request.Title
	current.Description = request.Description
	current.UpdatedAt = updatedAt
	return current, nil
}

func cloneTaskListCommandTx(ctx context.Context, tx *gorm.DB, request TaskListCommandMutationRequest) (*TaskList, error) {
	source, fingerprint, err := readTaskListCommandTargetTx(ctx, tx, request.ID)
	if err != nil {
		return nil, err
	}
	if fingerprint != request.ExpectedFingerprint {
		return nil, ErrTaskListCommandStale
	}
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := tx.Model(&TaskList{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTaskLists {
		return nil, errors.New("limite de tasklists atingido")
	}

	clone := &TaskList{
		UserID:            userID,
		Title:             request.Title,
		Description:       source.Description,
		PreferredViewMode: source.PreferredViewMode,
		ValidationPolicy:  source.ValidationPolicy,
		CustomActions:     source.CustomActions,
		ConversationID:    cloneStringPointer(source.ConversationID),
		// Slug não é copiado: a semântica existente cria a cópia sem slug.
	}
	if err := tx.Create(clone).Error; err != nil {
		return nil, err
	}
	if source.Workflow == nil {
		return nil, errors.New("workflow da tasklist de origem ausente")
	}
	workflow, err := createWorkflowForTaskListWithDB(ctx, tx, clone.ID, source.Workflow)
	if err != nil {
		return nil, err
	}
	clone.Workflow = workflow
	return clone, nil
}

// clearTaskListCommandTx preserva a semântica de ClearTaskListWithContext:
// remove notas e tarefas (inclusive filhos) da lista, sem alterar a própria
// lista nem seu workflow. A leitura acima é o CAS final e inclui todo o
// conteúdo mutável no fingerprint.
func clearTaskListCommandTx(ctx context.Context, tx *gorm.DB, request TaskListCommandMutationRequest) (*TaskList, error) {
	target, fingerprint, err := readTaskListCommandTargetTx(ctx, tx, request.ID)
	if err != nil {
		return nil, err
	}
	if fingerprint != request.ExpectedFingerprint {
		return nil, ErrTaskListCommandStale
	}

	taskIDs := taskQuery(ctx, tx.Model(&Task{}).Select("tasks.id").Where("tasks.task_list_id = ?", request.ID))
	if err := tx.WithContext(ctx).Where("task_id IN (?)", taskIDs).Delete(&TaskNote{}).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Where("id IN (?)", taskIDs).Delete(&Task{}).Error; err != nil {
		return nil, err
	}
	return target, nil
}

func deleteTaskListCommandTx(ctx context.Context, tx *gorm.DB, request TaskListCommandMutationRequest) (*TaskList, error) {
	target, fingerprint, err := readTaskListCommandTargetTx(ctx, tx, request.ID)
	if err != nil {
		return nil, err
	}
	if fingerprint != request.ExpectedFingerprint {
		return nil, ErrTaskListCommandStale
	}
	taskIDs := tx.Model(&Task{}).Select("id").Where("task_list_id = ?", request.ID)
	if err := tx.Where("task_id IN (?)", taskIDs).Delete(&TaskNote{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("task_list_id = ?", request.ID).Delete(&Task{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("task_list_id = ?", request.ID).Delete(&TaskListWorkflow{}).Error; err != nil {
		return nil, err
	}
	result := ScopeByUser(ctx, tx.WithContext(ctx), "user_id").Where("id = ?", request.ID).Delete(&TaskList{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	return target, nil
}

func taskListCommandFingerprint(ownerID string, list *TaskList, workflow *TaskListWorkflow, tasks []Task, notes []TaskNote) (string, error) {
	type workflowSnapshot struct {
		ID                 string `json:"id"`
		TaskListID         string `json:"task_list_id"`
		Statuses           string `json:"statuses"`
		AllowedTransitions string `json:"allowed_transitions"`
		InitialStatusID    int    `json:"initial_status_id"`
		CreatedAt          string `json:"created_at"`
		UpdatedAt          string `json:"updated_at"`
	}
	type taskSnapshot struct {
		ID             string  `json:"id"`
		TaskListID     string  `json:"task_list_id"`
		Title          string  `json:"title"`
		Description    string  `json:"description"`
		Code           string  `json:"code"`
		Link           string  `json:"link"`
		StatusID       int     `json:"status_id"`
		ParentID       *string `json:"parent_id"`
		Order          int     `json:"order"`
		AssigneeName   string  `json:"assignee_name"`
		AssigneeID     string  `json:"assignee_id"`
		CreatorName    string  `json:"creator_name"`
		CreatorID      string  `json:"creator_id"`
		DueDate        string  `json:"due_date"`
		CompletedAt    string  `json:"completed_at"`
		ConversationID *string `json:"conversation_id"`
		CreatedAt      string  `json:"created_at"`
		UpdatedAt      string  `json:"updated_at"`
	}
	type noteSnapshot struct {
		ID                string       `json:"id"`
		TaskID            string       `json:"task_id"`
		Type              TaskNoteType `json:"type"`
		Content           string       `json:"content"`
		AuthorName        string       `json:"author_name"`
		AuthorID          string       `json:"author_id"`
		ExternalSource    string       `json:"external_source"`
		ExternalID        string       `json:"external_id"`
		ExternalParentID  string       `json:"external_parent_id"`
		ExternalUpdatedAt string       `json:"external_updated_at"`
		CreatedAt         string       `json:"created_at"`
		UpdatedAt         string       `json:"updated_at"`
	}
	type snapshot struct {
		Version           string           `json:"version"`
		OwnerID           string           `json:"owner_id"`
		ID                string           `json:"id"`
		CreatedAt         string           `json:"created_at"`
		UpdatedAt         string           `json:"updated_at"`
		Title             string           `json:"title"`
		Slug              string           `json:"slug"`
		Description       string           `json:"description"`
		PreferredViewMode string           `json:"preferred_view_mode"`
		ValidationPolicy  string           `json:"validation_policy"`
		CustomActions     string           `json:"custom_actions"`
		ConversationID    *string          `json:"conversation_id"`
		Workflow          workflowSnapshot `json:"workflow"`
		Tasks             []taskSnapshot   `json:"tasks"`
		Notes             []noteSnapshot   `json:"notes"`
	}
	if ownerID == "" || list == nil || workflow == nil {
		return "", ErrUserScopeRequired
	}
	value := snapshot{Version: "tasklist-command/v1", OwnerID: ownerID, ID: list.ID, CreatedAt: list.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), UpdatedAt: list.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Title: list.Title, Slug: list.Slug, Description: list.Description, PreferredViewMode: list.PreferredViewMode, ValidationPolicy: list.ValidationPolicy, CustomActions: list.CustomActions, ConversationID: cloneStringPointer(list.ConversationID), Workflow: workflowSnapshot{ID: workflow.ID, TaskListID: workflow.TaskListID, Statuses: workflow.Statuses, AllowedTransitions: workflow.AllowedTransitions, InitialStatusID: workflow.InitialStatusID, CreatedAt: workflow.CreatedAt.UTC().Format(timeLayout), UpdatedAt: workflow.UpdatedAt.UTC().Format(timeLayout)}}
	for _, task := range tasks {
		value.Tasks = append(value.Tasks, taskSnapshot{ID: task.ID, TaskListID: task.TaskListID, Title: task.Title, Description: task.Description, Code: task.Code, Link: task.Link, StatusID: task.StatusID, ParentID: cloneStringPointer(task.ParentID), Order: task.Order, AssigneeName: task.AssigneeName, AssigneeID: task.AssigneeID, CreatorName: task.CreatorName, CreatorID: task.CreatorID, DueDate: nullableTime(task.DueDate), CompletedAt: nullableTime(task.CompletedAt), ConversationID: cloneStringPointer(task.ConversationID), CreatedAt: task.CreatedAt.UTC().Format(timeLayout), UpdatedAt: task.UpdatedAt.UTC().Format(timeLayout)})
	}
	for _, note := range notes {
		value.Notes = append(value.Notes, noteSnapshot{ID: note.ID, TaskID: note.TaskID, Type: note.Type, Content: note.Content, AuthorName: note.AuthorName, AuthorID: note.AuthorID, ExternalSource: note.ExternalSource, ExternalID: note.ExternalID, ExternalParentID: note.ExternalParentID, ExternalUpdatedAt: nullableTime(note.ExternalUpdatedAt), CreatedAt: note.CreatedAt.UTC().Format(timeLayout), UpdatedAt: note.UpdatedAt.UTC().Format(timeLayout)})
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "v1:" + hex.EncodeToString(digest[:]), nil
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

func nullableTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// withTaskListCommandTransaction é uma transação de commit sem retry/busy
// prolongado, própria para ser chamada dentro do guard de lifecycle.
func withTaskListCommandTransaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return db.WithContext(ctx).Connection(func(connection *gorm.DB) error {
		tx := connection.Session(&gorm.Session{SkipDefaultTransaction: true})
		var originalBusyTimeout int
		if err := connection.Raw("PRAGMA busy_timeout").Scan(&originalBusyTimeout).Error; err != nil {
			return err
		}
		// O pool pode ter herdado busy_timeout do SQLite global. O commit de
		// comando deve falhar rápido e deixar a decisão para o caller.
		if err := tx.Exec("PRAGMA busy_timeout=0").Error; err != nil {
			return err
		}
		restoreCtx := context.WithoutCancel(ctx)
		defer func() {
			_ = tx.WithContext(restoreCtx).Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", originalBusyTimeout)).Error
		}()
		if err := tx.Exec("BEGIN IMMEDIATE").Error; err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.WithContext(restoreCtx).Exec("ROLLBACK").Error
			}
		}()
		if err := fn(tx); err != nil {
			return err
		}
		if err := tx.Exec("COMMIT").Error; err != nil {
			return err
		}
		committed = true
		return nil
	})
}

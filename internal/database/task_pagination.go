package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	DefaultTaskPageLimit = 100
	MaxTaskPageLimit     = 100

	TaskSortCreatedAtAsc  = "created_at:asc"
	TaskSortCreatedAtDesc = "created_at:desc"
)

// TaskPageQuery descreve uma consulta paginada de tasks. O cursor é opaco para
// consumidores e fica vinculado à lista, filtro e ordenação que o geraram.
type TaskPageQuery struct {
	TaskListID string
	StatusID   *int
	Limit      int
	Cursor     string
	Sort       string
}

// TaskPage contém uma página plana. Subtasks aparecem como itens independentes
// com ParentID, para que Limit seja um teto real do número de tasks retornadas.
type TaskPage struct {
	TaskList   TaskList
	Tasks      []Task
	NextCursor string
	HasMore    bool
}

type taskPageCursor struct {
	Version    int    `json:"v"`
	TaskListID string `json:"task_list_id"`
	StatusID   *int   `json:"status_id,omitempty"`
	Sort       string `json:"sort"`
	CreatedAt  string `json:"created_at"`
	ID         string `json:"id"`
}

func normalizeTaskPageQuery(query TaskPageQuery) (TaskPageQuery, error) {
	query.TaskListID = strings.TrimSpace(query.TaskListID)
	query.Cursor = strings.TrimSpace(query.Cursor)
	query.Sort = strings.TrimSpace(query.Sort)
	if query.TaskListID == "" {
		return query, errors.New("task_list_id é obrigatório")
	}
	if query.StatusID != nil && *query.StatusID <= 0 {
		return query, errors.New("status_id deve ser maior que zero")
	}
	if query.Limit == 0 {
		query.Limit = DefaultTaskPageLimit
	}
	if query.Limit < 1 || query.Limit > MaxTaskPageLimit {
		return query, fmt.Errorf("limit deve estar entre 1 e %d", MaxTaskPageLimit)
	}
	if query.Sort == "" {
		query.Sort = TaskSortCreatedAtAsc
	}
	switch query.Sort {
	case TaskSortCreatedAtAsc, TaskSortCreatedAtDesc:
	default:
		return query, fmt.Errorf("sort inválido: use %q ou %q", TaskSortCreatedAtAsc, TaskSortCreatedAtDesc)
	}
	return query, nil
}

func decodeTaskPageCursor(encoded string, query TaskPageQuery) (*taskPageCursor, time.Time, error) {
	if encoded == "" {
		return nil, time.Time{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	var cursor taskPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	if cursor.Version != 1 || cursor.TaskListID != query.TaskListID || cursor.Sort != query.Sort ||
		!sameOptionalInt(cursor.StatusID, query.StatusID) || strings.TrimSpace(cursor.ID) == "" {
		return nil, time.Time{}, errors.New("cursor não corresponde à lista, filtro e ordenação informados")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	return &cursor, createdAt, nil
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func encodeTaskPageCursor(query TaskPageQuery, task Task) (string, error) {
	cursor := taskPageCursor{
		Version:    1,
		TaskListID: query.TaskListID,
		StatusID:   query.StatusID,
		Sort:       query.Sort,
		CreatedAt:  task.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:         task.ID,
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ListTasksPageWithContext executa paginação keyset no banco, usando
// (created_at, id) como chave total e determinística. A consulta busca limit+1
// para calcular HasMore sem carregar o restante do backlog em memória.
func ListTasksPageWithContext(ctx context.Context, input TaskPageQuery) (TaskPage, error) {
	query, err := normalizeTaskPageQuery(input)
	if err != nil {
		return TaskPage{}, err
	}
	var taskList TaskList
	err = WithSQLiteBusyRetry(ctx, "tasklist.get.page", func() error {
		return ScopeByUser(ctx, db.WithContext(ctx), "user_id").
			First(&taskList, "id = ?", query.TaskListID).Error
	})
	if err != nil {
		return TaskPage{}, err
	}
	cursor, cursorTime, err := decodeTaskPageCursor(query.Cursor, query)
	if err != nil {
		return TaskPage{}, err
	}

	dbQuery := taskQuery(ctx, db.Model(&Task{})).
		Where("tasks.task_list_id = ?", query.TaskListID)
	if query.StatusID != nil {
		dbQuery = dbQuery.Where("tasks.status_id = ?", *query.StatusID)
	}
	if cursor != nil {
		if query.Sort == TaskSortCreatedAtAsc {
			dbQuery = dbQuery.Where(
				"(tasks.created_at > ?) OR (tasks.created_at = ? AND tasks.id > ?)",
				cursorTime, cursorTime, cursor.ID,
			)
		} else {
			dbQuery = dbQuery.Where(
				"(tasks.created_at < ?) OR (tasks.created_at = ? AND tasks.id < ?)",
				cursorTime, cursorTime, cursor.ID,
			)
		}
	}

	direction := "ASC"
	if query.Sort == TaskSortCreatedAtDesc {
		direction = "DESC"
	}
	var tasks []Task
	err = WithSQLiteBusyRetry(ctx, "tasklist.tasks.page", func() error {
		return dbQuery.
			Order("tasks.created_at " + direction).
			Order("tasks.id " + direction).
			Limit(query.Limit + 1).
			Find(&tasks).Error
	})
	if err != nil {
		return TaskPage{}, err
	}

	page := TaskPage{TaskList: taskList, Tasks: tasks, HasMore: len(tasks) > query.Limit}
	if page.HasMore {
		page.Tasks = tasks[:query.Limit]
		nextCursor, err := encodeTaskPageCursor(query, page.Tasks[len(page.Tasks)-1])
		if err != nil {
			return TaskPage{}, err
		}
		page.NextCursor = nextCursor
	}
	if page.Tasks == nil {
		page.Tasks = []Task{}
	}
	return page, nil
}

func ensureTaskPaginationIndexes(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&Task{}) {
		return nil
	}
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_list_created_id ON tasks (task_list_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_list_status_created_id ON tasks (task_list_id, status_id, created_at, id)`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return fmt.Errorf("criar índice de paginação de tasks: %w", err)
		}
	}
	return nil
}

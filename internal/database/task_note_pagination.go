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

// NullableStringFilter distingue filtro omitido (Set=false), ausência de valor
// (Set=true, Value=nil) e correspondência exata (Set=true, Value!=nil).
type NullableStringFilter struct {
	Set   bool
	Value *string
}

// TaskNotePageQuery descreve uma leitura paginada de notas no escopo do
// usuário. TaskID e os demais filtros são opcionais.
type TaskNotePageQuery struct {
	TaskID           *string
	Source           NullableStringFilter
	Type             *TaskNoteType
	ExternalID       NullableStringFilter
	ExternalParentID NullableStringFilter
	Limit            int
	Cursor           string
	Sort             string
}

type TaskNotePage struct {
	Notes      []TaskNote
	NextCursor string
	HasMore    bool
}

type taskNotePageCursor struct {
	Version             int           `json:"v"`
	TaskID              *string       `json:"task_id,omitempty"`
	SourceSet           bool          `json:"source_set,omitempty"`
	Source              *string       `json:"source,omitempty"`
	Type                *TaskNoteType `json:"type,omitempty"`
	ExternalIDSet       bool          `json:"external_id_set,omitempty"`
	ExternalID          *string       `json:"external_id,omitempty"`
	ExternalParentIDSet bool          `json:"external_parent_id_set,omitempty"`
	ExternalParentID    *string       `json:"external_parent_id,omitempty"`
	Sort                string        `json:"sort"`
	CreatedAt           string        `json:"created_at"`
	ID                  string        `json:"id"`
}

func normalizeTaskNotePageQuery(query TaskNotePageQuery) (TaskNotePageQuery, error) {
	if query.TaskID != nil {
		taskID := strings.TrimSpace(*query.TaskID)
		if taskID == "" {
			return query, errors.New("task_id não pode ser vazio")
		}
		query.TaskID = &taskID
	}
	var err error
	if query.Source, err = normalizeNullableStringFilter("source", query.Source); err != nil {
		return query, err
	}
	if query.ExternalID, err = normalizeNullableStringFilter("external_id", query.ExternalID); err != nil {
		return query, err
	}
	if query.ExternalParentID, err = normalizeNullableStringFilter("external_parent_id", query.ExternalParentID); err != nil {
		return query, err
	}
	if query.Type != nil && (*query.Type < TaskNoteInternal || *query.Type > TaskNoteSystem) {
		return query, errors.New("type deve ser 1, 2, 3 ou 4")
	}
	query.Cursor = strings.TrimSpace(query.Cursor)
	query.Limit, query.Sort, err = normalizePageWindow(query.Limit, query.Sort)
	return query, err
}

func normalizeNullableStringFilter(name string, filter NullableStringFilter) (NullableStringFilter, error) {
	if !filter.Set || filter.Value == nil {
		return filter, nil
	}
	value := strings.TrimSpace(*filter.Value)
	if value == "" {
		return filter, fmt.Errorf("%s deve ser uma string não vazia ou null", name)
	}
	filter.Value = &value
	return filter, nil
}

func normalizePageWindow(limit int, sort string) (int, string, error) {
	if limit == 0 {
		limit = DefaultTaskPageLimit
	}
	if limit < 1 || limit > MaxTaskPageLimit {
		return limit, sort, fmt.Errorf("limit deve estar entre 1 e %d", MaxTaskPageLimit)
	}
	sort = strings.TrimSpace(sort)
	if sort == "" {
		sort = TaskSortCreatedAtAsc
	}
	switch sort {
	case TaskSortCreatedAtAsc, TaskSortCreatedAtDesc:
		return limit, sort, nil
	default:
		return limit, sort, fmt.Errorf("sort inválido: use %q ou %q", TaskSortCreatedAtAsc, TaskSortCreatedAtDesc)
	}
}

func decodeTaskNotePageCursor(encoded string, query TaskNotePageQuery) (*taskNotePageCursor, time.Time, error) {
	if encoded == "" {
		return nil, time.Time{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	var cursor taskNotePageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	if cursor.Version != 1 ||
		!sameOptionalString(cursor.TaskID, query.TaskID) ||
		cursor.SourceSet != query.Source.Set || !sameOptionalString(cursor.Source, query.Source.Value) ||
		!sameOptionalTaskNoteType(cursor.Type, query.Type) ||
		cursor.ExternalIDSet != query.ExternalID.Set || !sameOptionalString(cursor.ExternalID, query.ExternalID.Value) ||
		cursor.ExternalParentIDSet != query.ExternalParentID.Set || !sameOptionalString(cursor.ExternalParentID, query.ExternalParentID.Value) ||
		cursor.Sort != query.Sort || strings.TrimSpace(cursor.ID) == "" {
		return nil, time.Time{}, errors.New("cursor não corresponde aos filtros e à ordenação informados")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return nil, time.Time{}, errors.New("cursor inválido")
	}
	return &cursor, createdAt, nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameOptionalTaskNoteType(left, right *TaskNoteType) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func encodeTaskNotePageCursor(query TaskNotePageQuery, note TaskNote) (string, error) {
	cursor := taskNotePageCursor{
		Version:             1,
		TaskID:              query.TaskID,
		SourceSet:           query.Source.Set,
		Source:              query.Source.Value,
		Type:                query.Type,
		ExternalIDSet:       query.ExternalID.Set,
		ExternalID:          query.ExternalID.Value,
		ExternalParentIDSet: query.ExternalParentID.Set,
		ExternalParentID:    query.ExternalParentID.Value,
		Sort:                query.Sort,
		CreatedAt:           note.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:                  note.ID,
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ListTaskNotesPageWithContext usa keyset (created_at, id), busca limit+1 no
// SQLite e mantém o escopo canônico herdado de task → task_list → user.
func ListTaskNotesPageWithContext(ctx context.Context, input TaskNotePageQuery) (TaskNotePage, error) {
	query, err := normalizeTaskNotePageQuery(input)
	if err != nil {
		return TaskNotePage{}, err
	}
	cursor, cursorTime, err := decodeTaskNotePageCursor(query.Cursor, query)
	if err != nil {
		return TaskNotePage{}, err
	}

	dbQuery := taskNoteQuery(ctx, db.Model(&TaskNote{}))
	if query.TaskID != nil {
		dbQuery = dbQuery.Where("task_notes.task_id = ?", *query.TaskID)
	}
	dbQuery = applyNullableStringFilter(dbQuery, "task_notes.external_source", query.Source)
	if query.Type != nil {
		dbQuery = dbQuery.Where("task_notes.type = ?", *query.Type)
	}
	dbQuery = applyNullableStringFilter(dbQuery, "task_notes.external_id", query.ExternalID)
	dbQuery = applyNullableStringFilter(dbQuery, "task_notes.external_parent_id", query.ExternalParentID)
	if cursor != nil {
		operator := ">"
		if query.Sort == TaskSortCreatedAtDesc {
			operator = "<"
		}
		dbQuery = dbQuery.Where(
			fmt.Sprintf("(task_notes.created_at %s ?) OR (task_notes.created_at = ? AND task_notes.id %s ?)", operator, operator),
			cursorTime, cursorTime, cursor.ID,
		)
	}

	direction := "ASC"
	if query.Sort == TaskSortCreatedAtDesc {
		direction = "DESC"
	}
	var notes []TaskNote
	err = WithSQLiteBusyRetry(ctx, "tasklist.notes.page", func() error {
		return dbQuery.
			Order("task_notes.created_at " + direction).
			Order("task_notes.id " + direction).
			Limit(query.Limit + 1).
			Find(&notes).Error
	})
	if err != nil {
		return TaskNotePage{}, err
	}

	page := TaskNotePage{Notes: notes, HasMore: len(notes) > query.Limit}
	if page.HasMore {
		page.Notes = notes[:query.Limit]
		page.NextCursor, err = encodeTaskNotePageCursor(query, page.Notes[len(page.Notes)-1])
		if err != nil {
			return TaskNotePage{}, err
		}
	}
	if page.Notes == nil {
		page.Notes = []TaskNote{}
	}
	return page, nil
}

func applyNullableStringFilter(query *gorm.DB, column string, filter NullableStringFilter) *gorm.DB {
	if !filter.Set {
		return query
	}
	if filter.Value == nil {
		return query.Where("(" + column + " IS NULL OR " + column + " = '')")
	}
	return query.Where(column+" = ?", *filter.Value)
}

func ensureTaskNotePaginationIndexes(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&TaskNote{}) {
		return nil
	}
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_task_notes_created_id ON task_notes (created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_notes_task_created_id ON task_notes (task_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_notes_source_created_id ON task_notes (external_source, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_notes_type_created_id ON task_notes (type, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_notes_external_id_created_id ON task_notes (external_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_notes_parent_created_id ON task_notes (external_parent_id, created_at, id)`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return fmt.Errorf("criar índice de paginação de task notes: %w", err)
		}
	}
	return nil
}

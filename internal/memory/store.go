package memory

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

type Store interface {
	List(ctx context.Context, filter Filter) (ListResult, error)
	PromptCandidates(ctx context.Context, filter PromptCandidateFilter) ([]database.MemoryRecord, error)
	Get(ctx context.Context, id string) (*database.MemoryRecord, error)
	Create(ctx context.Context, record *database.MemoryRecord) (*database.MemoryRecord, error)
	Upsert(ctx context.Context, record *database.MemoryRecord) (*database.MemoryRecord, error)
	Update(ctx context.Context, id string, updates map[string]any) (*database.MemoryRecord, error)
	Delete(ctx context.Context, id string) error
	PolicySummary(ctx context.Context) (PolicySummary, error)
}

func (s *DBStore) PromptCandidates(ctx context.Context, filter PromptCandidateFilter) ([]database.MemoryRecord, error) {
	q := s.applyFilter(ctx, s.scoped(ctx), Filter{
		LoadPolicies: filter.LoadPolicies,
	})
	q = applyPromptScopeFilter(q, filter)
	q = applyPromptRelevanceFilter(q, filter)
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var records []database.MemoryRecord
	err := database.WithSQLiteBusyRetry(ctx, "memory.prompt_candidates", func() error {
		return q.Order("CASE load_policy WHEN 'core' THEN 0 WHEN 'pinned' THEN 1 WHEN 'auto' THEN 2 ELSE 9 END ASC").
			Order("importance DESC").
			Order("updated_at DESC").
			Limit(limit).
			Find(&records).Error
	})
	return records, err
}

func applyPromptScopeFilter(q *gorm.DB, filter PromptCandidateFilter) *gorm.DB {
	var clauses []string
	var args []any
	clauses = append(clauses, "scope IN ?")
	args = append(args, []string{database.MemoryScopeGlobal, database.MemoryScopeUser})
	if filter.ConversationID != "" {
		clauses = append(clauses, "(scope = ? AND scope_ref = ?)")
		args = append(args, database.MemoryScopeConversation, filter.ConversationID)
	}
	if filter.WorkspaceID != "" {
		clauses = append(clauses, "(scope = ? AND scope_ref = ?)")
		args = append(args, database.MemoryScopeWorkspace, filter.WorkspaceID)
	}
	if filter.ProjectID != "" {
		clauses = append(clauses, "(scope = ? AND scope_ref = ?)")
		args = append(args, database.MemoryScopeProject, filter.ProjectID)
	}
	return q.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

func applyPromptRelevanceFilter(q *gorm.DB, filter PromptCandidateFilter) *gorm.DB {
	tokens := relevanceTokens(filter.RelevanceText)
	if len(tokens) == 0 {
		return q.Where("load_policy <> ?", LoadPolicyAuto)
	}
	var clauses []string
	var args []any
	for _, token := range tokens {
		like := likeContains(token)
		clauses = append(clauses, "lower(content) LIKE ? ESCAPE '\\' OR lower(summary) LIKE ? ESCAPE '\\' OR lower(tags) LIKE ? ESCAPE '\\' OR lower(kind) LIKE ? ESCAPE '\\'")
		args = append(args, like, like, like, like)
	}
	return q.Where("(load_policy <> ? OR "+strings.Join(clauses, " OR ")+")", append([]any{LoadPolicyAuto}, args...)...)
}

type DBStore struct {
	db *gorm.DB
}

func NewDBStore(db *gorm.DB) *DBStore {
	return &DBStore{db: db}
}

func (s *DBStore) dbOrDefault() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return database.DB()
}

func (s *DBStore) scoped(ctx context.Context) *gorm.DB {
	return database.ScopeByUser(ctx, s.dbOrDefault().WithContext(ctx).Model(&database.MemoryRecord{}), "user_id")
}

func (s *DBStore) List(ctx context.Context, filter Filter) (ListResult, error) {
	q := s.applyFilter(ctx, s.scoped(ctx), filter)

	var total int64
	if err := database.WithSQLiteBusyRetry(ctx, "memory.list.count", func() error {
		return q.Count(&total).Error
	}); err != nil {
		return ListResult{}, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var records []database.MemoryRecord
	err := database.WithSQLiteBusyRetry(ctx, "memory.list.find", func() error {
		return q.Order("importance DESC").
			Order("updated_at DESC").
			Limit(limit).
			Offset(filter.Offset).
			Find(&records).Error
	})
	return ListResult{Records: records, Total: total}, err
}

func (s *DBStore) Get(ctx context.Context, id string) (*database.MemoryRecord, error) {
	var record database.MemoryRecord
	err := database.WithSQLiteBusyRetry(ctx, "memory.get", func() error {
		return s.scoped(ctx).First(&record, "id = ?", id).Error
	})
	return &record, err
}

func (s *DBStore) Create(ctx context.Context, record *database.MemoryRecord) (*database.MemoryRecord, error) {
	if userID, ok := database.UserIDFromContext(ctx); ok {
		record.UserID = userID
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	if err := database.WithSQLiteBusyRetry(ctx, "memory.create", func() error {
		return s.dbOrDefault().WithContext(ctx).Create(record).Error
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, record.ID)
}

func (s *DBStore) Upsert(ctx context.Context, record *database.MemoryRecord) (*database.MemoryRecord, error) {
	if userID, ok := database.UserIDFromContext(ctx); ok {
		record.UserID = userID
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var existing database.MemoryRecord
	err := database.WithSQLiteBusyRetry(ctx, "memory.upsert.lookup", func() error {
		return s.scoped(ctx).Where("id = ?", record.ID).First(&existing).Error
	})
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		if err := database.WithSQLiteBusyRetry(ctx, "memory.upsert.create", func() error {
			return s.dbOrDefault().WithContext(ctx).Create(record).Error
		}); err != nil {
			return nil, err
		}
		return s.Get(ctx, record.ID)
	}
	record.UserID = existing.UserID
	record.CreatedAt = existing.CreatedAt
	if err := database.WithSQLiteBusyRetry(ctx, "memory.upsert.save", func() error {
		return s.dbOrDefault().WithContext(ctx).Save(record).Error
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, record.ID)
}

func (s *DBStore) Update(ctx context.Context, id string, updates map[string]any) (*database.MemoryRecord, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	if err := database.WithSQLiteBusyRetry(ctx, "memory.update", func() error {
		return s.scoped(ctx).Where("id = ?", id).Updates(updates).Error
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *DBStore) Delete(ctx context.Context, id string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.WithSQLiteBusyRetry(ctx, "memory.delete", func() error {
		return s.scoped(ctx).Where("id = ?", id).Delete(&database.MemoryRecord{}).Error
	})
}

func (s *DBStore) PolicySummary(ctx context.Context) (PolicySummary, error) {
	var rows []struct {
		LoadPolicy string
		Count      int64
	}
	err := database.WithSQLiteBusyRetry(ctx, "memory.policy_summary", func() error {
		return s.scoped(ctx).Select("load_policy, count(*) as count").Group("load_policy").Scan(&rows).Error
	})
	if err != nil {
		return PolicySummary{}, err
	}
	var out PolicySummary
	for _, row := range rows {
		out.Total += row.Count
		switch row.LoadPolicy {
		case LoadPolicyCore:
			out.Core = row.Count
		case LoadPolicyPinned:
			out.Pinned = row.Count
		case LoadPolicyAuto:
			out.Auto = row.Count
		case LoadPolicyRetrievable:
			out.Retrievable = row.Count
		case LoadPolicyArchived:
			out.Archived = row.Count
		}
	}
	return out, nil
}

func (s *DBStore) applyFilter(ctx context.Context, q *gorm.DB, filter Filter) *gorm.DB {
	policies := cleanStrings(filter.LoadPolicies)
	if !filter.IncludeArchived && !containsString(policies, LoadPolicyArchived) {
		q = q.Where("load_policy <> ?", LoadPolicyArchived)
	}
	if len(policies) > 0 {
		q = q.Where("load_policy IN ?", policies)
	}
	if len(filter.Kinds) > 0 {
		q = q.Where("kind IN ?", cleanStrings(filter.Kinds))
	}
	if len(filter.Scopes) > 0 {
		q = q.Where("scope IN ?", cleanStrings(filter.Scopes))
	}
	for _, tag := range cleanStrings(filter.Tags) {
		needle, _ := json.Marshal(tag)
		q = q.Where("tags LIKE ? ESCAPE '\\'", likeContains(string(needle)))
	}
	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		like := likeContains(strings.ToLower(trimmed))
		q = q.Where("(lower(content) LIKE ? ESCAPE '\\' OR lower(summary) LIKE ? ESCAPE '\\' OR lower(tags) LIKE ? ESCAPE '\\')", like, like, like)
	}
	now := time.Now()
	q = q.Where("expires_at IS NULL OR expires_at > ?", now)
	return q
}

func likeContains(value string) string {
	return "%" + escapeLike(value) + "%"
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

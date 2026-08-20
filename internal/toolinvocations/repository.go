package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, inv *Invocation) error
	MarkRunning(ctx context.Context, id string, startedAt time.Time) error
	Complete(ctx context.Context, id string, inv *Invocation) error
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*Invocation, error)
	List(ctx context.Context, filter Filter) ([]Invocation, error)
	CleanOldDryRuns(ctx context.Context, maxAge time.Duration) (int, error)
	CleanOldChat(ctx context.Context, maxAge time.Duration) (int, error)
	CleanOrphanChat(ctx context.Context) (int, error)
	ResolveToolCatalogID(ctx context.Context, toolName string) (string, error)
	IsToolCatalogIDVisible(ctx context.Context, toolCatalogID string) (bool, error)
}

type DBRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{db: db, now: time.Now}
}

func (r *DBRepository) retry(ctx context.Context, operation string, fn func() error) error {
	return database.WithSQLiteBusyRetry(ctx, "toolinvocations."+operation, fn)
}

func (r *DBRepository) Create(ctx context.Context, inv *Invocation) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if inv == nil {
		return fmt.Errorf("tool invocation nil")
	}
	inv.UserID = userID
	if inv.QueuedAt.IsZero() {
		inv.QueuedAt = r.now()
	}
	if inv.Status == "" {
		inv.Status = StatusQueued
	}
	row := invocationDomainToModel(*inv)
	if err := r.retry(ctx, "create", func() error {
		return r.db.WithContext(ctx).Create(&row).Error
	}); err != nil {
		return err
	}
	*inv = invocationModelToDomain(row)
	return nil
}

func (r *DBRepository) MarkRunning(ctx context.Context, id string, startedAt time.Time) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	if startedAt.IsZero() {
		startedAt = r.now()
	}
	var tx *gorm.DB
	err := r.retry(ctx, "mark_running", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
			Where("id = ?", strings.TrimSpace(id)).
			Updates(map[string]any{
				"status":     StatusRunning,
				"started_at": startedAt,
			})
		return tx.Error
	})
	if err != nil {
		return err
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *DBRepository) Complete(ctx context.Context, id string, inv *Invocation) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	if inv == nil {
		return fmt.Errorf("tool invocation nil")
	}
	completedAt := inv.CompletedAt
	if completedAt == nil || completedAt.IsZero() {
		now := r.now()
		completedAt = &now
	}
	status := inv.Status
	if status == "" {
		status = StatusSucceeded
	}
	var tx *gorm.DB
	err := r.retry(ctx, "complete", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
			Where("id = ?", strings.TrimSpace(id)).
			Updates(map[string]any{
				"status":        status,
				"output":        string(inv.Output),
				"metadata":      string(inv.Metadata),
				"error_kind":    inv.ErrorKind,
				"error_message": inv.ErrorMessage,
				"retryable":     inv.Retryable,
				"completed_at":  completedAt,
				"duration_ms":   inv.DurationMs,
			})
		return tx.Error
	})
	if err != nil {
		return err
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *DBRepository) Delete(ctx context.Context, id string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	var tx *gorm.DB
	err := r.retry(ctx, "delete", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
			Where("id = ?", strings.TrimSpace(id)).
			Delete(&database.ToolInvocation{})
		return tx.Error
	})
	if err != nil {
		return err
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *DBRepository) Get(ctx context.Context, id string) (*Invocation, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.ToolInvocation
	if err := r.retry(ctx, "get", func() error {
		return database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			First(&row, "id = ?", strings.TrimSpace(id)).Error
	}); err != nil {
		return nil, err
	}
	inv := invocationModelToDomain(row)
	return &inv, nil
}

func (r *DBRepository) List(ctx context.Context, filter Filter) ([]Invocation, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id")
	if filter.OriginType != "" {
		q = q.Where("origin_type = ?", filter.OriginType)
	}
	if filter.OriginID != "" {
		q = q.Where("origin_id = ?", filter.OriginID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.DryRun != nil {
		q = q.Where("dry_run = ?", *filter.DryRun)
	}
	var rows []database.ToolInvocation
	if err := r.retry(ctx, "list", func() error {
		return q.Order("queued_at DESC, created_at DESC").Limit(limit).Find(&rows).Error
	}); err != nil {
		return nil, err
	}
	out := make([]Invocation, 0, len(rows))
	for _, row := range rows {
		out = append(out, invocationModelToDomain(row))
	}
	return out, nil
}

// CleanOldDryRuns remove invocações dry-run de origens operacionais (job_run /
// tool_catalog) mais antigas que maxAge. São dados efêmeros, sem valor histórico
// (AEP-0074). NÃO toca em invocações de chat nem em execuções reais de job
// (estas saem em cascata quando o run/conversa é removido). maxAge <= 0 é no-op.
func (r *DBRepository) CleanOldDryRuns(ctx context.Context, maxAge time.Duration) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	if maxAge <= 0 {
		return 0, nil
	}
	cutoff := r.now().Add(-maxAge)
	var tx *gorm.DB
	err := r.retry(ctx, "clean_old_dry_runs", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where(
				"queued_at < ? AND dry_run = ? AND origin_type IN (?, ?)",
				cutoff,
				true,
				OriginToolCatalog,
				OriginJobRun,
			).
			Delete(&database.ToolInvocation{})
		return tx.Error
	})
	if tx == nil {
		return 0, err
	}
	return int(tx.RowsAffected), err
}

// CleanOldChat remove invocações de CHAT mais antigas que maxAge. É um cap de
// idade OPCIONAL: por padrão a retenção de chat é o ciclo de vida da conversa
// (AEP-0074), então o chamador só deve invocar quando o usuário configurar um
// limite explícito. maxAge <= 0 é no-op.
func (r *DBRepository) CleanOldChat(ctx context.Context, maxAge time.Duration) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	if maxAge <= 0 {
		return 0, nil
	}
	cutoff := r.now().Add(-maxAge)
	var tx *gorm.DB
	err := r.retry(ctx, "clean_old_chat", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("queued_at < ? AND origin_type = ?", cutoff, OriginChat).
			Delete(&database.ToolInvocation{})
		return tx.Error
	})
	if tx == nil {
		return 0, err
	}
	return int(tx.RowsAffected), err
}

// CleanOrphanChat remove invocações de chat cujo turno/mensagem de origem não
// existe mais em chat_messages — uma rede de segurança para o ciclo de vida
// (deleções de conversa já removem em cascata, mas falhas podem deixar órfãos).
// Se a tabela de chat_messages não existir (migrações parciais em teste), é no-op.
func (r *DBRepository) CleanOrphanChat(ctx context.Context) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	if !r.db.Migrator().HasTable(&database.ChatMessage{}) {
		return 0, nil
	}
	// NOT EXISTS faz lookup por chave primária por invocação, evitando
	// materializar/varrer todos os ids de chat_messages (caro conforme o
	// histórico cresce). Mesma semântica: remove apenas quando não há mensagem.
	var tx *gorm.DB
	err := r.retry(ctx, "clean_orphan_chat", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("origin_type = ?", OriginChat).
			Where("NOT EXISTS (SELECT 1 FROM chat_messages WHERE chat_messages.id = tool_invocations.origin_id)").
			Delete(&database.ToolInvocation{})
		return tx.Error
	})
	if tx == nil {
		return 0, err
	}
	return int(tx.RowsAffected), err
}

func (r *DBRepository) ResolveToolCatalogID(ctx context.Context, toolName string) (string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "", fmt.Errorf("tool name is required")
	}
	var row database.ToolCatalog
	q := r.db.WithContext(ctx)
	// Alguns testes/migrações parciais não criam mcp_servers; não pode falhar por isso.
	if q.Migrator().HasTable("mcp_servers") {
		q = q.Joins("LEFT JOIN mcp_servers ON mcp_servers.id = tool_catalog.mcp_server_id").
			Where(
				"tool_catalog.name = ? AND (tool_catalog.user_id = ? OR (tool_catalog.origin = ? AND (tool_catalog.user_id IS NULL OR tool_catalog.user_id = '') AND tool_catalog.mcp_server_id IS NULL) OR mcp_servers.user_id = ?)",
				name, userID, tools.ToolOriginBuiltin, userID,
			)
	} else {
		q = q.Where(
			"tool_catalog.name = ? AND (tool_catalog.user_id = ? OR (tool_catalog.origin = ? AND (tool_catalog.user_id IS NULL OR tool_catalog.user_id = '') AND tool_catalog.mcp_server_id IS NULL))",
			name, userID, tools.ToolOriginBuiltin,
		)
	}
	err = r.retry(ctx, "resolve_tool_catalog_id", func() error {
		return q.
			Order("tool_catalog.mcp_server_id IS NULL ASC").
			Order("tool_catalog.user_id IS NULL ASC").
			First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("tool catalog entry not found: %s", name)
	}
	if err != nil {
		return "", err
	}
	return row.ID, nil
}

func (r *DBRepository) IsToolCatalogIDVisible(ctx context.Context, toolCatalogID string) (bool, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(toolCatalogID)
	if id == "" {
		return false, fmt.Errorf("tool catalog id is required")
	}

	var row database.ToolCatalog
	q := r.db.WithContext(ctx)
	if q.Migrator().HasTable("mcp_servers") {
		q = q.Joins("LEFT JOIN mcp_servers ON mcp_servers.id = tool_catalog.mcp_server_id").
			Where(
				"tool_catalog.id = ? AND (tool_catalog.user_id = ? OR (tool_catalog.origin = ? AND (tool_catalog.user_id IS NULL OR tool_catalog.user_id = '') AND tool_catalog.mcp_server_id IS NULL) OR mcp_servers.user_id = ?)",
				id,
				userID,
				tools.ToolOriginBuiltin,
				userID,
			)
	} else {
		q = q.Where(
			"tool_catalog.id = ? AND (tool_catalog.user_id = ? OR (tool_catalog.origin = ? AND (tool_catalog.user_id IS NULL OR tool_catalog.user_id = '') AND tool_catalog.mcp_server_id IS NULL))",
			id,
			userID,
			tools.ToolOriginBuiltin,
		)
	}
	err = r.retry(ctx, "is_tool_catalog_id_visible", func() error {
		return q.First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func invocationDomainToModel(inv Invocation) database.ToolInvocation {
	var parentID *string
	if strings.TrimSpace(inv.ParentInvocationID) != "" {
		parent := strings.TrimSpace(inv.ParentInvocationID)
		parentID = &parent
	}
	return database.ToolInvocation{
		UUIDModel:          database.UUIDModel{ID: strings.TrimSpace(inv.ID), CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt},
		UserID:             strings.TrimSpace(inv.UserID),
		ToolCatalogID:      strings.TrimSpace(inv.ToolCatalogID),
		OriginType:         strings.TrimSpace(inv.OriginType),
		OriginID:           strings.TrimSpace(inv.OriginID),
		ParentInvocationID: parentID,
		ToolCallID:         strings.TrimSpace(inv.ToolCallID),
		Status:             strings.TrimSpace(inv.Status),
		DryRun:             inv.DryRun,
		Input:              string(inv.Input),
		Output:             string(inv.Output),
		Metadata:           string(inv.Metadata),
		ErrorKind:          strings.TrimSpace(inv.ErrorKind),
		ErrorMessage:       strings.TrimSpace(inv.ErrorMessage),
		Retryable:          inv.Retryable,
		QueuedAt:           inv.QueuedAt,
		StartedAt:          inv.StartedAt,
		CompletedAt:        inv.CompletedAt,
		DurationMs:         inv.DurationMs,
	}
}

func invocationModelToDomain(row database.ToolInvocation) Invocation {
	parentID := ""
	if row.ParentInvocationID != nil {
		parentID = *row.ParentInvocationID
	}
	return Invocation{
		ID:                 row.ID,
		UserID:             row.UserID,
		ToolCatalogID:      row.ToolCatalogID,
		OriginType:         row.OriginType,
		OriginID:           row.OriginID,
		ParentInvocationID: parentID,
		ToolCallID:         row.ToolCallID,
		Status:             row.Status,
		DryRun:             row.DryRun,
		Input:              json.RawMessage(row.Input),
		Output:             json.RawMessage(row.Output),
		Metadata:           json.RawMessage(row.Metadata),
		ErrorKind:          row.ErrorKind,
		ErrorMessage:       row.ErrorMessage,
		Retryable:          row.Retryable,
		QueuedAt:           row.QueuedAt,
		StartedAt:          row.StartedAt,
		CompletedAt:        row.CompletedAt,
		DurationMs:         row.DurationMs,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func resultOutput(result tools.ToolResult) json.RawMessage {
	payload := map[string]any{
		"content":  result.Content,
		"is_error": result.IsError,
	}
	if len(result.Metadata) > 0 {
		payload["metadata"] = result.Metadata
	}
	if result.Annotations != nil {
		payload["annotations"] = result.Annotations
	}
	data, err := json.Marshal(payload)
	if err == nil {
		return data
	}

	// Fallback: persiste pelo menos content/is_error e anotações mesmo se
	// metadata tiver valores não serializáveis.
	delete(payload, "metadata")
	data, err2 := json.Marshal(payload)
	if err2 == nil {
		return data
	}

	// Último fallback: JSON mínimo válido.
	msg := fmt.Sprintf("[toolinvocations] erro ao serializar resultado: %v", err)
	minimal, _ := json.Marshal(map[string]any{"content": msg, "is_error": true})
	return minimal
}

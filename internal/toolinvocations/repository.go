package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrChatOriginIDRequired           = errors.New("chat origin ID required")
	ErrToolCatalogNotFound            = errors.New("tool catalog entry not found")
	ErrCanonicalToolCatalogIDRequired = errors.New("canonical tool catalog ID required")
	ErrCanonicalToolCatalogMismatch   = errors.New("canonical tool catalog ID does not match tool name")
)

// toolCatalogResolveCacheTTL limita a validade de cada mapeamento
// (user_id, nome) -> tool_catalog_id em memória. A resolução é feita a CADA
// invocação de tool (chat e jobs); sob carga de jobs, esse SELECT vira o maior
// ofensor de contenção do writer SQLite (observado até 80s e "context deadline
// exceeded" no assistente.log). O mapeamento é estável (upsert reusa o mesmo ID
// e o detach preserva a linha), então um TTL curto elimina ~99% das leituras sem
// risco relevante de staleness. Ver AEP-0104.
const toolCatalogResolveCacheTTL = 60 * time.Second

type toolCatalogCacheEntry struct {
	id        string
	expiresAt time.Time
}

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
	ValidateChatOrigin(ctx context.Context, originID string) error
	ResolveToolCatalogID(ctx context.Context, toolName string) (string, error)
	IsToolCatalogIDVisible(ctx context.Context, toolCatalogID string) (bool, error)
}

type DBRepository struct {
	db  *gorm.DB
	now func() time.Time

	// catalogCache guarda mapeamentos (user_id, nome) -> tool_catalog_id com TTL.
	// Chaveado por usuário para preservar isolamento (AEP-0104): nenhuma entrada
	// de um usuário é servida a outro. Só resoluções não-archival são cacheadas.
	catalogCacheMu sync.RWMutex
	catalogCache   map[string]toolCatalogCacheEntry
}

func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{
		db:           db,
		now:          time.Now,
		catalogCache: make(map[string]toolCatalogCacheEntry),
	}
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
	inv.OriginType = strings.TrimSpace(inv.OriginType)
	if inv.OriginType == "" {
		inv.OriginType = OriginChat
	}
	inv.OriginID = strings.TrimSpace(inv.OriginID)
	if inv.OriginType == OriginChat && inv.OriginID == "" {
		return ErrChatOriginIDRequired
	}
	row := invocationDomainToModel(*inv)
	create := func(exec *gorm.DB) error {
		var lastAttempt int
		if err := exec.WithContext(ctx).Model(&database.ToolInvocation{}).
			Where("user_id = ? AND origin_type = ? AND origin_id = ? AND tool_call_id = ?", userID, row.OriginType, row.OriginID, row.ToolCallID).
			Select("COALESCE(MAX(attempt), 0)").
			Scan(&lastAttempt).Error; err != nil {
			return err
		}
		row.Attempt = lastAttempt + 1
		return withoutInvocationPayloadLogging(exec).WithContext(ctx).Create(&row).Error
	}
	createErr := database.WithSQLiteImmediateTransaction(ctx, r.db, "toolinvocations.create", func(tx *gorm.DB) error {
		if row.OriginType == OriginChat {
			link, err := resolveChatOriginTx(ctx, tx, userID, row.OriginID)
			if err != nil {
				return err
			}
			row.ConversationID = &link.ConversationID
			row.TurnID = &link.TurnID
		} else {
			row.ConversationID = nil
			row.TurnID = nil
		}
		return create(tx)
	})
	if createErr != nil {
		return createErr
	}
	*inv = invocationModelToDomain(row)
	return nil
}

func (r *DBRepository) ValidateChatOrigin(ctx context.Context, originID string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	originID = strings.TrimSpace(originID)
	if originID == "" {
		return ErrChatOriginIDRequired
	}
	return r.retry(ctx, "validate_chat_origin", func() error {
		_, err := resolveChatOriginTx(ctx, r.db, userID, originID)
		return err
	})
}

type chatOriginLink struct {
	ConversationID string
	TurnID         string
}

func resolveChatOriginTx(ctx context.Context, tx *gorm.DB, userID, originID string) (chatOriginLink, error) {
	if tx == nil ||
		!tx.Migrator().HasTable(&database.ChatMessage{}) ||
		!tx.Migrator().HasTable(&database.Conversation{}) {
		return chatOriginLink{}, fmt.Errorf("não é possível validar origem chat sem tabelas de conversa e mensagens")
	}
	var link chatOriginLink
	if err := tx.WithContext(ctx).Model(&database.ChatMessage{}).
		Select("chat_messages.conversation_id, COALESCE(NULLIF(chat_messages.turn_id, ''), chat_messages.id) AS turn_id").
		Joins("JOIN conversations ON conversations.id = chat_messages.conversation_id").
		Where("conversations.user_id = ? AND (chat_messages.id = ? OR chat_messages.turn_id = ?)", userID, originID, originID).
		Order("chat_messages.created_at, chat_messages.id").
		Limit(1).
		Scan(&link).Error; err != nil {
		return chatOriginLink{}, err
	}
	if link.ConversationID == "" || link.TurnID == "" {
		return chatOriginLink{}, gorm.ErrRecordNotFound
	}
	return link, nil
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
		tx = database.ScopeByUser(ctx, withoutInvocationPayloadLogging(r.db).WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
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

func withoutInvocationPayloadLogging(database *gorm.DB) *gorm.DB {
	return database.Session(&gorm.Session{Logger: logger.Discard})
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
		tx = database.ScopeByUser(ctx, withoutInvocationPayloadLogging(r.db).WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
			Where("id = ?", strings.TrimSpace(id)).
			Updates(map[string]any{
				"status":              status,
				"output":              string(inv.Output),
				"metadata":            string(inv.Metadata),
				"model_iteration":     inv.ModelIteration,
				"external":            inv.External,
				"output_preview":      inv.OutputPreview,
				"output_bytes":        inv.OutputBytes,
				"output_hash":         inv.OutputHash,
				"result_availability": inv.ResultAvailability,
				"error_kind":          inv.ErrorKind,
				"error_code":          inv.ErrorCode,
				"error_message":       inv.ErrorMessage,
				"retryable":           inv.Retryable,
				"retryability_known":  inv.RetryabilityKnown,
				"completed_at":        completedAt,
				"duration_ms":         inv.DurationMs,
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
	// Vínculos canônicos usam conversation_id/turn_id. origin_id só permanece
	// para linhas pré-v18 que ainda não receberam o backfill aditivo.
	var tx *gorm.DB
	err := r.retry(ctx, "clean_orphan_chat", func() error {
		tx = database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("origin_type = ?", OriginChat).
			Where(`(
				tool_invocations.conversation_id IS NOT NULL
				AND (
					NOT EXISTS (
						SELECT 1 FROM conversations
						WHERE conversations.id = tool_invocations.conversation_id
							AND conversations.user_id = tool_invocations.user_id
					)
					OR (
						tool_invocations.turn_id IS NOT NULL
						AND NOT EXISTS (
							SELECT 1 FROM chat_messages
							WHERE chat_messages.id = tool_invocations.turn_id
								AND chat_messages.conversation_id = tool_invocations.conversation_id
						)
					)
				)
			) OR (
				tool_invocations.conversation_id IS NULL
				AND NOT EXISTS (
					SELECT 1 FROM chat_messages
					WHERE chat_messages.id = tool_invocations.origin_id
				)
			)`).
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
	if id, ok := r.lookupCatalogCache(userID, name); ok {
		return id, nil
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
			Order("CASE WHEN tool_catalog.origin = 'archival' THEN 1 ELSE 0 END ASC").
			Order("tool_catalog.mcp_server_id IS NULL ASC").
			Order("tool_catalog.user_id IS NULL ASC").
			First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Não cacheia "não encontrado": uma tool nova pode ser inserida no catálogo
		// a qualquer momento e precisa resolver imediatamente na próxima chamada.
		return "", fmt.Errorf("%w: %s", ErrToolCatalogNotFound, name)
	}
	if err != nil {
		return "", err
	}
	// Só cacheia resoluções estáveis (não-archival). Uma entrada archival é um
	// placeholder que a ordenação suplanta assim que a tool real aparece no
	// catálogo; fixá-la retornaria o ID errado até o TTL expirar.
	if !strings.EqualFold(strings.TrimSpace(row.Origin), ToolOriginArchival) {
		r.storeCatalogCache(userID, name, row.ID)
	}
	return row.ID, nil
}

// catalogCacheKey compõe a chave (user_id, nome) com separador que não ocorre em
// UUIDs nem em nomes de tool, evitando colisão entre pares distintos.
func (r *DBRepository) catalogCacheKey(userID, name string) string {
	return userID + "\x00" + name
}

// lookupCatalogCache devolve o tool_catalog_id cacheado para (userID, nome) se a
// entrada existir e ainda estiver dentro do TTL.
func (r *DBRepository) lookupCatalogCache(userID, name string) (string, bool) {
	r.catalogCacheMu.RLock()
	entry, ok := r.catalogCache[r.catalogCacheKey(userID, name)]
	r.catalogCacheMu.RUnlock()
	if !ok || !r.now().Before(entry.expiresAt) {
		return "", false
	}
	return entry.id, true
}

// storeCatalogCache registra (userID, nome) -> id com validade limitada pelo TTL.
func (r *DBRepository) storeCatalogCache(userID, name, id string) {
	r.catalogCacheMu.Lock()
	if r.catalogCache == nil {
		r.catalogCache = make(map[string]toolCatalogCacheEntry)
	}
	r.catalogCache[r.catalogCacheKey(userID, name)] = toolCatalogCacheEntry{
		id:        id,
		expiresAt: r.now().Add(toolCatalogResolveCacheTTL),
	}
	r.catalogCacheMu.Unlock()
}

// ResolveOrCreateArchivalToolCatalogID preserva identidade de uma tool
// executável no runtime que ainda não consta do catálogo persistido. A entrada
// é user-scoped e explicitamente indisponível para execução pela UI.
func (r *DBRepository) ResolveOrCreateArchivalToolCatalogID(ctx context.Context, toolName string) (string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "", fmt.Errorf("tool name is required")
	}
	var catalog database.ToolCatalog
	err = database.WithSQLiteImmediateTransaction(ctx, r.db, "toolinvocations.resolve_archival_catalog", func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).
			Where("user_id = ? AND name = ? AND origin = ?", userID, name, ToolOriginArchival).
			Limit(1).
			Find(&catalog)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return nil
		}
		owner := userID
		catalog = database.ToolCatalog{
			UserID:             &owner,
			Name:               name,
			DisplayName:        name,
			Origin:             ToolOriginArchival,
			AvailabilityStatus: tools.ToolAvailabilityUnavailable,
			AvailabilityReason: "runtime_catalog_missing",
		}
		return tx.WithContext(ctx).Create(&catalog).Error
	})
	if err != nil {
		return "", err
	}
	return catalog.ID, nil
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
	var conversationID *string
	if strings.TrimSpace(inv.ConversationID) != "" {
		value := strings.TrimSpace(inv.ConversationID)
		conversationID = &value
	}
	var turnID *string
	if strings.TrimSpace(inv.TurnID) != "" {
		value := strings.TrimSpace(inv.TurnID)
		turnID = &value
	}
	return database.ToolInvocation{
		UUIDModel:          database.UUIDModel{ID: strings.TrimSpace(inv.ID), CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt},
		UserID:             strings.TrimSpace(inv.UserID),
		ToolCatalogID:      strings.TrimSpace(inv.ToolCatalogID),
		OriginType:         strings.TrimSpace(inv.OriginType),
		OriginID:           strings.TrimSpace(inv.OriginID),
		ConversationID:     conversationID,
		TurnID:             turnID,
		ParentInvocationID: parentID,
		ToolCallID:         strings.TrimSpace(inv.ToolCallID),
		Attempt:            inv.Attempt,
		Status:             strings.TrimSpace(inv.Status),
		DryRun:             inv.DryRun,
		Input:              string(inv.Input),
		Output:             string(inv.Output),
		Metadata:           string(inv.Metadata),
		ModelIteration:     inv.ModelIteration,
		External:           inv.External,
		DisplayName:        strings.TrimSpace(inv.DisplayName),
		InputPreview:       inv.InputPreview,
		OutputPreview:      inv.OutputPreview,
		InputBytes:         inv.InputBytes,
		OutputBytes:        inv.OutputBytes,
		InputHash:          inv.InputHash,
		OutputHash:         inv.OutputHash,
		ResultAvailability: inv.ResultAvailability,
		ErrorKind:          strings.TrimSpace(inv.ErrorKind),
		ErrorCode:          strings.TrimSpace(inv.ErrorCode),
		ErrorMessage:       strings.TrimSpace(inv.ErrorMessage),
		Retryable:          inv.Retryable,
		RetryabilityKnown:  inv.RetryabilityKnown,
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
	conversationID := ""
	if row.ConversationID != nil {
		conversationID = *row.ConversationID
	}
	turnID := ""
	if row.TurnID != nil {
		turnID = *row.TurnID
	}
	return Invocation{
		ID:                 row.ID,
		UserID:             row.UserID,
		ToolCatalogID:      row.ToolCatalogID,
		OriginType:         row.OriginType,
		OriginID:           row.OriginID,
		ConversationID:     conversationID,
		TurnID:             turnID,
		ParentInvocationID: parentID,
		ToolCallID:         row.ToolCallID,
		Attempt:            row.Attempt,
		Status:             row.Status,
		DryRun:             row.DryRun,
		Input:              json.RawMessage(row.Input),
		Output:             json.RawMessage(row.Output),
		Metadata:           json.RawMessage(row.Metadata),
		ModelIteration:     row.ModelIteration,
		External:           row.External,
		DisplayName:        row.DisplayName,
		InputPreview:       row.InputPreview,
		OutputPreview:      row.OutputPreview,
		InputBytes:         row.InputBytes,
		OutputBytes:        row.OutputBytes,
		InputHash:          row.InputHash,
		OutputHash:         row.OutputHash,
		ResultAvailability: row.ResultAvailability,
		ErrorKind:          row.ErrorKind,
		ErrorCode:          row.ErrorCode,
		ErrorMessage:       row.ErrorMessage,
		Retryable:          row.Retryable,
		RetryabilityKnown:  row.RetryabilityKnown,
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
	if result.Structured {
		payload["structured"] = true
	}
	if result.RawExact {
		payload["raw_exact"] = true
	}
	if result.Failure != nil {
		payload["failure"] = result.Failure
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

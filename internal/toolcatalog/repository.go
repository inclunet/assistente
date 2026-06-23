// Package toolcatalog é o dono dedicado da persistência e sincronização do
// catálogo de tools (tabela tool_catalog). O catálogo cobre builtins, MCP
// bridge e MCP native; por isso não pertence ao pacote MCP (AEP-0077, Fase 2 /
// #120). O MCP e demais consumidores passam a CONSUMIR este pacote.
//
// O registry runtime continua sendo a fonte executável; o catálogo permanece
// índice/metadata. Mesma tabela e migrações de antes — apenas a propriedade do
// código mudou de pacote.
package toolcatalog

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

// Repository persiste e consulta o catálogo de tools (tabela tool_catalog).
type Repository interface {
	UpsertTool(ctx context.Context, entry *tools.ToolCatalogEntry) error
	ListTools(ctx context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error)
	MarkServerToolsUnavailable(ctx context.Context, serverID string, seenNames []string, reason string) (int, error)
	RecordToolTest(ctx context.Context, toolName, status, errorMessage string) error
	RecordToolTestByID(ctx context.Context, toolCatalogID, status, errorMessage string) error
}

// DBRepository implementa Repository usando GORM.
type DBRepository struct {
	db  *gorm.DB
	now func() time.Time
}

// NewDBRepository cria um DBRepository sobre a instância GORM informada.
func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{
		db:  db,
		now: time.Now,
	}
}

func (r *DBRepository) UpsertTool(ctx context.Context, entry *tools.ToolCatalogEntry) error {
	if entry == nil {
		return fmt.Errorf("tool catalog entry nil")
	}
	normalized, err := r.normalizeToolEntry(ctx, *entry)
	if err != nil {
		return err
	}
	row, err := toolEntryToModel(normalized)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.ToolCatalog
		query := tx
		if normalized.Origin == tools.ToolOriginBuiltin {
			query = query.Where("origin = ? AND name = ? AND mcp_server_id IS NULL", normalized.Origin, normalized.Name)
		} else {
			query = query.Where("user_id = ? AND mcp_server_id = ? AND name = ?", normalized.UserID, normalized.MCPServerID, normalized.Name)
		}
		err := query.First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if normalized.Origin != tools.ToolOriginBuiltin {
				var detached database.ToolCatalog
				reattachErr := tx.
					Where("user_id = ? AND origin = ? AND name = ? AND mcp_server_id IS NULL", normalized.UserID, normalized.Origin, normalized.Name).
					Order("updated_at DESC, id DESC").
					First(&detached).Error
				switch {
				case reattachErr == nil:
					row.ID = detached.ID
					row.CreatedAt = detached.CreatedAt
					if err := tx.Model(&detached).Select("*").Omit("id", "created_at").Updates(&row).Error; err != nil {
						return err
					}
					entry.ID = detached.ID
					return nil
				case !errors.Is(reattachErr, gorm.ErrRecordNotFound):
					return reattachErr
				}
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			entry.ID = row.ID
			return nil
		case err != nil:
			return err
		default:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if err := tx.Model(&existing).Select("*").Omit("id", "created_at").Updates(&row).Error; err != nil {
				return err
			}
			entry.ID = existing.ID
			return nil
		}
	})
}

func (r *DBRepository) ListTools(ctx context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx).
		Where("((origin = ? AND (user_id IS NULL OR user_id = '')) OR user_id = ?)", tools.ToolOriginBuiltin, userID)
	if filter.Origin != "" {
		query = query.Where("origin = ?", filter.Origin)
	}
	if filter.MCPServerID != "" {
		query = query.Where("mcp_server_id = ?", filter.MCPServerID)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Class != "" {
		query = query.Where("class = ?", filter.Class)
	}
	if filter.Package != "" {
		query = query.Where("package = ?", filter.Package)
	}
	if filter.Risk != "" {
		query = query.Where("risk = ?", filter.Risk)
	}
	if filter.AvailabilityStatus != "" {
		query = query.Where("availability_status = ?", filter.AvailabilityStatus)
	} else if !filter.IncludeUnavailable {
		query = query.Where("availability_status = ?", tools.ToolAvailabilityAvailable)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	var rows []database.ToolCatalog
	if err := query.Order("origin ASC, name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]tools.ToolCatalogEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := toolModelToEntry(row)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, nil
}

func (r *DBRepository) MarkServerToolsUnavailable(ctx context.Context, serverID string, seenNames []string, reason string) (int, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return 0, err
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return 0, fmt.Errorf("serverID é obrigatório")
	}
	if err := r.requireOwnedServer(ctx, serverID); err != nil {
		return 0, err
	}
	now := r.now()
	base := r.db.WithContext(ctx).
		Model(&database.ToolCatalog{}).
		Where("user_id = ? AND mcp_server_id = ?", userID, serverID)
	updates := map[string]any{
		"availability_status": tools.ToolAvailabilityUnavailable,
		"availability_reason": reason,
		"last_unavailable_at": &now,
	}
	if len(seenNames) == 0 {
		tx := base.Updates(updates)
		return int(tx.RowsAffected), tx.Error
	}

	var rows []database.ToolCatalog
	if err := r.db.WithContext(ctx).
		Select("name").
		Where("user_id = ? AND mcp_server_id = ?", userID, serverID).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	seen := make(map[string]struct{}, len(seenNames))
	for _, name := range seenNames {
		seen[name] = struct{}{}
	}
	unseen := make([]string, 0)
	for _, row := range rows {
		if _, ok := seen[row.Name]; !ok {
			unseen = append(unseen, row.Name)
		}
	}
	affected := int64(0)
	for start := 0; start < len(unseen); start += 500 {
		end := start + 500
		if end > len(unseen) {
			end = len(unseen)
		}
		tx := r.db.WithContext(ctx).
			Model(&database.ToolCatalog{}).
			Where("user_id = ? AND mcp_server_id = ?", userID, serverID).
			Where("name IN ?", unseen[start:end]).
			Updates(updates)
		if tx.Error != nil {
			return int(affected), tx.Error
		}
		affected += tx.RowsAffected
	}
	return int(affected), nil
}

func (r *DBRepository) RecordToolTest(ctx context.Context, toolName, status, errorMessage string) error {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return fmt.Errorf("toolName é obrigatório")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = tools.ToolTestStatusOK
	}
	now := r.now()
	query := r.db.WithContext(ctx).Model(&database.ToolCatalog{}).Where("name = ?", toolName)
	if isMCPNamespacedName(toolName) {
		userID, err := database.RequireUserID(ctx)
		if err != nil {
			return err
		}
		query = query.Where("user_id = ?", userID)
	} else {
		query = query.Where("(user_id IS NULL OR user_id = '')")
	}
	tx := query.Updates(map[string]any{
		"last_tested_at":   &now,
		"last_test_status": status,
		"last_test_error":  errorMessage,
	})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *DBRepository) RecordToolTestByID(ctx context.Context, toolCatalogID, status, errorMessage string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	toolCatalogID = strings.TrimSpace(toolCatalogID)
	if toolCatalogID == "" {
		return fmt.Errorf("toolCatalogID é obrigatório")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = tools.ToolTestStatusOK
	}
	now := r.now()
	tx := r.db.WithContext(ctx).Model(&database.ToolCatalog{}).
		Where("id = ? AND (user_id = ? OR user_id IS NULL OR user_id = '')", toolCatalogID, userID).
		Updates(map[string]any{
			"last_tested_at":   &now,
			"last_test_status": status,
			"last_test_error":  errorMessage,
		})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *DBRepository) normalizeToolEntry(ctx context.Context, entry tools.ToolCatalogEntry) (tools.ToolCatalogEntry, error) {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Origin = strings.TrimSpace(entry.Origin)
	if entry.Name == "" {
		return entry, fmt.Errorf("nome da tool é obrigatório")
	}
	if entry.DisplayName == "" {
		entry.DisplayName = entry.Name
	}
	if entry.AvailabilityStatus == "" {
		entry.AvailabilityStatus = tools.ToolAvailabilityAvailable
	}
	now := r.now()
	if entry.LastSeenAt == nil {
		entry.LastSeenAt = &now
	}
	if entry.AvailabilityStatus == tools.ToolAvailabilityAvailable && entry.LastAvailableAt == nil {
		entry.LastAvailableAt = &now
	}
	if entry.AvailabilityStatus == tools.ToolAvailabilityUnavailable && entry.LastUnavailableAt == nil {
		entry.LastUnavailableAt = &now
	}
	switch entry.Origin {
	case tools.ToolOriginBuiltin:
		entry.UserID = ""
		entry.MCPServerID = ""
	case tools.ToolOriginMCPBridge, tools.ToolOriginMCPNative:
		userID, err := database.RequireUserID(ctx)
		if err != nil {
			return entry, err
		}
		if strings.TrimSpace(entry.MCPServerID) == "" {
			return entry, fmt.Errorf("mcp_server_id é obrigatório para tool MCP")
		}
		if err := r.requireOwnedServer(ctx, entry.MCPServerID); err != nil {
			return entry, err
		}
		entry.UserID = userID
	default:
		return entry, fmt.Errorf("origem de tool inválida: %s", entry.Origin)
	}
	if entry.SchemaBytes == 0 && len(entry.Schema) > 0 {
		entry.SchemaBytes = len(entry.Schema)
	}
	return entry, nil
}

// requireOwnedServer valida que o servidor MCP existe e pertence ao usuário do
// contexto. Lê diretamente a model database.MCPServer (cuja propriedade segue
// no pacote MCP), evitando acoplamento de import com internal/mcp.
func (r *DBRepository) requireOwnedServer(ctx context.Context, serverID string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	var row database.MCPServer
	return database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("id = ?", strings.TrimSpace(serverID)).
		First(&row).Error
}

// DetachServerTools desanexa as tools de um servidor MCP removido, mantendo as
// linhas no catálogo (para preservar referências como jobs por tool_catalog_id)
// mas marcando-as como indisponíveis. Recebe a *gorm.DB da transação do caller
// (tipicamente a remoção do servidor no pacote MCP) para manter atomicidade.
//
// Mantém ownership explícito ao desanexar (user_id preservado), evitando rows
// "unowned" (user_id NULL) que poderiam vazar entre usuários.
func DetachServerTools(tx *gorm.DB, serverID, ownerUserID, reason string, now time.Time) error {
	return tx.Model(&database.ToolCatalog{}).
		Where("mcp_server_id = ?", serverID).
		Updates(map[string]any{
			"user_id":             ownerUserID,
			"mcp_server_id":       nil,
			"availability_status": tools.ToolAvailabilityUnavailable,
			"availability_reason": reason,
			"last_unavailable_at": now,
		}).Error
}

func toolEntryToModel(entry tools.ToolCatalogEntry) (database.ToolCatalog, error) {
	tags, err := marshalJSONString(entry.Tags)
	if err != nil {
		return database.ToolCatalog{}, err
	}
	var userID *string
	if strings.TrimSpace(entry.UserID) != "" {
		v := strings.TrimSpace(entry.UserID)
		userID = &v
	}
	var serverID *string
	if strings.TrimSpace(entry.MCPServerID) != "" {
		v := strings.TrimSpace(entry.MCPServerID)
		serverID = &v
	}
	return database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: entry.ID},
		UserID:             userID,
		MCPServerID:        serverID,
		Name:               entry.Name,
		DisplayName:        entry.DisplayName,
		Description:        entry.Description,
		Origin:             entry.Origin,
		Category:           entry.Category,
		Class:              entry.Class,
		Package:            entry.Package,
		Risk:               entry.Risk,
		Schema:             string(entry.Schema),
		SchemaHash:         entry.SchemaHash,
		SchemaBytes:        entry.SchemaBytes,
		Tags:               tags,
		AvailabilityStatus: entry.AvailabilityStatus,
		AvailabilityReason: entry.AvailabilityReason,
		LastSeenAt:         entry.LastSeenAt,
		LastAvailableAt:    entry.LastAvailableAt,
		LastUnavailableAt:  entry.LastUnavailableAt,
		LastTestedAt:       entry.LastTestedAt,
		LastTestStatus:     entry.LastTestStatus,
		LastTestError:      entry.LastTestError,
	}, nil
}

func toolModelToEntry(row database.ToolCatalog) (tools.ToolCatalogEntry, error) {
	var tags []string
	if err := unmarshalJSONString(row.Tags, &tags); err != nil {
		return tools.ToolCatalogEntry{}, err
	}
	entry := tools.ToolCatalogEntry{
		ID:                 row.ID,
		Name:               row.Name,
		DisplayName:        row.DisplayName,
		Description:        row.Description,
		Origin:             row.Origin,
		Category:           row.Category,
		Class:              row.Class,
		Package:            row.Package,
		Risk:               row.Risk,
		Schema:             json.RawMessage(row.Schema),
		SchemaHash:         row.SchemaHash,
		SchemaBytes:        row.SchemaBytes,
		Tags:               tags,
		AvailabilityStatus: row.AvailabilityStatus,
		AvailabilityReason: row.AvailabilityReason,
		LastSeenAt:         row.LastSeenAt,
		LastAvailableAt:    row.LastAvailableAt,
		LastUnavailableAt:  row.LastUnavailableAt,
		LastTestedAt:       row.LastTestedAt,
		LastTestStatus:     row.LastTestStatus,
		LastTestError:      row.LastTestError,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.UserID != nil {
		entry.UserID = *row.UserID
	}
	if row.MCPServerID != nil {
		entry.MCPServerID = *row.MCPServerID
	}
	return entry, nil
}

// isMCPNamespacedName reporta se o nome segue o padrão namespaced de uma tool
// MCP ("mcp_<slug>__<tool>"). Espelha mcp.ParseToolName sem importar o pacote
// MCP, evitando ciclo de import (mcp consome este pacote).
func isMCPNamespacedName(name string) bool {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "mcp_") {
		return false
	}
	parts := strings.SplitN(name[4:], "__", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func marshalJSONString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(b) == "null" {
		return "", nil
	}
	return string(b), nil
}

func unmarshalJSONString(raw string, dest any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}

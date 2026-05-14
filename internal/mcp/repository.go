package mcp

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

// Repository persiste configurações MCP, logs e catálogo de tools.
type Repository interface {
	ListServers(ctx context.Context) ([]ServerConfig, error)
	GetServer(ctx context.Context, slug string) (*ServerConfig, error)
	GetServerByID(ctx context.Context, id string) (*ServerConfig, error)
	SaveServer(ctx context.Context, cfg *ServerConfig) error
	DeleteServer(ctx context.Context, slug string) error
	DuplicateServer(ctx context.Context, slug, newSlug string) (*ServerConfig, error)
	LogEvent(ctx context.Context, entry *MCPServerLog) error
	GetLogs(ctx context.Context, slug string, limit int) ([]MCPServerLog, error)
	CleanOldLogs(maxAge time.Duration) (int, error)

	UpsertTool(ctx context.Context, entry *tools.ToolCatalogEntry) error
	ListTools(ctx context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error)
	MarkServerToolsUnavailable(ctx context.Context, serverID string, seenNames []string, reason string) (int, error)
	RecordToolTest(ctx context.Context, toolName, status, errorMessage string) error
}

// DBRepository implementa Repository usando GORM.
type DBRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{
		db:  db,
		now: time.Now,
	}
}

func (r *DBRepository) ListServers(ctx context.Context) ([]ServerConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var rows []database.MCPServer
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Order("slug ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]ServerConfig, 0, len(rows))
	for _, row := range rows {
		cfg, err := serverModelToConfig(row)
		if err != nil {
			return nil, err
		}
		result = append(result, cfg)
	}
	return result, nil
}

func (r *DBRepository) GetServer(ctx context.Context, slug string) (*ServerConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.MCPServer
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("slug = ?", strings.TrimSpace(slug)).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	cfg, err := serverModelToConfig(row)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *DBRepository) GetServerByID(ctx context.Context, id string) (*ServerConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.MCPServer
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("id = ?", strings.TrimSpace(id)).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	cfg, err := serverModelToConfig(row)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *DBRepository) SaveServer(ctx context.Context, cfg *ServerConfig) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("config MCP nil")
	}
	cfg.Slug = strings.TrimSpace(cfg.Slug)
	if cfg.Slug == "" {
		return fmt.Errorf("slug do servidor MCP é obrigatório")
	}
	cfg.UserID = userID
	cfg.applyDefaults(cfg.Slug)

	row, err := serverConfigToModel(*cfg)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.MCPServer
		err := tx.Where("user_id = ? AND slug = ?", userID, cfg.Slug).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if row.ID == "" {
				row.ID = cfg.ID
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			cfg.ID = row.ID
			return nil
		case err != nil:
			return err
		default:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if err := tx.Model(&existing).Select("*").Omit("id", "created_at").Updates(&row).Error; err != nil {
				return err
			}
			cfg.ID = existing.ID
			return nil
		}
	})
}

func (r *DBRepository) DeleteServer(ctx context.Context, slug string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row database.MCPServer
		if err := database.ScopeByUser(ctx, tx, "user_id").Where("slug = ?", strings.TrimSpace(slug)).First(&row).Error; err != nil {
			return err
		}
		now := r.now()
		if err := tx.Model(&database.ToolCatalog{}).
			Where("mcp_server_id = ?", row.ID).
			Updates(map[string]any{
				"mcp_server_id":       nil,
				"availability_status": "unavailable",
				"availability_reason": fmt.Sprintf("MCP server %q was deleted", row.Slug),
				"last_unavailable_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", row.ID).Delete(&database.MCPServerLog{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

func (r *DBRepository) DuplicateServer(ctx context.Context, slug, newSlug string) (*ServerConfig, error) {
	cfg, err := r.GetServer(ctx, slug)
	if err != nil {
		return nil, err
	}
	cfg.ID = ""
	cfg.Slug = strings.TrimSpace(newSlug)
	if cfg.Slug == "" {
		return nil, fmt.Errorf("novo slug é obrigatório")
	}
	if _, err := r.GetServer(ctx, cfg.Slug); err == nil {
		return nil, fmt.Errorf("servidor MCP '%s' já existe", cfg.Slug)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if cfg.Name == "" {
		cfg.Name = slug
	}
	cfg.Name = fmt.Sprintf("%s (Cópia)", cfg.Name)
	if err := r.SaveServer(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (r *DBRepository) LogEvent(ctx context.Context, entry *MCPServerLog) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("log MCP nil")
	}
	serverID := strings.TrimSpace(entry.ServerID)
	if serverID == "" {
		if strings.TrimSpace(entry.Slug) == "" {
			return fmt.Errorf("server_id ou slug é obrigatório para log MCP")
		}
		cfg, err := r.GetServer(ctx, entry.Slug)
		if err != nil {
			return err
		}
		serverID = cfg.ID
	} else {
		if _, err := r.GetServerByID(ctx, serverID); err != nil {
			return err
		}
	}
	when := entry.Timestamp
	if when.IsZero() {
		when = r.now()
	}
	row := database.MCPServerLog{
		UUIDModel: database.UUIDModel{ID: entry.ID},
		ServerID:  serverID,
		Timestamp: when,
		Type:      strings.TrimSpace(entry.Type),
		Message:   entry.Message,
		Data:      string(entry.Data),
	}
	if row.Type == "" {
		return fmt.Errorf("tipo do log MCP é obrigatório")
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	entry.ID = row.ID
	entry.ServerID = row.ServerID
	entry.Timestamp = row.Timestamp
	entry.CreatedAt = row.CreatedAt
	return nil
}

func (r *DBRepository) GetLogs(ctx context.Context, slug string, limit int) ([]MCPServerLog, error) {
	cfg, err := r.GetServer(ctx, slug)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []database.MCPServerLog
	if err := r.db.WithContext(ctx).
		Where("server_id = ?", cfg.ID).
		Order("timestamp DESC, created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]MCPServerLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, logModelToDomain(row, cfg.Slug))
	}
	return result, nil
}

func (r *DBRepository) CleanOldLogs(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}
	cutoff := r.now().Add(-maxAge)
	tx := r.db.Where("timestamp < ?", cutoff).Delete(&database.MCPServerLog{})
	return int(tx.RowsAffected), tx.Error
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
	if _, err := r.GetServerByID(ctx, serverID); err != nil {
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
	if _, _, ok := ParseToolName(toolName); ok {
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
		if _, err := r.GetServerByID(ctx, entry.MCPServerID); err != nil {
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

func serverConfigToModel(cfg ServerConfig) (database.MCPServer, error) {
	args, err := marshalJSONString(cfg.Args)
	if err != nil {
		return database.MCPServer{}, err
	}
	env, err := marshalJSONString(cfg.Env)
	if err != nil {
		return database.MCPServer{}, err
	}
	scopes, err := marshalJSONString(cfg.OAuth2Scopes)
	if err != nil {
		return database.MCPServer{}, err
	}
	return database.MCPServer{
		UUIDModel:             database.UUIDModel{ID: cfg.ID},
		UserID:                cfg.UserID,
		Slug:                  cfg.Slug,
		Name:                  cfg.Name,
		Description:           cfg.Description,
		Transport:             string(cfg.Transport),
		Command:               cfg.Command,
		Args:                  args,
		Env:                   env,
		URL:                   cfg.URL,
		AuthType:              string(cfg.AuthType),
		OAuth2ClientID:        cfg.OAuth2ClientID,
		OAuth2AuthURL:         cfg.OAuth2AuthURL,
		OAuth2TokenURL:        cfg.OAuth2TokenURL,
		OAuth2Scopes:          scopes,
		OAuth2CallbackPort:    cfg.OAuth2CallbackPort,
		OAuth2CallbackHost:    cfg.OAuth2CallbackHost,
		OAuth2RegistrationURL: cfg.OAuth2RegistrationURL,
		OAuth2DeviceAuthURL:   cfg.OAuth2DeviceAuthURL,
		DisableSSE:            cfg.DisableSSE,
		PreferBridge:          cfg.PreferBridge,
		Enabled:               cfg.Enabled,
		AutoConnect:           cfg.AutoConnect,
	}, nil
}

func serverModelToConfig(row database.MCPServer) (ServerConfig, error) {
	var args []string
	if err := unmarshalJSONString(row.Args, &args); err != nil {
		return ServerConfig{}, err
	}
	env := map[string]string{}
	if err := unmarshalJSONString(row.Env, &env); err != nil {
		return ServerConfig{}, err
	}
	var scopes []string
	if err := unmarshalJSONString(row.OAuth2Scopes, &scopes); err != nil {
		return ServerConfig{}, err
	}
	return ServerConfig{
		ID:                    row.ID,
		UserID:                row.UserID,
		Slug:                  row.Slug,
		Name:                  row.Name,
		Description:           row.Description,
		Transport:             TransportType(row.Transport),
		Command:               row.Command,
		Args:                  args,
		Env:                   env,
		URL:                   row.URL,
		AuthType:              AuthType(row.AuthType),
		OAuth2ClientID:        row.OAuth2ClientID,
		OAuth2AuthURL:         row.OAuth2AuthURL,
		OAuth2TokenURL:        row.OAuth2TokenURL,
		OAuth2Scopes:          scopes,
		OAuth2CallbackPort:    row.OAuth2CallbackPort,
		OAuth2CallbackHost:    row.OAuth2CallbackHost,
		OAuth2RegistrationURL: row.OAuth2RegistrationURL,
		OAuth2DeviceAuthURL:   row.OAuth2DeviceAuthURL,
		DisableSSE:            row.DisableSSE,
		PreferBridge:          row.PreferBridge,
		Enabled:               row.Enabled,
		AutoConnect:           row.AutoConnect,
	}, nil
}

func logModelToDomain(row database.MCPServerLog, slug string) MCPServerLog {
	return MCPServerLog{
		ID:        row.ID,
		ServerID:  row.ServerID,
		Slug:      slug,
		Timestamp: row.Timestamp,
		Type:      row.Type,
		Message:   row.Message,
		Data:      json.RawMessage(row.Data),
		CreatedAt: row.CreatedAt,
	}
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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolcatalog"

	"gorm.io/gorm"
)

// Repository persiste configurações e logs de servidores MCP.
//
// A persistência/sync do catálogo de tools (tool_catalog) NÃO pertence mais ao
// MCP: ela vive no pacote dedicado internal/toolcatalog (AEP-0077, Fase 2 /
// #120). O MCP consome esse catálogo via Manager.SetCatalog.
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
	if strings.EqualFold(cfg.Slug, "native") {
		return fmt.Errorf("slug do servidor MCP 'native' é reservado")
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
		// Desanexa as tools do catálogo (lógica de tool_catalog vive em
		// internal/toolcatalog), dentro desta transação para manter atomicidade
		// com a remoção do servidor.
		reason := fmt.Sprintf("MCP server %q was deleted", row.Slug)
		if err := toolcatalog.DetachServerTools(tx, row.ID, row.UserID, reason, now); err != nil {
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

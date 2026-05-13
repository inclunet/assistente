package portability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"gorm.io/gorm"
)

func buildMCPServerExports(ctx context.Context, slugs []string) ([]MCPServerExport, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	unique := make([]string, 0, len(slugs))
	seen := make(map[string]struct{}, len(slugs))
	for _, raw := range slugs {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			return nil, fmt.Errorf("mcpServerSlug inválido: %q", raw)
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		unique = append(unique, slug)
	}

	var rows []database.MCPServer
	if err := database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").
		Where("slug IN ?", unique).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar servidores MCP para exportação: %w", err)
	}
	bySlug := make(map[string]database.MCPServer, len(rows))
	for _, row := range rows {
		bySlug[row.Slug] = row
	}

	result := make([]MCPServerExport, 0, len(unique))
	for _, slug := range unique {
		row, ok := bySlug[slug]
		if !ok {
			return nil, fmt.Errorf("erro ao buscar servidor MCP %s: %w", slug, gorm.ErrRecordNotFound)
		}
		exported, err := exportMCPServer(row)
		if err != nil {
			return nil, err
		}
		result = append(result, exported)
	}
	return result, nil
}

func exportMCPServer(row database.MCPServer) (MCPServerExport, error) {
	args, err := decodeStringSlice(row.Args)
	if err != nil {
		return MCPServerExport{}, fmt.Errorf("erro ao decodificar args do servidor MCP %s: %w", row.Slug, err)
	}
	scopes, err := decodeStringSlice(row.OAuth2Scopes)
	if err != nil {
		return MCPServerExport{}, fmt.Errorf("erro ao decodificar oauth scopes do servidor MCP %s: %w", row.Slug, err)
	}
	return MCPServerExport{
		ID:                    row.ID,
		Slug:                  row.Slug,
		Name:                  row.Name,
		Description:           row.Description,
		Transport:             row.Transport,
		Command:               row.Command,
		Args:                  args,
		URL:                   row.URL,
		AuthType:              row.AuthType,
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
		CreatedAt:             row.CreatedAt,
	}, nil
}

// ImportMCPServerWithContext imports a canonical portable MCP server. Existing
// slugs are skipped so import remains idempotent and never overwrites local config.
func ImportMCPServerWithContext(ctx context.Context, server MCPServerExport) (bool, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return false, err
	}
	server = normalizeMCPServerExport(server)
	if server.Slug == "" {
		return false, fmt.Errorf("slug do servidor MCP é obrigatório")
	}

	var existing database.MCPServer
	err = database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").
		Where("slug = ?", server.Slug).
		First(&existing).Error
	switch {
	case err == nil:
		return false, nil
	case err != nil && !errorsIsRecordNotFound(err):
		return false, err
	}
	if err := validateMCPServerExport(server); err != nil {
		return false, err
	}

	row, err := mcpServerExportToModel(userID, server)
	if err != nil {
		return false, err
	}
	if err := database.DB().WithContext(ctx).Create(&row).Error; err != nil {
		return false, fmt.Errorf("erro ao importar servidor MCP %s: %w", server.Slug, err)
	}
	return true, nil
}

func importMCPServerWithCredentials(ctx context.Context, credMgr *credentials.Manager, server MCPServerExport) (bool, error) {
	imported, err := ImportMCPServerWithContext(ctx, server)
	if err != nil || !imported {
		return imported, err
	}
	if err := importMCPServerInlineCredential(ctx, credMgr, server); err != nil {
		return true, fmt.Errorf("erro ao importar credencial do servidor MCP %s: %w", server.Slug, err)
	}
	return true, nil
}

func normalizeMCPServerExport(server MCPServerExport) MCPServerExport {
	server.Slug = strings.TrimSpace(server.Slug)
	server.Name = strings.TrimSpace(server.Name)
	if server.Name == "" {
		server.Name = formatMCPNameFromSlug(server.Slug)
	}
	server.Transport = strings.TrimSpace(server.Transport)
	if server.Transport == "" {
		if strings.TrimSpace(server.URL) != "" {
			server.Transport = "streamable"
		} else if strings.TrimSpace(server.Command) != "" {
			server.Transport = "stdio"
		}
	}
	server.AuthType = strings.TrimSpace(server.AuthType)
	if server.AuthType == "" {
		server.AuthType = inferMCPAuthType(server)
	}
	return server
}

func validateMCPServerExport(server MCPServerExport) error {
	switch server.Transport {
	case "stdio":
		if strings.TrimSpace(server.Command) == "" {
			return fmt.Errorf("servidor MCP %s usa transport stdio mas command é obrigatório", server.Slug)
		}
	case "sse", "streamable":
		rawURL := strings.TrimSpace(server.URL)
		if rawURL == "" {
			return fmt.Errorf("servidor MCP %s usa transport %s mas url é obrigatória", server.Slug, server.Transport)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("servidor MCP %s usa url inválida: %q", server.Slug, server.URL)
		}
	default:
		return fmt.Errorf("servidor MCP %s tem transport inválido ou ausente: %q", server.Slug, server.Transport)
	}
	return nil
}

func mcpServerExportToModel(userID string, server MCPServerExport) (database.MCPServer, error) {
	args, err := encodeJSON(server.Args)
	if err != nil {
		return database.MCPServer{}, fmt.Errorf("erro ao serializar args do servidor MCP %s: %w", server.Slug, err)
	}
	env, err := encodeJSON(server.Env)
	if err != nil {
		return database.MCPServer{}, fmt.Errorf("erro ao serializar env do servidor MCP %s: %w", server.Slug, err)
	}
	scopes, err := encodeJSON(server.OAuth2Scopes)
	if err != nil {
		return database.MCPServer{}, fmt.Errorf("erro ao serializar oauth scopes do servidor MCP %s: %w", server.Slug, err)
	}
	row := database.MCPServer{
		UUIDModel:             database.UUIDModel{ID: strings.TrimSpace(server.ID)},
		UserID:                userID,
		Slug:                  server.Slug,
		Name:                  server.Name,
		Description:           server.Description,
		Transport:             server.Transport,
		Command:               server.Command,
		Args:                  args,
		Env:                   env,
		URL:                   server.URL,
		AuthType:              server.AuthType,
		OAuth2ClientID:        server.OAuth2ClientID,
		OAuth2AuthURL:         server.OAuth2AuthURL,
		OAuth2TokenURL:        server.OAuth2TokenURL,
		OAuth2Scopes:          scopes,
		OAuth2CallbackPort:    server.OAuth2CallbackPort,
		OAuth2CallbackHost:    server.OAuth2CallbackHost,
		OAuth2RegistrationURL: server.OAuth2RegistrationURL,
		OAuth2DeviceAuthURL:   server.OAuth2DeviceAuthURL,
		DisableSSE:            server.DisableSSE,
		PreferBridge:          server.PreferBridge,
		Enabled:               server.Enabled,
		AutoConnect:           server.AutoConnect,
	}
	if !server.CreatedAt.IsZero() {
		row.CreatedAt = server.CreatedAt
		row.UpdatedAt = server.CreatedAt
	}
	return row, nil
}

func decodeStringSlice(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var result []string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func encodeJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return "", nil
	}
	return string(raw), nil
}

func formatMCPNameFromSlug(slug string) string {
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.ReplaceAll(slug, "_", " ")
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

func errorsIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

type externalMCPExportFile struct {
	MCPServers map[string]externalMCPServer `json:"mcpServers"`
}

type externalMCPServer struct {
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	RequestInit struct {
		Headers map[string]string `json:"headers,omitempty"`
	} `json:"requestInit,omitempty"`
}

// ExportMCPServersExternalJSONWithContext returns Cursor/Claude-compatible
// {"mcpServers": {...}} JSON for the selected slugs.
func ExportMCPServersExternalJSONWithContext(ctx context.Context, slugs []string) (string, error) {
	servers, err := buildMCPServerExports(ctx, slugs)
	if err != nil {
		return "", err
	}
	out := externalMCPExportFile{MCPServers: make(map[string]externalMCPServer, len(servers))}
	for _, server := range servers {
		out.MCPServers[server.Slug] = externalMCPServer{
			Command: server.Command,
			Args:    server.Args,
			Env:     server.Env,
			URL:     server.URL,
		}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar export MCP compatível: %w", err)
	}
	return string(raw), nil
}

func parseExternalMCPServers(data []byte) ([]MCPServerExport, bool, error) {
	var wrapped externalMCPExportFile
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, false, err
	}
	var servers map[string]externalMCPServer
	wrappedFormat := wrapped.MCPServers != nil
	if wrapped.MCPServers != nil {
		servers = wrapped.MCPServers
	} else {
		if err := json.Unmarshal(data, &servers); err != nil {
			return nil, false, err
		}
	}
	if len(servers) == 0 {
		return nil, wrappedFormat, nil
	}
	if !wrappedFormat && !allExternalMCPServerConfigs(servers) {
		return nil, false, nil
	}
	result := make([]MCPServerExport, 0, len(servers))
	for name, entry := range servers {
		slug := sanitizeMCPSlug(name)
		server := MCPServerExport{
			Slug:        slug,
			Name:        formatMCPNameFromSlug(slug),
			Command:     entry.Command,
			Args:        entry.Args,
			Env:         entry.Env,
			URL:         entry.URL,
			Enabled:     true,
			AutoConnect: true,
		}
		if token := extractExternalBearerToken(entry.RequestInit.Headers); token != "" {
			server.AuthType = "bearer"
			server.BearerToken = token
		} else if token := extractExternalBearerToken(entry.Headers); token != "" {
			server.AuthType = "bearer"
			server.BearerToken = token
		}
		server = normalizeMCPServerExport(server)
		result = append(result, server)
	}
	return result, true, nil
}

func allExternalMCPServerConfigs(servers map[string]externalMCPServer) bool {
	for _, entry := range servers {
		if strings.TrimSpace(entry.Command) == "" && strings.TrimSpace(entry.URL) == "" {
			return false
		}
	}
	return true
}

func ImportMCPServersJSONWithContext(ctx context.Context, data []byte, credMgr *credentials.Manager) (LegacyImportResult, error) {
	result := LegacyImportResult{
		ResourceType: "servidor MCP",
		Warnings:     make([]string, 0),
		Errors:       make([]string, 0),
	}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}
	servers, ok, err := parseExternalMCPServers(data)
	if err != nil {
		result.Failed++
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}
	if !ok {
		err := fmt.Errorf("JSON não parece um arquivo de servidores MCP")
		result.Failed++
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}
	for _, server := range servers {
		imported, err := importMCPServerWithCredentials(ctx, credMgr, server)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if imported {
			result.Imported++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

// ImportLegacyMCPServersWithContext imports read-only legacy JSON config files
// into the canonical portability model. Existing DB slugs are skipped by
// ImportMCPServerWithContext, keeping repeated startup imports idempotent.
func ImportLegacyMCPServersWithContext(ctx context.Context, source LegacyImportSource, credMgr *credentials.Manager) (LegacyImportResult, error) {
	return ImportLegacyResourcesWithContext(ctx, LegacyImportRequest[MCPServerExport]{
		ResourceType: "servidor MCP",
		Source:       source,
		FileSuffix:   ".json",
		Parse:        parseLegacyMCPServerFile,
		Import: func(ctx context.Context, server MCPServerExport) (bool, error) {
			return importMCPServerWithCredentials(ctx, credMgr, server)
		},
	})
}

func parseLegacyMCPServerFile(file LegacyImportFile, data []byte) (MCPServerExport, error) {
	slug := strings.TrimSpace(file.Name)
	if slug == "" {
		slug = strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	}

	var cfg legacyMCPServerFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return MCPServerExport{}, err
	}
	server := MCPServerExport{
		ID:                    cfg.ID,
		Slug:                  firstNonEmpty(cfg.Slug, slug),
		Name:                  cfg.Name,
		Description:           cfg.Description,
		Transport:             firstNonEmpty(cfg.Transport, cfg.TransportCamel),
		Command:               cfg.Command,
		Args:                  cfg.Args,
		Env:                   cfg.Env,
		URL:                   cfg.URL,
		AuthType:              firstNonEmpty(cfg.AuthType, cfg.AuthTypeCamel),
		OAuth2ClientID:        firstNonEmpty(cfg.OAuth2ClientID, cfg.OAuth2ClientIDCamel),
		OAuth2AuthURL:         firstNonEmpty(cfg.OAuth2AuthURL, cfg.OAuth2AuthURLCamel),
		OAuth2TokenURL:        firstNonEmpty(cfg.OAuth2TokenURL, cfg.OAuth2TokenURLCamel),
		OAuth2Scopes:          cfg.OAuth2Scopes,
		OAuth2CallbackPort:    firstNonZero(cfg.OAuth2CallbackPort, cfg.OAuth2CallbackPortCamel),
		OAuth2CallbackHost:    firstNonEmpty(cfg.OAuth2CallbackHost, cfg.OAuth2CallbackHostCamel),
		OAuth2RegistrationURL: firstNonEmpty(cfg.OAuth2RegistrationURL, cfg.OAuth2RegistrationURLCamel),
		OAuth2DeviceAuthURL:   firstNonEmpty(cfg.OAuth2DeviceAuthURL, cfg.OAuth2DeviceAuthURLCamel),
		DisableSSE:            cfg.DisableSSE || cfg.DisableSSECamel,
		PreferBridge:          cfg.PreferBridge || cfg.PreferBridgeCamel,
		Enabled:               true,
		AutoConnect:           true,
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	if hasJSONKey(raw, "enabled") {
		server.Enabled = cfg.Enabled
	}
	if hasJSONKey(raw, "auto_connect") {
		server.AutoConnect = cfg.AutoConnect
	} else if hasJSONKey(raw, "autoConnect") {
		server.AutoConnect = cfg.AutoConnectCamel
	}
	if token := extractExternalBearerToken(cfg.RequestInit.Headers); token != "" {
		server.AuthType = "bearer"
		server.BearerToken = token
	} else if token := extractExternalBearerToken(cfg.Headers); token != "" {
		server.AuthType = "bearer"
		server.BearerToken = token
	}
	return normalizeMCPServerExport(server), nil
}

type legacyMCPServerFile struct {
	ID            string            `json:"id"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Transport     string            `json:"transport"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	Env           map[string]string `json:"env"`
	URL           string            `json:"url"`
	AuthType      string            `json:"auth_type"`
	AuthTypeCamel string            `json:"authType"`

	OAuth2ClientID             string            `json:"oauth2_client_id"`
	OAuth2ClientIDCamel        string            `json:"oauth2ClientId"`
	OAuth2AuthURL              string            `json:"oauth2_auth_url"`
	OAuth2AuthURLCamel         string            `json:"oauth2AuthUrl"`
	OAuth2TokenURL             string            `json:"oauth2_token_url"`
	OAuth2TokenURLCamel        string            `json:"oauth2TokenUrl"`
	OAuth2Scopes               []string          `json:"oauth2_scopes"`
	OAuth2CallbackPort         int               `json:"oauth2_callback_port"`
	OAuth2CallbackPortCamel    int               `json:"oauth2CallbackPort"`
	OAuth2CallbackHost         string            `json:"oauth2_callback_host"`
	OAuth2CallbackHostCamel    string            `json:"oauth2CallbackHost"`
	OAuth2RegistrationURL      string            `json:"oauth2_registration_url"`
	OAuth2RegistrationURLCamel string            `json:"oauth2RegistrationUrl"`
	OAuth2DeviceAuthURL        string            `json:"oauth2_device_auth_url"`
	OAuth2DeviceAuthURLCamel   string            `json:"oauth2DeviceAuthUrl"`
	DisableSSE                 bool              `json:"disable_sse"`
	DisableSSECamel            bool              `json:"disableSse"`
	PreferBridge               bool              `json:"prefer_bridge"`
	PreferBridgeCamel          bool              `json:"preferBridge"`
	Enabled                    bool              `json:"enabled"`
	AutoConnect                bool              `json:"auto_connect"`
	AutoConnectCamel           bool              `json:"autoConnect"`
	Headers                    map[string]string `json:"headers"`
	RequestInit                struct {
		Headers map[string]string `json:"headers"`
	} `json:"requestInit"`
	TransportCamel string `json:"transportType"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func hasJSONKey(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

// Mirrors internal/mcp.ServerConfig.inferAuthType. Keep these branches aligned
// while portability cannot import internal/mcp without creating a package cycle.
func inferMCPAuthType(server MCPServerExport) string {
	hasOAuthEndpoints := server.OAuth2AuthURL != "" || server.OAuth2TokenURL != ""
	hasClientID := server.OAuth2ClientID != ""
	hasRegistrationURL := server.OAuth2RegistrationURL != ""
	hasDeviceAuthURL := server.OAuth2DeviceAuthURL != ""
	if hasOAuthEndpoints || hasClientID || hasRegistrationURL || hasDeviceAuthURL {
		return "oauth2_pkce"
	}
	if server.URL != "" && server.Command == "" {
		return "oauth2_pkce"
	}
	return "none"
}

func importMCPServerInlineCredential(ctx context.Context, credMgr *credentials.Manager, server MCPServerExport) error {
	if credMgr == nil || strings.TrimSpace(server.BearerToken) == "" {
		return nil
	}
	hostname := hostnameFromMCPURL(server.URL)
	if hostname == "" {
		return nil
	}
	return credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{
		Type:  "bearer",
		Token: server.BearerToken,
	})
}

func extractExternalBearerToken(headers map[string]string) string {
	for name, value := range headers {
		if !strings.EqualFold(name, "Authorization") {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "bearer ") {
			return strings.TrimSpace(value[len("bearer "):])
		}
	}
	return ""
}

func hostnameFromMCPURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return parsed.Hostname()
}

func sanitizeMCPSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	var clean []byte
	for _, c := range []byte(slug) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		}
	}
	return string(clean)
}

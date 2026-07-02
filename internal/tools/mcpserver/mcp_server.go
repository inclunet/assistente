package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	mcpmgr "assistente/internal/mcp"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

const ToolName = "mcp_server"

type Manager interface {
	List() []mcpmgr.ServerInfo
	GetConfig(slug string) (*mcpmgr.ServerConfig, error)
	SaveConfig(slug string, cfg mcpmgr.ServerConfig) error
	DeleteConfig(slug string) error
	DuplicateConfig(slug string) (string, error)
	Connect(slug string) error
	Disconnect(slug string) error
	Reconnect(slug string) error
	LoadConfigs() error
	GetLogs(slug string, limit int) ([]mcpmgr.MCPServerLog, error)
}

type ManagerProvider func() Manager

type Tool struct {
	mgr ManagerProvider
}

type request struct {
	Action      string            `json:"action,omitempty"`
	Slug        string            `json:"slug,omitempty"`
	NewSlug     string            `json:"new_slug,omitempty"`
	Limit       int               `json:"limit,omitempty"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Transport   string            `json:"transport,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	AuthType    string            `json:"auth_type,omitempty"`

	OAuth2ClientID        string   `json:"oauth2_client_id,omitempty"`
	OAuth2AuthURL         string   `json:"oauth2_auth_url,omitempty"`
	OAuth2TokenURL        string   `json:"oauth2_token_url,omitempty"`
	OAuth2Scopes          []string `json:"oauth2_scopes,omitempty"`
	OAuth2CallbackPort    int      `json:"oauth2_callback_port,omitempty"`
	OAuth2CallbackHost    string   `json:"oauth2_callback_host,omitempty"`
	OAuth2RegistrationURL string   `json:"oauth2_registration_url,omitempty"`
	OAuth2DeviceAuthURL   string   `json:"oauth2_device_auth_url,omitempty"`

	DisableSSE   bool `json:"disable_sse,omitempty"`
	PreferBridge bool `json:"prefer_bridge,omitempty"`
	Enabled      bool `json:"enabled,omitempty"`
	AutoConnect  bool `json:"auto_connect,omitempty"`

	present map[string]json.RawMessage
}

func New(mgr Manager) *Tool {
	return NewWithProvider(func() Manager { return mgr })
}

func NewWithProvider(provider ManagerProvider) *Tool {
	return &Tool{mgr: provider}
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{
		Category: "mcp",
		Class:    "mcp_management",
		Package:  "mcp",
		Risk:     "write",
		Tags:     []string{"mcp", "servers", "configuration"},
	}
}

func (t *Tool) Description() string {
	return "Manage persisted MCP server configurations using the existing MCP manager. No params lists servers. slug reads safe details without env values. action supports create, update, delete, duplicate, connect, disconnect, reconnect, reload and logs. When action is omitted, slug plus any configuration field infers create for a missing server or update for an existing server. Writes require slug and use the same user-scoped backend contracts as the MCP page; env values are accepted for create/update but are redacted from read responses."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["list", "get", "create", "update", "delete", "duplicate", "connect", "disconnect", "reconnect", "reload", "logs"], "description": "Operation to perform. When omitted, no slug lists servers; slug without configuration fields reads server details; slug plus configuration fields infers create for a missing server or update for an existing server."},
    "slug": {"type": "string", "pattern": "^(?![Nn][Aa][Tt][Ii][Vv][Ee]$)(?!.*__)[A-Za-z0-9_-]+$", "description": "User-scoped MCP server slug. Required for get/update/delete/duplicate/connect/disconnect/reconnect/logs. For create, this is the new slug. Use only letters, numbers, underscore or hyphen; the slug native and the substring __ are reserved for MCP bridge/native tool names."},
    "new_slug": {"type": "string", "description": "Reserved for future explicit duplicate target slugs. The current backend generates copy slugs; do not use unless supported by the backend."},
    "limit": {"type": "integer", "description": "Log limit for action=logs. Defaults to 100 and caps at 500.", "minimum": 1, "maximum": 500},
    "name": {"type": "string", "description": "Display name. Required when creating."},
    "description": {"type": "string"},
    "transport": {"type": "string", "enum": ["stdio", "sse", "streamable"], "description": "MCP transport. Required when creating unless command or url allows backend defaults."},
    "command": {"type": "string", "description": "Command for stdio servers."},
    "args": {"type": "array", "items": {"type": "string"}, "description": "Arguments for stdio servers. Send [] to clear args on update; null is rejected."},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Environment variables for stdio servers. Values may be sensitive; they are never returned by this tool. On update, omitted env preserves existing values, a non-empty object merges keys into the existing env, and {} clears env explicitly; null is rejected."},
    "url": {"type": "string", "description": "URL for sse or streamable servers."},
    "auth_type": {"type": "string", "enum": ["none", "bearer", "basic", "oauth2_client_credentials", "oauth2_pkce"]},
    "oauth2_client_id": {"type": "string"},
    "oauth2_auth_url": {"type": "string"},
    "oauth2_token_url": {"type": "string"},
    "oauth2_scopes": {"type": "array", "items": {"type": "string"}, "description": "OAuth2 scopes. Send [] to clear scopes on update; null is rejected."},
    "oauth2_callback_port": {"type": "integer"},
    "oauth2_callback_host": {"type": "string"},
    "oauth2_registration_url": {"type": "string"},
    "oauth2_device_auth_url": {"type": "string"},
    "disable_sse": {"type": "boolean"},
    "prefer_bridge": {"type": "boolean"},
    "enabled": {"type": "boolean", "description": "Enable or disable this server configuration."},
    "auto_connect": {"type": "boolean", "description": "Whether this server should auto-connect after login/startup."}
  },
  "additionalProperties": false
}`)
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	mgr := t.manager()
	if mgr == nil {
		return tools.ToolResult{Content: "MCP manager não configurado", IsError: true}, nil
	}
	req, err := parseRequest(args)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("argumentos inválidos para mcp_server: %v", err), IsError: true}, nil
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	slug := strings.TrimSpace(req.Slug)
	if slug != "" && !validServerSlug(slug) {
		return tools.ToolResult{Content: fmt.Sprintf("slug MCP inválido %q: use apenas letras, números, '_' ou '-' e não use o slug reservado 'native' nem '__'", slug), IsError: true}, nil
	}
	if action == "" {
		if slug == "" {
			action = "list"
		} else if req.hasWriteFields() {
			inferred, errResult, err := inferWriteAction(mgr, slug)
			if err != nil {
				return errResult, nil
			}
			action = inferred
		} else {
			action = "get"
		}
	}
	if req.NewSlug != "" {
		return tools.ToolResult{Content: "new_slug ainda não é suportado pelo backend MCP atual; use duplicate sem new_slug para gerar o slug automaticamente", IsError: true}, nil
	}

	switch action {
	case "list":
		return listServers(mgr), nil
	case "get":
		return getServer(mgr, slug)
	case "create":
		return saveServer(ctx, mgr, req, true)
	case "update":
		return saveServer(ctx, mgr, req, false)
	case "delete":
		return deleteServer(mgr, slug)
	case "duplicate":
		return duplicateServer(mgr, slug)
	case "connect":
		return connectionAction(mgr, slug, "connected", "conectar", mgr.Connect)
	case "disconnect":
		return connectionAction(mgr, slug, "disconnected", "desconectar", mgr.Disconnect)
	case "reconnect":
		return connectionAction(mgr, slug, "reconnected", "reconectar", mgr.Reconnect)
	case "reload":
		return reloadServers(mgr)
	case "logs":
		return logs(mgr, slug, req.Limit)
	default:
		return tools.ToolResult{Content: fmt.Sprintf("ação MCP inválida: %s", action), IsError: true}, nil
	}
}

func parseRequest(args json.RawMessage) (request, error) {
	if strings.TrimSpace(string(args)) == "" {
		args = json.RawMessage(`{}`)
	}
	var req request
	if err := json.Unmarshal(args, &req); err != nil {
		return req, err
	}
	if err := json.Unmarshal(args, &req.present); err != nil {
		return req, err
	}
	if raw, ok := req.present["env"]; ok && rawJSONIsNull(raw) {
		return req, errors.New("env não aceita null; omita o campo para preservar ou envie {} para limpar")
	}
	if raw, ok := req.present["args"]; ok && rawJSONIsNull(raw) {
		return req, errors.New("args não aceita null; omita o campo para preservar ou envie [] para limpar")
	}
	if raw, ok := req.present["oauth2_scopes"]; ok && rawJSONIsNull(raw) {
		return req, errors.New("oauth2_scopes não aceita null; omita o campo para preservar ou envie [] para limpar")
	}
	return req, nil
}

func rawJSONIsNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func (p request) has(field string) bool {
	_, ok := p.present[field]
	return ok
}

func (p request) hasWriteFields() bool {
	for _, field := range []string{
		"name", "description", "transport", "command", "args", "env", "url", "auth_type",
		"oauth2_client_id", "oauth2_auth_url", "oauth2_token_url", "oauth2_scopes",
		"oauth2_callback_port", "oauth2_callback_host", "oauth2_registration_url",
		"oauth2_device_auth_url", "disable_sse", "prefer_bridge", "enabled", "auto_connect",
	} {
		if p.has(field) {
			return true
		}
	}
	return false
}

func validServerSlug(slug string) bool {
	if slug == "" || strings.Contains(slug, "__") || strings.EqualFold(slug, "native") {
		return false
	}
	for _, r := range slug {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (t *Tool) manager() Manager {
	if t.mgr == nil {
		return nil
	}
	mgr := t.mgr()
	if managerIsNil(mgr) {
		return nil
	}
	return mgr
}

func inferWriteAction(mgr Manager, slug string) (string, tools.ToolResult, error) {
	if _, err := mgr.GetConfig(slug); err == nil {
		return "update", tools.ToolResult{}, nil
	} else if isNotFound(err) {
		return "create", tools.ToolResult{}, nil
	} else {
		return "", tools.ToolResult{Content: fmt.Sprintf("erro ao verificar servidor MCP %q: %v", slug, err), IsError: true}, err
	}
}

func managerIsNil(mgr Manager) bool {
	if mgr == nil {
		return true
	}
	v := reflect.ValueOf(mgr)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func listServers(mgr Manager) tools.ToolResult {
	servers := mgr.List()
	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].Slug != servers[j].Slug {
			return servers[i].Slug < servers[j].Slug
		}
		return servers[i].Name < servers[j].Name
	})
	payload := make([]listServerResponse, 0, len(servers))
	for _, server := range servers {
		payload = append(payload, toListServerResponse(server))
	}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{
		Content:    string(data),
		Metadata:   map[string]any{"count": len(servers), "action": "list"},
		Structured: true,
	}
}

func getServer(mgr Manager, slug string) (tools.ToolResult, error) {
	if slug == "" {
		return tools.ToolResult{Content: "slug é obrigatório para ler servidor MCP", IsError: true}, nil
	}
	cfg, err := mgr.GetConfig(slug)
	if err != nil {
		if isNotFound(err) {
			return tools.ToolResult{Content: fmt.Sprintf("servidor MCP %q não encontrado", slug), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("erro ao ler servidor MCP %q: %v", slug, err), IsError: true}, nil
	}
	info, ok := serverInfoBySlug(mgr.List(), slug)
	if !ok {
		info = mcpmgr.ServerInfo{
			ID:          cfg.ID,
			Slug:        cfg.Slug,
			Name:        cfg.Name,
			Description: cfg.Description,
			Transport:   cfg.Transport,
			Status:      mcpmgr.StatusDisconnected,
			Enabled:     cfg.Enabled,
			AutoConnect: cfg.AutoConnect,
			Command:     cfg.Command,
			Args:        cfg.Args,
			URL:         cfg.URL,
		}
	}
	payload := detailResponse{
		listServerResponse: toListServerResponse(info),
		Config:             safeConfig(*cfg),
		EnvKeys:            sortedEnvKeys(cfg.Env),
		EnvRedacted:        len(cfg.Env) > 0,
	}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"slug": slug, "action": "get"}, Structured: true}, nil
}

func saveServer(_ context.Context, mgr Manager, req request, create bool) (tools.ToolResult, error) {
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return tools.ToolResult{Content: "slug é obrigatório para criar ou atualizar servidor MCP", IsError: true}, nil
	}
	if !req.hasWriteFields() {
		return tools.ToolResult{Content: "nenhum campo de configuração MCP foi informado para salvar", IsError: true}, nil
	}
	if create && strings.TrimSpace(req.Name) == "" {
		return tools.ToolResult{Content: "name é obrigatório para criar servidor MCP", IsError: true}, nil
	}

	var cfg mcpmgr.ServerConfig
	if create {
		if _, err := mgr.GetConfig(slug); err == nil {
			return tools.ToolResult{Content: fmt.Sprintf("servidor MCP %q já existe", slug), IsError: true}, nil
		} else if !isNotFound(err) {
			return tools.ToolResult{Content: fmt.Sprintf("erro ao verificar servidor MCP %q: %v", slug, err), IsError: true}, nil
		}
		if !req.has("enabled") {
			cfg.Enabled = true
		}
		if !req.has("auto_connect") {
			cfg.AutoConnect = true
		}
	} else {
		existing, err := mgr.GetConfig(slug)
		if err != nil {
			if isNotFound(err) {
				return tools.ToolResult{Content: fmt.Sprintf("servidor MCP %q não encontrado para atualização", slug), IsError: true}, nil
			}
			return tools.ToolResult{Content: fmt.Sprintf("erro ao ler servidor MCP %q para atualização: %v", slug, err), IsError: true}, nil
		}
		if existing != nil {
			cfg = *existing
		}
	}
	applyConfigFields(&cfg, slug, req)
	if err := validateConfigForSave(cfg); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := mgr.SaveConfig(slug, cfg); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao salvar servidor MCP %q: %v", slug, err), IsError: true}, nil
	}
	action := "updated"
	if create {
		action = "created"
	}
	return actionResult(action, slug), nil
}

func applyConfigFields(cfg *mcpmgr.ServerConfig, slug string, req request) {
	cfg.Slug = slug
	if req.has("name") {
		cfg.Name = strings.TrimSpace(req.Name)
	}
	if req.has("description") {
		cfg.Description = req.Description
	}
	if req.has("transport") {
		cfg.Transport = mcpmgr.TransportType(strings.TrimSpace(req.Transport))
	}
	if req.has("command") {
		cfg.Command = strings.TrimSpace(req.Command)
	}
	if req.has("args") {
		cfg.Args = append([]string{}, req.Args...)
	}
	if req.has("env") {
		cfg.Env = mergeEnv(cfg.Env, req.Env)
	}
	if req.has("url") {
		cfg.URL = strings.TrimSpace(req.URL)
	}
	if req.has("auth_type") {
		cfg.AuthType = mcpmgr.AuthType(strings.TrimSpace(req.AuthType))
	}
	if req.has("oauth2_client_id") {
		cfg.OAuth2ClientID = strings.TrimSpace(req.OAuth2ClientID)
	}
	if req.has("oauth2_auth_url") {
		cfg.OAuth2AuthURL = strings.TrimSpace(req.OAuth2AuthURL)
	}
	if req.has("oauth2_token_url") {
		cfg.OAuth2TokenURL = strings.TrimSpace(req.OAuth2TokenURL)
	}
	if req.has("oauth2_scopes") {
		cfg.OAuth2Scopes = append([]string{}, req.OAuth2Scopes...)
	}
	if req.has("oauth2_callback_port") {
		cfg.OAuth2CallbackPort = req.OAuth2CallbackPort
	}
	if req.has("oauth2_callback_host") {
		cfg.OAuth2CallbackHost = strings.TrimSpace(req.OAuth2CallbackHost)
	}
	if req.has("oauth2_registration_url") {
		cfg.OAuth2RegistrationURL = strings.TrimSpace(req.OAuth2RegistrationURL)
	}
	if req.has("oauth2_device_auth_url") {
		cfg.OAuth2DeviceAuthURL = strings.TrimSpace(req.OAuth2DeviceAuthURL)
	}
	if req.has("disable_sse") {
		cfg.DisableSSE = req.DisableSSE
	}
	if req.has("prefer_bridge") {
		cfg.PreferBridge = req.PreferBridge
	}
	if req.has("enabled") {
		cfg.Enabled = req.Enabled
	}
	if req.has("auto_connect") {
		cfg.AutoConnect = req.AutoConnect
	}
}

func mergeEnv(existing, updates map[string]string) map[string]string {
	if len(updates) == 0 {
		return map[string]string{}
	}
	env := copyStringMap(existing)
	if env == nil {
		env = map[string]string{}
	}
	for key, value := range updates {
		env[key] = value
	}
	return env
}

func validateConfigForSave(cfg mcpmgr.ServerConfig) error {
	switch cfg.Transport {
	case mcpmgr.TransportStdio:
		if strings.TrimSpace(cfg.Command) == "" {
			return errors.New("command é obrigatório para transport stdio")
		}
	case mcpmgr.TransportSSE, mcpmgr.TransportStreamable:
		if strings.TrimSpace(cfg.URL) == "" {
			return fmt.Errorf("url é obrigatória para transport %s", cfg.Transport)
		}
	case "":
		if strings.TrimSpace(cfg.Command) == "" && strings.TrimSpace(cfg.URL) == "" {
			return errors.New("transport, command ou url é obrigatório para servidor MCP")
		}
	default:
		return fmt.Errorf("transport MCP inválido: %s", cfg.Transport)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return errors.New("name é obrigatório para servidor MCP")
	}
	return nil
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func deleteServer(mgr Manager, slug string) (tools.ToolResult, error) {
	if slug == "" {
		return tools.ToolResult{Content: "slug é obrigatório para remover servidor MCP", IsError: true}, nil
	}
	if err := mgr.DeleteConfig(slug); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao remover servidor MCP %q: %v", slug, err), IsError: true}, nil
	}
	return actionResult("deleted", slug), nil
}

func duplicateServer(mgr Manager, slug string) (tools.ToolResult, error) {
	if slug == "" {
		return tools.ToolResult{Content: "slug é obrigatório para duplicar servidor MCP", IsError: true}, nil
	}
	newSlug, err := mgr.DuplicateConfig(slug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao duplicar servidor MCP %q: %v", slug, err), IsError: true}, nil
	}
	payload := map[string]any{"action": "duplicated", "slug": slug, "new_slug": newSlug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload, Structured: true}, nil
}

func connectionAction(mgr Manager, slug, action, actionLabel string, fn func(string) error) (tools.ToolResult, error) {
	if slug == "" {
		return tools.ToolResult{Content: fmt.Sprintf("slug é obrigatório para %s servidor MCP", actionLabel), IsError: true}, nil
	}
	if result, ok := ensureServerLoaded(mgr, slug, actionLabel); !ok {
		return result, nil
	}
	if err := fn(slug); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao executar %s em servidor MCP %q: %v", actionLabel, slug, err), IsError: true}, nil
	}
	info, _ := serverInfoBySlug(mgr.List(), slug)
	payload := map[string]any{"action": action, "slug": slug, "server": toListServerResponse(info)}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"slug": slug, "action": action}, Structured: true}, nil
}

func ensureServerLoaded(mgr Manager, slug, actionLabel string) (tools.ToolResult, bool) {
	if _, ok := serverInfoBySlug(mgr.List(), slug); ok {
		return tools.ToolResult{}, true
	}
	if _, err := mgr.GetConfig(slug); err != nil {
		if isNotFound(err) {
			return tools.ToolResult{Content: fmt.Sprintf("servidor MCP %q não encontrado para %s", slug, actionLabel), IsError: true}, false
		}
		return tools.ToolResult{Content: fmt.Sprintf("erro ao ler servidor MCP %q para %s: %v", slug, actionLabel, err), IsError: true}, false
	}
	if err := mgr.LoadConfigs(); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao recarregar configurações MCP antes de %s servidor MCP %q: %v", actionLabel, slug, err), IsError: true}, false
	}
	if _, ok := serverInfoBySlug(mgr.List(), slug); !ok {
		return tools.ToolResult{Content: fmt.Sprintf("servidor MCP %q não foi carregado para %s", slug, actionLabel), IsError: true}, false
	}
	return tools.ToolResult{}, true
}

func reloadServers(mgr Manager) (tools.ToolResult, error) {
	if err := mgr.LoadConfigs(); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao recarregar configurações MCP: %v", err), IsError: true}, nil
	}
	result := listServers(mgr)
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["action"] = "reload"
	return result, nil
}

func logs(mgr Manager, slug string, limit int) (tools.ToolResult, error) {
	if slug == "" {
		return tools.ToolResult{Content: "slug é obrigatório para listar logs MCP", IsError: true}, nil
	}
	if limit <= 0 {
		limit = 100
	} else if limit > 500 {
		limit = 500
	}
	entries, err := mgr.GetLogs(slug, limit)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao listar logs MCP de %q: %v", slug, err), IsError: true}, nil
	}
	data, _ := json.Marshal(entries)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"slug": slug, "count": len(entries), "action": "logs"}, Structured: true}, nil
}

func actionResult(action, slug string) tools.ToolResult {
	payload := map[string]any{"action": action, "slug": slug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload, Structured: true}
}

type detailResponse struct {
	listServerResponse
	Config      safeServerConfig `json:"config"`
	EnvKeys     []string         `json:"env_keys,omitempty"`
	EnvRedacted bool             `json:"env_redacted,omitempty"`
}

type listServerResponse struct {
	ID            string                  `json:"id,omitempty"`
	Slug          string                  `json:"slug"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description,omitempty"`
	Transport     mcpmgr.TransportType    `json:"transport"`
	Status        mcpmgr.ConnectionStatus `json:"status"`
	Error         string                  `json:"error,omitempty"`
	ToolCount     int                     `json:"tool_count"`
	ResourceCount int                     `json:"resource_count"`
	PromptCount   int                     `json:"prompt_count"`
	Enabled       bool                    `json:"enabled"`
	AutoConnect   bool                    `json:"auto_connect"`
	ConnectedAt   string                  `json:"connected_at,omitempty"`
	LastPing      string                  `json:"last_ping,omitempty"`
	Command       string                  `json:"command,omitempty"`
	Args          []string                `json:"args,omitempty"`
	URL           string                  `json:"url,omitempty"`
}

type safeServerConfig struct {
	ID                    string               `json:"id,omitempty"`
	Slug                  string               `json:"slug,omitempty"`
	Name                  string               `json:"name"`
	Description           string               `json:"description,omitempty"`
	Transport             mcpmgr.TransportType `json:"transport"`
	Command               string               `json:"command,omitempty"`
	Args                  []string             `json:"args,omitempty"`
	URL                   string               `json:"url,omitempty"`
	AuthType              mcpmgr.AuthType      `json:"auth_type,omitempty"`
	OAuth2ClientID        string               `json:"oauth2_client_id,omitempty"`
	OAuth2AuthURL         string               `json:"oauth2_auth_url,omitempty"`
	OAuth2TokenURL        string               `json:"oauth2_token_url,omitempty"`
	OAuth2Scopes          []string             `json:"oauth2_scopes,omitempty"`
	OAuth2CallbackPort    int                  `json:"oauth2_callback_port,omitempty"`
	OAuth2CallbackHost    string               `json:"oauth2_callback_host,omitempty"`
	OAuth2RegistrationURL string               `json:"oauth2_registration_url,omitempty"`
	OAuth2DeviceAuthURL   string               `json:"oauth2_device_auth_url,omitempty"`
	DisableSSE            bool                 `json:"disable_sse,omitempty"`
	PreferBridge          bool                 `json:"prefer_bridge,omitempty"`
	Enabled               bool                 `json:"enabled"`
	AutoConnect           bool                 `json:"auto_connect"`
}

func safeConfig(cfg mcpmgr.ServerConfig) safeServerConfig {
	return safeServerConfig{
		ID:                    cfg.ID,
		Slug:                  cfg.Slug,
		Name:                  cfg.Name,
		Description:           cfg.Description,
		Transport:             cfg.Transport,
		Command:               cfg.Command,
		Args:                  append([]string{}, cfg.Args...),
		URL:                   cfg.URL,
		AuthType:              cfg.AuthType,
		OAuth2ClientID:        cfg.OAuth2ClientID,
		OAuth2AuthURL:         cfg.OAuth2AuthURL,
		OAuth2TokenURL:        cfg.OAuth2TokenURL,
		OAuth2Scopes:          append([]string{}, cfg.OAuth2Scopes...),
		OAuth2CallbackPort:    cfg.OAuth2CallbackPort,
		OAuth2CallbackHost:    cfg.OAuth2CallbackHost,
		OAuth2RegistrationURL: cfg.OAuth2RegistrationURL,
		OAuth2DeviceAuthURL:   cfg.OAuth2DeviceAuthURL,
		DisableSSE:            cfg.DisableSSE,
		PreferBridge:          cfg.PreferBridge,
		Enabled:               cfg.Enabled,
		AutoConnect:           cfg.AutoConnect,
	}
}

func toListServerResponse(server mcpmgr.ServerInfo) listServerResponse {
	return listServerResponse{
		ID:            server.ID,
		Slug:          server.Slug,
		Name:          server.Name,
		Description:   server.Description,
		Transport:     server.Transport,
		Status:        server.Status,
		Error:         server.Error,
		ToolCount:     server.ToolCount,
		ResourceCount: server.ResourceCount,
		PromptCount:   server.PromptCount,
		Enabled:       server.Enabled,
		AutoConnect:   server.AutoConnect,
		ConnectedAt:   server.ConnectedAt,
		LastPing:      server.LastPing,
		Command:       server.Command,
		Args:          append([]string{}, server.Args...),
		URL:           server.URL,
	}
}

func serverInfoBySlug(servers []mcpmgr.ServerInfo, slug string) (mcpmgr.ServerInfo, bool) {
	for _, server := range servers {
		if server.Slug == slug {
			return server, true
		}
	}
	return mcpmgr.ServerInfo{}, false
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mcpmgr "assistente/internal/mcp"

	"gorm.io/gorm"
)

type fakeManager struct {
	servers      []mcpmgr.ServerInfo
	configs      map[string]mcpmgr.ServerConfig
	logs         map[string][]mcpmgr.MCPServerLog
	savedSlug    string
	savedConfig  mcpmgr.ServerConfig
	deletedSlug  string
	duplicated   string
	logLimit     int
	connected    string
	disconnected string
	reconnected  string
	loadCalled   bool
	err          error
}

func (m *fakeManager) List() []mcpmgr.ServerInfo {
	return append([]mcpmgr.ServerInfo{}, m.servers...)
}

func (m *fakeManager) GetConfig(slug string) (*mcpmgr.ServerConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	cfg, ok := m.configs[slug]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &cfg, nil
}

func (m *fakeManager) SaveConfig(slug string, cfg mcpmgr.ServerConfig) error {
	if m.err != nil {
		return m.err
	}
	m.savedSlug = slug
	m.savedConfig = cfg
	if m.configs == nil {
		m.configs = map[string]mcpmgr.ServerConfig{}
	}
	cfg.Slug = slug
	m.configs[slug] = cfg
	return nil
}

func (m *fakeManager) DeleteConfig(slug string) error {
	if m.err != nil {
		return m.err
	}
	m.deletedSlug = slug
	delete(m.configs, slug)
	return nil
}

func (m *fakeManager) DuplicateConfig(slug string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.duplicated = slug
	return slug + "-copia", nil
}

func (m *fakeManager) Connect(slug string) error {
	m.connected = slug
	return m.err
}

func (m *fakeManager) Disconnect(slug string) error {
	m.disconnected = slug
	return m.err
}

func (m *fakeManager) Reconnect(slug string) error {
	m.reconnected = slug
	return m.err
}

func (m *fakeManager) LoadConfigs() error {
	m.loadCalled = true
	return m.err
}

func (m *fakeManager) GetLogs(slug string, limit int) ([]mcpmgr.MCPServerLog, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.logLimit = limit
	return append([]mcpmgr.MCPServerLog{}, m.logs[slug]...), nil
}

func TestToolListsServersAsStructuredJSON(t *testing.T) {
	mgr := &fakeManager{
		servers: []mcpmgr.ServerInfo{
			{
				Slug:          "zeta",
				Name:          "Zeta",
				Transport:     mcpmgr.TransportStdio,
				Status:        mcpmgr.StatusDisconnected,
				ToolCount:     1,
				Tools:         []mcpmgr.MCPToolInfo{{Name: "large-tool", Schema: json.RawMessage(`{"type":"object"}`)}},
				ResourceCount: 1,
				Resources:     []mcpmgr.MCPResourceInfo{{URI: "file://large", Name: "Large"}},
				PromptCount:   1,
				Prompts:       []mcpmgr.MCPPromptInfo{{Name: "prompt"}},
				Enabled:       true,
			},
			{Slug: "alpha", Name: "Alpha", Transport: mcpmgr.TransportStreamable, Status: mcpmgr.StatusConnected, Enabled: true},
		},
	}

	result, err := New(mgr).Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !result.Structured {
		t.Fatal("list result should be structured")
	}
	var payload []struct {
		Slug          string          `json:"slug"`
		ToolCount     int             `json:"toolCount"`
		ResourceCount int             `json:"resourceCount"`
		PromptCount   int             `json:"promptCount"`
		Tools         json.RawMessage `json:"tools"`
		Resources     json.RawMessage `json:"resources"`
		Prompts       json.RawMessage `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload) != 2 || payload[0].Slug != "alpha" || payload[1].Slug != "zeta" {
		t.Fatalf("servers not sorted by slug: %#v", payload)
	}
	if payload[1].ToolCount != 1 || payload[1].ResourceCount != 1 || payload[1].PromptCount != 1 {
		t.Fatalf("list should preserve item counts: %#v", payload[1])
	}
	if len(payload[1].Tools) != 0 || len(payload[1].Resources) != 0 || len(payload[1].Prompts) != 0 {
		t.Fatalf("list should omit large tools/resources/prompts arrays: %s", result.Content)
	}
}

func TestToolGetRedactsEnvValues(t *testing.T) {
	mgr := &fakeManager{
		servers: []mcpmgr.ServerInfo{{
			Slug:          "github",
			Name:          "GitHub",
			Transport:     mcpmgr.TransportStdio,
			ToolCount:     1,
			Tools:         []mcpmgr.MCPToolInfo{{Name: "large-tool", Schema: json.RawMessage(`{"type":"object"}`)}},
			ResourceCount: 1,
			Resources:     []mcpmgr.MCPResourceInfo{{URI: "file://large", Name: "Large"}},
			PromptCount:   1,
			Prompts:       []mcpmgr.MCPPromptInfo{{Name: "prompt"}},
			Enabled:       true,
		}},
		configs: map[string]mcpmgr.ServerConfig{
			"github": {
				Slug:        "github",
				UserID:      "user-123",
				Name:        "GitHub",
				Transport:   mcpmgr.TransportStdio,
				Command:     "npx",
				Args:        []string{"-y", "@modelcontextprotocol/server-github"},
				Env:         map[string]string{"GITHUB_TOKEN": "secret", "OTHER": "value"},
				Enabled:     true,
				AutoConnect: true,
			},
		},
	}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{"slug":"github"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if strings.Contains(result.Content, "secret") || strings.Contains(result.Content, "value") {
		t.Fatalf("env values leaked in response: %s", result.Content)
	}
	if strings.Contains(result.Content, "user-123") || strings.Contains(result.Content, "user_id") {
		t.Fatalf("user identifier leaked in response: %s", result.Content)
	}
	var payload struct {
		EnvKeys       []string        `json:"env_keys"`
		EnvRedacted   bool            `json:"env_redacted"`
		ToolCount     int             `json:"toolCount"`
		ResourceCount int             `json:"resourceCount"`
		PromptCount   int             `json:"promptCount"`
		Tools         json.RawMessage `json:"tools"`
		Resources     json.RawMessage `json:"resources"`
		Prompts       json.RawMessage `json:"prompts"`
		Config        struct {
			Command string            `json:"command"`
			Env     map[string]string `json:"env"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !payload.EnvRedacted || len(payload.EnvKeys) != 2 || payload.Config.Env != nil {
		t.Fatalf("unexpected env redaction payload: %#v", payload)
	}
	if payload.ToolCount != 1 || payload.ResourceCount != 1 || payload.PromptCount != 1 {
		t.Fatalf("get should preserve item counts: %#v", payload)
	}
	if len(payload.Tools) != 0 || len(payload.Resources) != 0 || len(payload.Prompts) != 0 {
		t.Fatalf("get should omit large tools/resources/prompts arrays: %s", result.Content)
	}
}

func TestToolGetDefaultsStatusWhenServerIsMissingFromRuntimeList(t *testing.T) {
	mgr := &fakeManager{
		configs: map[string]mcpmgr.ServerConfig{
			"runtime-missing": {
				Slug:        "runtime-missing",
				Name:        "Runtime Missing",
				Transport:   mcpmgr.TransportStdio,
				Command:     "npx",
				Enabled:     true,
				AutoConnect: true,
			},
		},
	}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{"slug":"runtime-missing"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	var payload struct {
		Status mcpmgr.ConnectionStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if payload.Status != mcpmgr.StatusDisconnected {
		t.Fatalf("fallback status should be disconnected, got %q in %s", payload.Status, result.Content)
	}
}

func TestToolCreateSavesConfigThroughManager(t *testing.T) {
	mgr := &fakeManager{}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{
		"action":"create",
		"slug":"local-files",
		"name":"Local Files",
		"transport":"stdio",
		"command":"npx",
		"args":["-y","server"],
		"env":{"TOKEN":"secret"},
		"enabled":true,
		"auto_connect":false
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.savedSlug != "local-files" {
		t.Fatalf("saved slug = %q", mgr.savedSlug)
	}
	if mgr.savedConfig.Name != "Local Files" ||
		mgr.savedConfig.Transport != mcpmgr.TransportStdio ||
		mgr.savedConfig.Command != "npx" ||
		mgr.savedConfig.Env["TOKEN"] != "secret" ||
		mgr.savedConfig.AutoConnect {
		t.Fatalf("unexpected saved config: %#v", mgr.savedConfig)
	}
}

func TestToolImplicitWriteCreatesWhenSlugDoesNotExist(t *testing.T) {
	mgr := &fakeManager{}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{
		"slug":"implicit",
		"name":"Implicit",
		"transport":"streamable",
		"url":"https://example.test/mcp"
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.savedSlug != "implicit" {
		t.Fatalf("implicit create did not save expected slug: %q", mgr.savedSlug)
	}
	if !mgr.savedConfig.Enabled || !mgr.savedConfig.AutoConnect {
		t.Fatalf("implicit create should default enabled and auto_connect: %#v", mgr.savedConfig)
	}
}

func TestToolUpdatePreservesExistingFieldsAndCanDisable(t *testing.T) {
	mgr := &fakeManager{
		configs: map[string]mcpmgr.ServerConfig{
			"remote": {
				Slug:        "remote",
				Name:        "Remote",
				Transport:   mcpmgr.TransportStreamable,
				URL:         "https://example.test/mcp",
				Enabled:     true,
				AutoConnect: true,
			},
		},
	}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{"slug":"remote","enabled":false}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.savedConfig.URL != "https://example.test/mcp" || mgr.savedConfig.Enabled {
		t.Fatalf("update did not preserve URL or disable server: %#v", mgr.savedConfig)
	}
}

func TestToolRejectsInvalidActionCombinations(t *testing.T) {
	result, err := New(&fakeManager{}).Execute(context.Background(), json.RawMessage(`{"action":"delete"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "slug é obrigatório") {
		t.Fatalf("unexpected delete validation result: %#v", result)
	}

	result, err = New(&fakeManager{}).Execute(context.Background(), json.RawMessage(`{"action":"connect"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "conectar servidor MCP") || strings.Contains(result.Content, "connected") {
		t.Fatalf("unexpected connect validation result: %#v", result)
	}

	result, err = New(&fakeManager{}).Execute(context.Background(), json.RawMessage(`{"action":"create","slug":"bad","name":"Bad","transport":"stdio"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "command é obrigatório") {
		t.Fatalf("unexpected create validation result: %#v", result)
	}
}

func TestToolCreateRejectsExistingSlug(t *testing.T) {
	mgr := &fakeManager{
		configs: map[string]mcpmgr.ServerConfig{
			"existing": {Slug: "existing", Name: "Existing", Transport: mcpmgr.TransportStdio, Command: "npx", Enabled: true},
		},
	}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{
		"action":"create",
		"slug":"existing",
		"name":"Replacement",
		"transport":"stdio",
		"command":"other"
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "já existe") {
		t.Fatalf("expected duplicate slug error, got %#v", result)
	}
	if mgr.savedSlug != "" {
		t.Fatalf("create should not save over existing config, saved %q", mgr.savedSlug)
	}
}

func TestToolUpdateDoesNotUpsertOnReadError(t *testing.T) {
	mgr := &fakeManager{err: errors.New("database offline")}

	result, err := New(mgr).Execute(context.Background(), json.RawMessage(`{
		"action":"update",
		"slug":"remote",
		"name":"Remote",
		"enabled":false
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "não encontrado para atualização") {
		t.Fatalf("expected update read error, got %#v", result)
	}
	if mgr.savedSlug != "" {
		t.Fatalf("update should not save after read error, saved %q", mgr.savedSlug)
	}
}

func TestToolRuntimeActionsAndLogs(t *testing.T) {
	now := time.Now()
	mgr := &fakeManager{
		servers: []mcpmgr.ServerInfo{{
			Slug:          "remote",
			Name:          "Remote",
			Status:        mcpmgr.StatusDisconnected,
			ToolCount:     1,
			Tools:         []mcpmgr.MCPToolInfo{{Name: "large-tool", Schema: json.RawMessage(`{"type":"object"}`)}},
			ResourceCount: 1,
			Resources:     []mcpmgr.MCPResourceInfo{{URI: "file://large", Name: "Large"}},
			PromptCount:   1,
			Prompts:       []mcpmgr.MCPPromptInfo{{Name: "prompt"}},
		}},
		logs: map[string][]mcpmgr.MCPServerLog{
			"remote": {{Slug: "remote", Type: "connected", Timestamp: now}},
		},
	}
	tool := New(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"connect","slug":"remote"}`))
	if err != nil {
		t.Fatalf("connect Execute: %v", err)
	}
	if result.IsError || mgr.connected != "remote" {
		t.Fatalf("unexpected connect result: %#v connected=%q", result, mgr.connected)
	}
	var connectPayload struct {
		Server struct {
			ToolCount     int             `json:"toolCount"`
			ResourceCount int             `json:"resourceCount"`
			PromptCount   int             `json:"promptCount"`
			Tools         json.RawMessage `json:"tools"`
			Resources     json.RawMessage `json:"resources"`
			Prompts       json.RawMessage `json:"prompts"`
		} `json:"server"`
	}
	if err := json.Unmarshal([]byte(result.Content), &connectPayload); err != nil {
		t.Fatalf("decode connect result: %v", err)
	}
	if connectPayload.Server.ToolCount != 1 || connectPayload.Server.ResourceCount != 1 || connectPayload.Server.PromptCount != 1 {
		t.Fatalf("connect should preserve item counts: %#v", connectPayload.Server)
	}
	if len(connectPayload.Server.Tools) != 0 || len(connectPayload.Server.Resources) != 0 || len(connectPayload.Server.Prompts) != 0 {
		t.Fatalf("connect should omit large tools/resources/prompts arrays: %s", result.Content)
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"reload"}`))
	if err != nil {
		t.Fatalf("reload Execute: %v", err)
	}
	if result.IsError || !mgr.loadCalled {
		t.Fatalf("unexpected reload result: %#v", result)
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"logs","slug":"remote","limit":999}`))
	if err != nil {
		t.Fatalf("logs Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected logs error: %s", result.Content)
	}
	var logs []mcpmgr.MCPServerLog
	if err := json.Unmarshal([]byte(result.Content), &logs); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Type != "connected" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
	if mgr.logLimit != 500 {
		t.Fatalf("logs limit should cap at 500, got %d", mgr.logLimit)
	}
}

func TestToolMetadataAndName(t *testing.T) {
	tool := New(&fakeManager{})
	if tool.Name() != ToolName {
		t.Fatalf("Name = %q", tool.Name())
	}
	meta := tool.CatalogMetadata()
	if meta.Category != "mcp" || meta.Class != "mcp_management" || meta.Package != "mcp" || meta.Risk != "write" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

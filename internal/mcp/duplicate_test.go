package mcp

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/toolcatalog"
	"assistente/internal/tools"
)

func TestDuplicateConfig(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })

	original := ServerConfig{
		Name:        "Servidor MCP",
		Description: "Descricao",
		Transport:   TransportStdio,
		Command:     "node",
		Args:        []string{"server.js"},
		Enabled:     true,
		AutoConnect: false,
	}

	if err := mgr.SaveConfig("server", original); err != nil {
		t.Fatalf("SaveConfig falhou: %v", err)
	}

	newSlug, err := mgr.DuplicateConfig("server")
	if err != nil {
		t.Fatalf("DuplicateConfig falhou: %v", err)
	}

	if newSlug != "server-copia" {
		t.Fatalf("slug duplicado inesperado: %s", newSlug)
	}

	copied, err := mgr.GetConfig(newSlug)
	if err != nil {
		t.Fatalf("GetConfig copia falhou: %v", err)
	}

	if copied.Name != "Servidor MCP (Cópia)" {
		t.Fatalf("nome da copia inesperado: %s", copied.Name)
	}

	if copied.Transport != original.Transport || copied.Command != original.Command {
		t.Fatalf("config da copia nao preservou campos principais")
	}
}

func TestSaveConfigInitializesWorkspaceRoots(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })
	if err := mgr.SetWorkspaceRoots([]Root{{URI: "file:///workspace", Name: "workspace"}}); err != nil {
		t.Fatalf("SetWorkspaceRoots falhou: %v", err)
	}

	if err := mgr.SaveConfig("server", ServerConfig{
		Name:        "Servidor MCP",
		Transport:   TransportStdio,
		Command:     "node",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig falhou: %v", err)
	}

	mgr.mu.RLock()
	status := mgr.servers["server"]
	mgr.mu.RUnlock()
	if status == nil || len(status.Roots) != 1 || status.Roots[0].URI != "file:///workspace" {
		t.Fatalf("roots do server recém-criado = %#v", status)
	}
}

func TestSaveConfigUpdatesWorkspaceRoots(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })
	if err := mgr.SaveConfig("server", ServerConfig{
		Name:        "Servidor MCP",
		Transport:   TransportStdio,
		Command:     "node",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig inicial falhou: %v", err)
	}
	if err := mgr.SetWorkspaceRoots([]Root{{URI: "file:///workspace-updated", Name: "workspace"}}); err != nil {
		t.Fatalf("SetWorkspaceRoots falhou: %v", err)
	}

	if err := mgr.SaveConfig("server", ServerConfig{
		Name:        "Servidor MCP Atualizado",
		Transport:   TransportStdio,
		Command:     "node",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig update falhou: %v", err)
	}

	mgr.mu.RLock()
	status := mgr.servers["server"]
	mgr.mu.RUnlock()
	if status == nil || len(status.Roots) != 1 || status.Roots[0].URI != "file:///workspace-updated" {
		t.Fatalf("roots do server atualizado = %#v", status)
	}
}

func TestSaveConfigNormalizesSlugForRuntimeState(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })

	if err := mgr.SaveConfig(" server ", ServerConfig{
		Name:        "Servidor MCP",
		Transport:   TransportStdio,
		Command:     "node",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig falhou: %v", err)
	}

	mgr.mu.RLock()
	_, hasTrimmed := mgr.servers["server"]
	_, hasRaw := mgr.servers[" server "]
	mgr.mu.RUnlock()
	if !hasTrimmed || hasRaw {
		t.Fatalf("estado runtime não normalizou slug: trimmed=%v raw=%v", hasTrimmed, hasRaw)
	}
	if _, err := repo.GetServer(ctx, "server"); err != nil {
		t.Fatalf("repo não encontrou slug normalizado: %v", err)
	}
}

func TestDeleteConfigNormalizesSlugForRuntimeState(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })
	if err := mgr.SaveConfig("server", ServerConfig{
		Name:        "Servidor MCP",
		Transport:   TransportStdio,
		Command:     "node",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig falhou: %v", err)
	}

	if err := mgr.DeleteConfig(" server "); err != nil {
		t.Fatalf("DeleteConfig falhou: %v", err)
	}

	mgr.mu.RLock()
	_, exists := mgr.servers["server"]
	mgr.mu.RUnlock()
	if exists {
		t.Fatal("DeleteConfig não removeu estado runtime com slug normalizado")
	}
}

func TestDisconnectMarksCatalogToolsUnavailable(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	catalog := toolcatalog.NewService(toolcatalog.NewDBRepository(repo.db))
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetCatalog(catalog)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })
	if err := mgr.SaveConfig("server", ServerConfig{
		Name:        "Servidor MCP",
		Transport:   TransportStreamable,
		URL:         "https://mcp.example/mcp",
		Enabled:     true,
		AutoConnect: false,
	}); err != nil {
		t.Fatalf("SaveConfig falhou: %v", err)
	}
	cfg, err := mgr.GetConfig("server")
	if err != nil {
		t.Fatalf("GetConfig falhou: %v", err)
	}
	if err := catalog.UpsertTool(ctx, &tools.ToolCatalogEntry{
		Name:               "mcp_server__do",
		DisplayName:        "do",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        cfg.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}); err != nil {
		t.Fatalf("UpsertTool falhou: %v", err)
	}
	mgr.mu.Lock()
	mgr.connections["server"] = &serverConnection{}
	if status := mgr.servers["server"]; status != nil {
		status.Status = StatusConnected
	}
	mgr.mu.Unlock()

	if err := mgr.Disconnect("server"); err != nil {
		t.Fatalf("Disconnect falhou: %v", err)
	}

	entries, err := catalog.ListTools(ctx, tools.ToolCatalogFilter{IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("ListTools falhou: %v", err)
	}
	if len(entries) != 1 || entries[0].AvailabilityStatus != tools.ToolAvailabilityUnavailable {
		t.Fatalf("tool deveria ficar unavailable após disconnect: %#v", entries)
	}
}

func TestGetConfigMissingReturnsDomainError(t *testing.T) {
	mgr := NewManager(tools.NewRegistry(), credentials.NewManager(nil), func(string, any) {})
	repo, _, _ := setupRepositoryTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })

	_, err := mgr.GetConfig("missing")
	if err == nil {
		t.Fatal("esperava erro para servidor ausente")
	}
	if !strings.Contains(err.Error(), "servidor MCP 'missing' não encontrado") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

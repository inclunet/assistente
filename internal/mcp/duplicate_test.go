package mcp

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
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

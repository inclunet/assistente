package mcp

import (
	"context"
	"testing"

	"assistente/internal/database"
)

func TestLoadConfigsLoadsOnlyDatabaseConfigs(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	if err := repo.SaveServer(userA, &ServerConfig{
		Slug:        "github",
		Name:        "GitHub",
		Transport:   TransportStreamable,
		URL:         "https://github.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	m := NewManager(nil, nil, nil)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return userA })

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs: %v", err)
	}

	if got := m.List(); len(got) != 1 || got[0].Slug != "github" {
		t.Fatalf("manager list after load = %#v", got)
	}
}

func TestLoadConfigsWithRepositoryWaitsForAuthenticatedUser(t *testing.T) {
	repo, _, _ := setupRepositoryTest(t)
	m := NewManager(nil, nil, nil)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return context.Background() })

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs sem user deveria ser noop, got %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("manager sem user não deveria carregar servers, got %#v", got)
	}

	if _, err := repo.GetServer(database.WithUserID(context.Background(), "user-a"), "github"); err == nil {
		t.Fatal("repo não deveria conter server")
	}
}

func TestLoadConfigsPreservesExistingRuntimeState(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	if err := repo.SaveServer(userA, &ServerConfig{
		Slug:        "github",
		Name:        "GitHub",
		Transport:   TransportStreamable,
		URL:         "https://github.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}
	m := NewManager(nil, nil, nil)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return userA })
	m.servers["github"] = &ServerStatus{
		Slug:   "github",
		Status: StatusConnected,
		Tools:  []MCPToolInfo{{Name: "search", FullName: "mcp_github__search"}},
	}

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs: %v", err)
	}

	m.mu.RLock()
	status := m.servers["github"]
	m.mu.RUnlock()
	if status == nil {
		t.Fatal("github server missing after LoadConfigs")
	}
	if status.Status != StatusConnected {
		t.Fatalf("status = %q, want connected", status.Status)
	}
	if len(status.Tools) != 1 || status.Tools[0].FullName != "mcp_github__search" {
		t.Fatalf("tools not preserved: %#v", status.Tools)
	}
	if status.Config.URL != "https://github.example/mcp" {
		t.Fatalf("config not refreshed from DB: %#v", status.Config)
	}
}

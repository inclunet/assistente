package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

func TestLoadConfigsImportsFilesystemConfigsToRepositoryWithoutTouchingFiles(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	legacyDir := filepath.Join(t.TempDir(), "mcp")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	data := []byte(`{
		"name": "GitHub",
		"transport": "streamable",
		"url": "https://github.example/mcp",
		"enabled": true,
		"auto_connect": true
	}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "github.json"), data, 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	m := NewManager(nil, nil, nil)
	m.resolver = configdir.NewResolverWithBase(legacyDir)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return userA })

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs: %v", err)
	}

	cfg, err := repo.GetServer(userA, "github")
	if err != nil {
		t.Fatalf("GetServer imported: %v", err)
	}
	if cfg.UserID != "user-a" {
		t.Fatalf("UserID = %q, want user-a", cfg.UserID)
	}
	if cfg.URL != "https://github.example/mcp" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "github.json")); err != nil {
		t.Fatalf("legacy file should remain untouched: %v", err)
	}
	if got := m.List(); len(got) != 1 || got[0].Slug != "github" {
		t.Fatalf("manager list after import = %#v", got)
	}
}

func TestLoadConfigsLegacyImportIsIdempotentAndDoesNotOverwriteDB(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	legacyDir := filepath.Join(t.TempDir(), "mcp")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	data := []byte(`{"name":"Legacy GitHub","transport":"streamable","url":"https://legacy.example/mcp","enabled":true,"auto_connect":true}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "github.json"), data, 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	existing := &ServerConfig{
		Slug:        "github",
		Name:        "DB GitHub",
		Transport:   TransportStreamable,
		URL:         "https://db.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}
	if err := repo.SaveServer(userA, existing); err != nil {
		t.Fatalf("SaveServer existing: %v", err)
	}

	m := NewManager(nil, nil, nil)
	m.resolver = configdir.NewResolverWithBase(legacyDir)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return userA })

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs first: %v", err)
	}
	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs second: %v", err)
	}
	cfg, err := repo.GetServer(userA, "github")
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if cfg.Name != "DB GitHub" || cfg.URL != "https://db.example/mcp" {
		t.Fatalf("legacy import overwrote DB config: %#v", cfg)
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

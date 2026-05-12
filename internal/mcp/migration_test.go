package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

func TestLoadConfigsMigratesFilesystemConfigsToRepository(t *testing.T) {
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
		t.Fatalf("GetServer migrated: %v", err)
	}
	if cfg.UserID != "user-a" {
		t.Fatalf("UserID = %q, want user-a", cfg.UserID)
	}
	if cfg.URL != "https://github.example/mcp" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if _, err := os.Stat(legacyDir + ".migrated"); err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy dir should be renamed, stat err=%v", err)
	}
	if got := m.List(); len(got) != 1 || got[0].Slug != "github" {
		t.Fatalf("manager list after migration = %#v", got)
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

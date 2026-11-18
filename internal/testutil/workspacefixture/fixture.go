// Package workspacefixture constrói managers com índice SQLite privado para
// testes de consumidores. Não inicializa serviços, credenciais ou agentes.
package workspacefixture

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/sqlite"
	"assistente/internal/workspace"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenIndex(t testing.TB, home string) *gorm.DB {
	t.Helper()
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(database.SQLiteDSN(filepath.Join(home, "workspace-index-test.db"))), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.DesktopWorkspaceEntry{}, &database.DesktopWorkspaceState{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func New(t testing.TB, home string) *workspace.Manager {
	t.Helper()
	db := OpenIndex(t, home)
	return workspace.NewManager(home, func() *gorm.DB { return db })
}

// Create prepara dados de teste pelo mesmo caminho durável usado pelo app.
func Create(t testing.TB, manager *workspace.Manager, name, directory string) (*workspace.Workspace, error) {
	t.Helper()
	snapshot, err := manager.CommandSnapshot()
	if err != nil {
		return nil, err
	}
	return manager.CreateForCommand(context.Background(), snapshot, name, directory)
}

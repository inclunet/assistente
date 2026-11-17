// Package authorizationfixture fornece bancos privados para testes de
// autorização. Não inicia aplicativo, agentes ACP nem processos auxiliares.
package authorizationfixture

import (
	"context"
	"path/filepath"
	"testing"

	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/sqlite"
	"assistente/internal/trustscope"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Context() context.Context {
	return database.WithUserID(context.Background(), "authorization-test-user")
}

func Open(t testing.TB, dir string) (*authorization.Repository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(database.SQLiteDSN(filepath.Join(dir, "authorizations.db"))), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	if err := db.AutoMigrate(&database.AuthorizationRule{}, &database.AuthorizationImport{}, &database.AuthorizationPolicy{}, &database.AuthorizationPolicyBinding{}); err != nil {
		t.Fatal(err)
	}
	return authorization.NewRepository(func() *gorm.DB { return db }), db
}

func Options(t testing.TB, home, work string) trustscope.Options {
	t.Helper()
	dir := home
	if dir == "" {
		dir = work
	}
	if dir == "" {
		dir = t.TempDir()
	}
	repo, _ := Open(t, dir)
	return trustscope.Options{Repository: repo, HomeDir: func() string { return home }, WorkDir: func() string { return work }, MayImport: func(context.Context) bool { return true }}
}

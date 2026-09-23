package app

import (
	"context"
	"testing"

	"assistente/internal/commandinstance"
	"assistente/internal/database"
)

func TestBeforeCommandDatabaseResetDrainsCoreAndLeavesAppTerminal(t *testing.T) {
	a := NewApp()
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	called := make(chan struct{}, 1)
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error {
		called <- struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := a.beforeCommandDatabaseReset(); err != nil {
		t.Fatalf("beforeCommandDatabaseReset() error = %v", err)
	}
	select {
	case <-called:
	default:
		t.Fatal("o callback de drain não foi executado")
	}
	if _, err := core.CaptureAuthenticated(context.Background(), func(context.Context) (string, string, error) {
		t.Fatal("core terminal consultou autenticação")
		return "", "", nil
	}); err == nil {
		t.Fatal("core terminal aceitou nova captura")
	}
	if _, err := a.commandSecurityService(); err != nil {
		t.Fatalf("sync.Once perdeu o core após reset: %v", err)
	}
}

func TestBeforeCommandDatabaseResetReleasesReadyProductInstance(t *testing.T) {
	a := readyCommandProduct(t)
	var rows []struct {
		Name string `gorm:"column:name"`
		File string `gorm:"column:file"`
	}
	if err := database.DB().Raw("PRAGMA database_list").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	var databasePath string
	for _, row := range rows {
		if row.Name == "main" {
			databasePath = row.File
			break
		}
	}
	if databasePath == "" {
		t.Fatal("fixture não expôs o caminho do banco principal")
	}

	if err := a.beforeCommandDatabaseReset(); err != nil {
		t.Fatalf("beforeCommandDatabaseReset() error = %v", err)
	}
	lease, err := commandinstance.Acquire(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("lease não foi liberada após drain: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

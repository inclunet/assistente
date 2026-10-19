package app

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// A falha atinge o preflight real do lifecycle, depois da retirada do produto
// anterior. Liberar o armazenamento não emite outro evento MCP: só o worker
// produtivo pode retomar e publicar o novo mapa.
func TestCommandCatalogStorageFailureRecoversWithoutAnotherMCPEvent(t *testing.T) {
	a := commandJobPublicationApp(t)
	old := a.commandProduct.Load()
	before, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	logs := captureCommandLoadLogs(t)
	db := database.DB()
	var busy atomic.Bool
	busy.Store(true)
	failed := make(chan struct{})
	var once sync.Once
	const hook = "test:catalog_storage_busy"
	if err := db.Callback().Row().Before("gorm:row").Register(hook, func(tx *gorm.DB) {
		if busy.Load() && strings.Contains(tx.Statement.SQL.String(), "AS invocations") {
			_ = tx.AddError(commandLoadCodedError{})
			once.Do(func() { close(failed) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.stopCommandCatalogRecovery(ctx)
		_ = db.Callback().Row().Remove(hook)
	})
	a.requestCommandCatalogRecovery()
	a.commandCatalogRecoveryMu.Lock()
	done := a.commandCatalogRecoveryDone
	a.commandCatalogRecoveryMu.Unlock()
	if done == nil {
		t.Fatal("evento não iniciou recuperação")
	}
	select {
	case <-failed:
	case <-time.After(10 * time.Second):
		t.Fatal("hot-swap não alcançou falha injetada no preflight")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("falha de bootstrap manteve mapa autorizado")
	}
	busy.Store(false)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("armazenamento liberado não recuperou comandos sem novo evento")
	}
	assertCommandCatalogReadyAfterRecovery(t, a, old, before.Generation)
	var sawFailure, sawSuccess bool
	for _, record := range commandLoadRecords(t, logs) {
		if record["operation"] != "command_catalog_hot_swap" {
			continue
		}
		if record["status"] == "failed" && record["error_code"] == float64(517) {
			sawFailure = true
		}
		if record["status"] == "succeeded" {
			sawSuccess = true
		}
	}
	if !sawFailure || !sawSuccess || strings.Contains(logs.text(), "SECRET") {
		t.Fatalf("diagnóstico não preservou falha tipada/recuperação segura: failure=%v success=%v", sawFailure, sawSuccess)
	}
}

package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
	"gorm.io/gorm"
)

// O lock é de uma conexão SQLite real; o callback apenas detecta BUSY.
func commandBootstrapBusyWriter(t *testing.T, operation func(context.Context) error) {
	t.Helper()
	db := database.DB()
	assertCommandBootstrapTemporaryDatabase(t, db)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// PRAGMA is per connection: configure every fixture connection before
	// returning it to the pool, rather than inheriting the driver's timeout.
	var connections []*sql.Conn
	for i := 0; i < 4; i++ {
		conn, err := pool.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=1"); err != nil {
			t.Fatal(err)
		}
	}
	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writer, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("fechar conexão writer: %v", err)
		}
	})
	if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	writerTransactionActive := true
	t.Cleanup(func() {
		if !writerTransactionActive {
			return
		}
		if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
			t.Errorf("reverter transação writer: %v", err)
			return
		}
		writerTransactionActive = false
	})
	busy := make(chan struct{})
	var once sync.Once
	observe := func(tx *gorm.DB) {
		if database.IsSQLiteBusyError(tx.Error) {
			once.Do(func() { close(busy) })
		}
	}
	const hook = "test:command_bootstrap_busy"
	if err := db.Callback().Raw().After("gorm:raw").Register(hook, observe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Raw().Remove(hook); err != nil {
			t.Errorf("remover callback raw: %v", err)
		}
	})
	if err := db.Callback().Update().After("gorm:update").Register(hook, observe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Update().Remove(hook); err != nil {
			t.Errorf("remover callback update: %v", err)
		}
	})
	if err := db.Callback().Create().After("gorm:create").Register(hook, observe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(hook); err != nil {
			t.Errorf("remover callback create: %v", err)
		}
	})
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() { defer close(exited); done <- operation(ctx) }()
	defer func() { cancel(); <-exited }()
	select {
	case <-busy:
	case err := <-done:
		t.Fatalf("operação terminou sem encontrar lock real: %v", err)
	case <-ctx.Done():
		t.Fatal("operação não encontrou lock real")
	}
	if _, err := writer.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatal(err)
	}
	writerTransactionActive = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("não recuperou após liberar writer: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("operação não retomou")
	}
}

// Both callers install appLifecycleProductMountFixture's t.TempDir database.
// Refuse to acquire a writer or change journal mode on any other database.
func assertCommandBootstrapTemporaryDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	var databases []struct {
		Name string
		File string
	}
	if err := db.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range databases {
		if entry.Name != "main" {
			continue
		}
		rel, err := filepath.Rel(filepath.Dir(t.TempDir()), entry.File)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) && filepath.Base(entry.File) == "lifecycle-mount.db" {
			return
		}
		t.Fatalf("refusing non-fixture database: %q", entry.File)
	}
	t.Fatal("missing temporary fixture database")
}

func TestCommandBootstrapBusyEnsuresScopeThenPublishesPaletteAndKeyboard(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	if err := ResetCommandLifecycle(ctx, a, "busy_restart"); err != nil {
		t.Fatal(err)
	}
	commandBootstrapBusyWriter(t, a.rebuildCommandLifecyclePersistedConfiguration)
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		t.Fatal(err)
	}
	commandBootstrapAssertReady(t, a)
}

func TestCommandBootstrapBusyRestoresPersistentClaimWithoutDuplication(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyC","modifiers":["Alt"]}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := database.DB().Table("command_layer_activation_state").Count(&before).Error; err != nil || before == 0 {
		t.Fatalf("claims antes=%d err=%v", before, err)
	}
	ctx := context.Background()
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	commandBootstrapBusyWriter(t, a.restoreCommandLifecyclePersistentClaims)
	a.bootstrapCommandLifecycleAfterUnlock(ctx)
	commandBootstrapAssertReady(t, a)
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	active := false
	for _, item := range settings.Layers {
		if item.ID == layer {
			active = item.Active
		}
	}
	if !active {
		t.Fatal("camada persistente não foi restaurada")
	}
	var after int64
	if err := database.DB().Table("command_layer_activation_state").Count(&after).Error; err != nil || after != before {
		t.Fatalf("claims duplicadas: antes=%d depois=%d err=%v", before, after, err)
	}
}

func commandBootstrapAssertReady(t *testing.T, a *App) {
	t.Helper()
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"})
	if err != nil || len(items) != 150 {
		t.Fatalf("catálogo=%+v err=%v", items, err)
	}
	layerActions := 0
	for _, item := range items {
		if isCommandLayerAction(item.ID) {
			// Estes fixtures não configuram bindings de ações de camada.
			// Recuperar o bootstrap publica o catálogo, não inventa um alvo.
			layerActions++
			if item.Available || item.AvailabilityStatus != "available" || item.ReadinessReason == "" {
				t.Fatalf("ação de camada sem binding após retry: %+v", item)
			}
			continue
		}
		if item.ID == commandGlobalJobID || item.ID == commandGlobalVoiceID {
			if item.Available || item.ReadinessReason == "" || len(item.AllowedSources) != 1 || item.AllowedSources[0] != "keyboard.global" {
				t.Fatalf("global exposto como comando de paleta: %+v", item)
			}
			continue
		}
		if !item.Available {
			if item.ID == commandConversationClearID || item.ID == commandTerminalInterruptID || isChatActionCommand(item.ID) || isChatMessageCommand(item.ID) || isPageMutationCommand(item.ID) || item.ID == commandTerminalSessionCreateID || item.ID == commandTerminalSessionCloseID {
				if item.ReadinessReason == "" {
					t.Fatal("clear indisponível sem motivo")
				}
				continue
			}
			if item.ID == commandWorkspaceTabTerminalCreateID {
				if item.AvailabilityStatus != "available" || item.ReadinessReason == "" {
					t.Fatalf("terminal indisponível sem readiness explícita: %+v", item)
				}
				continue
			}
			t.Fatalf("indisponível após retry: %+v", item)
		}
	}
	if layerActions != 3 {
		t.Fatalf("ações de camada ausentes do catálogo: %d", layerActions)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyC", Modifiers: []string{"Alt"}}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != "navigation.settings.open" || binding.Handler != "local_ui" {
		t.Fatalf("Alt+C=%+v", binding)
	}
	if reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false); err == nil || reservation != nil {
		t.Fatalf("Alt+C entrou no ledger UI: reservation=%+v err=%v", reservation, err)
	}
}

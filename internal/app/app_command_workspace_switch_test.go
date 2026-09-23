package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func wireCommandWorkspaceForTest(a *App) *testEmitter {
	emitter := &testEmitter{}
	a.emitter = emitter
	a.configureWorkspaceController()
	return emitter
}

func TestCommandWorkspaceSwitchRebuildsExactConfigurationAndRetiresOldRuntime(t *testing.T) {
	a := readyCommandProduct(t)
	emitter := wireCommandWorkspaceForTest(a)
	previous := a.commandProduct.Load()
	firstID := previous.workspaceID
	second, err := a.workspaceMgr.Create("segundo")
	if err != nil {
		t.Fatal(err)
	}
	delta := installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	if err := database.DB().Model(&commandconfig.Binding{}).Where("id = ?", delta).Update("workspace_id", firstID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`)); err != nil || result.Status != "suppressed" {
		t.Fatalf("delta do primeiro workspace: %+v err=%v", result, err)
	}
	observedReload := false
	hook := "test:workspace_switch_unpublishes_before_reload"
	if err := database.DB().Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table != "command_config_generations" {
			return
		}
		observedReload = true
		snapshot, err := CommandLifecycleSnapshot(a)
		if err != nil || snapshot.Published || snapshot.State == commandruntime.StateReady {
			t.Errorf("recarregou configuração com mapa anterior ainda publicado: %+v err=%v", snapshot, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.DB().Callback().Query().Remove(hook) })
	if _, err := a.workspaceCtrl.SwitchWorkspace(second.ID); err != nil {
		t.Fatal(err)
	}
	if !observedReload {
		t.Fatal("reconstrução não consultou configuração persistida")
	}
	if err := database.DB().Callback().Query().Remove(hook); err != nil {
		t.Fatal(err)
	}
	current := a.commandProduct.Load()
	if current == previous || current.workspaceID != second.ID {
		t.Fatal("produto não foi recomposto para o workspace atual")
	}
	select {
	case <-previous.done:
	default:
		t.Fatal("runtime anterior não foi drenado")
	}
	if _, err := previous.execute(context.Background(), productRemountCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("runtime antigo não recusou: %v", err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err != nil || result.Status != "succeeded" {
		t.Fatalf("segundo workspace herdou delta anterior: %+v err=%v", result, err)
	}
	if _, err := a.workspaceCtrl.SwitchWorkspace(firstID); err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err != nil || result.Status != "suppressed" {
		t.Fatalf("retorno ao primeiro perdeu o delta: %+v err=%v", result, err)
	}
	if a.commandProduct.Load() == previous {
		t.Fatal("retorno ABA ressuscitou runtime antigo")
	}
	if len(emitter.find("workspace:switched")) != 2 {
		t.Fatal("eventos de troca não preservados")
	}
}

func TestCommandWorkspaceSwitchStorageFailureLeavesNoOldMapAndCanRetry(t *testing.T) {
	a := readyCommandProduct(t)
	wireCommandWorkspaceForTest(a)
	previous := a.commandProduct.Load()
	second, err := a.workspaceMgr.Create("segundo")
	if err != nil {
		t.Fatal(err)
	}
	hook := "test:workspace_switch_storage_failure"
	if err := database.DB().Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_config_generations" {
			tx.AddError(errors.New("storage unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.DB().Callback().Query().Remove(hook) })
	if ws, err := a.workspaceCtrl.SwitchWorkspace(second.ID); err != nil || ws.ID != second.ID {
		t.Fatalf("erro dos comandos desfez troca já confirmada: ws=%+v err=%v", ws, err)
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.Published || snapshot.State == commandruntime.StateReady {
		t.Fatalf("publicação antiga sobreviveu à falha: %+v err=%v", snapshot, err)
	}
	if _, _, err := a.commandHost.UserConfiguration(context.Background(), previous.principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa antigo ainda disponível: %v", err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err == nil && result.Status == "succeeded" {
		t.Fatal("executou com recarga falha")
	}
	if err := database.DB().Callback().Query().Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceCtrl.SwitchWorkspace(second.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err != nil || result.Status != "succeeded" {
		t.Fatalf("retry não recuperou composição: %+v err=%v", result, err)
	}
}

func TestCommandWorkspaceSwitchInvalidTargetPreservesReadyRuntime(t *testing.T) {
	a := readyCommandProduct(t)
	emitter := wireCommandWorkspaceForTest(a)
	previous := a.commandProduct.Load()
	if _, err := a.workspaceCtrl.SwitchWorkspace("missing-workspace"); err == nil {
		t.Fatal("workspace inexistente aceito")
	}
	if a.commandProduct.Load() != previous || len(emitter.find("workspace:switched")) != 0 {
		t.Fatal("troca recusada alterou runtime ou emitiu evento")
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err != nil || result.Status != "succeeded" {
		t.Fatalf("troca recusada desabilitou comandos: %+v err=%v", result, err)
	}
}

func TestCommandWorkspaceSwitchRetiresPendingUIHandoff(t *testing.T) {
	a := readyCommandProduct(t)
	wireCommandWorkspaceForTest(a)
	old := beginAuditedTabCreate(t, a)
	waitCommandUIAdmission(t, a, old)
	second, err := a.workspaceMgr.Create("segundo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceCtrl.SwitchWorkspace(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TakeUICommand(old.Ticket); err == nil {
		t.Fatal("ticket de workspace anterior entregou efeito visual")
	}
	assertUICommandLedgerStatus(t, old.InvocationID, "outcome_unknown")
	fresh := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, fresh.Ticket)
	if err := a.CommitWorkspaceTabCommand(fresh.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand create após switch: %v", err)
	}
	if result := getUIResultEventually(t, a, fresh.Ticket); result.Status != "succeeded" {
		t.Fatalf("handoff novo falhou: %+v", result)
	}
}

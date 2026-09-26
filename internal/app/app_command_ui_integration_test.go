package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"github.com/google/uuid"
)

const uiCommandTestTimeout = 3 * time.Second

func beginUICommand(t *testing.T, a *App, commandID string) commandui.Reservation {
	t.Helper()
	reservation, err := a.BeginUICommand(commandID)
	if err != nil {
		t.Fatalf("BeginUICommand: %v", err)
	}
	if reservation.Ticket == "" || reservation.InvocationID == "" || reservation.CommandID != commandID {
		t.Fatal("BeginUICommand retornou reservation incompleta")
	}
	return reservation
}

func takeUICommandFor(t *testing.T, a *App, ticket, commandID string) commandui.Handoff {
	t.Helper()
	resultCh := make(chan struct {
		handoff commandui.Handoff
		err     error
	}, 1)
	go func() {
		handoff, err := a.TakeUICommand(ticket)
		resultCh <- struct {
			handoff commandui.Handoff
			err     error
		}{handoff: handoff, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("TakeUICommand falhou: %v", result.err)
		}
		if result.handoff.Ticket != ticket || result.handoff.InvocationID == "" || result.handoff.CommandID != commandID || result.handoff.HandoffID == "" {
			t.Fatal("TakeUICommand retornou handoff incompleto")
		}
		return result.handoff
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("TakeUICommand não liberou o handoff")
		return commandui.Handoff{}
	}
}

func beginAuditedTabCreate(t *testing.T, a *App) commandui.Reservation {
	t.Helper()
	return beginUICommand(t, a, commandWorkspaceTabChatCreateID)
}

func takeAuditedTabCreate(t *testing.T, a *App, ticket string) commandui.Handoff {
	t.Helper()
	return takeUICommandFor(t, a, ticket, commandWorkspaceTabChatCreateID)
}

func getUIResultEventually(t *testing.T, a *App, ticket string) CommandExecutionResult {
	t.Helper()
	resultCh := make(chan struct {
		result CommandExecutionResult
		err    error
	}, 1)
	go func() {
		result, err := a.GetUICommandResult(ticket)
		resultCh <- struct {
			result CommandExecutionResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("GetUICommandResult falhou: %v", result.err)
		}
		return result.result
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("GetUICommandResult não concluiu")
		return CommandExecutionResult{}
	}
}

func assertGetUIStillPending(t *testing.T, a *App, ticket string) {
	t.Helper()
	resultCh := make(chan struct{}, 1)
	go func() {
		_, _ = a.GetUICommandResult(ticket)
		resultCh <- struct{}{}
	}()
	select {
	case <-resultCh:
		t.Fatal("GetUICommandResult retornou antes de CompleteUICommand")
	case <-time.After(75 * time.Millisecond):
	}
}

func assertUICommandLedgerStatus(t *testing.T, invocationID, want string) {
	t.Helper()
	var ledgerRows []struct{ Status commandledger.Status }
	if err := database.DB().Table("command_idempotency_keys").Select("status").Where("invocation_id = ?", invocationID).Find(&ledgerRows).Error; err != nil {
		t.Fatalf("ler ledger: %v", err)
	}
	if len(ledgerRows) != 1 || string(ledgerRows[0].Status) != want {
		t.Fatalf("ledger status=%v, esperado exatamente %q", ledgerRows, want)
	}
	var auditRows []struct{ Status commandledger.Status }
	if err := database.DB().Table("command_invocations").Select("status").Where("invocation_id = ?", invocationID).Find(&auditRows).Error; err != nil {
		t.Fatalf("ler auditoria: %v", err)
	}
	if len(auditRows) != 1 || string(auditRows[0].Status) != want {
		t.Fatalf("auditoria status=%v, esperado exatamente %q", auditRows, want)
	}
}

func commandInvocationCount(t *testing.T, commandID string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_invocations").Where("command_id = ?", commandID).Count(&count).Error; err != nil {
		t.Fatalf("contar invocações de %q: %v", commandID, err)
	}
	return count
}

func commandLedgerCount(t *testing.T, commandID string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Raw(`
		SELECT COUNT(*)
		FROM command_idempotency_keys AS ledger
		JOIN command_invocations AS invocation ON invocation.invocation_id = ledger.invocation_id
		WHERE invocation.command_id = ?`, commandID).Scan(&count).Error; err != nil {
		t.Fatalf("contar ledger de %q: %v", commandID, err)
	}
	return count
}

func TestUICommandCatalogAndPaletteBypass(t *testing.T) {
	a := readyCommandProduct(t)
	registry, _, err := a.commandProductCatalog()
	if err != nil {
		t.Fatalf("commandProductCatalog: %v", err)
	}
	definitions := registry.List()
	if len(definitions) != 150 {
		t.Fatalf("catálogo produtivo tem %d registros, esperado 150: %+v", len(definitions), definitions)
	}
	if _, ok := registry.Lookup(commandProductWorkspaceListID); !ok {
		t.Fatal("catálogo não contém workspace.list")
	}
	definition, ok := registry.Lookup(commandProductShortcutsShowID)
	if !ok || definition.HandlerClassification != commandcatalog.HandlerUI {
		t.Fatalf("catálogo não contém help.shortcuts.show como HandlerUI: %+v", definition)
	}

	result, err := a.ExecutePaletteCommand(commandProductShortcutsShowID, json.RawMessage(`{}`))
	if err == nil || result.Status == "succeeded" {
		t.Fatalf("ExecutePaletteCommand bypassou o caminho UI: result=%+v err=%v", result, err)
	}
}

func TestUINavigationCommandsRejectBackendUIHandoff(t *testing.T) {
	localCommands := append([]commandUINavigation{}, commandProductUINavigation...)
	localCommands = append(localCommands, commandProductChatPickers...)
	localCommands = append(localCommands, commandProductEditorMenus...)
	for _, navigation := range localCommands {
		t.Run(navigation.id, func(t *testing.T) {
			a := readyCommandProduct(t)
			beforeInvocations := commandInvocationCount(t, navigation.id)
			beforeLedger := commandLedgerCount(t, navigation.id)
			if _, err := a.BeginUICommand(navigation.id); err == nil {
				t.Fatal("comando de navegação local aceitou BeginUICommand")
			}
			if got := commandInvocationCount(t, navigation.id); got != beforeInvocations {
				t.Fatalf("Begin recusado criou %d invocações para %q", got-beforeInvocations, navigation.id)
			}
			if got := commandLedgerCount(t, navigation.id); got != beforeLedger {
				t.Fatalf("Begin recusado criou %d linhas de ledger para %q", got-beforeLedger, navigation.id)
			}
		})
	}
}

func TestUICommandContextualCreateRequiresCommitAndAllowsCancel(t *testing.T) {
	a := readyCommandProduct(t)
	beforeTabs := commandWorkspaceChatTabCount(t, a)
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)

	for _, status := range []string{"failed", "succeeded"} {
		if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, status); !errors.Is(err, commandui.ErrCommitRequired) {
			t.Fatalf("CompleteUICommand(%q) = %v, esperado ErrCommitRequired", status, err)
		}
	}
	if got := commandWorkspaceChatTabCount(t, a); got != beforeTabs {
		t.Fatalf("CompleteUICommand sem commit alterou abas: antes=%d depois=%d", beforeTabs, got)
	}
	if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
		t.Fatalf("cancelamento contextual: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "cancelled" {
		t.Fatalf("resultado cancelado = %q", result.Status)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != beforeTabs {
		t.Fatalf("cancelamento contextual alterou abas: antes=%d depois=%d", beforeTabs, got)
	}

	success := beginAuditedTabCreate(t, a)
	successHandoff := takeAuditedTabCreate(t, a, success.Ticket)
	if err := a.CommitWorkspaceTabCommand(success.Ticket, successHandoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand create: %v", err)
	}
	if result := getUIResultEventually(t, a, success.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado create: %+v", result)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != beforeTabs+1 {
		t.Fatalf("create não persistiu: antes=%d depois=%d", beforeTabs, got)
	}
}

func TestUICommandSucceededResultOnlyAfterCompleteAndTakeOnce(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)
	assertGetUIStillPending(t, a, reservation.Ticket)
	// O handoff público só pode ficar disponível depois da reserva e do CAS
	// para running, tanto no ledger quanto na auditoria; ack não é conclusão.
	assertUICommandLedgerStatus(t, reservation.InvocationID, "running")

	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "succeeded" {
		t.Fatalf("resultado após complete = %q, esperado succeeded", result.Status)
	}
	if result.ResultSummary == nil || !strings.Contains(*result.ResultSummary, `"status":"succeeded"`) || result.ErrorCode != nil {
		t.Fatal("help.shortcuts.show não produziu resumo terminal succeeded sem erro")
	}
	if result.InvocationID != reservation.InvocationID {
		t.Fatal("resultado não corresponde à invocation da reservation")
	}

	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("TakeUICommand aceitou segundo take do mesmo ticket")
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("CommitWorkspaceTabCommand aceitou finalização duplicada")
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "succeeded")
}

func TestUICommandExplicitStatuses(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)
	for _, status := range []string{"failed", "succeeded"} {
		if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, status); !errors.Is(err, commandui.ErrCommitRequired) {
			t.Fatalf("CompleteUICommand(%q) = %v, esperado ErrCommitRequired", status, err)
		}
	}
	if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
		t.Fatalf("CompleteUICommand(cancelled): %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "cancelled" {
		t.Fatalf("status=%q, esperado cancelled", result.Status)
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "cancelled")
}

func TestUICommandForgedDuplicateAndCrossSessionDenied(t *testing.T) {
	a := readyCommandProduct(t)
	if _, err := a.TakeUICommand("forged-ticket"); err == nil {
		t.Fatal("TakeUICommand aceitou ticket forjado")
	}
	if err := a.CompleteUICommand("forged-ticket", "forged-handoff", "succeeded"); err == nil {
		t.Fatal("CompleteUICommand aceitou ticket/handoff forjados")
	}
	if _, err := a.GetUICommandResult("forged-ticket"); err == nil {
		t.Fatal("GetUICommandResult aceitou ticket forjado")
	}
	if err := a.CancelUICommand("forged-ticket"); err == nil {
		t.Fatal("CancelUICommand aceitou ticket forjado")
	}

	reservation := beginAuditedTabCreate(t, a)
	// BeginUICommand reserva e inicia a admissão de forma assíncrona. Aguarde
	// essa fronteira antes de trocar a sessão; caso contrário, a tentativa
	// cross-session pode correr junto da preparação contextual e tornar o
	// resultado dependente do escalonamento do worker.
	waitCommandUIAdmission(t, a, reservation)
	original := *a.currentAuthUser
	foreign := original
	foreign.SessionID = uuid.Must(uuid.NewV7()).String()
	a.setCurrentAuthUser(&foreign)
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("sessão diferente conseguiu tomar ticket")
	}
	a.setCurrentAuthUser(&original)
	takeAuditedTabCreate(t, a, reservation.Ticket)
	if err := a.CancelUICommand(reservation.Ticket); err != nil {
		t.Fatalf("cancelar ticket pelo dono após tentativa cross-session: %v", err)
	}
}

func TestUICommandLostConfirmationBecomesOutcomeUnknown(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	_ = takeAuditedTabCreate(t, a, reservation.Ticket)
	if err := a.CancelUICommand(reservation.Ticket); err != nil {
		t.Fatalf("CancelUICommand após handoff: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "outcome_unknown" {
		t.Fatalf("cancelamento sem confirmação = %q, esperado outcome_unknown", result.Status)
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "outcome_unknown")
}

func TestUICommandFrontendStaleCompleteCancelled(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)
	if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
		t.Fatalf("CompleteUICommand(cancelled): %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "cancelled" {
		t.Fatalf("complete stale = %q, esperado cancelled", result.Status)
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "cancelled")
}

func TestUICommandCompletedResultExpiresLazilyButLedgerRemains(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "succeeded" {
		t.Fatalf("status antes da expiração = %q, esperado succeeded", result.Status)
	}

	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("runtime de produto ausente")
	}
	p.mu.Lock()
	run, ok := p.uiRuns[reservation.Ticket]
	if !ok {
		p.mu.Unlock()
		t.Fatal("resultado concluído não ficou retido até o TTL")
	}
	run.expiresAt = time.Now().Add(-time.Second)
	p.mu.Unlock()

	if _, err := a.GetUICommandResult(reservation.Ticket); err == nil {
		t.Fatal("resultado expirado foi consultável")
	}
	p.mu.Lock()
	_, retained := p.uiRuns[reservation.Ticket]
	p.mu.Unlock()
	if retained {
		t.Fatal("resultado expirado permaneceu no mapa de retenção")
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "succeeded")
}

func TestUICommandShutdownJoinsPendingWorkerAndRejectsHandoff(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginAuditedTabCreate(t, a)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("runtime de produto ausente")
	}

	if err := a.drainCommandExecutors(context.Background()); err != nil {
		t.Fatalf("drain do executor UI: %v", err)
	}
	if err := a.shutdownCommandBridgeIfConfigured(context.Background()); err != nil {
		t.Fatalf("shutdown da bridge UI: %v", err)
	}
	select {
	case <-p.done:
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("p.done não foi fechado pelo shutdown real da bridge")
	}
	p.mu.Lock()
	retained := len(p.uiRuns)
	p.mu.Unlock()
	if retained != 0 {
		t.Fatalf("shutdown deixou %d uiRuns retidos", retained)
	}
	joined := make(chan struct{})
	go func() {
		p.workers.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("shutdown não aguardou o worker UI pendente")
	}
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("TakeUICommand aceitou handoff depois do shutdown")
	}
}

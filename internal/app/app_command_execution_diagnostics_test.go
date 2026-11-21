package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/testutil/workspacefixture"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandExecutionDiagnosticsRedactsAndClassifies(t *testing.T) {
	b := captureCommandLoadLogs(t)
	id := uuid.NewString()
	secret := "secret content or path"
	code := "command_failed"
	logCommandExecution("workspace.tab.chat.create", id, "palette", "commit", time.Now(), commandledger.Failed, &code, errors.Join(workspace.ErrCommandCreateTabStale, errors.New(secret)))
	waitDiagnostic(t, b, id)
	var got map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.text()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["invocation_id"] == id {
			got = record
			break
		}
	}
	if got["command_id"] != "workspace.tab.chat.create" || got["invocation_id"] != id || got["stage"] != "commit" || got["source"] != "palette" || got["error_class"] != "workspace_tab_stale" || got["error_code"] != code {
		t.Fatalf("registro: %+v", got)
	}
	if strings.Contains(b.text(), secret) {
		t.Fatal("log expôs erro arbitrário")
	}
}

func TestCommandExecutionDiagnosticsRejectsUntrustedFields(t *testing.T) {
	b := captureCommandLoadLogs(t)
	secret := "secret path"
	logCommandExecution(secret, secret, secret, "prepare", time.Now(), "", &secret, errors.New(secret))
	waitDiagnostic(t, b, `"stage":"prepare"`)
	if strings.Contains(b.text(), secret) {
		t.Fatal("log expôs dados arbitrários")
	}
}

func TestCommandExecutionDiagnosticsFullQueueDoesNotWait(t *testing.T) {
	queue := make(chan commandDiagnosticRecord, 1)
	if !enqueueCommandDiagnostic(queue, commandDiagnosticRecord{}) || enqueueCommandDiagnostic(queue, commandDiagnosticRecord{}) {
		t.Fatal("fila cheia deve descartar o diagnóstico")
	}
}

func TestCommandExecutionDiagnosticsBeginAdmissionFailure(t *testing.T) {
	a := readyCommandProduct(t)
	if err := a.commandProduct.Load().host.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	b := captureCommandLoadLogs(t)
	if _, err := a.BeginUICommand(commandWorkspaceTabChatCreateID); err == nil {
		t.Fatal("comando aceito com cofre bloqueado")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, line := range strings.Split(b.text(), "\n") {
			if strings.Contains(line, `"stage":"admission"`) && strings.Contains(line, commandWorkspaceTabChatCreateID) && strings.Contains(line, `"error_class":`) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("recusa anterior à reserva não foi registrada")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCommandExecutionDiagnosticsWithoutProduct(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Swap(nil)
	defer a.commandProduct.Store(p)
	b := captureCommandLoadLogs(t)
	if _, err := a.BeginUICommand(commandWorkspaceTabChatCreateID); err == nil {
		t.Fatal("comando aceito sem produto")
	}
	if diagnosticRegisteredCommand(a, "senha.cliente") != "unknown" {
		t.Fatal("identificador arbitrário foi aceito")
	}
	id := uuid.NewString()
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, id, "cancelled", 12, "context_target", "ui.action"); err != nil {
		t.Fatalf("diagnóstico sem produto foi recusado: %v", err)
	}
	waitDiagnostic(t, b, id)
	output := b.text()
	if !strings.Contains(output, commandWorkspaceTabChatCreateID) || !strings.Contains(output, `"stage":"admission"`) {
		t.Fatal("identidade da admissão ausente")
	}
	if err := a.ReportLocalPaletteCommand("senha.cliente", uuid.NewString(), "failed", 1, "context_target", "ui.action"); err == nil {
		t.Fatal("aceitou ID desconhecido sem catálogo")
	}
	if strings.Contains(b.text(), "senha.cliente") {
		t.Fatal("registrou ID arbitrário")
	}
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, uuid.NewString(), "cancelled", 1, "context_target", "ui.action"); err == nil {
		t.Fatal("aceitou sessão revogada sem produto")
	}
}

func TestCommandExecutionDiagnosticsDuringWorkspaceSwitch(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	other, err := workspacefixture.Create(t, a.workspaceMgr, "outro", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceMgr.Switch(other.ID); err != nil {
		t.Fatal(err)
	}
	if p.dependenciesMatch(a) {
		t.Fatal("fixture não preservou o produto anterior")
	}
	b := captureCommandLoadLogs(t)
	id := uuid.NewString()
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, id, "cancelled", 10, "context_target", "keyboard.local"); err != nil {
		t.Fatalf("diagnóstico perdido na troca: %v", err)
	}
	waitDiagnostic(t, b, id)
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, uuid.NewString(), "not_completed", 10, "context_navigation", "palette"); err != nil {
		t.Fatalf("status neutro recusado: %v", err)
	}
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, uuid.NewString(), "not_completed", 10, "context_target", "palette"); err == nil {
		t.Fatal("status neutro aceito em etapa incorreta")
	}
	if !strings.Contains(b.text(), `"source":"keyboard.local"`) {
		t.Fatal("origem incorreta")
	}
}

func waitDiagnostic(t *testing.T, b *commandLoadLogBuffer, needle string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(b.text(), needle) {
		if time.Now().After(deadline) {
			t.Fatal("diagnóstico não foi publicado")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCommandExecutionDiagnosticsTransport(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeChat)
	b := captureCommandLoadLogs(t)
	r := beginUICommand(t, a, commandWorkspaceTabChatCreateID)
	h := takeUICommandFor(t, a, r.Ticket, commandWorkspaceTabChatCreateID)
	other := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: other, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(other); err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
		t.Fatal("commit obsoleto foi aceito")
	}
	_ = getUIResultEventually(t, a, r.Ticket)
	deadline := time.Now().Add(3 * time.Second)
	for {
		found := false
		for _, line := range strings.Split(b.text(), "\n") {
			if strings.Contains(line, r.InvocationID) && strings.Contains(line, `"stage":"commit"`) {
				found = true
				break
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("diagnóstico de commit da invocação não foi publicado")
		}
		time.Sleep(time.Millisecond)
	}
	output := b.text()
	if !strings.Contains(output, r.InvocationID) || !strings.Contains(output, commandWorkspaceTabChatCreateID) {
		t.Fatal("faltou correlacionar o comando e a invocação")
	}
	if strings.Contains(output, r.Ticket) || strings.Contains(output, h.HandoffID) {
		t.Fatal("log expôs credenciais transitórias")
	}
}

func TestCommandExecutionDiagnosticsUnknownCommandAndLocalPalette(t *testing.T) {
	a := readyCommandProduct(t)
	b := captureCommandLoadLogs(t)
	secret := "senha.cliente"
	_, _ = a.ExecutePaletteCommand(secret, nil)
	id := uuid.NewString()
	if err := a.ReportLocalPaletteCommand("navigation.settings.open", id, "succeeded", 4, "local_ui", "palette"); err != nil {
		t.Fatal(err)
	}
	waitDiagnostic(t, b, id)
	acceptedID := uuid.NewString()
	if err := a.ReportLocalPaletteCommand("workspace.tab.next", acceptedID, "accepted", 1, "local_ui", "palette"); err != nil {
		t.Fatal(err)
	}
	waitDiagnostic(t, b, acceptedID)
	if !strings.Contains(b.text(), `"status":"accepted"`) {
		t.Fatal("aceitação local ausente")
	}
	if err := a.ReportLocalPaletteCommand(commandWorkspaceTabChatCreateID, uuid.NewString(), "accepted", 1, "creation_selection", "keyboard.local"); err == nil {
		t.Fatal("aceitação em etapa inválida foi permitida")
	}
	if strings.Contains(b.text(), secret) {
		t.Fatal("comando desconhecido foi registrado literalmente")
	}
	if err := a.ReportLocalPaletteCommand(secret, uuid.NewString(), "failed", 4, "local_ui", "palette"); err == nil {
		t.Fatal("aceitou comando desconhecido")
	}
	if err := a.ReportLocalPaletteCommand("navigation.settings.open", uuid.NewString(), "failed", 4, secret, "palette"); err == nil {
		t.Fatal("aceitou etapa arbitrária")
	}
	if err := a.commandProduct.Load().host.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	id = uuid.NewString()
	if err := a.ReportLocalPaletteCommand("navigation.settings.open", id, "cancelled", 4, "local_ui", "palette"); err != nil {
		t.Fatalf("diagnóstico recusado com produto não pronto: %v", err)
	}
	waitDiagnostic(t, b, id)
	p := a.commandProduct.Load()
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.ReportLocalPaletteCommand("navigation.settings.open", uuid.NewString(), "succeeded", 1, "local_ui", "palette"); err == nil {
		t.Fatal("aceitou sessão revogada")
	}
}

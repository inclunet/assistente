package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestExecutePaletteCommandWorkspaceListReturnsEphemeralOutput(t *testing.T) {
	a := readyCommandProduct(t)
	activePath := a.workspaceMgr.ActivePath()
	active := a.workspaceMgr.Active()
	if active == nil || len(active.Tabs.Items) == 0 {
		t.Fatal("fixture deve ter workspace ativo com abas")
	}
	result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecutePaletteCommand workspace.list: %v", err)
	}
	if result.Status != string(commandledger.Succeeded) || result.Output == nil {
		t.Fatalf("execução não retornou sucesso/output: status=%q output-nil=%v", result.Status, result.Output == nil)
	}
	if result.Output.Kind != commandProductWorkspaceListID || len(result.Output.Workspaces) != 1 {
		t.Fatalf("output workspace.list inesperado: kind=%q workspaces=%d", result.Output.Kind, len(result.Output.Workspaces))
	}
	typedWorkspaces := result.Output.Workspaces
	if len(typedWorkspaces) != 1 {
		t.Fatal("output workspace.list perdeu o tipo de metadados público")
	}
	workspace := result.Output.Workspaces[0]
	if workspace.ID != active.ID || workspace.Name != active.Name || workspace.Profile == "" || workspace.TabCount != len(active.Tabs.Items) || !workspace.IsActive {
		t.Fatalf("metadados do workspace incompletos: %+v", workspace)
	}
	serializedOutput, err := json.Marshal(result.Output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serializedOutput), activePath) || strings.Contains(string(serializedOutput), ".assistente") {
		t.Fatal("output efêmero expôs path do workspace")
	}

	var ledgerSummaries []string
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", result.InvocationID).Pluck("result_summary", &ledgerSummaries).Error; err != nil {
		t.Fatalf("ler resumo do ledger: %v", err)
	}
	var auditRows []struct {
		ArgumentsSummary string  `gorm:"column:arguments_summary"`
		ResultSummary    *string `gorm:"column:result_summary"`
	}
	if err := database.DB().Table("command_invocations").Select("arguments_summary, result_summary").Where("invocation_id = ?", result.InvocationID).Find(&auditRows).Error; err != nil {
		t.Fatalf("ler auditoria: %v", err)
	}
	if len(ledgerSummaries) != 1 || len(auditRows) != 1 {
		t.Fatalf("execução deve ter exatamente um registro por trilha: ledger=%d auditoria=%d", len(ledgerSummaries), len(auditRows))
	}
	for _, summary := range ledgerSummaries {
		if strings.Contains(summary, workspace.Name) || strings.Contains(summary, activePath) {
			t.Fatal("ledger persistiu nome/path do workspace")
		}
	}
	for _, row := range auditRows {
		serializedRow, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serializedRow), workspace.Name) || strings.Contains(string(serializedRow), activePath) {
			t.Fatal("auditoria persistiu nome/path do workspace")
		}
	}
}

func TestGetPaletteInvocationDoesNotExposeEphemeralOutput(t *testing.T) {
	a := readyCommandProduct(t)
	executed, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecutePaletteCommand workspace.list: %v", err)
	}
	got, err := a.GetPaletteInvocation(executed.InvocationID)
	if err != nil {
		t.Fatalf("GetPaletteInvocation: %v", err)
	}
	if got.Status != string(commandledger.Succeeded) || got.Output != nil {
		t.Fatalf("lookup persistente expôs output efêmero: status=%q output-nil=%v", got.Status, got.Output == nil)
	}
}

func TestExecutePaletteCommandRejectsUICommandWithoutOutput(t *testing.T) {
	a := readyCommandProduct(t)
	result, err := a.ExecutePaletteCommand(commandProductShortcutsShowID, json.RawMessage(`{}`))
	if err == nil || result.Output != nil {
		t.Fatal("help.shortcuts.show foi executado pelo caminho Palette ou retornou output")
	}
}

func TestCommandProductOutputRejectsInvalidRawResult(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("runtime de produto ausente")
	}
	definition, ok := p.registry.Lookup(commandProductWorkspaceListID)
	if !ok {
		t.Fatal("definição workspace.list ausente no registry montado")
	}
	if definition.HandlerClassification != commandcatalog.HandlerBackend {
		t.Fatal("workspace.list não é backend no registry montado")
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"kind":"workspace.list"}`),
		json.RawMessage(`{"kind":"other.command","workspaces":[]}`),
		json.RawMessage(`{"kind":"workspace.list","workspaces":[{"id":"","name":"x","profile":"p","tab_count":1,"is_active":true}]}`),
		json.RawMessage(`not-json`),
		json.RawMessage(`{"workspaces":null}`),
		json.RawMessage(`{"workspaces":[{"id":"","name":"x","profile":"p","tab_count":1,"is_active":true}]}`),
		json.RawMessage(`{"workspaces":[{"id":"a","name":"x","profile":"p","tab_count":-1,"is_active":true}]}`),
		json.RawMessage(`{"workspaces":[{"id":"a","name":"x","profile":"p","tab_count":1,"is_active":true,"path":"private"}]}`),
		json.RawMessage(`{"workspaces":[{"id":"a","name":"x","profile":"p","tab_count":1,"is_active":false},{"id":"a","name":"y","profile":"p","tab_count":1,"is_active":false}]}`),
	} {
		if output, err := commandProductOutput(definition, raw); err == nil || output != nil {
			t.Fatalf("raw inválido aceito: err=%v output-nil=%v", err, output == nil)
		}
	}
}

func TestCommandProductOutputIsDroppedAfterAuthHostOrWorkspaceInvalidation(t *testing.T) {
	testInvalidation := func(t *testing.T, invalidate func(*App, *commandProductRuntime) error) {
		t.Helper()
		a := readyCommandProduct(t)
		p := a.commandProduct.Load()
		if p == nil {
			t.Fatal("runtime de produto ausente")
		}
		candidate := commandexecution.EnvelopeCandidate{
			InvocationID:  uuid.Must(uuid.NewV7()).String(),
			CorrelationID: uuid.Must(uuid.NewV7()).String(),
			CommandID:     commandProductWorkspaceListID,
			Arguments:     json.RawMessage(`{}`),
		}
		record, raw, err := p.service.ExecuteEnvelopeWithResult(a.commandBridgeContext(), "", candidate)
		if err != nil || record.Status != commandledger.Succeeded || len(raw) == 0 {
			t.Fatalf("execução inicial: status=%q raw=%d err=%v", record.Status, len(raw), err)
		}
		if !commandRecordMatchesPrincipal(record, p.principal) {
			t.Fatalf("registro não está vinculado ao principal da sessão: ownership=%+v principal=%+v", record.Ownership, p.principal)
		}
		if err := invalidate(a, p); err != nil {
			t.Fatal(err)
		}
		result, err := a.commandProductResultWithOutput(p, commandProductWorkspaceListID, record, raw)
		if err == nil || result.Output != nil {
			t.Fatalf("output sobreviveu à invalidação: result=%+v err=%v", result, err)
		}
	}

	t.Run("session revoked", func(t *testing.T) {
		testInvalidation(t, func(_ *App, p *commandProductRuntime) error {
			return database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error
		})
	})
	t.Run("vault locked", func(t *testing.T) {
		testInvalidation(t, func(a *App, _ *commandProductRuntime) error {
			return a.commandHost.SetVaultUnlocked(context.Background(), false)
		})
	})
	t.Run("workspace switched", func(t *testing.T) {
		testInvalidation(t, func(a *App, p *commandProductRuntime) error {
			active := a.workspaceMgr.Active()
			if active == nil || active.ID != p.workspaceID {
				return fmt.Errorf("workspace ativo inesperado antes da troca: active=%+v runtime=%q", active, p.workspaceID)
			}
			other, err := a.workspaceMgr.Create("Output test workspace")
			if err != nil {
				return err
			}
			_, err = a.workspaceMgr.Switch(other.ID)
			return err
		})
	})
}

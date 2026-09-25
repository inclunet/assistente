package app

import (
	"testing"
	"time"

	"assistente/internal/commandcatalog"
)

func TestTerminalSessionCommandsUseCentralBackendContracts(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	for _, tc := range []struct {
		id           string
		effect       commandcatalog.Effect
		decision     commandcatalog.Decision
		risk         commandcatalog.Risk
		presentation string
	}{
		{id: commandTerminalSessionCreateID, effect: commandcatalog.Write, decision: commandcatalog.NoDecision, risk: commandcatalog.RiskLow, presentation: "terminal-session-create-v1"},
		{id: commandTerminalSessionCloseID, effect: commandcatalog.Destructive, decision: commandcatalog.Interactive, risk: commandcatalog.RiskHigh, presentation: "terminal-session-close-v1"},
	} {
		d, ok := p.registry.Lookup(tc.id)
		if !ok || d.Effect != tc.effect || d.Decision != tc.decision || d.Risk != tc.risk || d.Presentation == nil || d.Presentation.Version != tc.presentation || !d.HasMutableTarget || d.HandlerClassification != commandcatalog.HandlerBackend || !isWorkspaceMutationCommand(tc.id) || isLocalUICommand(tc.id) || !commandDeckLedgerCommand(d) {
			t.Fatalf("contrato terminal inválido para %s: %+v", tc.id, d)
		}
		if !d.AllowsSource(commandcatalog.Palette) || !d.AllowsSource(commandcatalog.KeyboardLocal) || !d.AllowsSource(commandcatalog.StreamDeck) {
			t.Fatalf("origens incompletas para %s", tc.id)
		}
		if d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Audit != commandcatalog.PersistenceRedacted {
			t.Fatalf("persistência insegura para %s: %+v", tc.id, d.Persistence)
		}
	}
	if commandUIRunTimeout(commandTerminalSessionCloseID) != 5*time.Minute || commandUITakeTimeout(commandTerminalSessionCloseID) != 5*time.Minute || commandUIResultTTL(commandTerminalSessionCloseID) != 6*time.Minute {
		t.Fatal("prazos de fechamento não cobrem a decisão")
	}
}

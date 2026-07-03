package app

import (
	"assistente/internal/allowlist"
	"assistente/internal/logging"
	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
	"assistente/internal/terminal"
	"context"
)

// ============================================================================
// Terminal Management API (sessões PTY compartilhadas LLM + usuário)
// ============================================================================

func (a *App) ListTerminalSessions() []terminal.SessionInfo {
	return a.terminalCtrl.ListTerminalSessions()
}

func (a *App) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	return a.terminalCtrl.CreateTerminalSession(name)
}

func (a *App) CloseTerminalSession(sessionID string) error {
	return a.terminalCtrl.CloseTerminalSession(sessionID)
}

func (a *App) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	return a.terminalCtrl.GetTerminalHistory(sessionID)
}

func (a *App) RunTerminalCommand(sessionID string, command string) error {
	return a.terminalCtrl.RunTerminalCommand(sessionID, command)
}

func (a *App) SendTerminalInput(sessionID string, input string) error {
	return a.terminalCtrl.SendTerminalInput(sessionID, input)
}

func (a *App) InterruptTerminalCommand(sessionID string) error {
	return a.terminalCtrl.InterruptTerminalCommand(sessionID)
}

func (a *App) GetTerminalStats() *terminal.ManagerStats {
	return a.terminalCtrl.GetTerminalStats()
}

// initTerminalAndAllowlists inicializa os managers de terminal, questionário e allowlists.
func (a *App) initTerminalAndAllowlists() {
	emitEvent := func(event string, data any) {
		a.emitter.Emit(event, data)
	}

	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), emitEvent)
	a.questionnaireMgr = questionnaire.NewManager(emitEvent)
	a.allowlistMgr = allowlist.NewManager()
	if err := a.allowlistMgr.EnsureDefaults(); err != nil {
		logging.Errorf(context.Background(), "app.app-terminal", "[Allowlist] Erro ao garantir allowlist padrão: %v", err)
	}

	// Allowlist de rede (override anti-SSRF escopável). Sem defaults: começa vazia
	// e só cresce por autorização explícita do usuário.
	a.netTrustMgr = nettrust.NewManager()

	logging.Infof(context.Background(), "app.app-terminal", "[Terminal] Managers de terminal, questionário e allowlist inicializados")
}

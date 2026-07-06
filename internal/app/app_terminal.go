package app

import (
	"assistente/internal/allowlist"
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
	"assistente/internal/terminal"
	"context"
	"path/filepath"
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
	// O escopo "workspace" deve seguir o workspace ATIVO (que muda em runtime sem
	// alterar o cwd do processo). A closure é avaliada a cada operação, então lê o
	// workspaceMgr no momento do uso (já inicializado por initWorkspace).
	a.netTrustMgr.SetWorkspaceDirFunc(func() string {
		if a.workspaceMgr != nil {
			if base := a.workspaceMgr.ActivePath(); base != "" {
				return filepath.Join(base, ".assistente")
			}
		}
		return configdir.GetWorkDir()
	})
	// Fallback do slug de perfil ativo: garante que o escopo "profile" persista e
	// case mesmo quando o invocationctx da chamada não traz ProfileSlug — e de
	// forma consistente com a API de gestão (networkManagementContext).
	a.netTrustMgr.SetActiveProfileSlugFunc(func() string {
		if a.profileManager != nil {
			return a.profileManager.GetActiveSlug()
		}
		return ""
	})

	logging.Infof(context.Background(), "app.app-terminal", "[Terminal] Managers de terminal, questionário e allowlist inicializados")
}

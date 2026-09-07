package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/terminal"
	"context"
	"sync"
)

// Terminal é o bind Wails do domínio terminal (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Shell é sensível: nenhum método aqui roda sem sessão autenticada, mesmo
// que a versão anterior no *App não autenticasse (fail-closed corrigido na
// borda).
//
// initTerminalAndAllowlists (managers), Shutdown/CloseAll e o protocolo de
// eventos PTY (terminal:*) continuam no *App — fora do escopo desta migração.
type Terminal struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.TerminalController
}

// NewTerminal cria o bind vazio; AttachTerminal preenche session + controller no startup.
func NewTerminal() *Terminal {
	return &Terminal{}
}

// AttachTerminal associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachTerminal(t *Terminal, session Session, ctrl *controllers.TerminalController) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.session = session
	t.ctrl = ctrl
}

func (t *Terminal) deps() (Session, *controllers.TerminalController, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.session == nil || t.ctrl == nil {
		return nil, nil, ErrTerminalNotWired
	}
	return t.session, t.ctrl, nil
}

// ListTerminalSessions retorna todas as sessões de terminal ativas.
func (t *Terminal) ListTerminalSessions() ([]terminal.SessionInfo, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]terminal.SessionInfo, error) {
		return ctrl.ListTerminalSessions(), nil
	})
}

// CreateTerminalSession cria uma nova sessão de terminal.
func (t *Terminal) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*terminal.SessionInfo, error) {
		return ctrl.CreateTerminalSession(name)
	})
}

// CloseTerminalSession encerra uma sessão de terminal.
func (t *Terminal) CloseTerminalSession(sessionID string) error {
	session, ctrl, err := t.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.CloseTerminalSession(sessionID)
	})
	return err
}

// GetTerminalHistory retorna o histórico de comandos de uma sessão.
func (t *Terminal) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]terminal.HistoryEntry, error) {
		return ctrl.GetTerminalHistory(sessionID)
	})
}

// RunTerminalCommand executa um comando com markers em uma sessão de terminal.
func (t *Terminal) RunTerminalCommand(sessionID string, command string) error {
	session, ctrl, err := t.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RunTerminalCommand(sessionID, command)
	})
	return err
}

// SendTerminalInput envia input raw para uma sessão de terminal (modo interativo).
func (t *Terminal) SendTerminalInput(sessionID string, input string) error {
	session, ctrl, err := t.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SendTerminalInput(sessionID, input)
	})
	return err
}

// InterruptTerminalCommand envia Ctrl+C para uma sessão de terminal.
func (t *Terminal) InterruptTerminalCommand(sessionID string) error {
	session, ctrl, err := t.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.InterruptTerminalCommand(sessionID)
	})
	return err
}

// GetTerminalStats retorna estatísticas do gerenciador de terminais.
func (t *Terminal) GetTerminalStats() (*terminal.ManagerStats, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*terminal.ManagerStats, error) {
		return ctrl.GetTerminalStats(), nil
	})
}

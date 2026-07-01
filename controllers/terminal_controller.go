package controllers

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"os"
	"time"

	"assistente/internal/terminal"
)

// TerminalControllerConfig agrupa as dependências do TerminalController.
type TerminalControllerConfig struct {
	TerminalMgr *terminal.Manager
}

// TerminalController é o adapter primário (Inbound) para operações de sessões PTY.
type TerminalController struct {
	terminalMgr *terminal.Manager
}

// NewTerminalController cria um TerminalController com suas dependências.
func NewTerminalController(cfg TerminalControllerConfig) *TerminalController {
	return &TerminalController{
		terminalMgr: cfg.TerminalMgr,
	}
}

// ListTerminalSessions retorna todas as sessões de terminal ativas.
func (c *TerminalController) ListTerminalSessions() []terminal.SessionInfo {
	if c.terminalMgr == nil {
		return []terminal.SessionInfo{}
	}
	return c.terminalMgr.List()
}

// CreateTerminalSession cria uma nova sessão de terminal.
func (c *TerminalController) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	if c.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}
	workDir, _ := os.Getwd()
	session, err := c.terminalMgr.Create(name, workDir)
	if err != nil {
		return nil, err
	}
	info := session.Info()
	return &info, nil
}

// CloseTerminalSession encerra uma sessão de terminal.
func (c *TerminalController) CloseTerminalSession(sessionID string) error {
	if c.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return c.terminalMgr.Close(sessionID)
}

// GetTerminalHistory retorna o histórico de comandos de uma sessão.
func (c *TerminalController) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	if c.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}
	return c.terminalMgr.GetHistory(sessionID)
}

// RunTerminalCommand executa um comando com markers em uma sessão de terminal.
func (c *TerminalController) RunTerminalCommand(sessionID string, command string) error {
	if c.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_, err := c.terminalMgr.RunCommand(ctx, sessionID, command, 0, "user")
		if err != nil {
			logging.Errorf(context.Background(), "controllers.terminal-controller", "[Terminal] Erro ao executar comando: %v", err)
		}
	}()
	return nil
}

// SendTerminalInput envia input raw para uma sessão de terminal (modo interativo).
func (c *TerminalController) SendTerminalInput(sessionID string, input string) error {
	if c.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	_, err := c.terminalMgr.SendInput(sessionID, input)
	if err != nil {
		logging.Errorf(context.Background(), "controllers.terminal-controller", "[Terminal] Erro ao enviar input: %v", err)
		return err
	}
	return nil
}

// InterruptTerminalCommand envia Ctrl+C para uma sessão de terminal.
func (c *TerminalController) InterruptTerminalCommand(sessionID string) error {
	if c.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return c.terminalMgr.Interrupt(sessionID)
}

// GetTerminalStats retorna estatísticas do gerenciador de terminais.
func (c *TerminalController) GetTerminalStats() *terminal.ManagerStats {
	if c.terminalMgr == nil {
		return &terminal.ManagerStats{}
	}
	stats := c.terminalMgr.Stats()
	return &stats
}

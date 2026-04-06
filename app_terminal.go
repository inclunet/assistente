package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"assistente/internal/terminal"
)

// ============================================================================
// Terminal Management API (sessões PTY compartilhadas LLM + usuário)
// ============================================================================

// ListTerminalSessions retorna todas as sessões de terminal ativas.
func (a *App) ListTerminalSessions() []terminal.SessionInfo {
	if a.terminalMgr == nil {
		return []terminal.SessionInfo{}
	}
	return a.terminalMgr.List()
}

// CreateTerminalSession cria uma nova sessão de terminal.
func (a *App) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}

	workDir, _ := os.Getwd()
	session, err := a.terminalMgr.Create(name, workDir)
	if err != nil {
		return nil, err
	}

	info := session.Info()
	return &info, nil
}

// CloseTerminalSession encerra uma sessão de terminal.
func (a *App) CloseTerminalSession(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Close(sessionID)
}

// GetTerminalHistory retorna o histórico de comandos de uma sessão.
func (a *App) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.GetHistory(sessionID)
}

// RunTerminalCommand executa um comando com markers em uma sessão de terminal.
// Mantido para compatibilidade — usado internamente pelo LLM.
func (a *App) RunTerminalCommand(sessionID string, command string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	// Executa em goroutine para não bloquear o binding
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		_, err := a.terminalMgr.RunCommand(ctx, sessionID, command, 0, "user")
		if err != nil {
			log.Printf("[Terminal] Erro ao executar comando: %v", err)
		}
	}()

	return nil
}

// SendTerminalInput envia input raw para uma sessão de terminal (modo interativo).
// Diferente de RunTerminalCommand, não usa markers — o input vai direto ao PTY.
// Suporta comandos interativos (wsl, python, ssh, etc.) e input para programas em execução.
func (a *App) SendTerminalInput(sessionID string, input string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	_, err := a.terminalMgr.SendInput(sessionID, input)
	if err != nil {
		log.Printf("[Terminal] Erro ao enviar input: %v", err)
		return err
	}
	return nil
}

// InterruptTerminalCommand envia Ctrl+C para uma sessão de terminal.
func (a *App) InterruptTerminalCommand(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Interrupt(sessionID)
}

// GetTerminalStats retorna estatísticas do gerenciador de terminais.
func (a *App) GetTerminalStats() *terminal.ManagerStats {
	if a.terminalMgr == nil {
		return &terminal.ManagerStats{}
	}
	stats := a.terminalMgr.Stats()
	return &stats
}

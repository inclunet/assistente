package main

import (
	"fmt"

	mcpmgr "assistente/internal/mcp"
)

// ============================================================================
// MCP Server Management API
// ============================================================================

// ListMCPServers retorna informações de todos os servidores MCP configurados.
func (a *App) ListMCPServers() []mcpmgr.ServerInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.ServerInfo{}
	}
	return a.mcpMgr.List()
}

// ConnectMCPServer conecta a um servidor MCP pelo slug.
func (a *App) ConnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Connect(slug)
}

// DisconnectMCPServer desconecta de um servidor MCP.
func (a *App) DisconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Disconnect(slug)
}

// ReconnectMCPServer reconecta a um servidor MCP.
func (a *App) ReconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Reconnect(slug)
}

// SaveMCPServer salva (cria ou atualiza) a configuração de um servidor MCP.
func (a *App) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SaveConfig(slug, cfg)
}

// DuplicateMCPServer cria uma copia da configuracao de um servidor MCP.
func (a *App) DuplicateMCPServer(slug string) (string, error) {
	if a.mcpMgr == nil {
		return "", fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DuplicateConfig(slug)
}

// DeleteMCPServer remove a configuração de um servidor MCP.
func (a *App) DeleteMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteConfig(slug)
}

// GetMCPServerTools retorna as ferramentas de um servidor MCP específico.
func (a *App) GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.MCPToolInfo{}
	}
	return a.mcpMgr.GetTools(slug)
}

// GetMCPServerConfig retorna a configuração de um servidor MCP.
func (a *App) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.GetConfig(slug)
}

// ReadMCPResource lê o conteúdo de um resource MCP.
func (a *App) ReadMCPResource(slug, uri string) (string, error) {
	if a.mcpMgr == nil {
		return "", fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.ReadResource(slug, uri)
}

// GetMCPPrompt executa um prompt MCP e retorna as mensagens geradas.
func (a *App) GetMCPPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.GetPrompt(slug, name, arguments)
}

// LLMSettings contém configurações da API LLM
type LLMSettings struct {
	APIKey  string
	BaseURL string
}

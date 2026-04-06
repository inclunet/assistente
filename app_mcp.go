package main

import (
	"fmt"
	"log"

	mcpmgr "assistente/internal/mcp"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

// SetMCPWorkspaceRoots configura os diretórios raiz do workspace para servidores MCP.
func (a *App) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SetWorkspaceRoots(roots)
}

// GetMCPWorkspaceRoots retorna os workspace roots configurados.
func (a *App) GetMCPWorkspaceRoots() []mcpmgr.Root {
	if a.mcpMgr == nil {
		return []mcpmgr.Root{}
	}
	return a.mcpMgr.GetWorkspaceRoots()
}

// SubscribeToMCPResource inscreve para receber notificações de um resource.
func (a *App) SubscribeToMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SubscribeToResource(slug, uri)
}

// UnsubscribeFromMCPResource cancela inscrição de um resource.
func (a *App) UnsubscribeFromMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.UnsubscribeFromResource(slug, uri)
}

// SaveMCPServerAuth salva credenciais de autenticação para um servidor MCP.
func (a *App) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	return a.mcpMgr.SaveServerAuth(slug, authType, token, username, password, clientSecret)
}

// DeleteMCPServerAuth remove credenciais de autenticação de um servidor MCP.
func (a *App) DeleteMCPServerAuth(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteServerAuth(slug)
}

// GetMCPServerAuthInfo retorna informações sobre a autenticação de um servidor MCP
// (tipo e se existe, sem expor valores sensíveis).
func (a *App) GetMCPServerAuthInfo(slug string) (map[string]any, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	authType, hasAuth, err := a.mcpMgr.GetServerAuthInfo(slug)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"hasAuth":  hasAuth,
		"authType": authType,
	}, nil
}

// DiscoverMCPServerAuth consulta os endpoints well-known de um servidor MCP
// para auto-discovery de configuração OAuth.
func (a *App) DiscoverMCPServerAuth(serverURL string) mcpmgr.OAuthDiscoveryResult {
	return mcpmgr.DiscoverOAuth(serverURL)
}

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)

		// Quando o set de tools MCP muda, regenera o catalogo de jobs
		if event == "mcp:tools_changed" && a.jobMgr != nil {
			go func() {
				if err := a.jobMgr.RegenerateCatalog(); err != nil {
					log.Printf("[Jobs] Catalog regeneration on MCP change failed: %v", err)
				} else {
					log.Printf("[Jobs] Catalog regenerated after MCP tools change")
				}
			}()
		}
	}

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, a.credMgr, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

	// Observa mudanças externas nos arquivos de config
	go a.mcpMgr.WatchConfigs()

	log.Printf("[MCP] Manager inicializado")
}

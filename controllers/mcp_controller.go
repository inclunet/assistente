package controllers

import (
	"assistente/internal/core/ports"
	"assistente/internal/jobs"
	"assistente/internal/logging"
	mcpmgr "assistente/internal/mcp"
	"context"
	"fmt"
)

// MCPController é o adapter primário (Inbound) para operações de MCP.
// Encapsula toda a API exposta ao frontend sobre servidores MCP,
// delegando para o mcpmgr.Manager sem referências ao megastruct App.
type MCPController struct {
	mcpMgr  *mcpmgr.Manager
	jobMgr  *jobs.Manager
	emitter ports.Emitter
}

// NewMCPController cria um MCPController com suas dependências.
func NewMCPController(mcpMgr *mcpmgr.Manager, jobMgr *jobs.Manager, emitter ports.Emitter) *MCPController {
	return &MCPController{mcpMgr: mcpMgr, jobMgr: jobMgr, emitter: emitter}
}

func (c *MCPController) guardMgr() error {
	if c.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return nil
}

func (c *MCPController) ListMCPServers() []mcpmgr.ServerInfo {
	if c.mcpMgr == nil {
		return []mcpmgr.ServerInfo{}
	}
	return c.mcpMgr.List()
}

func (c *MCPController) ConnectMCPServer(slug string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.Connect(slug)
}

func (c *MCPController) DisconnectMCPServer(slug string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.Disconnect(slug)
}

func (c *MCPController) ReconnectMCPServer(slug string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.Reconnect(slug)
}

func (c *MCPController) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.SaveConfig(slug, cfg)
}

func (c *MCPController) DuplicateMCPServer(slug string) (string, error) {
	if err := c.guardMgr(); err != nil {
		return "", err
	}
	return c.mcpMgr.DuplicateConfig(slug)
}

func (c *MCPController) DeleteMCPServer(slug string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.DeleteConfig(slug)
}

func (c *MCPController) GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo {
	if c.mcpMgr == nil {
		return []mcpmgr.MCPToolInfo{}
	}
	return c.mcpMgr.GetTools(slug)
}

func (c *MCPController) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	if err := c.guardMgr(); err != nil {
		return nil, err
	}
	return c.mcpMgr.GetConfig(slug)
}

func (c *MCPController) ReadMCPResource(slug, uri string) (string, error) {
	if err := c.guardMgr(); err != nil {
		return "", err
	}
	return c.mcpMgr.ReadResource(slug, uri)
}

func (c *MCPController) GetMCPPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	if err := c.guardMgr(); err != nil {
		return nil, err
	}
	return c.mcpMgr.GetPrompt(slug, name, arguments)
}

func (c *MCPController) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.SetWorkspaceRoots(roots)
}

func (c *MCPController) GetMCPWorkspaceRoots() []mcpmgr.Root {
	if c.mcpMgr == nil {
		return []mcpmgr.Root{}
	}
	return c.mcpMgr.GetWorkspaceRoots()
}

func (c *MCPController) SubscribeToMCPResource(slug, uri string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.SubscribeToResource(slug, uri)
}

func (c *MCPController) UnsubscribeFromMCPResource(slug, uri string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.UnsubscribeFromResource(slug, uri)
}

func (c *MCPController) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.SaveServerAuth(slug, authType, token, username, password, clientSecret)
}

func (c *MCPController) DeleteMCPServerAuth(slug string) error {
	if err := c.guardMgr(); err != nil {
		return err
	}
	return c.mcpMgr.DeleteServerAuth(slug)
}

func (c *MCPController) GetMCPServerAuthInfo(slug string) (map[string]any, error) {
	if err := c.guardMgr(); err != nil {
		return nil, err
	}
	authType, hasAuth, err := c.mcpMgr.GetServerAuthInfo(slug)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"hasAuth":  hasAuth,
		"authType": authType,
	}, nil
}

func (c *MCPController) DiscoverMCPServerAuth(serverURL string) mcpmgr.OAuthDiscoveryResult {
	return mcpmgr.DiscoverOAuth(serverURL)
}

func (c *MCPController) GetMCPServerLogs(slug string, limit int) ([]mcpmgr.MCPServerLog, error) {
	if err := c.guardMgr(); err != nil {
		return nil, err
	}
	return c.mcpMgr.GetLogs(slug, limit)
}

// NewMCPEventEmitter retorna a função de emit usada internamente pelo mcpmgr.
// Mantém a lógica de regenerar o catálogo de jobs quando tools mudam.
func (c *MCPController) NewMCPEventEmitter() func(event string, data any) {
	return func(event string, data any) {
		c.emitter.Emit(event, data)
		if event == "mcp:tools_changed" && c.jobMgr != nil {
			go func() {
				if err := c.jobMgr.RegenerateCatalog(); err != nil {
					logging.Errorf(context.Background(), "controllers.mcp-controller", "[Jobs] Catalog regeneration on MCP change failed: %v", err)
				} else {
					logging.Infof(context.Background(), "controllers.mcp-controller", "[Jobs] Catalog regenerated after MCP tools change")
				}
			}()
		}
	}
}

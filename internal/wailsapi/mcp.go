package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	mcpmgr "assistente/internal/mcp"
	"context"
	"sync"
)

// MCP é o bind Wails do domínio MCP (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type MCP struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.MCPController
}

// NewMCP cria o bind vazio; AttachMCP preenche session + controller no startup.
func NewMCP() *MCP {
	return &MCP{}
}

// AttachMCP associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachMCP(m *MCP, session Session, ctrl *controllers.MCPController) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = session
	m.ctrl = ctrl
}

func (m *MCP) deps() (Session, *controllers.MCPController, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil || m.ctrl == nil {
		return nil, nil, ErrMCPNotWired
	}
	return m.session, m.ctrl, nil
}

// ListMCPServers lista servidores MCP configurados.
func (m *MCP) ListMCPServers() ([]mcpmgr.ServerInfo, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]mcpmgr.ServerInfo, error) {
		return ctrl.ListMCPServers(), nil
	})
}

// ConnectMCPServer conecta um servidor pelo slug.
func (m *MCP) ConnectMCPServer(slug string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ConnectMCPServer(slug)
	})
	return err
}

// DisconnectMCPServer desconecta um servidor pelo slug.
func (m *MCP) DisconnectMCPServer(slug string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DisconnectMCPServer(slug)
	})
	return err
}

// ReconnectMCPServer reconecta um servidor pelo slug.
func (m *MCP) ReconnectMCPServer(slug string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ReconnectMCPServer(slug)
	})
	return err
}

// SaveMCPServer cria ou atualiza a configuração de um servidor.
func (m *MCP) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveMCPServer(slug, cfg)
	})
	return err
}

// DuplicateMCPServer duplica a configuração de um servidor.
func (m *MCP) DuplicateMCPServer(slug string) (string, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.DuplicateMCPServer(slug)
	})
}

// DeleteMCPServer remove um servidor pelo slug.
func (m *MCP) DeleteMCPServer(slug string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteMCPServer(slug)
	})
	return err
}

// GetMCPServerTools lista tools expostas por um servidor.
func (m *MCP) GetMCPServerTools(slug string) ([]mcpmgr.MCPToolInfo, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]mcpmgr.MCPToolInfo, error) {
		return ctrl.GetMCPServerTools(slug), nil
	})
}

// GetMCPServerConfig retorna a configuração de um servidor.
func (m *MCP) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*mcpmgr.ServerConfig, error) {
		return ctrl.GetMCPServerConfig(slug)
	})
}

// ReadMCPResource lê um resource MCP.
func (m *MCP) ReadMCPResource(slug, uri string) (string, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.ReadMCPResource(slug, uri)
	})
}

// GetMCPPrompt obtém um prompt MCP com argumentos.
func (m *MCP) GetMCPPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.GetMCPPrompt(slug, name, arguments)
	})
}

// SetMCPWorkspaceRoots define as roots do workspace MCP.
func (m *MCP) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetMCPWorkspaceRoots(roots)
	})
	return err
}

// GetMCPWorkspaceRoots lista as roots do workspace MCP.
func (m *MCP) GetMCPWorkspaceRoots() ([]mcpmgr.Root, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]mcpmgr.Root, error) {
		return ctrl.GetMCPWorkspaceRoots(), nil
	})
}

// SubscribeToMCPResource assina notificações de um resource.
func (m *MCP) SubscribeToMCPResource(slug, uri string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SubscribeToMCPResource(slug, uri)
	})
	return err
}

// UnsubscribeFromMCPResource cancela a assinatura de um resource.
func (m *MCP) UnsubscribeFromMCPResource(slug, uri string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UnsubscribeFromMCPResource(slug, uri)
	})
	return err
}

// SaveMCPServerAuth grava autenticação do servidor MCP.
func (m *MCP) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveMCPServerAuth(slug, authType, token, username, password, clientSecret)
	})
	return err
}

// DeleteMCPServerAuth remove autenticação do servidor MCP.
func (m *MCP) DeleteMCPServerAuth(slug string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteMCPServerAuth(slug)
	})
	return err
}

// GetMCPServerAuthInfo retorna se há auth configurada (sem segredos).
func (m *MCP) GetMCPServerAuthInfo(slug string) (apidto.MCPServerAuthInfo, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return apidto.MCPServerAuthInfo{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.MCPServerAuthInfo, error) {
		return ctrl.GetMCPServerAuthInfo(slug)
	})
}

// DiscoverMCPServerAuth descobre endpoints OAuth de um servidor remoto.
func (m *MCP) DiscoverMCPServerAuth(serverURL string) (mcpmgr.OAuthDiscoveryResult, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return mcpmgr.OAuthDiscoveryResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (mcpmgr.OAuthDiscoveryResult, error) {
		return ctrl.DiscoverMCPServerAuth(serverURL), nil
	})
}

// GetMCPServerLogs retorna logs recentes do servidor.
func (m *MCP) GetMCPServerLogs(slug string, limit int) ([]mcpmgr.MCPServerLog, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]mcpmgr.MCPServerLog, error) {
		return ctrl.GetMCPServerLogs(slug, limit)
	})
}

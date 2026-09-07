package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// NetTrust é o bind Wails do domínio nettrust / network allowlist (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type NetTrust struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.NetTrustController
}

// NewNetTrust cria o bind vazio; AttachNetTrust preenche session + controller no startup.
func NewNetTrust() *NetTrust {
	return &NetTrust{}
}

// AttachNetTrust associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachNetTrust(n *NetTrust, session Session, ctrl *controllers.NetTrustController) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.session = session
	n.ctrl = ctrl
}

func (n *NetTrust) deps() (Session, *controllers.NetTrustController, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.session == nil || n.ctrl == nil {
		return nil, nil, ErrNetTrustNotWired
	}
	return n.session, n.ctrl, nil
}

// GetNetworkAllowlist lista as entradas de allowlist de rede.
func (n *NetTrust) GetNetworkAllowlist() ([]apidto.NetworkAllowlistView, error) {
	session, ctrl, err := n.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.NetworkAllowlistView, error) {
		return ctrl.GetNetworkAllowlist(ctx), nil
	})
}

// RemoveNetworkAllowlistEntry remove uma entrada persistida por (scope, host, port).
func (n *NetTrust) RemoveNetworkAllowlistEntry(scope, host, port string) error {
	session, ctrl, err := n.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RemoveNetworkAllowlistEntry(ctx, scope, host, port)
	})
	return err
}

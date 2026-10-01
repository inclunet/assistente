package mcp

import (
	"assistente/internal/oauthflow"
	"context"
	"net"
)

func (m *Manager) SetOAuthNetworkAuthorizer(authorize oauthflow.NetworkAuthorizer) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()
	m.networkAuthorizer = authorize
}
func (m *Manager) authorizeOAuthNetwork(ctx context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
	m.networkMu.RLock()
	authorize := m.networkAuthorizer
	m.networkMu.RUnlock()
	if authorize == nil {
		return nil, false, nil
	}
	return authorize(ctx, destination)
}
func (m *Manager) DiscoverOAuth(ctx context.Context, resource string) OAuthDiscoveryResult {
	return DiscoverOAuthContext(oauthflow.WithNetworkAuthorizer(ctx, m.authorizeOAuthNetwork), resource)
}

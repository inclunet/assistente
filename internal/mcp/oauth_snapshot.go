package mcp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func (m *Manager) oauthSnapshotDirectory() (string, error) {
	if m.snapshotRoot != "" {
		return m.snapshotRoot, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", credentials.ErrSnapshot
	}
	// Deliberately outside .assistente: exports/config synchronization never see it.
	return filepath.Join(home, ".assistente-oauth-recovery"), nil
}
func (m *Manager) CreateOAuthSnapshot(ctx context.Context, consumerID string) (credentials.OAuthSnapshotInfo, error) {
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return credentials.OAuthSnapshotInfo{}, credentials.ErrSnapshot
	}
	return m.credMgr.CreateLegacyOAuthSnapshot(ctx, dir, consumerID)
}
func (m *Manager) ListOAuthSnapshots(ctx context.Context) ([]credentials.OAuthSnapshotInfo, error) {
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return nil, credentials.ErrSnapshot
	}
	return m.credMgr.ListLegacyOAuthSnapshots(ctx, dir)
}
func (m *Manager) RestoreOAuthSnapshot(ctx context.Context, id string) error {
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return credentials.ErrSnapshot
	}
	var slug string
	var restored ServerConfig
	cached := map[string]ServerConfig{}
	m.mu.RLock()
	for key, status := range m.servers {
		cached[key] = status.Config
	}
	m.mu.RUnlock()
	err = m.credMgr.RestoreLegacyOAuthSnapshot(ctx, dir, id, func(row database.MCPServer) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if current := m.servers[row.Slug]; current != nil && !reflect.DeepEqual(persistedLegacyConfig(current.Config), persistedLegacyConfig(cached[row.Slug])) {
			return
		}
		slug = row.Slug
		m.servers[slug] = &ServerStatus{ID: restored.ID, Slug: slug, Config: restored, Status: StatusDisconnected, NeedsReauth: true, Tools: []MCPToolInfo{}}
	}, func(row database.MCPServer) error {
		var convertErr error
		restored, convertErr = serverModelToConfig(row)
		return convertErr
	})
	if err != nil {
		return err
	}
	if slug != "" {
		_ = m.Disconnect(slug)
	}
	m.emit("mcp:config_changed", map[string]string{"slug": slug})
	return nil
}
func (m *Manager) DiscardOAuthSnapshot(ctx context.Context, id string, confirmed bool) error {
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return credentials.ErrSnapshot
	}
	return m.credMgr.DiscardLegacyOAuthSnapshot(ctx, dir, id, confirmed)
}

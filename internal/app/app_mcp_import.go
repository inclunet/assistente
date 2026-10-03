package app

import (
	"context"
	"errors"

	"assistente/internal/database"
)

func (a *App) reloadMCPAfterImport(ctx context.Context) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	// Login/logout own this same lock. Do not publish a previous user's import
	// into the runtime after the active identity has changed.
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()
	a.authMu.RLock()
	current := a.currentUserID
	a.authMu.RUnlock()
	if current != user || ctx.Err() != nil {
		return errors.New("mcp_import_session_changed")
	}
	if a.mcpMgr == nil {
		return errors.New("mcp_runtime_unavailable")
	}
	return a.mcpMgr.LoadConfigs()
}

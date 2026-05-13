package app

import (
	"assistente/internal/allowlist"
)

// ============================================================================
// Allowlist Management API
// ============================================================================

func (a *App) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	return a.allowlistCtrl.RespondQuestionnaire(requestID, answers, cancelled)
}

func (a *App) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	return a.allowlistCtrl.GetAllowlists()
}

func (a *App) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	return a.allowlistCtrl.GetAllowlist(slug)
}

func (a *App) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	return a.allowlistCtrl.CreateAllowlist(al)
}

func (a *App) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	return a.allowlistCtrl.UpdateAllowlist(slug, al)
}

func (a *App) DeleteAllowlist(slug string) error {
	return a.allowlistCtrl.DeleteAllowlist(slug)
}

func (a *App) GetAllowlistSearchPaths() []string {
	return a.allowlistCtrl.GetAllowlistSearchPaths()
}

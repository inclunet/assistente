package main

import (
	"fmt"

	"assistente/internal/allowlist"
)

// ============================================================================
// Allowlist Management API
// ============================================================================

// RespondQuestionnaire responde a uma solicitação de questionário.
// Chamado pelo frontend quando o usuário envia ou cancela o questionário.
func (a *App) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	if a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}
	return a.questionnaireMgr.Respond(requestID, answers, cancelled)
}

// GetAllowlists retorna a lista de allowlists disponíveis.
func (a *App) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.List()
}

// GetAllowlist retorna uma allowlist pelo slug.
func (a *App) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Get(slug)
}

// CreateAllowlist cria uma nova allowlist.
func (a *App) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	if a.allowlistMgr == nil {
		return "", fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Create(&al)
}

// UpdateAllowlist atualiza uma allowlist existente.
func (a *App) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Update(slug, &al)
}

// DeleteAllowlist exclui uma allowlist.
func (a *App) DeleteAllowlist(slug string) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Delete(slug)
}

// GetAllowlistSearchPaths retorna os caminhos de busca de allowlists.
func (a *App) GetAllowlistSearchPaths() []string {
	if a.allowlistMgr == nil {
		return []string{}
	}
	return a.allowlistMgr.GetSearchPaths()
}

package controllers

import (
	"fmt"

	"assistente/internal/allowlist"
	"assistente/internal/questionnaire"
)

// AllowlistControllerConfig agrupa as dependências do AllowlistController.
type AllowlistControllerConfig struct {
	AllowlistMgr     *allowlist.Manager
	QuestionnaireMgr *questionnaire.Manager
}

// AllowlistController é o adapter primário (Inbound) para operações de allowlists e questionários.
type AllowlistController struct {
	allowlistMgr     *allowlist.Manager
	questionnaireMgr *questionnaire.Manager
}

// NewAllowlistController cria um AllowlistController com suas dependências.
func NewAllowlistController(cfg AllowlistControllerConfig) *AllowlistController {
	return &AllowlistController{
		allowlistMgr:     cfg.AllowlistMgr,
		questionnaireMgr: cfg.QuestionnaireMgr,
	}
}

// RespondQuestionnaire responde a uma solicitação de questionário.
func (c *AllowlistController) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	if c.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}
	return c.questionnaireMgr.Respond(requestID, answers, cancelled)
}

// GetAllowlists retorna a lista de allowlists disponíveis.
func (c *AllowlistController) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	if c.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.List()
}

// GetAllowlist retorna uma allowlist pelo slug.
func (c *AllowlistController) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	if c.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Get(slug)
}

// CreateAllowlist cria uma nova allowlist.
func (c *AllowlistController) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	if c.allowlistMgr == nil {
		return "", fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Create(&al)
}

// UpdateAllowlist atualiza uma allowlist existente.
func (c *AllowlistController) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	if c.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Update(slug, &al)
}

// DeleteAllowlist exclui uma allowlist.
func (c *AllowlistController) DeleteAllowlist(slug string) error {
	if c.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Delete(slug)
}

// GetAllowlistSearchPaths retorna os caminhos de busca de allowlists.
func (c *AllowlistController) GetAllowlistSearchPaths() []string {
	if c.allowlistMgr == nil {
		return []string{}
	}
	return c.allowlistMgr.GetSearchPaths()
}

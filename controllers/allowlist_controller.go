package controllers

import (
	"context"
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
func (c *AllowlistController) GetAllowlists(ctx context.Context) ([]allowlist.AllowlistInfo, error) {
	if c.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.List(ctx)
}

// GetAllowlist retorna uma allowlist pelo slug.
func (c *AllowlistController) GetAllowlist(ctx context.Context, slug string) (*allowlist.Allowlist, error) {
	if c.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Get(ctx, slug)
}

// CreateAllowlist cria uma nova allowlist.
func (c *AllowlistController) CreateAllowlist(ctx context.Context, al allowlist.Allowlist) (string, error) {
	if c.allowlistMgr == nil {
		return "", fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Create(ctx, &al)
}

// UpdateAllowlist atualiza uma allowlist existente.
func (c *AllowlistController) UpdateAllowlist(ctx context.Context, slug string, al allowlist.Allowlist) error {
	if c.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Update(ctx, slug, &al)
}

// DeleteAllowlist exclui uma allowlist.
func (c *AllowlistController) DeleteAllowlist(ctx context.Context, slug string) error {
	if c.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return c.allowlistMgr.Delete(ctx, slug)
}

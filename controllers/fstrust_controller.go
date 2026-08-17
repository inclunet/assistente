package controllers

import (
	"context"
	"fmt"

	"assistente/internal/apidto"
	"assistente/internal/fstrust"
	"assistente/internal/profiles"
	"assistente/internal/tools/invocationctx"
)

// PathAllowlistView — alias estável durante a migração Strangler (AEP-0088 D5).
type PathAllowlistView = apidto.PathAllowlistView

// FSTrustControllerConfig agrupa dependências do FSTrustController.
type FSTrustControllerConfig struct {
	FSTrustMgr *fstrust.Manager
	ProfileMgr *profiles.Manager
}

// FSTrustController expõe a gestão da allowlist de paths fora do sandbox (AEP-0092).
type FSTrustController struct {
	fsTrustMgr *fstrust.Manager
	profileMgr *profiles.Manager
}

// NewFSTrustController cria um FSTrustController com as dependências fornecidas.
func NewFSTrustController(cfg FSTrustControllerConfig) *FSTrustController {
	return &FSTrustController{
		fsTrustMgr: cfg.FSTrustMgr,
		profileMgr: cfg.ProfileMgr,
	}
}

// managementContext injeta a identidade (perfil ativo) no ctx para que os
// escopos de perfil sejam considerados nas operações de gestão chamadas pela UI.
func (c *FSTrustController) managementContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.profileMgr != nil {
		if slug := c.profileMgr.GetActiveSlug(); slug != "" {
			ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ProfileSlug: slug})
		}
	}
	return ctx
}

// GetPathAllowlist lista as entradas de allowlist de path (workspace, global e
// perfil ativo). Escopos efêmeros (once/session) não aparecem — a API de gestão
// não injeta ConversationID.
func (c *FSTrustController) GetPathAllowlist(ctx context.Context) []apidto.PathAllowlistView {
	if c.fsTrustMgr == nil {
		return nil
	}
	entries := c.fsTrustMgr.List(c.managementContext(ctx))
	views := make([]apidto.PathAllowlistView, 0, len(entries))
	for _, e := range entries {
		views = append(views, apidto.PathAllowlistView{
			Path:      e.Path,
			Kind:      string(e.Kind),
			Operation: e.Operation,
			Scope:     string(e.Scope),
			CreatedBy: e.CreatedBy,
			CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Reason:    e.Reason,
		})
	}
	return views
}

// RemovePathAllowlistEntry remove uma entrada persistida por (scope, path, kind, operation).
// Aceita apenas escopos PERSISTIDOS (workspace/profile/global): once nunca é
// persistido e session vive em memória por conversa, sem relação com esta API de
// gestão — passar esses valores retorna erro de escopo inválido direto.
func (c *FSTrustController) RemovePathAllowlistEntry(ctx context.Context, scope, path, kind, operation string) error {
	if c.fsTrustMgr == nil {
		return fmt.Errorf("gerenciador de allowlist de path não inicializado")
	}
	s := fstrust.Scope(scope)
	if !s.IsPersistent() {
		return fmt.Errorf("escopo inválido para remoção: %q (use workspace, profile ou global)", scope)
	}
	k := fstrust.Kind(kind)
	if !fstrust.ValidKind(k) {
		return fmt.Errorf("kind inválido para remoção: %q (use file ou dir)", kind)
	}
	return c.fsTrustMgr.Remove(c.managementContext(ctx), s, path, k, operation)
}

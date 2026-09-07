package controllers

import (
	"context"
	"fmt"

	"assistente/internal/apidto"
	"assistente/internal/nettrust"
	"assistente/internal/profiles"
	"assistente/internal/tools/invocationctx"
)

// NetworkAllowlistView — alias estável durante a migração Strangler (AEP-0088 D5).
type NetworkAllowlistView = apidto.NetworkAllowlistView

// NetTrustControllerConfig agrupa dependências do NetTrustController.
type NetTrustControllerConfig struct {
	NetTrustMgr *nettrust.Manager
	ProfileMgr  *profiles.Manager
}

// NetTrustController expõe a gestão da allowlist de rede (anti-SSRF escopável).
type NetTrustController struct {
	netTrustMgr *nettrust.Manager
	profileMgr  *profiles.Manager
}

// NewNetTrustController cria um NetTrustController com as dependências fornecidas.
func NewNetTrustController(cfg NetTrustControllerConfig) *NetTrustController {
	return &NetTrustController{
		netTrustMgr: cfg.NetTrustMgr,
		profileMgr:  cfg.ProfileMgr,
	}
}

// managementContext injeta a identidade (perfil ativo) no ctx para que os
// escopos de perfil sejam considerados nas operações de gestão chamadas pela UI.
func (c *NetTrustController) managementContext(ctx context.Context) context.Context {
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

// GetNetworkAllowlist lista as entradas de allowlist de rede (workspace, global e
// perfil ativo).
func (c *NetTrustController) GetNetworkAllowlist(ctx context.Context) []apidto.NetworkAllowlistView {
	if c.netTrustMgr == nil {
		return nil
	}
	entries := c.netTrustMgr.List(c.managementContext(ctx))
	views := make([]apidto.NetworkAllowlistView, 0, len(entries))
	for _, e := range entries {
		views = append(views, apidto.NetworkAllowlistView{
			Host:        e.Host,
			Port:        e.Port,
			Scope:       string(e.Scope),
			Category:    e.Category,
			ResolvedIPs: e.ResolvedIPs,
			CreatedBy:   e.CreatedBy,
			CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Reason:      e.Reason,
		})
	}
	return views
}

// RemoveNetworkAllowlistEntry remove uma entrada persistida por (scope, host, port).
// Aceita apenas escopos PERSISTIDOS (workspace/profile/global): once nunca é
// persistido e session vive em memória por conversa, sem relação com esta API de
// gestão — passar esses valores retorna erro de escopo inválido direto, em vez de
// um erro confuso vindo do Manager.
func (c *NetTrustController) RemoveNetworkAllowlistEntry(ctx context.Context, scope, host, port string) error {
	if c.netTrustMgr == nil {
		return fmt.Errorf("gerenciador de allowlist de rede não inicializado")
	}
	s := nettrust.Scope(scope)
	if !s.IsPersistent() {
		return fmt.Errorf("escopo inválido para remoção: %q (use workspace, profile ou global)", scope)
	}
	return c.netTrustMgr.Remove(c.managementContext(ctx), s, host, port)
}

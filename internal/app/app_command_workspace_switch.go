package app

import (
	"context"
	"time"

	"assistente/internal/logging"
)

// A troca de workspace já foi confirmada pelo Manager. Uma falha de comandos
// não desfaz essa troca, mas deixa seu catálogo indisponível. O controller
// serializa trocas e só emite workspace:switched depois desta reconstrução.
// Não roda em troca de aba e não mantém authMu/DispatchGate durante o bootstrap.
func (a *App) reloadCommandsAfterWorkspaceSwitch() {
	if a == nil || a.commandProduct.Load() == nil {
		return
	}
	// Primeiro retire a publicação: montar o produto novo sobre o mapa pronto
	// do workspace anterior abriria uma janela de resolução com deltas errados.
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	err := a.resetCommandLifecycleIfConfigured(cleanup, "workspace_switch")
	cancel()
	if err != nil {
		logging.Errorf(context.Background(), "app.app-workspace", "comandos indisponíveis após troca de workspace: falha ao retirar mapa anterior: %v", err)
		return
	}
	ctx := a.commandBridgeContext()
	if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		logging.Warnf(ctx, "app.app-workspace", "comandos indisponíveis após troca de workspace: montagem: %v", err)
		return
	}
	if err := a.reconcileCommandLifecycleClaimsForTransition(ctx, false, true); err != nil {
		logging.Warnf(ctx, "app.app-workspace", "comandos indisponíveis após troca de workspace: restauração: %v", err)
		return
	}
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		logging.Warnf(ctx, "app.app-workspace", "comandos indisponíveis após troca de workspace: configuração: %v", err)
		return
	}
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		logging.Warnf(ctx, "app.app-workspace", "comandos indisponíveis após troca de workspace: publicação: %v", err)
	}
}

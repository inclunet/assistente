package app

import (
	"assistente/internal/channels"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/portability"
	"context"
)

// runPostLoginLegacyImports executa todos os imports somente-leitura FS→DB que
// exigem usuário autenticado. Os managers de runtime devem carregar do DB só
// depois desta fase.
func (a *App) runPostLoginLegacyImports(ctx context.Context) {
	service := portability.NewLegacyImportService()
	if a.mcpMgr != nil {
		if err := service.Register("MCP", func(ctx context.Context) (portability.LegacyImportResult, error) {
			return portability.ImportLegacyMCPServersWithContext(ctx, a.mcpMgr.LegacyConfigSource(), a.credMgr)
		}); err != nil {
			logging.Errorf(ctx, "app.app-legacy-imports", "[LegacyImport] erro ao registrar importador MCP: %v", err)
		}
	}
	if a.jobMgr != nil {
		if err := service.Register("Jobs", func(ctx context.Context) (portability.LegacyImportResult, error) {
			return a.jobMgr.ImportLegacyDefinitions(ctx)
		}); err != nil {
			logging.Errorf(ctx, "app.app-legacy-imports", "[LegacyImport] erro ao registrar importador Jobs: %v", err)
		}
	}
	if err := service.Register("Channels", func(ctx context.Context) (portability.LegacyImportResult, error) {
		return channels.ImportLegacyChannelsWithContext(ctx, a.credMgr)
	}); err != nil {
		logging.Errorf(ctx, "app.app-legacy-imports", "[LegacyImport] erro ao registrar importador Channels: %v", err)
	}
	summary := service.Run(ctx)
	if userID, ok := database.UserIDFromContext(ctx); ok {
		summary.UserID = userID
	}
	for _, entry := range summary.Entries {
		for _, warning := range entry.Warnings {
			logging.Warnf(ctx, "app.app-legacy-imports", "[LegacyImport] %s: %s", entry.Name, warning)
		}
		for _, itemErr := range entry.Errors {
			logging.Errorf(ctx, "app.app-legacy-imports", "[LegacyImport] %s: %s", entry.Name, itemErr)
		}
		if entry.Imported > 0 || entry.Skipped > 0 || entry.Failed > 0 {
			if entry.Failed > 0 {
				logging.Warnf(ctx, "app.app-legacy-imports", "[LegacyImport] %s: %d importados, %d já existentes, %d falhas", entry.Name, entry.Imported, entry.Skipped, entry.Failed)
			} else {
				logging.Infof(ctx, "app.app-legacy-imports", "[LegacyImport] %s: %d importados, %d já existentes, %d falhas", entry.Name, entry.Imported, entry.Skipped, entry.Failed)
			}
		}
	}
	if a.emitter != nil && a.shouldEmitLegacyImportSummary(summary) {
		a.emitter.Emit(portability.LegacyImportSummaryEvent, summary)
	}
}

func (a *App) shouldEmitLegacyImportSummary(summary portability.LegacyImportSummary) bool {
	if len(summary.Entries) == 0 {
		return false
	}
	if summary.Imported > 0 || summary.Failed > 0 || summary.WarningCount > 0 || summary.ErrorCount > 0 {
		return true
	}
	if summary.Skipped == 0 {
		return false
	}
	if summary.UserID == "" {
		return true
	}
	a.legacyImportSummaryMu.Lock()
	defer a.legacyImportSummaryMu.Unlock()
	if a.legacyImportSkippedSummaryUserID == summary.UserID {
		return false
	}
	a.legacyImportSkippedSummaryUserID = summary.UserID
	return true
}

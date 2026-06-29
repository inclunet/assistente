package app

import (
	"context"
	"log"

	"assistente/internal/portability"
)

// runPostLoginLegacyImports runs all read-only filesystem-to-DB imports that
// require an authenticated user. Runtime managers should load from DB only
// after this phase has completed.
func (a *App) runPostLoginLegacyImports(ctx context.Context) {
	service := portability.NewLegacyImportService()
	if a.mcpMgr != nil {
		if err := service.Register("MCP", func(ctx context.Context) (portability.LegacyImportResult, error) {
			return portability.ImportLegacyMCPServersWithContext(ctx, a.mcpMgr.LegacyConfigSource(), a.credMgr)
		}); err != nil {
			log.Printf("[LegacyImport] erro ao registrar importador MCP: %v", err)
		}
	}
	if a.jobMgr != nil {
		if err := service.Register("Jobs", func(ctx context.Context) (portability.LegacyImportResult, error) {
			return a.jobMgr.ImportLegacyDefinitions(ctx)
		}); err != nil {
			log.Printf("[LegacyImport] erro ao registrar importador Jobs: %v", err)
		}
	}
	summary := service.Run(ctx)
	for _, entry := range summary.Entries {
		for _, warning := range entry.Warnings {
			log.Printf("[LegacyImport] %s: %s", entry.Name, warning)
		}
		for _, itemErr := range entry.Errors {
			log.Printf("[LegacyImport] %s: %s", entry.Name, itemErr)
		}
		if entry.Imported > 0 || entry.Skipped > 0 || entry.Failed > 0 {
			log.Printf("[LegacyImport] %s: %d importados, %d já existentes, %d falhas", entry.Name, entry.Imported, entry.Skipped, entry.Failed)
		}
	}
	if a.emitter != nil && len(summary.Entries) > 0 {
		a.emitter.Emit(portability.LegacyImportSummaryEvent, summary)
	}
}

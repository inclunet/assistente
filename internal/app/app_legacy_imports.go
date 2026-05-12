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
	type legacyImporter struct {
		name string
		run  func(context.Context) (portability.LegacyImportResult, error)
	}

	importers := []legacyImporter{}
	if a.mcpMgr != nil {
		importers = append(importers, legacyImporter{
			name: "MCP",
			run: func(ctx context.Context) (portability.LegacyImportResult, error) {
				return portability.ImportLegacyMCPServersWithContext(ctx, a.mcpMgr.LegacyConfigSource(), a.credMgr)
			},
		})
	}
	for _, importer := range importers {
		result, err := importer.run(ctx)
		if err != nil {
			log.Printf("[LegacyImport] erro ao importar recursos legados %s: %v", importer.name, err)
			continue
		}
		for _, warning := range result.Warnings {
			log.Printf("[LegacyImport] %s: %s", importer.name, warning)
		}
		if result.Imported > 0 || result.Skipped > 0 || result.Failed > 0 {
			log.Printf("[LegacyImport] %s: %d importados, %d já existentes, %d falhas", importer.name, result.Imported, result.Skipped, result.Failed)
		}
	}
}

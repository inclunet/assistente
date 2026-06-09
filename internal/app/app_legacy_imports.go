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
	if a.jobMgr != nil {
		importers = append(importers, legacyImporter{
			name: "Jobs",
			run: func(ctx context.Context) (portability.LegacyImportResult, error) {
				return a.jobMgr.ImportLegacyDefinitions(ctx)
			},
		})
	}
	// Skills só importam quando o Manager está em modo banco (Fase 6 em diante).
	// Antes do corte, HasRepository()==false e o importador é omitido.
	if a.skillMgr != nil && a.skillMgr.HasRepository() {
		importers = append(importers, legacyImporter{
			name: "Skills",
			run: func(ctx context.Context) (portability.LegacyImportResult, error) {
				return a.skillMgr.ImportLegacySkills(ctx)
			},
		})
	}
	summary := make([]LegacyImportSummaryEntry, 0, len(importers))
	for _, importer := range importers {
		result, err := importer.run(ctx)
		if err != nil {
			log.Printf("[LegacyImport] erro ao importar recursos legados %s: %v", importer.name, err)
			summary = append(summary, LegacyImportSummaryEntry{
				ResourceType: importer.name,
				Errors:       []string{err.Error()},
			})
			continue
		}
		for _, warning := range result.Warnings {
			log.Printf("[LegacyImport] %s: %s", importer.name, warning)
		}
		for _, itemErr := range result.Errors {
			log.Printf("[LegacyImport] %s: %s", importer.name, itemErr)
		}
		if result.Imported > 0 || result.Skipped > 0 || result.Failed > 0 {
			log.Printf("[LegacyImport] %s: %d importados, %d já existentes, %d falhas", importer.name, result.Imported, result.Skipped, result.Failed)
		}
		summary = append(summary, legacyImportSummaryEntry(importer.name, result))
	}

	// Observabilidade (#123): emite um sumário estruturado único para a UI.
	if a.emitter != nil && len(summary) > 0 {
		a.emitter.Emit("legacy:import_summary", summary)
	}
}

// LegacyImportSummaryEntry é a projeção observável do resultado de importação de
// um tipo de recurso legado (#123).
type LegacyImportSummaryEntry struct {
	ResourceType string   `json:"resourceType"`
	Imported     int      `json:"imported"`
	Skipped      int      `json:"skipped"`
	Failed       int      `json:"failed"`
	Warnings     []string `json:"warnings,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

// legacyImportSummaryEntry projeta um LegacyImportResult na entrada de sumário,
// usando o nome do importador como rótulo do tipo de recurso.
func legacyImportSummaryEntry(name string, result portability.LegacyImportResult) LegacyImportSummaryEntry {
	entry := LegacyImportSummaryEntry{
		ResourceType: name,
		Imported:     result.Imported,
		Skipped:      result.Skipped,
		Failed:       result.Failed,
	}
	if len(result.Warnings) > 0 {
		entry.Warnings = result.Warnings
	}
	if len(result.Errors) > 0 {
		entry.Errors = result.Errors
	}
	return entry
}

package portability

import (
	"context"
	"fmt"
	"strings"
)

const LegacyImportSummaryEvent = "legacy:import_summary"

type LegacyImporterFunc func(context.Context) (LegacyImportResult, error)

type LegacyImporter struct {
	Name string
	Run  LegacyImporterFunc
}

type LegacyImportSummaryEntry struct {
	Name         string   `json:"name"`
	ResourceType string   `json:"resourceType"`
	Imported     int      `json:"imported"`
	Skipped      int      `json:"skipped"`
	Failed       int      `json:"failed"`
	Warnings     []string `json:"warnings,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type LegacyImportSummary struct {
	UserID       string                     `json:"userId,omitempty"`
	Entries      []LegacyImportSummaryEntry `json:"entries"`
	Imported     int                        `json:"imported"`
	Skipped      int                        `json:"skipped"`
	Failed       int                        `json:"failed"`
	WarningCount int                        `json:"warningCount"`
	ErrorCount   int                        `json:"errorCount"`
}

type LegacyImportService struct {
	importers []LegacyImporter
}

func NewLegacyImportService() *LegacyImportService {
	return &LegacyImportService{}
}

func (s *LegacyImportService) Register(name string, run LegacyImporterFunc) error {
	if s == nil {
		return fmt.Errorf("serviço de importação legada é obrigatório")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nome do importador legado é obrigatório")
	}
	if run == nil {
		return fmt.Errorf("handler do importador legado %s é obrigatório", name)
	}
	s.importers = append(s.importers, LegacyImporter{Name: name, Run: run})
	return nil
}

func (s *LegacyImportService) Run(ctx context.Context) LegacyImportSummary {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return LegacyImportSummary{Entries: []LegacyImportSummaryEntry{}}
	}
	summary := LegacyImportSummary{
		Entries: make([]LegacyImportSummaryEntry, 0, len(s.importers)),
	}
	for _, importer := range s.importers {
		result, err := importer.Run(ctx)
		entry := LegacyImportSummaryEntry{
			Name:         importer.Name,
			ResourceType: result.ResourceType,
			Imported:     result.Imported,
			Skipped:      result.Skipped,
			Failed:       result.Failed,
			Warnings:     append([]string(nil), result.Warnings...),
			Errors:       append([]string(nil), result.Errors...),
		}
		if strings.TrimSpace(entry.ResourceType) == "" {
			entry.ResourceType = importer.Name
		}
		if err != nil {
			entry.Failed++
			entry.Errors = append(entry.Errors, fmt.Sprintf("erro ao executar importador %s: %v", importer.Name, err))
		}
		summary.add(entry)
	}
	return summary
}

func (s *LegacyImportSummary) add(entry LegacyImportSummaryEntry) {
	s.Entries = append(s.Entries, entry)
	s.Imported += entry.Imported
	s.Skipped += entry.Skipped
	s.Failed += entry.Failed
	s.WarningCount += len(entry.Warnings)
	s.ErrorCount += len(entry.Errors)
}

package portability

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLegacyImportServiceAggregatesImporters(t *testing.T) {
	svc := NewLegacyImportService()
	if err := svc.Register("MCP", func(context.Context) (LegacyImportResult, error) {
		return LegacyImportResult{
			ResourceType: "servidor MCP",
			Imported:     2,
			Skipped:      1,
			Warnings:     []string{"configuração sem descrição"},
		}, nil
	}); err != nil {
		t.Fatalf("Register MCP: %v", err)
	}
	if err := svc.Register("Jobs", func(context.Context) (LegacyImportResult, error) {
		return LegacyImportResult{
			ResourceType: "jobs",
			Failed:       1,
			Errors:       []string{"erro ao parsear jobs legado exemplo.json"},
		}, nil
	}); err != nil {
		t.Fatalf("Register Jobs: %v", err)
	}

	summary := svc.Run(context.Background())

	if len(summary.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(summary.Entries))
	}
	if summary.Imported != 2 || summary.Skipped != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if summary.WarningCount != 1 || summary.ErrorCount != 1 {
		t.Fatalf("unexpected diagnostic totals: %+v", summary)
	}
	if summary.Entries[0].ResourceType != "servidor MCP" {
		t.Fatalf("resource type not preserved: %+v", summary.Entries[0])
	}
}

func TestLegacyImportServiceContinuesAfterImporterError(t *testing.T) {
	svc := NewLegacyImportService()
	if err := svc.Register("MCP", func(context.Context) (LegacyImportResult, error) {
		return LegacyImportResult{ResourceType: "servidor MCP"}, errors.New("falha de leitura")
	}); err != nil {
		t.Fatalf("Register MCP: %v", err)
	}
	if err := svc.Register("Jobs", func(context.Context) (LegacyImportResult, error) {
		return LegacyImportResult{ResourceType: "jobs", Imported: 1}, nil
	}); err != nil {
		t.Fatalf("Register Jobs: %v", err)
	}

	summary := svc.Run(context.Background())

	if summary.Imported != 1 || summary.Failed != 1 || summary.ErrorCount != 1 {
		t.Fatalf("unexpected summary after partial failure: %+v", summary)
	}
	if len(summary.Entries) != 2 {
		t.Fatalf("expected both importers to run, got %d entries", len(summary.Entries))
	}
	if !strings.Contains(summary.Entries[0].Errors[0], "falha de leitura") {
		t.Fatalf("expected runner error in diagnostics, got %+v", summary.Entries[0].Errors)
	}
}

func TestLegacyImportServiceRejectsInvalidImporter(t *testing.T) {
	svc := NewLegacyImportService()
	if err := svc.Register("", func(context.Context) (LegacyImportResult, error) {
		return LegacyImportResult{}, nil
	}); err == nil {
		t.Fatal("expected error for empty importer name")
	}
	if err := svc.Register("MCP", nil); err == nil {
		t.Fatal("expected error for nil importer handler")
	}
}

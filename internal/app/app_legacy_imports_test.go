package app

import (
	"testing"

	"assistente/internal/portability"
)

func TestLegacyImportSummaryEntry(t *testing.T) {
	res := portability.LegacyImportResult{
		ResourceType: "skills",
		Imported:     3,
		Skipped:      1,
		Failed:       0,
		Warnings:     []string{"skills foo: sem frase-gatilho"},
		Errors:       nil,
	}

	entry := legacyImportSummaryEntry("Skills", res)

	if entry.ResourceType != "Skills" {
		t.Errorf("resourceType: got %q want Skills", entry.ResourceType)
	}
	if entry.Imported != 3 || entry.Skipped != 1 || entry.Failed != 0 {
		t.Errorf("contagens: %+v", entry)
	}
	if len(entry.Warnings) != 1 {
		t.Errorf("warnings: got %d want 1", len(entry.Warnings))
	}
	if entry.Errors != nil {
		t.Errorf("errors deveria ser nil quando vazio, got %v", entry.Errors)
	}
}

func TestLegacyImportSummaryEntryOmitsEmptySlices(t *testing.T) {
	entry := legacyImportSummaryEntry("MCP", portability.LegacyImportResult{Imported: 1})
	if entry.Warnings != nil || entry.Errors != nil {
		t.Errorf("slices vazias deveriam ser nil: %+v", entry)
	}
}

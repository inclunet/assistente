package app

import (
	"context"
	"testing"

	"assistente/internal/commandportability"
)

func TestCommandDesktopImportOutcomePreservesCommittedReport(t *testing.T) {
	for _, cause := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		batch := commandMutationBatchResult{Committed: true, Report: &commandportability.ImportReport{
			Layers: []commandportability.ImportLayerReport{{TargetID: "persisted-copy", Action: commandportability.CopyMode}},
		}}
		result, err := commandDesktopImportOutcome(batch, nil, cause)
		if err != nil || result == nil || result.Success || result.Imported != 1 {
			t.Fatalf("commit com rebuild recusado: %+v %v", result, err)
		}
		found := false
		for _, warning := range result.Warnings {
			if warning.Code == "commandImport.committedNotPublished" {
				found = true
			}
		}
		if !found {
			t.Fatal("estado pós-commit não informado")
		}
	}
	result, err := commandDesktopImportOutcome(commandMutationBatchResult{}, nil, context.Canceled)
	if result != nil || err != context.Canceled {
		t.Fatalf("cancelamento antes do commit: %+v %v", result, err)
	}
}

package app

import (
	"testing"

	"assistente/internal/commandcatalog"
)

func TestEditorFileCommandsHaveContextualBackendContracts(t *testing.T) {
	for _, id := range []string{commandEditorFileOpenID, commandEditorFileSaveID, commandEditorFileSaveCopyID} {
		definition, handler := editorFileCommandRegistration(id)
		if definition.ID != id || definition.Effect != commandcatalog.Write || !definition.HasMutableTarget {
			t.Fatalf("%s: definição de escrita contextual incompleta: %+v", id, definition)
		}
		if definition.HandlerClassification != commandcatalog.HandlerBackend || handler.Classification != commandcatalog.HandlerBackend {
			t.Fatalf("%s: handler não é backend: definition=%+v handler=%+v", id, definition, handler)
		}
		if definition.Context.None || len(definition.Context.Facts) != 1 || definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
			t.Fatalf("%s: alvo não exige active_tab ExactVersion: %+v", id, definition.Context)
		}
	}
}

func TestEditorFileKeyboardDefaultsUseBackendSnapshotGuard(t *testing.T) {
	want := map[string]bool{
		commandEditorFileOpenID:     true,
		commandEditorFileSaveID:     true,
		commandEditorFileSaveCopyID: true,
	}
	for _, spec := range commandKeyboardDefaultSpecs {
		if !want[spec.CommandID] {
			continue
		}
		if spec.Contextual || spec.Condition != nil {
			t.Fatalf("default %s não deve exigir fatos DOM: %+v", spec.CommandID, spec)
		}
		delete(want, spec.CommandID)
	}
	if len(want) != 0 {
		t.Fatalf("defaults ausentes: %v", want)
	}
}

package commandconfig

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandcatalog"
)

func TestProjectLocalReadGuardsAndCancellation(t *testing.T) {
	registry, err := commandcatalog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Scope: Scope{UserID: storeTestUUID7(t)}}
	options := LocalReadProjection{Registry: registry}
	if result, err := ProjectLocalRead(context.Background(), snapshot, options); err != nil || result == nil {
		t.Fatal("baseline vazia", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := ProjectLocalRead(cancelled, snapshot, options); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("contexto cancelado aceito", err)
	}
	if result, err := ProjectLocalRead(nil, snapshot, options); !errors.Is(err, ErrInvalid) || result != nil { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatal("contexto nil aceito", err)
	}
	if result, err := ProjectLocalRead(context.Background(), snapshot, LocalReadProjection{}); !errors.Is(err, ErrInvalid) || result != nil {
		t.Fatal("catálogo nil aceito", err)
	}
	workspace := snapshot
	workspace.Scope.WorkspaceID = storeTestPtr(storeTestUUID7(t))
	if result, err := ProjectLocalRead(context.Background(), workspace, options); !errors.Is(err, ErrInvalid) || result != nil {
		t.Fatal("workspace não suportado aceito", err)
	}
	options.ActiveUserLayerIDs = []string{storeTestUUID7(t)}
	if result, err := ProjectLocalRead(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || result != nil {
		t.Fatal("ativação de camada inexistente aceita", err)
	}
}

func TestCanonicalLocalTriggerRequiresExactIdentity(t *testing.T) {
	for _, value := range []string{"keyboard.local:KeyK", "keyboard.local:Control+KeyK", "keyboard.local:Control+Alt+Shift+Meta+F24"} {
		if !canonicalLocalTrigger(value) {
			t.Errorf("identidade válida rejeitada: %s", value)
		}
	}
	for _, value := range []string{"", "keyboard.local:", "keyboard.global:KeyK", "keyboard.local:Shift+Control+KeyK", "keyboard.local:Control+Control+KeyK", "keyboard.local:F 1", "keyboard.local:F1 ", "keyboard.local:Control++KeyK", "keyboard.local:Key1"} {
		if canonicalLocalTrigger(value) {
			t.Errorf("identidade inválida aceita: %s", value)
		}
	}
}

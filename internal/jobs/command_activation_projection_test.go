package jobs

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandjobactivation"
)

func projectionManager(identity commandjobactivation.RuntimeIdentity, runID string, watchCtx context.Context) *Manager {
	m := &Manager{
		commandRuntime:          make(map[string]commandRuntimeEntry),
		commandRuntimeAccepting: true,
	}
	m.commandRuntime[runID] = commandRuntimeEntry{identity: identity, watchCtx: watchCtx}
	return m
}

func projectionIdentity() commandjobactivation.RuntimeIdentity {
	return commandjobactivation.RuntimeIdentity{
		Generation:         "generation-1",
		UserID:             "user-1",
		AuthContextType:    "local_session",
		AuthContextID:      "session-1",
		AuthGeneration:     "auth-1",
		SecurityGeneration: "security-1",
	}
}

func TestValidateCommandRuntimeProjectionRequiresExactLiveEntry(t *testing.T) {
	identity := projectionIdentity()
	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := projectionManager(identity, "run-1", watchCtx)

	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", identity); err != nil {
		t.Fatalf("projeção viva rejeitada: %v", err)
	}
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "missing", identity); err == nil {
		t.Fatal("run inexistente aceito")
	}
	stale := identity
	stale.Generation = "generation-2"
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", stale); err == nil {
		t.Fatal("geração diferente aceita")
	}
}

func TestValidateCommandRuntimeProjectionFailsClosedForContextAndWatch(t *testing.T) {
	identity := projectionIdentity()
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	m := projectionManager(identity, "run-1", watchCtx)

	cancelCtx, cancelCall := context.WithCancel(context.Background())
	cancelCall()
	if err := m.ValidateCommandRuntimeProjection(cancelCtx, "run-1", identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado = %v", err)
	}
	if err := m.ValidateCommandRuntimeProjection(nil, "run-1", identity); err == nil { //nolint:staticcheck // Prova a recusa explícita de contexto nil.
		t.Fatal("contexto nil aceito")
	}

	cancelWatch()
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", identity); err == nil {
		t.Fatal("watch cancelado aceito")
	}
}

func TestValidateCommandRuntimeProjectionStopUnregisterAndReregister(t *testing.T) {
	identity := projectionIdentity()
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	m := projectionManager(identity, "run-1", watchCtx)

	m.unregisterCommandRuntime("run-1")
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", identity); err == nil {
		t.Fatal("entrada removida aceita")
	}

	newIdentity := identity
	newIdentity.Generation = "generation-2"
	newWatchCtx, cancelNewWatch := context.WithCancel(context.Background())
	defer cancelNewWatch()
	m.commandRuntime["run-1"] = commandRuntimeEntry{identity: newIdentity, watchCtx: newWatchCtx}
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", identity); err == nil {
		t.Fatal("prova antiga aceita após re-registro")
	}
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", newIdentity); err != nil {
		t.Fatalf("prova nova rejeitada: %v", err)
	}

	m.invalidateCommandRuntimeTracking()
	if err := m.ValidateCommandRuntimeProjection(context.Background(), "run-1", newIdentity); err == nil {
		t.Fatal("projeção aceita após stop")
	}
	cancelWatch()
}

package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandcontext"
	"assistente/internal/commandforeground"
)

func TestCommandForegroundUnavailableDoesNotPublishFact(t *testing.T) {
	for _, captureErr := range []error{commandforeground.ErrUnavailable, commandforeground.ErrUnknown} {
		t.Run(captureErr.Error(), func(t *testing.T) {
			a, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
			reader := commandForegroundReaderFixture{err: captureErr}
			provider := &commandForegroundProvider{app: a, reader: reader, scopeBind: commandForegroundScopeBind(principal)}
			fact, err := provider.Snapshot(context.Background(), commandWorkspaceScope(principal, workspaceID), "foreground")
			if !errors.Is(err, commandcontext.ErrProviderUnavailable) || fact.Snapshot.Version != "" {
				t.Fatalf("adapter indisponível publicou fato: %+v, %v", fact, err)
			}
			product := &commandProductRuntime{app: a, principal: principal, workspaceMgr: manager, workspaceID: workspaceID, foregroundReader: reader}
			snapshot, err := product.capturePhysicalForeground(context.Background())
			if !errors.Is(err, captureErr) || snapshot != (commandforeground.Snapshot{}) {
				t.Fatalf("captura física disfarçou indisponibilidade: %+v, %v", snapshot, err)
			}
		})
	}
}

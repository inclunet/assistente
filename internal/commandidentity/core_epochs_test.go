package commandidentity

import (
	"assistente/internal/commandsecurity"
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestCoreEpochsUsesExecutorEpochAndRevocation(t *testing.T) {
	ctx := context.Background()
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewCoreEpochs(core)
	if err != nil {
		t.Fatal(err)
	}
	principal := ContextPrincipal{UserID: uuid.Must(uuid.NewV7()).String(), Type: "external_token", ID: "issuer-subject"}
	e, err := port.CaptureContextAuthenticated(ctx, func() (ContextPrincipal, error) { return principal, nil })
	if err != nil {
		t.Fatal(err)
	}
	shared, err := core.CaptureContextAuthenticated(ctx, func(context.Context) (commandsecurity.ContextPrincipal, error) {
		return commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: principal.Type, ID: principal.ID}, nil
	})
	if err != nil || shared.AuthGeneration != e.AuthGeneration || shared.SecurityGeneration != e.SecurityGeneration {
		t.Fatalf("domínio de epoch distinto: %v", err)
	}
	watch, release, err := core.WatchEpoch(ctx, shared)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := port.MutateContext(ctx, principal, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if watch.Err() == nil {
		t.Fatal("revogação não cancelou watch do executor")
	}
}

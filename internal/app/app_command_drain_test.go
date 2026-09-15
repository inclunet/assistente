package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandsecurity"
)

func TestAppShutdownPreservesDependenciesWhenExecutorDrainFails(t *testing.T) {
	a := &App{}
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	deactivated := false
	a.cancel = func() { deactivated = true }
	failure := errors.New("executor still finalizing")
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return failure }); err != nil {
		t.Fatal(err)
	}
	a.Shutdown()
	if deactivated {
		t.Fatal("App destruiu dependência antes da drenagem")
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("novo executor após shutdown=%v", err)
	}
}

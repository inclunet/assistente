package commandexecution

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
)

func TestHostChangeCancelsExecutionsBeforeWriterEvenOnFailure(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	other := auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}
	publishHostUserForChange(t, state, principal, configuration)
	publishHostUserForChange(t, state, other, configuration)
	watch := func(who auth.LocalSessionPrincipal) context.Context {
		t.Helper()
		epoch, err := state.Epochs().Capture(context.Background(), who.UserID, who.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		var execution context.Context
		release, err := state.Epochs().AdmitExecution(context.Background(), epoch,
			func(context.Context) error { return nil }, func(ctx context.Context) error { execution = ctx; return nil })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		return execution
	}
	executionA, executionB := watch(principal), watch(other)
	want := errors.New("falha SQLite simulada")
	err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(ctx context.Context) error {
			if !errors.Is(executionA.Err(), context.Canceled) {
				t.Fatal("writer iniciou antes do cancelamento")
			}
			if executionB.Err() != nil {
				t.Fatal("writer cancelou execução de outra conta")
			}
			if _, err := state.Snapshot(ctx, principal); !errors.Is(err, ErrHostUserNotPublished) {
				t.Fatal("writer iniciou com mapa antigo", err)
			}
			return want
		}, nil
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	if !errors.Is(executionA.Err(), context.Canceled) || executionB.Err() != nil {
		t.Fatal("falha de writer alterou cancelamento")
	}
	assertHostUserNotPublished(t, state, principal)
}

package commandexecution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
)

func TestHostChangeWithEpochEntregaSnapshotDoPrincipalAutenticado(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	var commitCalled atomic.Bool

	err := state.ChangeUserConfigurationWithEpoch(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(_ context.Context, gotPrincipal auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
		if gotPrincipal != principal {
			t.Fatalf("prepare recebeu principal diferente: got=%+v want=%+v", gotPrincipal, principal)
		}
		if epoch.UserID != principal.UserID || epoch.SessionID != principal.SessionID {
			t.Fatalf("epoch não corresponde ao principal: epoch=%+v principal=%+v", epoch, principal)
		}
		if epoch.AuthGeneration == "" || epoch.SecurityGeneration == "" {
			t.Fatalf("epoch sem gerações: %+v", epoch)
		}
		return func(context.Context) error {
			commitCalled.Store(true)
			return nil
		}, nil
	})
	if err != nil || !commitCalled.Load() {
		t.Fatalf("mudança com epoch = %v, commit=%v", err, commitCalled.Load())
	}
}

func TestHostChangeWithEpochPreparacaoForaDoGateEInvalidaEnquantoAguarda(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*HostState, auth.LocalSessionPrincipal) error
	}{
		{name: "segurança", mutate: func(state *HostState, _ auth.LocalSessionPrincipal) error {
			return state.SetVaultUnlocked(context.Background(), true)
		}},
		{name: "sessão", mutate: func(state *HostState, principal auth.LocalSessionPrincipal) error {
			return state.Epochs().InvalidateSession(context.Background(), principal.UserID, principal.SessionID)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, _ := readyHostForRebuild(t)
			var commitCalled atomic.Bool
			err := changeWithTimeout(t, func() error {
				return state.ChangeUserConfigurationWithEpoch(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
					return principal, nil
				}, func(_ context.Context, gotPrincipal auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
					if gotPrincipal != principal || epoch.UserID != principal.UserID || epoch.SessionID != principal.SessionID {
						return nil, errors.New("snapshot de preparação inconsistente")
					}
					if err := tt.mutate(state, principal); err != nil {
						return nil, err
					}
					return func(context.Context) error {
						commitCalled.Store(true)
						return nil
					}, nil
				})
			})
			if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
				t.Fatalf("mudança após invalidação durante prepare = %v, want stale", err)
			}
			if commitCalled.Load() {
				t.Fatal("commit executado após invalidação durante prepare")
			}
		})
	}
}

func changeWithTimeout(t *testing.T, fn func() error) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- fn() }()
	select {
	case err := <-result:
		return err
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("ChangeUserConfigurationWithEpoch não terminou no limite")
		return nil
	}
}

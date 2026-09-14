package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"assistente/internal/ossession"
)

func TestCommandHostObserverDropsMapsOnUnlockLockAndFailure(t *testing.T) {
	a, state, principal, config := newCommandHostAppFixture(t)
	ctx := context.Background()
	a.markCommandVaultUnlocked()
	failure := errors.New("fixture observer disconnected")
	err := a.observeCommandOSSession(ctx, func(_ context.Context, observe func(ossession.State) error) error {
		if _, _, err := state.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
			t.Fatal("monitor iniciou com mapa anterior", err)
		}
		for _, observed := range []ossession.State{{Known: true}, {Known: true, Locked: true}, {Known: true}} {
			if err := observe(observed); err != nil {
				t.Fatal(err)
			}
			if _, _, err := state.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				t.Fatal("evento preservou mapa", observed, err)
			}
			if !observed.Locked {
				if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
					return principal, nil
				}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
					return config, nil, nil
				}); err != nil {
					t.Fatal(err)
				}
				if snapshot, err := state.Snapshot(ctx, principal); err != nil || !snapshot.Unlocked {
					t.Fatal("reconstrução autenticada não liberou mapa", snapshot, err)
				}
			}
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, _, err := state.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("falha não removeu mapa", err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		t.Fatal("builder chamado depois de perder observação do SO")
		return config, nil, nil
	}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatal("falha deixou SO aberto", err)
	}
}

func TestCommandHostObserverCancellationStillClosesHost(t *testing.T) {
	a, state, principal, _ := newCommandHostAppFixture(t)
	a.markCommandVaultUnlocked()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := a.observeCommandOSSession(ctx, func(_ context.Context, observe func(ossession.State) error) error {
		if err := observe(ossession.State{Known: true}); err != nil {
			t.Fatal(err)
		}
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("cancelamento reteve mapa", err)
	}
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		t.Fatal("cancelamento deixou observação do SO desbloqueada")
		return nil, nil, nil
	}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatal("reconstrução admitida sem monitor", err)
	}
}

func TestCommandHostMonitorDoesNotStartWithoutLifecycle(t *testing.T) {
	empty := &App{}
	empty.ctx, empty.cancel = context.WithCancel(context.Background())
	defer empty.cancel()
	empty.authMu.Lock()
	empty.startCommandOSSessionMonitorLocked()
	empty.authMu.Unlock()
	if empty.commandOSStarted {
		t.Fatal("iniciou observador sem host")
	}
	a, _, _, _ := newCommandHostAppFixture(t)
	a.authMu.Lock()
	a.startCommandOSSessionMonitorLocked()
	a.authMu.Unlock()
	if a.commandOSStarted {
		t.Fatal("iniciou observador nativo sem lifecycle")
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.cancel()
	a.authMu.Lock()
	a.startCommandOSSessionMonitorLocked()
	a.authMu.Unlock()
	if a.commandOSStarted {
		t.Fatal("iniciou observador após cancelamento")
	}
}

func TestCommandHostMonitorStartsOnceAndJoinsOnShutdown(t *testing.T) {
	a, state, principal, _ := newCommandHostAppFixture(t)
	a.ctx, a.cancel = context.WithCancel(context.Background())
	defer a.cancel()
	started := make(chan struct{})
	watch := func(ctx context.Context, observe func(ossession.State) error) error {
		if err := observe(ossession.State{Known: true}); err != nil {
			return err
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	a.authMu.Lock()
	a.startCommandOSSessionMonitorWithLocked(watch)
	a.startCommandOSSessionMonitorWithLocked(watch)
	a.authMu.Unlock()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor não iniciou")
	}
	a.cancel()
	joined := make(chan struct{})
	go func() { a.bgWG.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor não acompanhou shutdown")
	}
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("shutdown reteve mapa", err)
	}
}

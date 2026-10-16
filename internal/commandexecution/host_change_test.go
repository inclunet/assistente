package commandexecution

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandsecurity"
)

func publishHostUserForChange(t *testing.T, state *HostState, principal auth.LocalSessionPrincipal, configuration *commandbindings.Configuration) {
	t.Helper()
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHostChangeGuardsAndNilCommit(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	commitCalled := false
	prepare := func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(context.Context) error { commitCalled = true; return nil }, nil
	}

	if err := state.ChangeUserConfiguration(nil, nil, nil); !errors.Is(err, ErrInvalidHostState) { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatalf("argumentos inválidos = %v", err)
	}
	if err := state.ChangeUserConfiguration(context.Background(), nil, prepare); !errors.Is(err, ErrInvalidHostState) {
		t.Fatalf("authenticate nil = %v", err)
	}
	if err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, nil); !errors.Is(err, ErrInvalidHostState) {
		t.Fatalf("prepare nil = %v", err)
	}
	if err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return nil, nil
	}); !errors.Is(err, ErrInvalidHostState) || commitCalled {
		t.Fatalf("commit nil = %v, chamado=%v", err, commitCalled)
	}

	if err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(context.Context) error { commitCalled = true; return nil }, nil
	}); err != nil {
		t.Fatalf("mudança válida = %v", err)
	}
	if !commitCalled {
		t.Fatal("commit válido não foi chamado")
	}
}

func TestHostChangePrepareForaDoGateEStalePorMutacaoConcorrente(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releasePrepare := func() { releaseOnce.Do(func() { close(release) }) }
	defer releasePrepare()
	var commitCalled atomic.Bool
	result := make(chan error, 1)

	go func() {
		result <- state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(ctx context.Context, _ auth.LocalSessionPrincipal) (func(context.Context) error, error) {
			close(entered)
			<-release
			return func(context.Context) error { commitCalled.Store(true); return nil }, nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("prepare não iniciou")
	}

	mutated := make(chan error, 1)
	go func() { mutated <- state.SetVaultUnlocked(context.Background(), true) }()
	select {
	case err := <-mutated:
		if err != nil {
			t.Fatalf("mutação concorrente = %v", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("mutação ficou bloqueada pelo prepare")
	}
	releasePrepare()
	select {
	case err := <-result:
		if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
			t.Fatalf("mudança após mutação = %v, want stale", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("mudança não terminou após mutação")
	}
	if commitCalled.Load() {
		t.Fatal("commit executado após prepare stale")
	}
}

func TestHostChangeLockLogoutEConfigChangeDurantePrepareNaoPublicam(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*HostState, auth.LocalSessionPrincipal) error
	}{
		{name: "lock/unlock", mutate: func(state *HostState, _ auth.LocalSessionPrincipal) error {
			if err := state.SetOSSessionState(context.Background(), true, true); err != nil {
				return err
			}
			return state.SetOSSessionState(context.Background(), true, false)
		}},
		{name: "logout", mutate: func(state *HostState, principal auth.LocalSessionPrincipal) error {
			return state.Epochs().MutatePrincipal(context.Background(), principal.UserID, principal.SessionID, func() error { return nil })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, configuration := readyHostForRebuild(t)
			publishHostUserForChange(t, state, principal, configuration)
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releasePrepare := func() { releaseOnce.Do(func() { close(release) }) }
			defer releasePrepare()
			var commitCalled atomic.Bool
			result := make(chan error, 1)
			go func() {
				result <- state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
					return principal, nil
				}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
					close(entered)
					<-release
					return func(context.Context) error { commitCalled.Store(true); return nil }, nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(hostRebuildTestTimeout):
				t.Fatal("prepare não iniciou")
			}
			mutated := make(chan error, 1)
			go func() { mutated <- tt.mutate(state, principal) }()
			select {
			case err := <-mutated:
				if err != nil {
					t.Fatalf("%s durante prepare = %v", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("%s ficou bloqueado pelo prepare", tt.name)
			}
			releasePrepare()
			select {
			case err := <-result:
				if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
					t.Fatalf("mudança após %s = %v, want stale", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("mudança não terminou após %s", tt.name)
			}
			if commitCalled.Load() {
				t.Fatal("commit executado após mutação concorrente")
			}
		})
	}

	t.Run("configuração", func(t *testing.T) {
		state, principal, configuration := readyHostForRebuild(t)
		publishHostUserForChange(t, state, principal, configuration)
		entered := make(chan struct{})
		release := make(chan struct{})
		var releaseOnce sync.Once
		releasePrepare := func() { releaseOnce.Do(func() { close(release) }) }
		defer releasePrepare()
		var commitCalled atomic.Bool
		result := make(chan error, 1)
		go func() {
			result <- state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
				return principal, nil
			}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
				close(entered)
				<-release
				return func(context.Context) error { commitCalled.Store(true); return nil }, nil
			})
		}()
		select {
		case <-entered:
		case <-time.After(hostRebuildTestTimeout):
			t.Fatal("prepare de configuração não iniciou")
		}
		mutated := make(chan error, 1)
		go func() { mutated <- state.SetActiveLayers(context.Background(), principal.UserID, []string{"changed"}) }()
		select {
		case err := <-mutated:
			if err != nil {
				t.Fatalf("configuração durante prepare = %v", err)
			}
		case <-time.After(hostRebuildTestTimeout):
			t.Fatal("mudança de configuração ficou bloqueada")
		}
		releasePrepare()
		select {
		case err := <-result:
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("mudança após configuração concorrente = %v, want denied", err)
			}
		case <-time.After(hostRebuildTestTimeout):
			t.Fatal("mudança não terminou após configuração concorrente")
		}
		if commitCalled.Load() {
			t.Fatal("commit executado após configuração concorrente")
		}
	})
}

func TestHostChangeRecusaTrocaDeSessaoNaReautenticacao(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	for _, other := range []struct {
		name      string
		principal auth.LocalSessionPrincipal
	}{
		{name: "mesmo usuário, outra sessão", principal: auth.LocalSessionPrincipal{UserID: principal.UserID, SessionID: hostUUID(t)}},
		{name: "outro usuário", principal: auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}},
	} {
		t.Run(other.name, func(t *testing.T) {
			var calls atomic.Int32
			var commitCalled atomic.Bool
			err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
				if calls.Add(1) == 1 {
					return principal, nil
				}
				return other.principal, nil
			}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
				return func(context.Context) error { commitCalled.Store(true); return nil }, nil
			})
			if !errors.Is(err, ErrDenied) || commitCalled.Load() || calls.Load() != 2 {
				t.Fatalf("troca na reautenticação = %v, commit=%v chamadas=%d", err, commitCalled.Load(), calls.Load())
			}
		})
	}
}

func TestHostChangeErroOuPanicNoCommitMantemMapaRemovido(t *testing.T) {
	want := errors.New("falha no writer")
	tests := []struct {
		name   string
		commit func(context.Context) error
		panic  bool
	}{
		{name: "erro", commit: func(context.Context) error { return want }},
		{name: "panic", commit: func(context.Context) error { panic("writer") }, panic: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, configuration := readyHostForRebuild(t)
			publishHostUserForChange(t, state, principal, configuration)
			if tt.panic {
				func() {
					defer func() {
						if recover() == nil {
							t.Fatal("panic do commit não foi propagado")
						}
					}()
					_ = state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
						return principal, nil
					}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
						return tt.commit, nil
					})
				}()
			} else if err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
				return principal, nil
			}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
				return tt.commit, nil
			}); !errors.Is(err, want) {
				t.Fatalf("erro do commit = %v, want %v", err, want)
			}
			assertHostUserNotPublished(t, state, principal)
		})
	}
}

func TestHostChangeSucessoRemoveSomenteMapaDoUsuarioAlvo(t *testing.T) {
	state, principalA, _, configuration := hostStateFixture(t)
	principalB := auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}
	if err := state.SetVaultUnlocked(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	publishHostUserForChange(t, state, principalA, configuration)
	publishHostUserForChange(t, state, principalB, configuration)

	var commitCalled atomic.Bool
	admissionStarted := make(chan struct{})
	admitted := make(chan error, 1)
	err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principalA, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(ctx context.Context) error {
			go func() {
				close(admissionStarted)
				_, err := state.Epochs().Capture(ctx, principalA.UserID, principalA.SessionID)
				admitted <- err
			}()
			select {
			case <-admissionStarted:
			case <-time.After(hostRebuildTestTimeout):
				return errors.New("admission concorrente não iniciou")
			}
			select {
			case <-admitted:
				return errors.New("admission não deveria atravessar o commit")
			case <-time.After(50 * time.Millisecond):
			}
			commitCalled.Store(true)
			return nil
		}, nil
	})
	if err != nil || !commitCalled.Load() {
		t.Fatalf("commit bem-sucedido = %v, chamado=%v", err, commitCalled.Load())
	}
	select {
	case err := <-admitted:
		if err != nil {
			t.Fatal("captura após liberar gate", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("admission bloqueada não terminou após liberar o gate")
	}
	assertHostUserNotPublished(t, state, principalA)
	if _, _, err := state.UserConfiguration(context.Background(), principalB.UserID); err != nil {
		t.Fatalf("mapa do outro usuário foi removido: %v", err)
	}
}

func TestHostChangeOverflowNaoExecutaCommitNemRemoveMapa(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	publishHostUserForChange(t, state, principal, configuration)
	state.counter = math.MaxUint64
	var commitCalled atomic.Bool
	err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(context.Context) error { commitCalled.Store(true); return nil }, nil
	})
	if !errors.Is(err, ErrHostGenerationOverflow) || commitCalled.Load() {
		t.Fatalf("overflow = %v, commit=%v", err, commitCalled.Load())
	}
	state.mu.RLock()
	_, published := state.users[principal.UserID]
	state.mu.RUnlock()
	if !published {
		t.Fatal("overflow removeu mapa antes da reserva")
	}
}

func TestHostChangeCancelamentoNaoExecutaCommit(t *testing.T) {
	t.Run("antes", func(t *testing.T) {
		state, principal, _ := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var authCalled, commitCalled atomic.Bool
		err := state.ChangeUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			authCalled.Store(true)
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
			return func(context.Context) error { commitCalled.Store(true); return nil }, nil
		})
		if !errors.Is(err, context.Canceled) || authCalled.Load() || commitCalled.Load() {
			t.Fatalf("cancelamento antes = %v, authenticate=%v commit=%v", err, authCalled.Load(), commitCalled.Load())
		}
	})

	t.Run("depois do prepare", func(t *testing.T) {
		state, principal, _ := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		var commitCalled atomic.Bool
		err := state.ChangeUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
			cancel()
			return func(context.Context) error { commitCalled.Store(true); return nil }, nil
		})
		if !errors.Is(err, context.Canceled) || commitCalled.Load() {
			t.Fatalf("cancelamento depois do prepare = %v, commit=%v", err, commitCalled.Load())
		}
	})

	t.Run("durante prepare", func(t *testing.T) {
		state, principal, _ := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		var commitCalled atomic.Bool
		err := state.ChangeUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
			cancel()
			return nil, ctx.Err()
		})
		if !errors.Is(err, context.Canceled) || commitCalled.Load() {
			t.Fatalf("cancelamento durante prepare = %v, commit=%v", err, commitCalled.Load())
		}
	})
}

func TestHostChangeCommitNaoReadquireMutexDoHost(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	var commitReturned atomic.Bool
	err := state.ChangeUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		return func(ctx context.Context) error {
			_, err := state.Snapshot(ctx, principal)
			if !errors.Is(err, ErrHostUserNotPublished) {
				return err
			}
			commitReturned.Store(true)
			return nil
		}, nil
	})
	if err != nil || !commitReturned.Load() {
		t.Fatalf("commit não pôde consultar host após unlock do mutex: err=%v chamado=%v", err, commitReturned.Load())
	}
}

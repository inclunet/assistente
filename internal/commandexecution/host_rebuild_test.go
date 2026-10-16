package commandexecution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandsecurity"
)

const hostRebuildTestTimeout = 5 * time.Second

func readyHostForRebuild(t *testing.T) (*HostState, auth.LocalSessionPrincipal, *commandbindings.Configuration) {
	t.Helper()
	state, principal, _, configuration := hostStateFixture(t)
	ctx := context.Background()
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	return state, principal, configuration
}

func rebuildWithTimeout(t *testing.T, fn func() error) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- fn() }()
	select {
	case err := <-result:
		return err
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("RebuildUserConfiguration não terminou no limite")
		return nil
	}
}

func assertHostUserNotPublished(t *testing.T, state *HostState, principal auth.LocalSessionPrincipal) {
	t.Helper()
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("usuário deveria estar sem mapa publicado: %v", err)
	}
}

func TestHostRebuildPublicaConfiguracaoEReautenticaDuasVezes(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	wantLayers := []string{"builtin", "profile.work"}
	layers := append([]string(nil), wantLayers...)
	var authenticateCalls atomic.Int32

	err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		authenticateCalls.Add(1)
		return principal, nil
	}, func(_ context.Context, got auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		if got != principal {
			t.Fatalf("build recebeu principal diferente: got=%+v want=%+v", got, principal)
		}
		return configuration, layers, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	layers[0] = "alterada-depois-do-build"
	if got := authenticateCalls.Load(); got != 2 {
		t.Fatalf("authenticate foi chamado %d vezes, want 2", got)
	}

	versions, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if !versions.Unlocked {
		t.Fatalf("configuração publicada não ficou pronta para a sessão: %+v", versions)
	}
	gotConfiguration, gotLayers, err := state.UserConfiguration(context.Background(), principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if gotConfiguration != configuration || len(gotLayers) != len(wantLayers) || gotLayers[0] != wantLayers[0] || gotLayers[1] != wantLayers[1] {
		t.Fatalf("snapshot publicado incorreto: configuration=%p layers=%v", gotConfiguration, gotLayers)
	}
}

func TestHostRebuildExecutaBuildForaDoGateEMutacaoDuranteBuildFicaStale(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	var mutationErr error

	err := rebuildWithTimeout(t, func() error {
		return state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(ctx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			mutationErr = state.SetVaultUnlocked(ctx, true)
			return configuration, nil, nil
		})
	})
	if mutationErr != nil {
		t.Fatalf("mutação dentro do build = %v", mutationErr)
	}
	if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("rebuild após mutação no build = %v, want stale", err)
	}
	assertHostUserNotPublished(t, state, principal)
}

func TestHostRebuildLockEUnlockDuranteBuildNaoPublicam(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBuild := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseBuild()
	result := make(chan error, 1)

	go func() {
		result <- state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(_ context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			close(entered)
			<-release
			return configuration, nil, nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("build não iniciou")
	}

	mutated := make(chan error, 1)
	go func() {
		if err := state.SetOSSessionState(context.Background(), true, true); err != nil {
			mutated <- err
			return
		}
		mutated <- state.SetOSSessionState(context.Background(), true, false)
	}()
	select {
	case err := <-mutated:
		if err != nil {
			t.Fatalf("lock/unlock durante build = %v", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("lock/unlock ficou bloqueado pelo build")
	}
	releaseBuild()

	select {
	case err := <-result:
		if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
			t.Fatalf("rebuild após lock/unlock = %v, want stale", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("rebuild não terminou após lock/unlock")
	}
	assertHostUserNotPublished(t, state, principal)
}

func TestHostRebuildLogoutEInvalidateSessionDuranteBuildNaoPublicam(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*HostState, auth.LocalSessionPrincipal) error
	}{
		{
			name: "logout",
			mutate: func(state *HostState, principal auth.LocalSessionPrincipal) error {
				return state.Epochs().MutatePrincipal(context.Background(), principal.UserID, principal.SessionID, func() error { return nil })
			},
		},
		{
			name: "invalidate session",
			mutate: func(state *HostState, principal auth.LocalSessionPrincipal) error {
				return state.Epochs().InvalidateSession(context.Background(), principal.UserID, principal.SessionID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, configuration := readyHostForRebuild(t)
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseBuild := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseBuild()
			result := make(chan error, 1)
			go func() {
				result <- state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
					return principal, nil
				}, func(_ context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
					close(entered)
					<-release
					return configuration, nil, nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(hostRebuildTestTimeout):
				t.Fatal("build não iniciou")
			}

			mutated := make(chan error, 1)
			go func() { mutated <- tt.mutate(state, principal) }()
			select {
			case err := <-mutated:
				if err != nil {
					t.Fatalf("%s durante build = %v", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("%s ficou bloqueado pelo build", tt.name)
			}
			releaseBuild()

			select {
			case err := <-result:
				if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
					t.Fatalf("rebuild após %s = %v, want stale", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("rebuild não terminou após %s", tt.name)
			}
			assertHostUserNotPublished(t, state, principal)
		})
	}
}

func TestHostRebuildRecusaTrocaDePrincipalNaReautenticacao(t *testing.T) {
	state, principalA, _, configuration := hostStateFixture(t)
	if err := state.SetVaultUnlocked(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	principalB := auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}
	var authenticateCalls atomic.Int32

	err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		if authenticateCalls.Add(1) == 1 {
			return principalA, nil
		}
		return principalB, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("troca de principal na reautenticação = %v", err)
	}
	assertHostUserNotPublished(t, state, principalA)
}

func TestHostRebuildRecusaVaultLockEnquantoBuildEDeixaMapaFechado(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	var mutationErr error
	err := rebuildWithTimeout(t, func() error {
		return state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(ctx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			mutationErr = state.SetVaultUnlocked(ctx, false)
			return configuration, nil, nil
		})
	})
	if mutationErr != nil {
		t.Fatalf("lock do cofre durante build = %v", mutationErr)
	}
	if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("rebuild após lock do cofre = %v, want stale", err)
	}
	assertHostUserNotPublished(t, state, principal)
}

func TestHostRebuildPublicacaoEForgetConcorrentesInvalidamBuild(t *testing.T) {
	tests := []struct {
		name       string
		forget     bool
		wantAbsent bool
	}{
		{name: "publish", wantAbsent: false},
		{name: "forget", forget: true, wantAbsent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, oldConfiguration := readyHostForRebuild(t)
			if err := state.PublishUserConfiguration(context.Background(), principal.UserID, oldConfiguration); err != nil {
				t.Fatal(err)
			}
			newConfiguration, err := commandbindings.NewConfiguration(nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseBuild := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseBuild()
			result := make(chan error, 1)
			go func() {
				result <- state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
					return principal, nil
				}, func(_ context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
					close(entered)
					<-release
					return newConfiguration, []string{"new.layer"}, nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(hostRebuildTestTimeout):
				t.Fatal("build não iniciou")
			}

			mutated := make(chan error, 1)
			go func() {
				if tt.forget {
					mutated <- state.ForgetUserConfiguration(context.Background(), principal.UserID)
					return
				}
				mutated <- state.PublishUserConfiguration(context.Background(), principal.UserID, oldConfiguration)
			}()
			select {
			case err := <-mutated:
				if err != nil {
					t.Fatalf("%s concorrente = %v", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("%s concorrente ficou bloqueado pelo build", tt.name)
			}
			releaseBuild()
			select {
			case err := <-result:
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("rebuild após %s concorrente = %v, want recusa", tt.name, err)
				}
			case <-time.After(hostRebuildTestTimeout):
				t.Fatalf("rebuild não terminou após %s concorrente", tt.name)
			}

			if tt.wantAbsent {
				assertHostUserNotPublished(t, state, principal)
				return
			}
			gotConfiguration, gotLayers, err := state.UserConfiguration(context.Background(), principal.UserID)
			if err != nil || gotConfiguration != oldConfiguration || len(gotLayers) != 0 {
				t.Fatalf("publish concorrente foi sobrescrito: configuration=%p layers=%v err=%v", gotConfiguration, gotLayers, err)
			}
			versions, err := state.Snapshot(context.Background(), principal)
			if err != nil {
				t.Fatal(err)
			}
			if versions.Unlocked {
				t.Fatalf("publish concorrente deixou mapa pronto indevidamente: %+v", versions)
			}
		})
	}
}

func TestHostRebuildRecusaConfiguracaoNulaECamadasDuplicadas(t *testing.T) {
	tests := []struct {
		name   string
		config *commandbindings.Configuration
		layers []string
		want   error
	}{
		{name: "configuração nula", layers: nil, want: ErrInvalidHostState},
		{name: "camadas duplicadas", layers: []string{"layer.a", "layer.a"}, want: ErrInvalidHostLayers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, configuration := readyHostForRebuild(t)
			if tt.config == nil && tt.name != "configuração nula" {
				tt.config = configuration
			}
			err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
				return principal, nil
			}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
				return tt.config, tt.layers, nil
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("erro = %v, want %v", err, tt.want)
			}
			assertHostUserNotPublished(t, state, principal)
		})
	}
}

func TestHostRebuildRespeitaCancelamentoAntesDuranteEDepoisDoBuild(t *testing.T) {
	t.Run("antes", func(t *testing.T) {
		state, principal, configuration := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			called = true
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return configuration, nil, nil
		})
		if !errors.Is(err, context.Canceled) || called {
			t.Fatalf("cancelamento antes do rebuild = %v, authenticate chamado=%v", err, called)
		}
		assertHostUserNotPublished(t, state, principal)
	})

	t.Run("durante", func(t *testing.T) {
		state, principal, configuration := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			cancel()
			return nil, nil, ctx.Err()
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelamento durante build = %v", err)
		}
		assertHostUserNotPublished(t, state, principal)
		_ = configuration
	})

	t.Run("depois", func(t *testing.T) {
		state, principal, configuration := readyHostForRebuild(t)
		ctx, cancel := context.WithCancel(context.Background())
		err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			cancel()
			return configuration, nil, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelamento depois do build = %v", err)
		}
		assertHostUserNotPublished(t, state, principal)
	})
}

func TestHostRebuildProntoFicaVinculadoASessaoAutenticada(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	otherSession := auth.LocalSessionPrincipal{UserID: principal.UserID, SessionID: hostUUID(t)}
	current, err := state.Snapshot(context.Background(), principal)
	if err != nil || !current.Unlocked {
		t.Fatalf("sessão autenticada não ficou pronta: versions=%+v err=%v", current, err)
	}
	other, err := state.Snapshot(context.Background(), otherSession)
	if err != nil {
		t.Fatal(err)
	}
	if other.Unlocked {
		t.Fatalf("sessão diferente herdou readiness: %+v", other)
	}
}

func TestHostRebuildFalhaDeBuildNuncaAbreMapa(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	want := errors.New("falha de build")
	err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return nil, nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("falha de build = %v, want %v", err, want)
	}
	assertHostUserNotPublished(t, state, principal)
}

func TestHostRebuildUsaGeracoesFrescasDepoisDeForget(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	build := func(layers []string) error {
		return state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
			return principal, nil
		}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return configuration, layers, nil
		})
	}
	if err := build([]string{"first"}); err != nil {
		t.Fatal(err)
	}
	first, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ForgetUserConfiguration(context.Background(), principal.UserID); err != nil {
		t.Fatal(err)
	}
	if err := build([]string{"second"}); err != nil {
		t.Fatal(err)
	}
	second, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if first.GlobalConfig == second.GlobalConfig || first.ActiveLayers == second.ActiveLayers {
		t.Fatalf("rebuild reutilizou gerações: primeira=%+v segunda=%+v", first, second)
	}
}

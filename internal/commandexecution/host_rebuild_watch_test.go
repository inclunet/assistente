package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func TestHostRebuildCancelaBuildBloqueadoAoInvalidarWatch(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buildStarted := make(chan context.Context, 1)
	buildFinished := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- state.RebuildUserConfiguration(ctx,
			func(context.Context) (auth.LocalSessionPrincipal, error) {
				return principal, nil
			},
			func(buildCtx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
				buildStarted <- buildCtx
				<-buildCtx.Done()
				close(buildFinished)
				return nil, nil, buildCtx.Err()
			})
	}()

	select {
	case <-buildStarted:
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("build não iniciou")
	}
	if err := state.SetOSSessionState(context.Background(), true, true); err != nil {
		t.Fatalf("bloqueio do sistema = %v", err)
	}

	select {
	case <-buildFinished:
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("invalidação não cancelou o build bloqueado")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("build cancelado = %v, want context.Canceled", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("rebuild não terminou após cancelar o build")
	}
	assertHostUserNotPublished(t, state, principal)
}

func TestHostRebuildWatchReleasedOnEveryExit(t *testing.T) {
	for _, scenario := range []string{"error", "nil_config", "invalid_layers", "panic", "success"} {
		t.Run(scenario, func(t *testing.T) {
			state, principal, configuration := readyHostForRebuild(t)
			var buildCtx context.Context
			var buildStartedWithActiveWatch bool
			var result error
			var panicked bool
			failure := errors.New("falha de build")

			func() {
				defer func() { panicked = recover() != nil }()
				result = state.RebuildUserConfiguration(context.Background(),
					func(context.Context) (auth.LocalSessionPrincipal, error) {
						return principal, nil
					},
					func(ctx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
						buildCtx = ctx
						buildStartedWithActiveWatch = ctx.Err() == nil
						switch scenario {
						case "error":
							return nil, nil, failure
						case "nil_config":
							return nil, nil, nil
						case "invalid_layers":
							return configuration, []string{"duplicate", "duplicate"}, nil
						case "panic":
							panic("fixture")
						default:
							return configuration, nil, nil
						}
					})
			}()

			if buildCtx == nil {
				t.Fatal("build não recebeu contexto do watch")
			}
			if !buildStartedWithActiveWatch {
				t.Fatal("build começou com watch já cancelado")
			}
			if !errors.Is(buildCtx.Err(), context.Canceled) {
				t.Fatalf("watch não foi liberado: %v", buildCtx.Err())
			}

			switch scenario {
			case "error":
				if !errors.Is(result, failure) || panicked {
					t.Fatalf("saída do erro: result=%v panicked=%v", result, panicked)
				}
				assertHostUserNotPublished(t, state, principal)
			case "nil_config":
				if !errors.Is(result, ErrInvalidHostState) || panicked {
					t.Fatalf("saída da configuração nula: result=%v panicked=%v", result, panicked)
				}
				assertHostUserNotPublished(t, state, principal)
			case "invalid_layers":
				if !errors.Is(result, ErrInvalidHostLayers) || panicked {
					t.Fatalf("saída das camadas inválidas: result=%v panicked=%v", result, panicked)
				}
				assertHostUserNotPublished(t, state, principal)
			case "panic":
				if !panicked {
					t.Fatalf("panic do build foi ocultado: result=%v", result)
				}
				assertHostUserNotPublished(t, state, principal)
			case "success":
				if result != nil || panicked {
					t.Fatalf("saída do sucesso: result=%v panicked=%v", result, panicked)
				}
				got, _, err := state.UserConfiguration(context.Background(), principal.UserID)
				if err != nil || got != configuration {
					t.Fatalf("configuração não publicada: got=%p want=%p err=%v", got, configuration, err)
				}
			}
		})
	}
}

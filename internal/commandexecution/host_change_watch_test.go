package commandexecution

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
)

func TestHostChangePreparationWatchReleasedOnEveryExit(t *testing.T) {
	for _, scenario := range []string{"error", "nil_commit", "panic", "success"} {
		t.Run(scenario, func(t *testing.T) {
			state, principal, _ := readyHostForRebuild(t)
			var preparation context.Context
			var panicked bool
			var result error
			called := false
			failure := errors.New("fixture")
			func() {
				defer func() { panicked = recover() != nil }()
				result = state.ChangeUserConfigurationWithEpoch(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
					func(ctx context.Context, _ auth.LocalSessionPrincipal, _ commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
						preparation = ctx
						if ctx.Err() != nil {
							return nil, ctx.Err()
						}
						switch scenario {
						case "error":
							return nil, failure
						case "nil_commit":
							return nil, nil
						case "panic":
							panic("fixture")
						}
						return func(commitCtx context.Context) error {
							called = true
							if preparation.Err() != context.Canceled {
								return errors.New("watch não liberado antes de publicar")
							}
							return commitCtx.Err()
						}, nil
					})
			}()
			if preparation == nil || !errors.Is(preparation.Err(), context.Canceled) {
				t.Fatal("watch não encerrado")
			}
			if scenario == "success" {
				if result != nil || !called || panicked {
					t.Fatal("publicação autocancelada", result)
				}
			} else if called {
				t.Fatal("commit inesperado")
			}
			if scenario == "error" && !errors.Is(result, failure) {
				t.Fatal(result)
			}
			if scenario == "nil_commit" && !errors.Is(result, ErrInvalidHostState) {
				t.Fatal(result)
			}
			if panicked != (scenario == "panic") {
				t.Fatal("panic inesperado/ocultado")
			}
		})
	}
}

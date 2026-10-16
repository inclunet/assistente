package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
)

func TestCommandKeyboardMapReadRetriesOnlyTransientProjection(t *testing.T) {
	for _, scenario := range []string{"host_snapshot", "admit", "continuous", "reset", "reset_during_publication"} {
		t.Run(scenario, func(t *testing.T) {
			a := readyCommandProduct(t)
			p := a.commandProduct.Load()
			ctx := context.Background()
			configuration, layers, _, err := p.host.ResolutionSnapshot(ctx, p.principal)
			if err != nil {
				t.Fatal(err)
			}
			var attempts atomic.Int32
			stage := "admit"
			if scenario == "host_snapshot" {
				stage = scenario
			}
			guard := func(ctx context.Context) error {
				trace, ok := ctx.Value(commandLoadTraceKey{}).(*commandLoadTrace)
				if !ok {
					return nil
				}
				trace.mu.Lock()
				current := trace.currentStage
				trace.mu.Unlock()
				if current != stage {
					return nil
				}
				attempt := attempts.Add(1)
				if scenario == "reset_during_publication" {
					p.invalidateCommandProjectionRecovery()
					return nil
				}
				if scenario == "reset" {
					p.invalidateCommandProjectionRecovery()
				}
				if attempt == 1 || scenario == "continuous" || scenario == "reset" {
					return commandexecution.ErrStale
				}
				return nil
			}
			if err := p.host.RebuildUserConfigurationGuarded(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
				return p.principal, nil
			}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
				return configuration, layers, nil
			}, guard); err != nil {
				t.Fatal(err)
			}
			view, err := a.GetLocalCommandKeyboardMap()
			if scenario == "reset" || scenario == "reset_during_publication" || scenario == "continuous" {
				if !errors.Is(err, commandexecution.ErrStale) || view.Generation != "" {
					t.Fatalf("publicação indevida: generation=%q err=%v", view.Generation, err)
				}
				want := int32(3)
				if scenario != "continuous" {
					want = 1
				}
				if attempts.Load() != want {
					t.Fatalf("tentativas=%d, esperado=%d", attempts.Load(), want)
				}
				return
			}
			want := int32(2)
			if scenario == "host_snapshot" {
				want = 3 // autenticação e resolução revalidam o host na segunda tentativa.
			}
			if err != nil || view.Generation == "" || attempts.Load() != want {
				t.Fatalf("leitura não recuperada: attempts=%d generation=%q err=%v", attempts.Load(), view.Generation, err)
			}
			p.keyboardMu.Lock()
			defer p.keyboardMu.Unlock()
			if p.keyboardMap == nil || p.keyboardMap.ctx.Err() != nil {
				t.Fatal("mapa retido foi cancelado ao terminar a leitura")
			}
		})
	}
}

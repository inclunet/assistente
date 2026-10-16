package commandexecution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func publishGuardedHost(t *testing.T, state *HostState, principal auth.LocalSessionPrincipal, configuration *commandbindings.Configuration, guard func(context.Context) error) error {
	t.Helper()
	return state.RebuildUserConfigurationGuarded(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, []string{"layer.guard"}, nil
	}, guard)
}

func TestHostProjectionGuardFalhaAntesDaPublicacaoEPreservaStale(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error {
		return ErrStale
	})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("guard na publicação = %v, want ErrStale", err)
	}
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("guard falho publicou configuração: %v", err)
	}
}

func TestHostProjectionGuardRejeitaAmbosSnapshotsSemDerrubarSourceSecurityReady(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	var stale atomic.Bool
	if err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error {
		if stale.Load() {
			return ErrStale
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ready, err := state.SourceSecurityReady(context.Background())
	if err != nil || !ready {
		t.Fatalf("SourceSecurityReady inicial = %v, %v", ready, err)
	}
	stale.Store(true)
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, ErrStale) {
		t.Fatalf("Snapshot com guard stale = %v", err)
	}
	if _, _, _, err := state.ResolutionSnapshot(context.Background(), principal); !errors.Is(err, ErrStale) {
		t.Fatalf("ResolutionSnapshot com guard stale = %v", err)
	}
	if ready, err := state.SourceSecurityReady(context.Background()); err != nil || !ready {
		t.Fatalf("guard stale derrubou SourceSecurityReady = %v, %v", ready, err)
	}
}

func TestHostProjectionGuardReplacementLimpaGuardAntigoAtomically(t *testing.T) {
	state, principal, oldConfiguration := readyHostForRebuild(t)
	var oldGuardStale atomic.Bool
	if err := publishGuardedHost(t, state, principal, oldConfiguration, func(context.Context) error {
		if oldGuardStale.Load() {
			return ErrStale
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	oldGuardStale.Store(true)
	newConfiguration, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return newConfiguration, []string{"layer.replacement"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Snapshot(context.Background(), principal); err != nil {
		t.Fatalf("replacement herdou guard antigo: %v", err)
	}
	if got, _, _, err := state.ResolutionSnapshot(context.Background(), principal); err != nil || got != newConfiguration {
		t.Fatalf("snapshot após replacement = config=%p err=%v", got, err)
	}
}

func TestHostProjectionGuardRefreshEquivalenteNaoCancelaExecucao(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	if err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(context.Background(), principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	runContext, release, err := state.Epochs().WatchEpoch(context.Background(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var newGuardStale atomic.Bool
	if err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error {
		if newGuardStale.Load() {
			return ErrStale
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := state.Snapshot(context.Background(), principal)
	if err != nil || after != before {
		t.Fatalf("refresh alterou versões: before=%+v after=%+v err=%v", before, after, err)
	}
	newGuardStale.Store(true)
	if _, err := state.Snapshot(context.Background(), principal); !errors.Is(err, ErrStale) {
		t.Fatalf("guard novo não ficou efetivo: %v", err)
	}
	select {
	case <-runContext.Done():
		t.Fatal("refresh equivalente cancelou execução existente")
	default:
	}
}

func TestHostProjectionGuardForaDoHostMutexDetectaPublicacaoConcorrente(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	var blocking atomic.Bool
	entered := make(chan struct{})
	releaseGuard := make(chan struct{})
	if err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error {
		if !blocking.Load() {
			return nil
		}
		close(entered)
		<-releaseGuard
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	blocking.Store(true)
	result := make(chan error, 1)
	go func() {
		_, err := state.Snapshot(context.Background(), principal)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("guard não foi executado")
	}
	if err := state.PublishUserConfiguration(context.Background(), principal.UserID, configuration); err != nil {
		t.Fatalf("publicação concorrente = %v", err)
	}
	close(releaseGuard)
	select {
	case err := <-result:
		if !errors.Is(err, ErrStale) {
			t.Fatalf("snapshot após publicação concorrente = %v, want ErrStale", err)
		}
	case <-time.After(hostRebuildTestTimeout):
		t.Fatal("snapshot ficou bloqueado pelo guard/publicação")
	}
}

func TestHostSourceSecurityReadyFechaComCofreOuSOBloqueadoMasNaoDependeDaConfiguracao(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	var stale atomic.Bool
	if err := publishGuardedHost(t, state, principal, configuration, func(context.Context) error {
		if stale.Load() {
			return ErrStale
		}
		return nil
	}); err != nil {
		t.Fatalf("publicação de fixture: %v", err)
	}
	stale.Store(true)
	if ready, err := state.SourceSecurityReady(context.Background()); err != nil || !ready {
		t.Fatalf("SourceSecurityReady com guard stale = %v, %v", ready, err)
	}
	if err := state.SetOSSessionState(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	if ready, err := state.SourceSecurityReady(context.Background()); err != nil || ready {
		t.Fatalf("SourceSecurityReady após lock = %v, %v", ready, err)
	}
	if err := state.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.ForgetUserConfiguration(context.Background(), principal.UserID); err != nil {
		t.Fatal(err)
	}
	if ready, err := state.SourceSecurityReady(context.Background()); err != nil || !ready {
		t.Fatalf("SourceSecurityReady após forget = %v, %v", ready, err)
	}
	if err := state.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if ready, err := state.SourceSecurityReady(context.Background()); err != nil || ready {
		t.Fatalf("SourceSecurityReady após bloqueio do cofre = %v, %v", ready, err)
	}
}

func TestHostProjectionGuardNaoAlteraCancelamento(t *testing.T) {
	state, principal, configuration := readyHostForRebuild(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var guardCalls, buildCalls atomic.Int32
	err := state.RebuildUserConfigurationGuarded(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		buildCalls.Add(1)
		return configuration, nil, nil
	}, func(context.Context) error {
		guardCalls.Add(1)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("rebuild cancelado = %v", err)
	}
	if buildCalls.Load() != 0 || guardCalls.Load() != 0 {
		t.Fatalf("callbacks executados após cancelamento: build=%d guard=%d", buildCalls.Load(), guardCalls.Load())
	}
}

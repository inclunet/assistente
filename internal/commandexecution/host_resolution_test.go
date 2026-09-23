package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func readyHostForResolution(t *testing.T) (*HostState, auth.LocalSessionPrincipal, *commandbindings.Configuration) {
	t.Helper()
	state, principal, configuration := readyHostForRebuild(t)
	if err := state.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, []string{"layer.a", "layer.b"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return state, principal, configuration
}

func TestHostStateResolutionSnapshotExigeEstadoProntoESessaoExata(t *testing.T) {
	state, principal, configuration := readyHostForResolution(t)
	ctx := context.Background()

	gotConfiguration, gotLayers, versions, err := state.ResolutionSnapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if gotConfiguration != configuration || !slicesEqual(gotLayers, []string{"layer.a", "layer.b"}) {
		t.Fatalf("snapshot incoerente: config=%p layers=%v", gotConfiguration, gotLayers)
	}
	if !versions.Unlocked || versions.Registry != "registry-v1" || versions.GlobalConfig == "" || versions.ActiveLayers == "" {
		t.Fatalf("versões inválidas: %+v", versions)
	}

	gotLayers[0] = "detached"
	_, layersAgain, versionsAgain, err := state.ResolutionSnapshot(ctx, principal)
	if err != nil || layersAgain[0] != "layer.a" {
		t.Fatalf("camadas não detached: layers=%v err=%v", layersAgain, err)
	}
	if versionsAgain != versions {
		t.Fatalf("leitura posterior misturou versões: antes=%+v depois=%+v", versions, versionsAgain)
	}

	wrongOwner := principal
	wrongOwner.UserID = hostUUID(t)
	if _, _, _, err := state.ResolutionSnapshot(ctx, wrongOwner); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("usuário diferente = %v", err)
	}
	wrongSession := principal
	wrongSession.SessionID = hostUUID(t)
	if _, _, _, err := state.ResolutionSnapshot(ctx, wrongSession); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("sessão diferente = %v", err)
	}
}

func TestHostStateResolutionSnapshotRecusaEstadosNaoProntos(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		setup func(*HostState)
	}{
		{name: "cofre fechado", setup: func(state *HostState) {
			if err := state.SetVaultUnlocked(ctx, false); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "SO desconhecido", setup: func(state *HostState) {
			if err := state.SetOSSessionState(ctx, false, false); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "SO bloqueado", setup: func(state *HostState) {
			if err := state.SetOSSessionState(ctx, true, true); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, principal, _ := readyHostForResolution(t)
			tc.setup(state)
			if _, _, _, err := state.ResolutionSnapshot(ctx, principal); !errors.Is(err, ErrHostUserNotPublished) {
				t.Fatalf("estado não pronto = %v", err)
			}
		})
	}
}

func TestHostStateResolutionSnapshotValidaNilCancelamentoEDesabilitado(t *testing.T) {
	state, principal, _ := readyHostForResolution(t)
	if _, _, _, err := state.ResolutionSnapshot(nil, principal); !errors.Is(err, ErrInvalidHostState) {
		t.Fatalf("contexto nil = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := state.ResolutionSnapshot(ctx, principal); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado = %v", err)
	}
	state.disabled = true
	if _, _, _, err := state.ResolutionSnapshot(context.Background(), principal); !errors.Is(err, ErrHostStateDisabled) {
		t.Fatalf("host desabilitado = %v", err)
	}
}

func TestHostStateResolutionSnapshotPublicaConfiguracaoEVersoesComoUmaUnidade(t *testing.T) {
	state, principal, firstConfiguration := readyHostForRebuild(t)
	ctx := context.Background()
	firstLayers := []string{"first.layer"}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return firstConfiguration, firstLayers, nil
	}); err != nil {
		t.Fatal(err)
	}
	firstLayers[0] = "mutated-after-publish"

	gotConfiguration, gotLayers, firstVersions, err := state.ResolutionSnapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if gotConfiguration != firstConfiguration || !slicesEqual(gotLayers, []string{"first.layer"}) {
		t.Fatalf("primeira publicação perdeu coesão/detachment: config=%p layers=%v", gotConfiguration, gotLayers)
	}

	secondConfiguration, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return secondConfiguration, []string{"second.layer"}, nil
	}); err != nil {
		t.Fatal(err)
	}

	gotConfiguration, gotLayers, secondVersions, err := state.ResolutionSnapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if gotConfiguration != secondConfiguration || !slicesEqual(gotLayers, []string{"second.layer"}) {
		t.Fatalf("segunda publicação misturou configuração/camadas: config=%p layers=%v", gotConfiguration, gotLayers)
	}
	if secondVersions.Registry != firstVersions.Registry || !secondVersions.Unlocked || secondVersions.GlobalConfig == firstVersions.GlobalConfig || secondVersions.ActiveLayers == firstVersions.ActiveLayers {
		t.Fatalf("publicação não atualizou versões coerentemente: primeira=%+v segunda=%+v", firstVersions, secondVersions)
	}
}

func TestHostStateResolutionSnapshotAguardaUmRLock(t *testing.T) {
	state, principal, _ := readyHostForResolution(t)
	state.mu.Lock()
	result := make(chan error, 1)
	go func() {
		_, _, _, err := state.ResolutionSnapshot(context.Background(), principal)
		result <- err
	}()
	select {
	case err := <-result:
		state.mu.Unlock()
		t.Fatalf("leitura não aguardou o lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	state.mu.Unlock()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("leitura após liberar lock = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("leitura não terminou após liberar lock")
	}
}

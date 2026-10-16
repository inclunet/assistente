package commandexecution

import (
	"context"
	"errors"
	"math"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

func hostStateFixture(t *testing.T) (*HostState, auth.LocalSessionPrincipal, auth.LocalSessionPrincipal, *commandbindings.Configuration) {
	t.Helper()
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewHostState(epochs, "registry-v1")
	if err != nil {
		t.Fatal(err)
	}
	config, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state, auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}, auth.LocalSessionPrincipal{UserID: hostUUID(t), SessionID: hostUUID(t)}, config
}

func hostUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestHostStateInicialSeguroEEstadosDeLock(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	ctx := context.Background()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	versions, err := state.Snapshot(ctx, principalA)
	if err != nil {
		t.Fatal(err)
	}
	if versions.Unlocked {
		t.Fatal("estado inicial deveria estar fechado")
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	versions, err = state.Snapshot(ctx, principalA)
	if err != nil || versions.Unlocked {
		t.Fatalf("cofre sozinho abriu o host: versions=%+v err=%v", versions, err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Snapshot(ctx, principalA); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatal("unlock preservou mapa antigo", err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principalA, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return config, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	versions, err = state.Snapshot(ctx, principalA)
	if err != nil || !versions.Unlocked {
		t.Fatalf("sessão conhecida desbloqueada não abriu o host: versions=%+v err=%v", versions, err)
	}
	if err := state.SetOSSessionState(ctx, true, true); err != nil {
		t.Fatal(err)
	}
	versions, err = state.Snapshot(ctx, principalA)
	if !errors.Is(err, ErrHostUserNotPublished) || versions.Unlocked {
		t.Fatalf("lock do SO não removeu o mapa: versions=%+v err=%v", versions, err)
	}
	if err := state.SetOSSessionState(ctx, false, false); err != nil {
		t.Fatal(err)
	}
	versions, err = state.Snapshot(ctx, principalA)
	if !errors.Is(err, ErrHostUserNotPublished) || versions.Unlocked {
		t.Fatalf("estado desconhecido abriu o host: versions=%+v err=%v", versions, err)
	}
}

func TestHostStateIsolaGeracoesPorUsuario(t *testing.T) {
	state, principalA, principalB, config := hostStateFixture(t)
	ctx := context.Background()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, principalB.UserID, config); err != nil {
		t.Fatal(err)
	}
	beforeB, err := state.Snapshot(ctx, principalB)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActiveLayers(ctx, principalA.UserID, []string{"layer.a"}); err != nil {
		t.Fatal(err)
	}
	afterB, err := state.Snapshot(ctx, principalB)
	if err != nil {
		t.Fatal(err)
	}
	if beforeB != afterB {
		t.Fatalf("mutação de A alterou versões de B: antes=%+v depois=%+v", beforeB, afterB)
	}
	afterA, err := state.Snapshot(ctx, principalA)
	if err != nil {
		t.Fatal(err)
	}
	if afterA.GlobalConfig == beforeB.GlobalConfig || afterA.ActiveLayers == beforeB.ActiveLayers {
		t.Fatalf("gerações de A não foram próprias: A=%+v B=%+v", afterA, beforeB)
	}
}

func TestHostStateCopiaCamadasEConservaConfigurationImutavel(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	ctx := context.Background()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	layers := []string{"layer.a", "layer.b"}
	if err := state.SetActiveLayers(ctx, principalA.UserID, layers); err != nil {
		t.Fatal(err)
	}
	layers[0] = "alterada"
	gotConfig, gotLayers, err := state.UserConfiguration(ctx, principalA.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if gotConfig != config || len(gotLayers) != 2 || gotLayers[0] != "layer.a" {
		t.Fatalf("snapshot publicado foi alterado pela entrada: config=%p layers=%v", gotConfig, gotLayers)
	}
	gotLayers[0] = "alterada-na-saida"
	_, again, err := state.UserConfiguration(ctx, principalA.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if again[0] != "layer.a" {
		t.Fatalf("camadas retornadas não são detached: %v", again)
	}
}

func TestHostStateValidaErrosECancelamento(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); !errors.Is(err, context.Canceled) {
		t.Fatalf("publicação cancelada = %v", err)
	}
	if _, err := state.Snapshot(ctx, principalA); !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot cancelado = %v", err)
	}
	if _, _, err := state.UserConfiguration(ctx, principalA.UserID); !errors.Is(err, context.Canceled) {
		t.Fatalf("consulta cancelada = %v", err)
	}
	if err := state.SetActiveLayers(context.Background(), principalA.UserID, []string{"x", "x"}); !errors.Is(err, ErrInvalidHostLayers) {
		t.Fatalf("camadas duplicadas = %v", err)
	}
	if err := state.SetActiveLayers(context.Background(), principalA.UserID, []string{" "}); !errors.Is(err, ErrInvalidHostLayers) {
		t.Fatalf("camada vazia = %v", err)
	}
	if err := state.SetActiveLayers(context.Background(), principalA.UserID, []string{"x"}); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("usuário não publicado = %v", err)
	}
	if _, _, err := state.UserConfiguration(context.Background(), hostUUID(t)); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("configuração ausente = %v", err)
	}
}

func TestHostStateSnapshotDentroDeAdmitNaoReadquireGate(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	ctx := context.Background()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(ctx, principalA.UserID, principalA.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Epochs().Admit(ctx, epoch, func(ctx context.Context) error {
		_, err := state.Snapshot(ctx, principalA)
		return err
	}, func() error { return nil }); err != nil {
		t.Fatalf("snapshot dentro de Admit bloqueou ou falhou: %v", err)
	}
}

func TestHostStateOverflowDesabilitaEFalhaFechado(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	state.counter = math.MaxUint64
	if err := state.PublishUserConfiguration(context.Background(), principalA.UserID, config); !errors.Is(err, ErrHostGenerationOverflow) {
		t.Fatalf("overflow = %v", err)
	}
	if _, err := state.Snapshot(context.Background(), principalA); !errors.Is(err, ErrHostStateDisabled) {
		t.Fatalf("snapshot após overflow = %v", err)
	}
	if err := state.SetVaultUnlocked(context.Background(), true); !errors.Is(err, ErrHostStateDisabled) {
		t.Fatalf("mutação após overflow = %v", err)
	}
}

func TestHostStateForgetERepublicacaoNaoReutilizaGeracoes(t *testing.T) {
	state, principalA, _, config := hostStateFixture(t)
	ctx := context.Background()
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	first, err := state.Snapshot(ctx, principalA)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SetActiveLayers(ctx, principalA.UserID, []string{"layer.a"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ForgetUserConfiguration(ctx, principalA.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Snapshot(ctx, principalA); !errors.Is(err, ErrHostUserNotPublished) {
		t.Fatalf("snapshot após forget = %v", err)
	}
	if err := state.ForgetUserConfiguration(ctx, principalA.UserID); err != nil {
		t.Fatalf("forget idempotente = %v", err)
	}
	if err := state.PublishUserConfiguration(ctx, principalA.UserID, config); err != nil {
		t.Fatal(err)
	}
	second, err := state.Snapshot(ctx, principalA)
	if err != nil {
		t.Fatal(err)
	}
	if second.GlobalConfig == first.GlobalConfig || second.ActiveLayers == first.ActiveLayers {
		t.Fatalf("republicação reutilizou geração: primeira=%+v segunda=%+v", first, second)
	}
}

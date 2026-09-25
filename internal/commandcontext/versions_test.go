package commandcontext

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
)

type testProvider struct {
	snapshot Snapshot
	err      error
	calls    int
}

func (p *testProvider) Snapshot(context.Context, string) (Snapshot, error) {
	p.calls++
	return p.snapshot, p.err
}

type nilTestProvider struct{}

func (*nilTestProvider) Snapshot(context.Context, string) (Snapshot, error) { return Snapshot{}, nil }

func exactPolicy(facts ...commandcatalog.ContextFact) commandcatalog.ContextPolicy {
	return commandcatalog.ContextPolicy{Facts: facts}
}

func TestNewVersionServiceValidaRegistroECopiaMapa(t *testing.T) {
	provider := &testProvider{snapshot: Snapshot{Version: "v1"}}
	input := map[string]Provider{"surface": provider}
	service, err := NewVersionService(input)
	if err != nil {
		t.Fatal(err)
	}
	delete(input, "surface")
	if _, _, err := service.Capture(context.Background(), exactPolicy(commandcatalog.ContextFact{Provider: "surface", Fact: "active", Mode: commandcatalog.ExactVersion})); err != nil {
		t.Fatalf("registro não foi copiado: %v", err)
	}

	for name, providers := range map[string]map[string]Provider{
		"ID vazio":           {" ": provider},
		"provider nil":       {"surface": nil},
		"provider typed nil": {"surface": (*nilTestProvider)(nil)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewVersionService(providers); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
}

func TestCaptureFingerprintIndependeDaOrdemEMudaComVersao(t *testing.T) {
	service, err := NewVersionService(map[string]Provider{
		"b": &testProvider{snapshot: Snapshot{Version: "2"}},
		"a": &testProvider{snapshot: Snapshot{Version: "1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, fingerprint1, err := service.Capture(context.Background(), exactPolicy(
		commandcatalog.ContextFact{Provider: "b", Fact: "z", Mode: commandcatalog.ExactVersion},
		commandcatalog.ContextFact{Provider: "a", Fact: "x", Mode: commandcatalog.ExactVersion},
	))
	if err != nil {
		t.Fatal(err)
	}
	second, fingerprint2, err := service.Capture(context.Background(), exactPolicy(
		commandcatalog.ContextFact{Provider: "a", Fact: "x", Mode: commandcatalog.ExactVersion},
		commandcatalog.ContextFact{Provider: "b", Fact: "z", Mode: commandcatalog.ExactVersion},
	))
	if err != nil || fingerprint1 != fingerprint2 {
		t.Fatalf("fingerprints dependem da ordem: %q, %q, erro=%v", fingerprint1, fingerprint2, err)
	}
	if first[FactKey{"a", "x"}] != second[FactKey{"a", "x"}] || len(fingerprint1) != 64 {
		t.Fatal("captura ou fingerprint inválido")
	}

	changedService, err := NewVersionService(map[string]Provider{
		"a": &testProvider{snapshot: Snapshot{Version: "1"}},
		"b": &testProvider{snapshot: Snapshot{Version: "3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, fingerprint3, err := changedService.Capture(context.Background(), exactPolicy(
		commandcatalog.ContextFact{Provider: "a", Fact: "x", Mode: commandcatalog.ExactVersion},
		commandcatalog.ContextFact{Provider: "b", Fact: "z", Mode: commandcatalog.ExactVersion},
	))
	if err != nil || changed == nil || fingerprint3 == fingerprint1 {
		t.Fatalf("mudança de versão não alterou fingerprint: %q -> %q", fingerprint1, fingerprint3)
	}
}

func TestCaptureFalhaSemResultadoParcialEMExigeVersao(t *testing.T) {
	provider := &testProvider{snapshot: Snapshot{Version: "v1"}}
	service, _ := NewVersionService(map[string]Provider{"ok": provider})
	policy := exactPolicy(
		commandcatalog.ContextFact{Provider: "ok", Fact: "a", Mode: commandcatalog.ExactVersion},
		commandcatalog.ContextFact{Provider: "missing", Fact: "b", Mode: commandcatalog.ExactVersion},
	)
	if snapshots, version, err := service.Capture(context.Background(), policy); !errors.Is(err, ErrMissingSnapshot) || snapshots != nil || version != "" {
		t.Fatalf("missing = snapshots=%v version=%q err=%v", snapshots, version, err)
	}

	for name, providerErr := range map[string]error{"erro do provider": errors.New("falha"), "versão vazia": nil} {
		t.Run(name, func(t *testing.T) {
			p := &testProvider{snapshot: Snapshot{Version: "v1"}, err: providerErr}
			if name == "versão vazia" {
				p.snapshot.Version = " \t"
			}
			s, _ := NewVersionService(map[string]Provider{"p": p})
			if snapshots, version, err := s.Capture(context.Background(), exactPolicy(commandcatalog.ContextFact{Provider: "p", Fact: "f", Mode: commandcatalog.ExactVersion})); err == nil || snapshots != nil || version != "" {
				t.Fatalf("esperava falha fechada: snapshots=%v version=%q err=%v", snapshots, version, err)
			}
		})
	}
}

func TestCaptureCancelamentoENone(t *testing.T) {
	p := &testProvider{snapshot: Snapshot{Version: "v1"}}
	service, _ := NewVersionService(map[string]Provider{"p": p})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if snapshots, version, err := service.Capture(ctx, exactPolicy(commandcatalog.ContextFact{Provider: "p", Fact: "f", Mode: commandcatalog.ExactVersion})); !errors.Is(err, context.Canceled) || snapshots != nil || version != "" || p.calls != 0 {
		t.Fatalf("cancelamento = snapshots=%v version=%q err=%v calls=%d", snapshots, version, err, p.calls)
	}

	none := &testProvider{}
	noneService, _ := NewVersionService(map[string]Provider{"p": none})
	snapshots, version, err := noneService.Capture(context.Background(), commandcatalog.ContextPolicy{None: true})
	if err != nil || len(snapshots) != 0 || version != "" || none.calls != 0 {
		t.Fatalf("none = snapshots=%v version=%q err=%v calls=%d", snapshots, version, err, none.calls)
	}

	var nilService *VersionService
	if snapshots, version, err := nilService.Capture(context.Background(), commandcatalog.ContextPolicy{None: true}); err == nil || snapshots != nil || version != "" {
		t.Fatalf("receiver nil em Capture: snapshots=%v version=%q err=%v", snapshots, version, err)
	}
	if err := nilService.Revalidate(context.Background(), commandcatalog.ContextPolicy{None: true}, nil, time.Now()); err == nil {
		t.Fatal("receiver nil em Revalidate deveria falhar")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if snapshots, version, err := noneService.Capture(cancelled, commandcatalog.ContextPolicy{None: true}); !errors.Is(err, context.Canceled) || snapshots != nil || version != "" {
		t.Fatalf("none cancelado em Capture: snapshots=%v version=%q err=%v", snapshots, version, err)
	}
	if err := noneService.Revalidate(cancelled, commandcatalog.ContextPolicy{None: true}, nil, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("none cancelado em Revalidate: %v", err)
	}
}

func TestRevalidateIntegraFreshnessSemAlterarCapturado(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	provider := &testProvider{snapshot: Snapshot{Version: "v1"}}
	service, _ := NewVersionService(map[string]Provider{"p": provider})
	policy := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "p", Fact: "f", Mode: commandcatalog.MaxAge, MaxAgeMS: 1000}}}
	captured := Snapshots{FactKey{"p", "f"}: {Version: "v1", CapturedAt: now}}
	if err := service.Revalidate(context.Background(), policy, captured, now); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"", " \t"} {
		captured[FactKey{"p", "f"}] = Snapshot{Version: version, CapturedAt: now}
		if err := service.Revalidate(context.Background(), policy, captured, now); !errors.Is(err, ErrVersionMismatch) {
			t.Fatalf("versão capturada %q = %v", version, err)
		}
	}
	captured[FactKey{"p", "f"}] = Snapshot{Version: "v1", CapturedAt: now}
	provider.snapshot.Version = "v2"
	if err := service.Revalidate(context.Background(), policy, captured, now); err != nil {
		t.Fatalf("modo temporal não deve comparar versão: %v", err)
	}
	if !captured[FactKey{"p", "f"}].CapturedAt.Equal(now) {
		t.Fatal("Revalidate alterou timestamp capturado")
	}

	provider.snapshot.Version = "v1"
	captured[FactKey{"p", "f"}] = Snapshot{Version: "v1", CapturedAt: now.Add(-time.Second - time.Nanosecond)}
	if err := service.Revalidate(context.Background(), policy, captured, now); !errors.Is(err, ErrSnapshotExpired) {
		t.Fatalf("freshness não foi validada: %v", err)
	}
	if strings.TrimSpace(captured[FactKey{"p", "f"}].Version) == "" {
		t.Fatal("snapshot capturado foi alterado")
	}
}

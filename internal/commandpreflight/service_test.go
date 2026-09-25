package commandpreflight_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandpreflight"
)

type providerFunc func(context.Context, string) (commandcontext.Snapshot, error)

func (f providerFunc) Snapshot(ctx context.Context, fact string) (commandcontext.Snapshot, error) {
	return f(ctx, fact)
}

func fixture(t *testing.T, effect commandcatalog.Effect, deltas []commandbindings.Delta) (commandpreflight.HostSnapshot, *commandcontext.VersionService) {
	t.Helper()
	presentation := &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{}}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		presentation.Locales[locale] = commandcatalog.LocalizedMetadata{Name: "Test", Description: "Test", Category: "Test"}
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{
		ID: "workspace.inspect", Effect: effect, Decision: commandcatalog.NoDecision, AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal}, Presentation: presentation,
		Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.ExactVersion}}},
	}, Handler: commandcatalog.HandlerContract{Effect: effect}}})
	if err != nil {
		t.Fatal(err)
	}
	config, err := commandbindings.NewConfiguration([]commandbindings.Default{{Version: "1", Fingerprint: "fp", Candidate: commandbindings.Candidate{ID: "default", Trigger: "Ctrl+I", CommandID: "workspace.inspect", ArgumentsKey: "empty", ExecutionScopeKey: "workspace:1", Scope: commandbindings.Workspace, Enabled: true, LayerActive: true}}}, deltas, nil)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := commandcontext.NewVersionService(map[string]commandcontext.Provider{"workspace": providerFunc(func(context.Context, string) (commandcontext.Snapshot, error) {
		return commandcontext.Snapshot{Version: "v1"}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	return commandpreflight.HostSnapshot{UserID: "u1", AuthContextID: "s1", Generation: "g1", Registry: registry, Bindings: config}, versions
}

func service(t *testing.T, host commandpreflight.SnapshotProvider, versions *commandcontext.VersionService) *commandpreflight.Service {
	t.Helper()
	s, err := commandpreflight.New(host, versions, func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFluxoIntegraBindingsCatalogoEProvidersSemExecutar(t *testing.T) {
	h, v := fixture(t, commandcatalog.Read, nil)
	reads := 0
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) { reads++; return h, nil }, v)
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if err != nil || got.Status != commandpreflight.ReadChecksPassed || got.CommandID != "workspace.inspect" || got.ContextVersion == "" || !reflect.DeepEqual(got.BindingIDs, []string{"default"}) || reads != 2 {
		t.Fatalf("%+v reads=%d err=%v", got, reads, err)
	}
	got.BindingIDs[0] = "corrupt"
	again, err := s.Inspect(context.Background(), "Ctrl+I")
	if err != nil || again.BindingIDs[0] != "default" {
		t.Fatalf("retorno compartilhado: %+v %v", again, err)
	}
}

func TestPreflightRecusaEscritaMesmoComBindingValido(t *testing.T) {
	h, v := fixture(t, commandcatalog.Write, nil)
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) { return h, nil }, v)
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if err == nil || got.CommandID != "" {
		t.Fatalf("escrita aceita: %+v %v", got, err)
	}
}

func TestHostReautenticadoEComparadoNoFinal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*commandpreflight.HostSnapshot)
	}{
		{"logout", func(h *commandpreflight.HostSnapshot) { h.UserID = "" }},
		{"troca usuário", func(h *commandpreflight.HostSnapshot) { h.UserID = "u2" }},
		{"sessão", func(h *commandpreflight.HostSnapshot) { h.AuthContextID = "s2" }},
		{"configuração", func(h *commandpreflight.HostSnapshot) { h.Generation = "g2" }},
		{"foco", func(h *commandpreflight.HostSnapshot) {
			h.Facts = commandbindings.Facts{commandbindings.AppFocused: false}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, v := fixture(t, commandcatalog.Read, nil)
			calls := 0
			s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) {
				calls++
				copy := h
				if calls == 2 {
					tc.change(&copy)
				}
				return copy, nil
			}, v)
			got, err := s.Inspect(context.Background(), "Ctrl+I")
			if err == nil || got.CommandID != "" {
				t.Fatalf("mudança aceita: %+v %v", got, err)
			}
		})
	}
}

func TestContextoMudaEntreCapturaERevalidacao(t *testing.T) {
	h, _ := fixture(t, commandcatalog.Read, nil)
	calls := 0
	v, err := commandcontext.NewVersionService(map[string]commandcontext.Provider{"workspace": providerFunc(func(context.Context, string) (commandcontext.Snapshot, error) {
		calls++
		version := "v1"
		if calls > 1 {
			version = "v2"
		}
		return commandcontext.Snapshot{Version: version}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) { return h, nil }, v)
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if !errors.Is(err, commandcontext.ErrVersionMismatch) || got.CommandID != "" {
		t.Fatalf("stale aceito: %+v %v", got, err)
	}
}

func TestSupressaoESemMatchSaoSomenteDiagnostico(t *testing.T) {
	x := commandbindings.Delta{ID: "suppress", DefaultID: "default", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: "Ctrl+I", Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active}
	h, v := fixture(t, commandcatalog.Read, []commandbindings.Delta{x})
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) { return h, nil }, v)
	for trigger, want := range map[string]string{"Ctrl+I": commandpreflight.WouldSuppress, "Ctrl+X": string(commandbindings.NoMatch)} {
		got, err := s.Inspect(context.Background(), trigger)
		if err != nil || got.Status != want || got.CommandID != "" || got.ContextVersion != "" {
			t.Fatalf("%+v %v", got, err)
		}
	}
}

func TestCancelamentoAntesDaConsultaNaoTocaHost(t *testing.T) {
	_, v := fixture(t, commandcatalog.Read, nil)
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) {
		t.Fatal("host consultado")
		return commandpreflight.HostSnapshot{}, nil
	}, v)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Inspect(ctx, "Ctrl+I"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestHostSemAutenticacaoNaoConsultaProviders(t *testing.T) {
	h, _ := fixture(t, commandcatalog.Read, nil)
	h.UserID = ""
	v, err := commandcontext.NewVersionService(map[string]commandcontext.Provider{"workspace": providerFunc(func(context.Context, string) (commandcontext.Snapshot, error) {
		t.Fatal("provider consultado")
		return commandcontext.Snapshot{}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) { return h, nil }, v)
	if _, err = s.Inspect(context.Background(), "Ctrl+I"); !errors.Is(err, commandpreflight.ErrUnauthenticated) {
		t.Fatal(err)
	}
}

func TestTempoGastoNaReautenticacaoPodeExpirarSnapshot(t *testing.T) {
	h, _ := fixture(t, commandcatalog.Read, nil)
	d, _ := h.Registry.Lookup("workspace.inspect")
	d.Context.Facts[0].Mode = commandcatalog.MaxAge
	d.Context.Facts[0].MaxAgeMS = 100
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: d, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
	if err != nil {
		t.Fatal(err)
	}
	h.Registry = registry
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := at
	v, err := commandcontext.NewVersionService(map[string]commandcontext.Provider{"workspace": providerFunc(func(context.Context, string) (commandcontext.Snapshot, error) {
		return commandcontext.Snapshot{Version: "v1", CapturedAt: at}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s, err := commandpreflight.New(func(context.Context) (commandpreflight.HostSnapshot, error) {
		calls++
		if calls == 2 {
			now = at.Add(time.Second)
		}
		return h, nil
	}, v, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if !errors.Is(err, commandcontext.ErrSnapshotExpired) || got.CommandID != "" {
		t.Fatalf("contexto vencido aceito: %+v %v", got, err)
	}
}

func TestErroNoHostFinalNaoDevolveDiagnosticoParcial(t *testing.T) {
	h, v := fixture(t, commandcatalog.Read, nil)
	calls := 0
	denied := errors.New("revogado")
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) {
		calls++
		if calls == 2 {
			return commandpreflight.HostSnapshot{}, denied
		}
		return h, nil
	}, v)
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if !errors.Is(err, denied) || !reflect.DeepEqual(got, commandpreflight.Diagnostic{}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSnapshotsReconstruidosNaMesmaGeracaoNaoSaoStale(t *testing.T) {
	first, v := fixture(t, commandcatalog.Read, nil)
	second, _ := fixture(t, commandcatalog.Read, nil)
	calls := 0
	s := service(t, func(context.Context) (commandpreflight.HostSnapshot, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}, v)
	got, err := s.Inspect(context.Background(), "Ctrl+I")
	if err != nil || got.Status != commandpreflight.ReadChecksPassed {
		t.Fatalf("reconstrução equivalente recusada: %+v %v", got, err)
	}
}

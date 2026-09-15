package commandcontext

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

type scopedFixtureProvider struct {
	mu       sync.Mutex
	owner    Scope
	version  string
	captured time.Time
	byFact   map[string]time.Time
	calls    int
	err      error
}

func (p *scopedFixtureProvider) Snapshot(_ context.Context, scope Scope, fact string) (OwnedSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return OwnedSnapshot{}, p.err
	}
	captured := p.captured
	if p.byFact != nil {
		if byFact, ok := p.byFact[fact]; ok {
			captured = byFact
		}
	}
	return OwnedSnapshot{Owner: p.owner, Snapshot: Snapshot{Version: p.version, CapturedAt: captured}}, nil
}

func newScopeFixture(t *testing.T, workspace bool) Scope {
	t.Helper()
	user, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{UserID: user.String(), AuthContextID: "opaque-auth-context"}
	if workspace {
		// O formato existente é deliberadamente opaco para este pacote.
		ws := "ws-0192f3a4b5c6d7e8"
		scope.WorkspaceID = &ws
	}
	return scope
}

func temporalPolicy(mode commandcatalog.ContextMode, maxAge int64) commandcatalog.ContextPolicy {
	return commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
		Provider: "surface", Fact: "active", Mode: mode, MaxAgeMS: maxAge,
	}}}
}

func TestScopeIDsUserUUID7WorkspaceOpacoEAuthSemNormalizacao(t *testing.T) {
	scope := newScopeFixture(t, true)
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]Scope{
		"user não uuid7":       {UserID: "user-1", AuthContextID: "auth"},
		"workspace vazio":      {UserID: scope.UserID, AuthContextID: "auth", WorkspaceID: ptr("")},
		"workspace whitespace": {UserID: scope.UserID, AuthContextID: "auth", WorkspaceID: ptr(" \t")},
		"auth vazio":           {UserID: scope.UserID, AuthContextID: ""},
		"auth utf8 inválido":   {UserID: scope.UserID, AuthContextID: string([]byte{0xff})},
		"NUL no workspace":     {UserID: scope.UserID, AuthContextID: "auth", WorkspaceID: ptr("ws-\x00")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("scope aceito: %v", err)
			}
		})
	}
	withSpaces := scope
	withSpaces.AuthContextID = " auth com espaços "
	if err := withSpaces.Validate(); err != nil {
		t.Fatalf("auth opaco válido foi normalizado/rejeitado: %v", err)
	}
}

func TestFactBusOwnerSpoofNilProviderETimestampNaoFabricado(t *testing.T) {
	scope := newScopeFixture(t, true)
	foreign := newScopeFixture(t, true)
	provider := &scopedFixtureProvider{owner: foreign, version: "v1", captured: time.Now().UTC()}
	bus, err := NewFactBus(map[string]ScopedProvider{"surface": provider})
	if err != nil {
		t.Fatal(err)
	}
	_, err = bus.Capture(context.Background(), scope, temporalPolicy(commandcatalog.MaxAge, 1000))
	if !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("owner spoof não rejeitado: %v", err)
	}
	var typedNil *scopedFixtureProvider
	if _, err := NewFactBus(map[string]ScopedProvider{"surface": typedNil}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("provider typed nil: %v", err)
	}
	provider.owner = scope
	provider.captured = time.Time{}
	if _, err := bus.Capture(context.Background(), scope, temporalPolicy(commandcatalog.EventSnapshot, 1000)); !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("timestamp foi fabricado/aceito: %v", err)
	}
}

func TestFactBusTTLReconsultaFonteEventSnapshotEProviderCapturedAt(t *testing.T) {
	scope := newScopeFixture(t, true)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	provider := &scopedFixtureProvider{owner: scope, version: "v1", captured: now.Add(-500 * time.Millisecond)}
	bus, _ := NewFactBus(map[string]ScopedProvider{"surface": provider})
	policy := temporalPolicy(commandcatalog.EventSnapshot, 1000)
	proof, err := bus.Capture(context.Background(), scope, policy)
	if err != nil {
		t.Fatal(err)
	}
	provider.captured = now
	if err := bus.Revalidate(context.Background(), scope, policy, proof, now); err != nil {
		t.Fatalf("event snapshot válido rejeitado: %v", err)
	}
	provider.captured = now.Add(-time.Second - time.Nanosecond)
	if err := bus.Revalidate(context.Background(), scope, policy, proof, now); err != nil {
		t.Fatalf("revalidação deve usar timestamp inicial: %v", err)
	}
	provider.captured = now
	provider.version = "v2"
	provider.captured = now.Add(-2 * time.Second)
	if err := bus.Revalidate(context.Background(), scope, temporalPolicy(commandcatalog.MaxAge, 1000), proof, now); !errors.Is(err, ErrPolicyMismatch) {
		t.Fatalf("policy divergente: %v", err)
	}
	if got := proof.ProviderCapturedAt()["surface"]; !got.Equal(now.Add(-500 * time.Millisecond)) {
		t.Fatalf("timestamp inicial não preservado: %v", got)
	}

	first := &scopedFixtureProvider{owner: scope, version: "first", byFact: map[string]time.Time{
		"active": now.Add(-2 * time.Millisecond), "focused": now.Add(-time.Millisecond),
	}}
	bus, _ = NewFactBus(map[string]ScopedProvider{"surface": first})
	proof, err = bus.Capture(context.Background(), scope, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{
		{Provider: "surface", Fact: "active", Mode: commandcatalog.MaxAge, MaxAgeMS: 1000},
		{Provider: "surface", Fact: "focused", Mode: commandcatalog.MaxAge, MaxAgeMS: 1000},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := proof.ProviderCapturedAt()["surface"]; !got.Equal(now.Add(-2 * time.Millisecond)) {
		t.Fatalf("capturedAt do provider incorreto: %v", got)
	}
}

func TestFactBusNotificationDropOutOfOrderNaoConcedeAtualidadeEProvaEntreBuses(t *testing.T) {
	scope := newScopeFixture(t, true)
	now := time.Now().UTC()
	provider := &scopedFixtureProvider{owner: scope, version: "v1", captured: now}
	bus, _ := NewFactBus(map[string]ScopedProvider{"surface": provider})
	proof, err := bus.Capture(context.Background(), scope, temporalPolicy(commandcatalog.ExactVersion, 0))
	if err != nil {
		t.Fatal(err)
	}
	changes, cancel, err := bus.Subscribe(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	provider.version = "v2"
	if err := bus.Notify(context.Background(), scope, "surface", "active", OwnedSnapshot{Owner: scope}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Notify(context.Background(), scope, "surface", "active", OwnedSnapshot{Owner: scope}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("notify não publicou hint dirty")
	}
	if err := bus.Revalidate(context.Background(), scope, temporalPolicy(commandcatalog.ExactVersion, 0), proof, now); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("revalidação não consultou fonte: %v", err)
	}
	other, _ := NewFactBus(map[string]ScopedProvider{"surface": provider})
	if err := other.Revalidate(context.Background(), scope, temporalPolicy(commandcatalog.ExactVersion, 0), proof, now); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("prova de outro bus aceita: %v", err)
	}
	if provider.calls < 2 {
		t.Fatalf("fonte não foi reconsultada: %d", provider.calls)
	}
}

func TestGenerationStoreGlobalPorUserLocalPorWorkspaceEStartupPrefix(t *testing.T) {
	gate := &commandsecurity.DispatchGate{}
	store, err := NewGenerationStore(gate)
	if err != nil {
		t.Fatal(err)
	}
	userScope := newScopeFixture(t, false)
	ws1, ws2 := "ws-0192f3a4b5c6d7e8", "ws-0192f3a4b5c6d7e9"
	a := userScope
	a.WorkspaceID = &ws1
	b := userScope
	b.AuthContextID = "second-session"
	b.WorkspaceID = &ws1
	c := userScope
	c.WorkspaceID = &ws2
	initialA, _ := store.Snapshot(a)
	initialB, _ := store.Snapshot(b)
	initialC, _ := store.Snapshot(c)
	if initialA.GlobalConfigGeneration != initialB.GlobalConfigGeneration {
		t.Fatal("sessões do mesmo usuário não compartilham geração global")
	}
	if err := store.BumpGlobalConfig(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	afterGlobalB, _ := store.Snapshot(b)
	afterGlobalC, _ := store.Snapshot(c)
	if afterGlobalB.GlobalConfigGeneration == initialB.GlobalConfigGeneration || afterGlobalC.GlobalConfigGeneration == initialC.GlobalConfigGeneration {
		t.Fatal("mutação global não invalidou todos os workspaces do usuário")
	}
	if err := store.BumpWorkspaceConfig(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	afterLocalA, _ := store.Snapshot(a)
	afterLocalC, _ := store.Snapshot(c)
	if afterLocalA.WorkspaceConfigGeneration == nil || afterLocalC.WorkspaceConfigGeneration == nil ||
		*afterLocalA.WorkspaceConfigGeneration == *initialA.WorkspaceConfigGeneration ||
		*afterLocalC.WorkspaceConfigGeneration != *initialC.WorkspaceConfigGeneration {
		t.Fatal("mutação local não isolou workspace")
	}
	if err := store.BumpActiveLayers(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	activeA, _ := store.Snapshot(a)
	activeC, _ := store.Snapshot(c)
	if activeA.ActiveLayersGeneration == afterLocalA.ActiveLayersGeneration || activeC.ActiveLayersGeneration != afterLocalC.ActiveLayersGeneration {
		t.Fatal("active layers não foi isolado por workspace")
	}
	global, _ := store.Snapshot(userScope)
	if err := store.BumpActiveLayers(context.Background(), userScope); err != nil {
		t.Fatal(err)
	}
	globalChangeA, _ := store.Snapshot(a)
	globalChangeC, _ := store.Snapshot(c)
	if globalChangeA.ActiveLayersGeneration == activeA.ActiveLayersGeneration || globalChangeC.ActiveLayersGeneration == activeC.ActiveLayersGeneration {
		t.Fatal("mudança de camadas globais não alcançou todos os workspaces do usuário")
	}
	if global.WorkspaceConfigGeneration != nil {
		t.Fatal("workspace config global não é nil")
	}
	otherStore, _ := NewGenerationStore(gate)
	other, _ := otherStore.Snapshot(a)
	if other.GlobalConfigGeneration == activeA.GlobalConfigGeneration {
		t.Fatal("startup não gerou prefixo novo")
	}
}

func ptr(value string) *string { return &value }

func TestFactBusProviderPanicFailsClosed(t *testing.T) {
	bus, err := NewFactBus(map[string]ScopedProvider{"surface": ScopedProviderFunc(func(context.Context, Scope, string) (OwnedSnapshot, error) { panic("sensitive-provider-error") })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = bus.Capture(context.Background(), newScopeFixture(t, true), temporalPolicy(commandcatalog.ExactVersion, 0))
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("panic não virou indisponibilidade: %v", err)
	}
}

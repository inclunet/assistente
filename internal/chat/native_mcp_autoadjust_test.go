package chat

import (
	"sync"
	"testing"

	"assistente/internal/profiles"
)

// newAutoAdjustInteractor cria um Interactor mínimo com apenas o profileMgr, que é
// tudo que HandleNativeMCPUnsupported precisa.
func newAutoAdjustInteractor(mgr *profiles.Manager) *Interactor {
	return NewInteractor(InteractorConfig{ProfileMgr: mgr})
}

func createProfile(t *testing.T, mgr *profiles.Manager, name string, active bool, native *bool) string {
	t.Helper()
	p := profiles.DefaultProfile()
	p.Name = name
	p.Active = active
	p.Chat.NativeMCP = native
	slug, err := mgr.Create(p)
	if err != nil {
		t.Fatalf("create profile %q: %v", name, err)
	}
	return slug
}

// TestHandleNativeMCPUnsupported_AutoPersistsFalse: no modo auto (nil), o erro de
// não-suporte deve persistir Profile.Chat.NativeMCP=false (memória no perfil).
func TestHandleNativeMCPUnsupported_AutoPersistsFalse(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	slug := createProfile(t, mgr, "Auto", true, nil)
	inter := newAutoAdjustInteractor(mgr)

	// override nil; slug vazio → recai sobre o perfil ativo.
	inter.HandleNativeMCPUnsupported("", "deepseek-v4-flash", nil)

	got, err := mgr.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.NativeMCP == nil {
		t.Fatal("esperava NativeMCP persistido (false), obteve nil")
	}
	if *got.Chat.NativeMCP {
		t.Fatal("esperava NativeMCP=false (adapter), obteve true")
	}
}

// TestHandleNativeMCPUnsupported_ForceTrueNotOverwritten: override explícito true
// não deve ser sobrescrito — o usuário mandou forçar nativo.
func TestHandleNativeMCPUnsupported_ForceTrueNotOverwritten(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	forceTrue := true
	slug := createProfile(t, mgr, "ForcaNativo", true, &forceTrue)
	inter := newAutoAdjustInteractor(mgr)

	inter.HandleNativeMCPUnsupported(slug, "deepseek-v4-flash", &forceTrue)

	got, err := mgr.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.NativeMCP == nil || !*got.Chat.NativeMCP {
		t.Fatalf("esperava NativeMCP=true preservado, obteve %v", got.Chat.NativeMCP)
	}
}

// TestHandleNativeMCPUnsupported_ForceFalseNoOp: override false já é adapter; nada
// a fazer (sem erro, permanece false).
func TestHandleNativeMCPUnsupported_ForceFalseNoOp(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	forceFalse := false
	slug := createProfile(t, mgr, "ForcaAdapter", true, &forceFalse)
	inter := newAutoAdjustInteractor(mgr)

	inter.HandleNativeMCPUnsupported(slug, "qualquer", &forceFalse)

	got, err := mgr.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.NativeMCP == nil || *got.Chat.NativeMCP {
		t.Fatalf("esperava NativeMCP=false inalterado, obteve %v", got.Chat.NativeMCP)
	}
}

// TestHandleNativeMCPUnsupported_SubAgentSlug: o auto-ajuste recai sobre o profile
// efetivamente usado no run (slug explícito), não sobre o ativo global.
func TestHandleNativeMCPUnsupported_SubAgentSlug(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	activeSlug := createProfile(t, mgr, "Ativo", true, nil)
	subSlug := createProfile(t, mgr, "SubAgente", false, nil)
	inter := newAutoAdjustInteractor(mgr)

	inter.HandleNativeMCPUnsupported(subSlug, "modelo-sub", nil)

	sub, err := mgr.Get(subSlug)
	if err != nil {
		t.Fatalf("get sub: %v", err)
	}
	if sub.Chat.NativeMCP == nil || *sub.Chat.NativeMCP {
		t.Fatalf("sub-agente: esperava NativeMCP=false, obteve %v", sub.Chat.NativeMCP)
	}
	// O perfil ativo global não deve ter sido tocado.
	active, err := mgr.Get(activeSlug)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active.Chat.NativeMCP != nil {
		t.Fatalf("perfil ativo não deveria ter sido ajustado, obteve %v", *active.Chat.NativeMCP)
	}
}

// TestHandleNativeMCPUnsupported_ConcurrentIdempotent: vários runs simultâneos do
// mesmo perfil em auto não causam corrida (rode com -race) e convergem para false.
func TestHandleNativeMCPUnsupported_ConcurrentIdempotent(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	slug := createProfile(t, mgr, "Concorrente", true, nil)
	inter := newAutoAdjustInteractor(mgr)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inter.HandleNativeMCPUnsupported("", "modelo", nil)
		}()
	}
	wg.Wait()

	got, err := mgr.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.NativeMCP == nil || *got.Chat.NativeMCP {
		t.Fatalf("esperava NativeMCP=false após concorrência, obteve %v", got.Chat.NativeMCP)
	}
}

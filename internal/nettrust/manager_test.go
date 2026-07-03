package nettrust

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/tools/invocationctx"
)

func ctxWith(convID, profileSlug string) context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: convID,
		ProfileSlug:    profileSlug,
	})
}

func TestManager_GlobalPersistenceMatch(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	if d := m.Match(ctx, "api.nu.workflows.dev", "443"); d.Allowed {
		t.Fatal("host não deveria estar autorizado antes de persistir")
	}

	if err := m.Add(ctx, AllowlistEntry{Host: "api.nu.workflows.dev", Port: "443", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add global: %v", err)
	}

	// Um Manager novo lendo os mesmos diretórios deve enxergar a entrada persistida.
	m2 := NewManagerWithDirs(dir, dir)
	d := m2.Match(ctx, "api.nu.workflows.dev", "443")
	if !d.Allowed || d.Scope != ScopeGlobal {
		t.Fatalf("esperado match global persistido, got %+v", d)
	}
}

func TestManager_WildcardMatchAndApex(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	if err := m.Add(ctx, AllowlistEntry{Host: "*.nu.workflows.dev", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add wildcard: %v", err)
	}

	if d := m.Match(ctx, "api.nu.workflows.dev", "443"); !d.Allowed {
		t.Fatal("wildcard deveria casar subdomínio")
	}
	if d := m.Match(ctx, "a.b.nu.workflows.dev", ""); !d.Allowed {
		t.Fatal("wildcard deveria casar subdomínio profundo")
	}
	// Apex NÃO deve casar o wildcard "*.dominio".
	if d := m.Match(ctx, "nu.workflows.dev", "443"); d.Allowed {
		t.Fatal("wildcard não deve casar o apex")
	}
}

func TestManager_SimilarHostStillBlocked(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	if err := m.Add(ctx, AllowlistEntry{Host: "api.nu.workflows.dev", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Hosts parecidos, mas diferentes, continuam bloqueados mesmo que caiam na
	// mesma faixa de IP interna.
	for _, h := range []string{
		"api.nu.workflows.dev.evil.com",
		"evil-api.nu.workflows.dev",
		"apinu.workflows.dev",
		"api.nu.workflows.dev.",
	} {
		d := m.Match(ctx, h, "")
		if h == "api.nu.workflows.dev." {
			// FQDN com ponto final é o MESMO host (normalizado) — deve casar.
			if !d.Allowed {
				t.Fatalf("host %q (FQDN com ponto) deveria casar após normalização", h)
			}
			continue
		}
		if d.Allowed {
			t.Fatalf("host semelhante %q não deveria estar autorizado", h)
		}
	}
}

func TestManager_PortScopedMatch(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	if err := m.Add(ctx, AllowlistEntry{Host: "svc.internal", Port: "8443", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if d := m.Match(ctx, "svc.internal", "8443"); !d.Allowed {
		t.Fatal("porta correta deveria casar")
	}
	if d := m.Match(ctx, "svc.internal", "443"); d.Allowed {
		t.Fatal("porta diferente não deveria casar quando a entrada fixa porta")
	}
}

func TestManager_SessionScopeIsolatedPerConversation(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)

	ctxA := ctxWith("conv-A", "")
	ctxB := ctxWith("conv-B", "")

	if err := m.Add(ctxA, AllowlistEntry{Host: "api.internal", Scope: ScopeSession}); err != nil {
		t.Fatalf("Add session: %v", err)
	}
	if d := m.Match(ctxA, "api.internal", ""); !d.Allowed || d.Scope != ScopeSession {
		t.Fatalf("sessão A deveria estar autorizada, got %+v", d)
	}
	if d := m.Match(ctxB, "api.internal", ""); d.Allowed {
		t.Fatal("sessão B não deveria herdar autorização da sessão A")
	}
	// Sessão não é persistida em disco.
	m2 := NewManagerWithDirs(dir, dir)
	if d := m2.Match(ctxA, "api.internal", ""); d.Allowed {
		t.Fatal("escopo de sessão não deveria persistir entre managers")
	}
}

func TestManager_ProfileScopePersistsPerSlug(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)

	ctxP := ctxWith("", "programacao")
	if err := m.Add(ctxP, AllowlistEntry{Host: "api.internal", Scope: ScopeProfile}); err != nil {
		t.Fatalf("Add profile: %v", err)
	}
	if d := m.Match(ctxP, "api.internal", ""); !d.Allowed || d.Scope != ScopeProfile {
		t.Fatalf("perfil deveria estar autorizado, got %+v", d)
	}
	// Outro perfil não vê a entrada.
	if d := m.Match(ctxWith("", "revisor"), "api.internal", ""); d.Allowed {
		t.Fatal("outro perfil não deveria ver a entrada")
	}
}

func TestManager_Remove(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	_ = m.Add(ctx, AllowlistEntry{Host: "api.internal", Scope: ScopeGlobal})
	if d := m.Match(ctx, "api.internal", ""); !d.Allowed {
		t.Fatal("deveria estar autorizado antes do remove")
	}
	if err := m.Remove(ctx, ScopeGlobal, "api.internal", ""); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if d := m.Match(ctx, "api.internal", ""); d.Allowed {
		t.Fatal("não deveria estar autorizado após remove")
	}
}

// Add/Remove/Match concorrentes num escopo persistido não podem perder entradas
// nem provocar data race (rodar com -race). Exercita o read-modify-write dos
// arquivos sob concorrência — a regressão que o RWMutex + escrita atômica corrige.
func TestManager_ConcurrentAddMatchNoRace(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			host := fmt.Sprintf("svc-%d.internal", i)
			if err := m.Add(ctx, AllowlistEntry{Host: host, Scope: ScopeGlobal}); err != nil {
				t.Errorf("Add concorrente: %v", err)
				return
			}
			// Leituras concorrentes durante as escritas de outras goroutines.
			m.Match(ctx, host, "")
			m.List(ctx)
		}(i)
	}
	wg.Wait()

	// Nenhuma entrada pode ter sido perdida pelo read-modify-write concorrente.
	for i := 0; i < workers; i++ {
		host := fmt.Sprintf("svc-%d.internal", i)
		if d := m.Match(ctx, host, ""); !d.Allowed {
			t.Errorf("entrada perdida sob concorrência: %s", host)
		}
	}
	// Recarrega de disco para garantir persistência íntegra.
	m2 := NewManagerWithDirs(dir, dir)
	for i := 0; i < workers; i++ {
		host := fmt.Sprintf("svc-%d.internal", i)
		if d := m2.Match(ctx, host, ""); !d.Allowed {
			t.Errorf("entrada não persistida sob concorrência: %s", host)
		}
	}
}

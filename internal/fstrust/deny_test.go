package fstrust

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_MatchDenyPrecedenciaSobreAllow(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	alvo := filepath.Join(dir, "segredo.txt")

	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindFile, Operation: "read", Effect: EffectAllow, Scope: ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add allow: %v", err)
	}
	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindFile, Operation: "read", Effect: EffectDeny, Scope: ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add deny: %v", err)
	}

	// Allow e deny coexistem; MatchDeny tem precedência no Authorizer/pathutil.
	if !m.Match(ctx, alvo, "read").Allowed {
		t.Fatal("allow deveria continuar existindo")
	}
	if d := m.MatchDeny(ctx, alvo, "read"); d.Entry == nil || NormalizedEffect(d.Entry.Effect) != EffectDeny {
		t.Fatalf("MatchDeny deveria achar deny: %#v", d)
	}
	if len(m.List(ctx)) != 2 {
		t.Fatalf("want 2 entradas (allow+deny), got %d", len(m.List(ctx)))
	}
}

func TestManager_DenyEAllowCoexistemEmOperacoesDiferentes(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	alvo := filepath.Join(dir, "docs")

	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindDir, Operation: "read", Effect: EffectAllow, Scope: ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add allow read: %v", err)
	}
	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindDir, Operation: "write", Effect: EffectDeny, Scope: ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add deny write: %v", err)
	}

	child := filepath.Join(alvo, "a.txt")
	if !m.Match(ctx, child, "read").Allowed {
		t.Fatal("read deveria continuar permitido")
	}
	if m.MatchDeny(ctx, child, "write").Entry == nil {
		t.Fatal("write deveria estar na denylist")
	}
}

func TestManager_LegacySemEffectContaComoAllow(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	alvo := filepath.Join(dir, "legado.txt")

	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindFile, Operation: "read", Scope: ScopeGlobal,
		// Effect vazio de propósito
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !m.Match(ctx, alvo, "read").Allowed {
		t.Fatal("entrada sem effect deveria casar como allow")
	}
	if m.MatchDeny(ctx, alvo, "read").Entry != nil {
		t.Fatal("entrada sem effect não deveria casar como deny")
	}
}

func TestAuthorizer_DenyBloqueiaSemPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	alvo := filepath.Join(dir, "bloqueado.txt")
	if err := m.Add(ctx, AllowlistEntry{
		Path: alvo, Kind: KindFile, Operation: "read", Effect: EffectDeny, Scope: ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add deny: %v", err)
	}

	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal, Kind: KindFile}}
	auth := NewAuthorizer(m, prompt)
	err := auth.Authorize(ctx, alvo, "read")
	if err == nil {
		t.Fatal("deny deveria bloquear")
	}
	if prompt.called != 0 {
		t.Fatalf("deny não deve abrir prompt, called=%d", prompt.called)
	}
	if !strings.Contains(err.Error(), "denylist") {
		t.Fatalf("mensagem deveria citar denylist: %v", err)
	}
}

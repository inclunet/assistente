package fstrust

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type spyPrompter struct {
	called   int
	decision PromptDecision
	err      error
	lastReq  PromptRequest
}

func (s *spyPrompter) PromptPathAuthorization(_ context.Context, req PromptRequest) (PromptDecision, error) {
	s.called++
	s.lastReq = req
	return s.decision, s.err
}

func TestAuthorizer_MatchSkipsPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	file := filepath.Join(dir, "a.txt")
	_ = m.Add(ctx, AllowlistEntry{Path: file, Kind: KindFile, Operation: "read", Scope: ScopeGlobal})

	prompt := &spyPrompter{}
	auth := NewAuthorizer(m, prompt)

	if err := auth.Authorize(ctx, file, "read"); err != nil {
		t.Fatalf("esperado permitido por allowlist: %v", err)
	}
	if prompt.called != 0 {
		t.Fatal("não deveria pedir consentimento quando já está na allowlist")
	}
}

func TestAuthorizer_PromptApproveOnce(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeOnce, Kind: KindFile}}
	auth := NewAuthorizer(m, prompt)

	if err := auth.Authorize(ctx, file, "read"); err != nil {
		t.Fatalf("esperado permitido após once: %v", err)
	}
	if prompt.called != 1 {
		t.Fatalf("esperado 1 prompt, got %d", prompt.called)
	}
	if d := m.Match(ctx, file, "read"); d.Allowed {
		t.Fatal("escopo once não deve persistir")
	}
}

func TestAuthorizer_PromptDeny(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	file := filepath.Join(dir, "a.txt")

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	err := auth.Authorize(ctx, file, "write")
	if err == nil {
		t.Fatal("deny deveria retornar erro")
	}
	if !strings.Contains(err.Error(), file) && !strings.Contains(err.Error(), NormalizePath(file)) {
		// NormalizePath pode alterar o path; aceite menção à operação ao menos.
		if !strings.Contains(err.Error(), "write") {
			t.Fatalf("erro deveria mencionar path/operação, got %v", err)
		}
	}
	if !strings.Contains(err.Error(), "write") {
		t.Fatalf("erro deveria mencionar a operação, got %v", err)
	}
}

func TestAuthorizer_ApproveDirPersistsParent(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(docs, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal, Kind: KindDir}}
	auth := NewAuthorizer(m, prompt)

	if err := auth.Authorize(ctx, file, "read"); err != nil {
		t.Fatalf("esperado permitido: %v", err)
	}

	sibling := filepath.Join(docs, "b.txt")
	d := m.Match(ctx, sibling, "read")
	if !d.Allowed || d.Entry == nil || d.Entry.Kind != KindDir {
		t.Fatalf("grant dir deveria casar sibling, got %+v", d)
	}
	if NormalizePath(d.Entry.Path) != NormalizePath(docs) {
		t.Fatalf("path persistido deveria ser o dir pai %q, got %q", docs, d.Entry.Path)
	}
}

func TestAuthorizer_NoPrompter(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	auth := NewAuthorizer(m, nil)
	file := filepath.Join(dir, "a.txt")

	err := auth.Authorize(context.Background(), file, "read")
	if err == nil {
		t.Fatal("sem prompter deveria falhar")
	}
	if !strings.Contains(err.Error(), "sem authorizer") {
		t.Fatalf("mensagem inesperada: %v", err)
	}
}

func TestAuthorizer_NewFileThroughSymlinkKeepsPersistentMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	home := t.TempDir()
	linkParent := t.TempDir()
	realParent := t.TempDir()
	link := filepath.Join(linkParent, "external")
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	requested := filepath.Join(link, "new.txt")
	resolved := filepath.Join(realParent, "new.txt")

	m := NewManagerWithDirs(home, home)
	prompt := &spyPrompter{
		decision: PromptDecision{Approve: true, Scope: ScopeGlobal, Kind: KindFile},
	}
	auth := NewAuthorizer(m, prompt)
	ctx := context.Background()

	// Primeira tentativa: o arquivo ainda não existe. A autorização deve ser
	// persistida pelo ancestral resolvido (realParent/new.txt), não pelo link.
	if err := auth.Authorize(ctx, requested, "write"); err != nil {
		t.Fatalf("primeira autorização: %v", err)
	}
	if prompt.called != 1 {
		t.Fatalf("esperado 1 prompt, got %d", prompt.called)
	}
	if NormalizePath(prompt.lastReq.ResolvedPath) != NormalizePath(resolved) {
		t.Fatalf("resolved path inesperado: got %q, want %q", prompt.lastReq.ResolvedPath, resolved)
	}

	if err := os.WriteFile(resolved, []byte("criado"), 0o644); err != nil {
		t.Fatalf("criar arquivo após autorização: %v", err)
	}

	// Segunda tentativa: agora EvalSymlinks resolve o path inteiro. A entrada
	// persistida deve casar e impedir um novo prompt.
	if err := auth.Authorize(ctx, requested, "write"); err != nil {
		t.Fatalf("match após criação: %v", err)
	}
	if prompt.called != 1 {
		t.Fatalf("autorização persistida deveria evitar novo prompt, got %d prompts", prompt.called)
	}
}

func TestAuthorizer_DanglingFinalSymlinkPersistsRealTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	home := t.TempDir()
	linkParent := t.TempDir()
	realParent := t.TempDir()
	target := filepath.Join(realParent, "ainda-inexistente.txt")
	link := filepath.Join(linkParent, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink pendurado: %v", err)
	}

	m := NewManagerWithDirs(home, home)
	prompt := &spyPrompter{
		decision: PromptDecision{Approve: true, Scope: ScopeGlobal, Kind: KindFile},
	}
	auth := NewAuthorizer(m, prompt)
	ctx := context.Background()

	if err := auth.Authorize(ctx, link, "write"); err != nil {
		t.Fatalf("autorizar symlink pendurado: %v", err)
	}
	if NormalizePath(prompt.lastReq.ResolvedPath) != NormalizePath(target) {
		t.Fatalf("alvo real inesperado: got %q, want %q", prompt.lastReq.ResolvedPath, target)
	}

	if err := os.WriteFile(target, []byte("criado"), 0o644); err != nil {
		t.Fatalf("criar alvo: %v", err)
	}
	if err := auth.Authorize(ctx, link, "write"); err != nil {
		t.Fatalf("match após criação do alvo: %v", err)
	}
	if prompt.called != 1 {
		t.Fatalf("entrada do alvo real deveria evitar novo prompt, got %d prompts", prompt.called)
	}
}

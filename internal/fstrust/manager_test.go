package fstrust

import (
	"context"
	"fmt"
	"path/filepath"
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

func TestManager_MatchFileExactAndOperation(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	file := filepath.Join(dir, "docs", "a.txt")
	if err := m.Add(ctx, AllowlistEntry{
		Path:      file,
		Kind:      KindFile,
		Operation: "read",
		Scope:     ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	d := m.Match(ctx, file, "read")
	if !d.Allowed || d.Scope != ScopeGlobal {
		t.Fatalf("esperado match file+read, got %+v", d)
	}
	if d := m.Match(ctx, file, "write"); d.Allowed {
		t.Fatal("read não deve liberar write")
	}
}

func TestManager_FileGrantDoesNotMatchSiblingOrParentList(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	docs := filepath.Join(dir, "docs")
	file := filepath.Join(docs, "a.txt")
	sibling := filepath.Join(docs, "b.txt")

	if err := m.Add(ctx, AllowlistEntry{
		Path:      file,
		Kind:      KindFile,
		Operation: "read",
		Scope:     ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if d := m.Match(ctx, sibling, "read"); d.Allowed {
		t.Fatal("grant de arquivo não deve casar sibling")
	}
	if d := m.Match(ctx, docs, "list"); d.Allowed {
		t.Fatal("grant de arquivo não deve casar list no pai")
	}
	if d := m.Match(ctx, file, "list"); d.Allowed {
		t.Fatal("grant read no arquivo não deve casar list")
	}
}

func TestManager_DirGrantMatchesChildrenSameOperationOnly(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	docs := filepath.Join(dir, "docs")
	child := filepath.Join(docs, "nested", "a.txt")

	if err := m.Add(ctx, AllowlistEntry{
		Path:      docs,
		Kind:      KindDir,
		Operation: "read",
		Scope:     ScopeGlobal,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if d := m.Match(ctx, child, "read"); !d.Allowed || d.Scope != ScopeGlobal {
		t.Fatalf("dir grant deveria casar filho para mesma op, got %+v", d)
	}
	if d := m.Match(ctx, docs, "read"); !d.Allowed {
		t.Fatal("dir grant deveria casar o próprio root")
	}
	if d := m.Match(ctx, child, "write"); d.Allowed {
		t.Fatal("dir grant read não deve liberar write")
	}
	outside := filepath.Join(dir, "other", "x.txt")
	if d := m.Match(ctx, outside, "read"); d.Allowed {
		t.Fatal("dir grant não deve casar fora da árvore")
	}
}

func TestManager_ClearSession(t *testing.T) {
	home := t.TempDir()
	m := NewManagerWithDirs(home, home)
	ctx := ctxWith("conv-1", "")
	file := filepath.Join(home, "secret.txt")

	if err := m.Add(ctx, AllowlistEntry{
		Path:      file,
		Kind:      KindFile,
		Operation: "read",
		Scope:     ScopeSession,
	}); err != nil {
		t.Fatalf("Add sessão: %v", err)
	}
	if d := m.Match(ctx, file, "read"); !d.Allowed {
		t.Fatal("entrada de sessão deveria casar antes do clear")
	}

	m.ClearSession("conv-1")
	if d := m.Match(ctx, file, "read"); d.Allowed {
		t.Fatal("após ClearSession a entrada não deveria mais casar")
	}
	m.ClearSession("")
}

func TestManager_WorkspacePersistenceRoundtrip(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	m := NewManagerWithDirs(home, ws)
	ctx := context.Background()

	file := filepath.Join(ws, "notes.md")
	if err := m.Add(ctx, AllowlistEntry{
		Path:      file,
		Kind:      KindFile,
		Operation: "edit",
		Scope:     ScopeWorkspace,
	}); err != nil {
		t.Fatalf("Add workspace: %v", err)
	}

	m2 := NewManagerWithDirs(home, ws)
	d := m2.Match(ctx, file, "edit")
	if !d.Allowed || d.Scope != ScopeWorkspace {
		t.Fatalf("esperado match workspace persistido, got %+v", d)
	}
	if d.Entry == nil || d.Entry.Kind != KindFile || d.Entry.Operation != "edit" {
		t.Fatalf("entrada persistida inesperada: %+v", d.Entry)
	}
}

func TestManager_Remove(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	file := filepath.Join(dir, "a.txt")

	_ = m.Add(ctx, AllowlistEntry{Path: file, Kind: KindFile, Operation: "read", Scope: ScopeGlobal})
	if err := m.Remove(ctx, ScopeGlobal, file, KindFile, "read"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if d := m.Match(ctx, file, "read"); d.Allowed {
		t.Fatal("não deveria casar após remove")
	}
	if err := m.Remove(ctx, ScopeGlobal, file, KindFile, "read"); err != ErrEntryNotFound {
		t.Fatalf("esperado ErrEntryNotFound, got %v", err)
	}
}

func TestManager_PersistentReadsAndWritesAreSynchronized(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	const entries = 20
	errs := make(chan error, entries)
	var wg sync.WaitGroup

	for i := 0; i < entries; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- m.Add(ctx, AllowlistEntry{
				Path:      filepath.Join(dir, "files", fmt.Sprintf("%d.txt", i)),
				Kind:      KindFile,
				Operation: "read",
				Scope:     ScopeGlobal,
			})
		}()
		go func() {
			defer wg.Done()
			_ = m.List(ctx)
			_ = m.Match(ctx, filepath.Join(dir, "files", fmt.Sprintf("%d.txt", i)), "read")
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Add concorrente: %v", err)
		}
	}
	if got := len(m.List(ctx)); got != entries {
		t.Fatalf("listagem final perdeu entradas: got %d, want %d", got, entries)
	}
}

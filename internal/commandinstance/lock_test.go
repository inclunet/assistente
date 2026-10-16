package commandinstance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireContentionReacquireAndBytesUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	want := []byte("database bytes")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() == "" {
		t.Fatal("identity vazia")
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Acquire(context.Background(), path)
	if second != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("contenção lock=%v err=%v", second, err)
	}
	identity := first.Identity()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close idempotente: %v", err)
	}
	reacquired, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Identity() != identity {
		t.Fatalf("identity instável: %q != %q", reacquired.Identity(), identity)
	}
	if err := reacquired.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("bytes alterados: %q", got)
	}
}

func TestAcquireHardlinkSharesIdentityAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	alias := filepath.Join(t.TempDir(), "alias.sqlite")
	if err := os.WriteFile(path, []byte("same inode"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hardlink indisponível: %v", err)
	}
	first, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	identity := first.Identity()
	second, err := Acquire(context.Background(), alias)
	if second != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("alias lock=%v err=%v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Acquire(context.Background(), alias)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.Identity() != identity {
		t.Fatalf("identity do hardlink mudou: %q != %q", reopened.Identity(), identity)
	}
}

func TestAcquireInvalidMissingAndCancelled(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := Acquire(context.Background(), missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ausente: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelado: %v", err)
	}
	if _, err := Acquire(context.Background(), " "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("inválido: %v", err)
	}
	if lock, err := Acquire(context.Background(), t.TempDir()); err == nil {
		_ = lock.Close()
		t.Fatal("diretório aceito como banco")
	}
}

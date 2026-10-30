package turnlease

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestExecutionLeaseExcludesAnotherOwnerUntilReleased(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Create(context.Background(), cache, id.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if second, err := Open(context.Background(), first.Path, id.String()); second != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("lease concorrente: %v %v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Open(context.Background(), first.Path, id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), first.Path, "../fora"); err == nil {
		t.Fatal("aceitou ID inválido")
	}
}

func TestMissingExecutionLeaseIsNotRecreated(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	lease, err := Create(context.Background(), t.TempDir(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lease.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), lease.Path, id.String()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("arquivo ausente: %v", err)
	}
	if _, err := os.Stat(lease.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sidecar foi recriado: %v", err)
	}
}

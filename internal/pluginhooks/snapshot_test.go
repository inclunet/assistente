package pluginhooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotCopiesOnlyReviewedRelativeFiles(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "package")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(t.Context(), dir, map[string][]byte{"hooks/rules.txt": []byte("original")}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside", "hooks/rules.txt", "C:stream", "."} {
		if err := writeSnapshot(t.Context(), dir, map[string][]byte{name: []byte("invalid")}); err == nil {
			t.Fatalf("aceitou %q", name)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "rules.txt"))
	if err != nil || string(got) != "original" {
		t.Fatalf("arquivo da revisão = %q, erro %v", got, err)
	}
	if _, err = os.Stat(filepath.Join(parent, "outside")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = writeSnapshot(ctx, dir, map[string][]byte{"unused": nil}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

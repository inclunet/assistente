package database

import (
	"assistente/internal/configdir"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathDoesNotCreateDatabase(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "new-home")
	path, err := resolvePath(configdir.NewResolverWithBase(root), home)
	if err != nil || path != filepath.Join(home, "conversations.db") {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("resolution created home: %v", err)
	}
}

func TestResolvePathSelectsExistingWithoutOpeningSQLite(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "conversations.db")
	if err := os.WriteFile(want, []byte("not a sqlite database"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolvePath(configdir.NewResolverWithBase(root), filepath.Join(root, "other"))
	if err != nil || got != want {
		t.Fatalf("path=%q err=%v", got, err)
	}
	data, err := os.ReadFile(want)
	if err != nil || string(data) != "not a sqlite database" {
		t.Fatalf("resolution modified database: %q %v", data, err)
	}
}

func TestResolvePathDoesNotFallbackOnInspectionError(t *testing.T) {
	if path, err := resolvePath(deniedPathResolver{}, t.TempDir()); !errors.Is(err, os.ErrPermission) || path != "" {
		t.Fatalf("inspection failure unexpectedly resolved %q: %v", path, err)
	}
}

type deniedPathResolver struct{}

func (deniedPathResolver) Resolve(string) (*configdir.ResolvedFile, error) {
	return nil, &os.PathError{Op: "stat", Path: "conversations.db", Err: os.ErrPermission}
}

func TestInitPathRejectsRelativePath(t *testing.T) {
	if err := InitPath("conversations.db"); err == nil {
		t.Fatal("relative path accepted")
	}
}

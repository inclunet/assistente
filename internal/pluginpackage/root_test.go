package pluginpackage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageFileBoundary(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "package")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := root.WriteFile("plugin.json", []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(root, "plugin.json", 2)
	if err != nil || string(got) != "{}" {
		t.Fatalf("exact limit: %q, %v", got, err)
	}
	if _, err := ReadFile(root, "plugin.json", 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	if _, err := ReadFile(root, "../secret", 100); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("traversal: %v", err)
	}
	t.Run("link outside package", func(t *testing.T) {
		outside := filepath.Join(parent, "secret")
		if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		if _, err := ReadFile(root, "link", 100); err == nil {
			t.Fatal("read outside package")
		}
	})
}

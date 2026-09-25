package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveProjectWorkDirAcceptsNestedRelativePath(t *testing.T) {
	root := t.TempDir()
	requested := filepath.Join("src", "module")
	got, err := resolveProjectWorkDir(root, requested)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, "src", "module")
	if got != want {
		t.Fatalf("diretório = %q, esperado %q", got, want)
	}
}

func TestResolveProjectWorkDirCanonicalizesMissingNestedTarget(t *testing.T) {
	root := t.TempDir()
	requested := filepath.Join("not-created", "yet")

	got, err := resolveProjectWorkDir(root, requested)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, requested)
	if got != want {
		t.Fatalf("diretório = %q, esperado %q", got, want)
	}
}

func TestResolveProjectWorkDirRejectsSymlinkOutsideWithMissingDescendant(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link-outside")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("permissão para symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}

	if _, err := resolveProjectWorkDir(root, filepath.Join("link-outside", "not-created")); err == nil {
		t.Fatal("symlink para fora com descendente ausente foi aceito")
	}
}

func TestResolveProjectWorkDirRejectsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("permissão para symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}

	if _, err := resolveProjectWorkDir(root, filepath.Join("dangling", "child")); err == nil {
		t.Fatal("symlink dangling foi tratado como componente inexistente")
	}
}

func TestResolveProjectWorkDirRejectsEscapesAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, requested := range []string{
		filepath.Join("..", "outside"),
		root,
		`C:\outside`,
		`\\server\share`,
	} {
		t.Run(requested, func(t *testing.T) {
			if _, err := resolveProjectWorkDir(root, requested); err == nil {
				t.Fatalf("esperava rejeição de %q", requested)
			}
		})
	}
}

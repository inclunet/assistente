package shell

import (
	"path/filepath"
	"testing"
)

func TestResolveProjectWorkDirAcceptsNestedRelativePath(t *testing.T) {
	root := t.TempDir()
	got, err := resolveProjectWorkDir(root, filepath.Join("src", "module"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "src", "module")
	if got != want {
		t.Fatalf("diretório = %q, esperado %q", got, want)
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

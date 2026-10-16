package filesystem

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileBytesReplacingPreservesOriginalWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("replace refused")
	err := writeFileBytesReplacing(path, []byte("new content"), 0600, func(temp, target string) error {
		if target != path {
			t.Fatal("destination changed")
		}
		got, err := os.ReadFile(temp)
		if err != nil || string(got) != "new content" {
			t.Fatalf("incomplete temporary file: %q %v", got, err)
		}
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("error=%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatalf("original lost: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}

func TestWriteFileBytesReplacingCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "document.md")
	for _, content := range []string{"first", "", "replacement"} {
		if err := WriteFileBytesReplacing(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("got=%q error=%v", got, err)
		}
	}
}

func TestWriteFileBytesReplacingRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFileBytesReplacing(dir, []byte("invalid"), 0600); err == nil {
		t.Fatal("directory accepted")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("directory changed: %v", err)
	}
}

func TestWriteFileBytesReplacingConcurrentWritersDoNotMixContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := WriteFileBytesReplacing(path, bytes.Repeat([]byte{byte('a' + i)}, 4096), 0600); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil || len(got) != 4096 || !bytes.Equal(got, bytes.Repeat(got[:1], 4096)) {
		t.Fatalf("mixed or incomplete content: %v, length=%d", err, len(got))
	}
}

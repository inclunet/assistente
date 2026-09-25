package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFileVersionRefusesChangedContentWithSameMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := CaptureFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, v.info.ModTime(), v.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileBytesReplacingVersion(path, []byte("lost"), 0600, v); !errors.Is(err, ErrFileVersionChanged) {
		t.Fatalf("got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("content=%q err=%v", got, err)
	}
}

func TestFileVersionRejectsReplacementWithSameContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := CaptureFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, v.info.ModTime(), v.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileBytesReplacingVersion(path, []byte("lost"), 0600, v); !errors.Is(err, ErrFileVersionChanged) {
		t.Fatalf("got %v", err)
	}
}

func TestFileVersionCreatesWithoutClobberAndRejectsInvalidReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.md")
	v, err := CaptureFileVersion(path)
	if err != nil || v.Exists() {
		t.Fatalf("capture: %v", err)
	}
	if err := WriteFileBytesReplacingVersion(path, []byte("first"), 0600, FileVersion{}); !errors.Is(err, ErrFileVersionChanged) {
		t.Fatalf("zero receipt: %v", err)
	}
	if err := WriteFileBytesReplacingVersion(path+"other", []byte("first"), 0600, v); !errors.Is(err, ErrFileVersionChanged) {
		t.Fatalf("wrong path: %v", err)
	}
	if err := WriteFileBytesReplacingVersion(path, []byte("first"), 0600, v); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileBytesReplacingVersion(path, []byte("second"), 0600, v); !errors.Is(err, ErrFileVersionChanged) {
		t.Fatalf("replay: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "first" {
		t.Fatalf("content=%q err=%v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temp leak: %v %v", entries, err)
	}
}

func TestFileVersionOnlyOneConcurrentReplacementWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := CaptureFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WriteFileBytesReplacingVersion(path, []byte("new"), 0600, v)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrFileVersionChanged) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes=%d", successes.Load())
	}
}

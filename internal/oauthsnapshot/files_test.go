package oauthsnapshot

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/google/uuid"
)

func TestPrivateSnapshotPublication(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	files, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	id := uuid.NewString()
	data := []byte("opaque-encrypted-envelope")
	if err := files.Write(id, data); err != nil {
		t.Fatal(err)
	}
	if err := files.Write(id, []byte("replacement")); err == nil {
		t.Fatal("snapshot overwritten")
	}
	read, err := files.Read(id)
	if err != nil || !bytes.Equal(read, data) {
		t.Fatalf("read: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != id+".oauth" {
		t.Fatalf("partial files leaked: %v", err)
	}
	if _, err := files.Read("../outside"); err == nil {
		t.Fatal("path accepted")
	}
	if err := files.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Read(id); err == nil {
		t.Fatal("deleted snapshot readable")
	}
}

func TestOpenRejectsDirectoryReplacedAfterProtection(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "snapshots")
	files, err := open(dir, func(path string) (*os.Root, error) {
		if err := os.Rename(path, path+"-original"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		return os.OpenRoot(path)
	})
	if err == nil || files != nil {
		t.Fatal("accepted unvalidated replacement directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("replacement directory received snapshot data")
	}
}

func TestOpenKeepsValidatedDirectoryAfterRename(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "snapshots")
	boundPath := dir
	files, err := open(dir, func(path string) (*os.Root, error) {
		root, err := os.OpenRoot(path)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(path, path+"-original"); err != nil {
			// Windows os.Root denies delete sharing, so the pinned directory
			// cannot be renamed at all. This is also a valid confinement result.
			if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32)) {
				return root, nil
			}
			_ = root.Close()
			t.Fatal(err)
		}
		boundPath = path + "-original"
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		return root, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	id := uuid.NewString()
	if err := files.Write(id, []byte("ciphertext")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(boundPath, id+".oauth")); err != nil {
		t.Fatal("write lost validated directory", err)
	}
	if boundPath != dir {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("write followed replacement directory")
		}
	}
}

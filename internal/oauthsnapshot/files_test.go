package oauthsnapshot

import (
	"bytes"
	"os"
	"path/filepath"
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

package oauthsnapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsSnapshotDACL(t *testing.T) {
	dir := t.TempDir()
	d, err := openPrivateDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err := protect(d, true); err != nil {
		t.Fatalf("directory ACL: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "encrypted"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := protect(f, false); err != nil {
		t.Fatalf("file ACL: %v", err)
	}
}

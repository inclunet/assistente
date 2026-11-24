package pluginpackage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
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

// appendAfterEOF models a regular file that grows after its first observed EOF.
type appendAfterEOF struct {
	io.Reader
}

func (r *appendAfterEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.Reader = strings.NewReader("!")
	}
	return n, err
}

func TestReadLimitedAfterGrowth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int64
		wantErr error
	}{
		{"below limit", 4, nil},
		{"at limit", 2, ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &appendAfterEOF{iotest.DataErrReader(strings.NewReader("{}"))}
			data, err := readLimited(reader, tc.limit)
			if !errors.Is(err, tc.wantErr) || (err == nil && string(data) != "{}") {
				t.Fatalf("readLimited = %q, %v", data, err)
			}
		})
	}
}

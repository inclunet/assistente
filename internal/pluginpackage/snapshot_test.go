package pluginpackage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotBudgets(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"name":"test","$schema":"` + ManifestSchema + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "extra"), []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	total := int64(len(raw) + 3)
	for _, tc := range []struct {
		name           string
		entries, depth int
		file, bytes    int64
		ok             bool
	}{
		{"exact", 3, 1, int64(len(raw)), total, true},
		{"entries", 2, 1, total, total, false},
		{"depth", 3, 0, total, total, false},
		{"single file", 3, 1, int64(len(raw) - 1), total, false},
		{"aggregate", 3, 1, total, total - 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := inspectLocal(dir, tc.entries, tc.depth, tc.file, tc.bytes)
			if (err == nil) != tc.ok {
				t.Fatalf("snapshot=%v err=%v", snapshot, err)
			}
		})
	}
}

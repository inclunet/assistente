package pluginpackage

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestSnapshotManifestSpellingAndGitMetadata(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"name":"test","$schema":"` + ManifestSchema + `"}`)
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Plugin.json")
	if err := os.Mkdir(filepath.Join(dir, ".GIT"), 0700); err != nil {
		t.Fatal(err)
	}
	write(".GIT/config")
	if _, err := os.Stat(filepath.Join(dir, "plugin.json")); os.IsNotExist(err) {
		write("plugin.json")
		if _, err := InspectLocal(dir); err == nil {
			t.Fatal("accepted two root manifest entries")
		}
		if err := os.Remove(filepath.Join(dir, "Plugin.json")); err != nil {
			t.Fatal(err)
		}
	}
	n := int64(len(raw))
	got, err := inspectLocal(dir, 2, 0, n, n)
	if err != nil || len(got.Files) != 1 || string(got.Files["plugin.json"]) != string(raw) {
		t.Fatalf("snapshot = %+v, err = %v", got, err)
	}
	if runtime.GOOS == "windows" && filepath.VolumeName(dir) != "" && dir[1] == ':' {
		extended, err := InspectLocal(`\\?\` + dir)
		if err != nil || extended.Origin != got.Origin || extended.Digest != got.Digest {
			t.Fatalf("extended path snapshot = %+v, err = %v, want origin %q", extended, err, got.Origin)
		}
	}
}

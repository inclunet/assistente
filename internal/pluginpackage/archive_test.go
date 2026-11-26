package pluginpackage

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArchiveLimitsAndPortablePaths(t *testing.T) {
	makeArchive := func(names ...string) []byte {
		t.Helper()
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		for _, name := range names {
			if _, err := w.Create(name); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	for _, names := range [][]string{{"x", "x/f"}, {"x/f", "x"}, {"A/f", "a/g"}, {"../escape"}, {"a", "a"}} {
		if _, err := OpenArchive(makeArchive(names...)); err == nil {
			t.Fatalf("accepted %q", names)
		}
	}
	archive := makeArchive("a/b", "a/", "a/c")
	if _, err := OpenArchive(archive); err != nil {
		t.Fatal(err)
	}
	ordinary := bytes.Clone(archive)
	binary.LittleEndian.PutUint16(ordinary[len(ordinary)-2:], 4)
	if _, err := OpenArchive(append(ordinary, "PK\x05\x06"...)); err != nil {
		t.Fatal("ordinary ZIP comment rejected", err)
	}
	withComment := bytes.Clone(archive)
	comment := append([]byte("PK\x05\x06"), make([]byte, 19)...)
	binary.LittleEndian.PutUint16(withComment[len(withComment)-2:], uint16(len(comment)))
	if _, err := OpenArchive(append(withComment, comment...)); err == nil {
		t.Fatal("ambiguous ZIP comment accepted")
	}
	for _, count := range []uint16{1, 65535} {
		modified := bytes.Clone(archive)
		end := modified[len(modified)-22:]
		binary.LittleEndian.PutUint16(end[8:], count)
		binary.LittleEndian.PutUint16(end[10:], count)
		if _, err := OpenArchive(modified); err == nil {
			t.Fatalf("accepted misleading count %d", count)
		}
	}
}

func TestDirectoryCandidateUniqueness(t *testing.T) {
	for _, tc := range []struct {
		name       string
		candidates []bool
		accept     bool
	}{
		{"ordinary", []bool{true}, true},
		{"incidental earlier signatures", []bool{true, false, false}, true},
		{"two directories", []bool{true, false, true}, false},
		{"invalid selected directory", []bool{false, true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := uniqueDirectory(slices.Values(tc.candidates))
			if (err == nil) != tc.accept {
				t.Fatalf("expected acceptance %v, error=%v", tc.accept, err)
			}
		})
	}
}

func TestInspectLocalZIPFindsPackageRoot(t *testing.T) {
	for _, name := range []string{"plugin.json", "release/plugin.json", ".Git/plugin.json", ".cursor-plugin/plugin.json"} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "example.zip")
			var buf bytes.Buffer
			writer := zip.NewWriter(&buf)
			member, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = member.Write([]byte(`{"$schema":"` + ManifestSchema + `","name":"demo","version":"1.0.0"}`)); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filename, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := InspectLocalZIP(filename)
			if name == ".Git/plugin.json" {
				if err == nil {
					t.Fatal("accepted .git as package root")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Manifest.Name != "demo" || snapshot.Origin != filename {
				t.Fatalf("unexpected package %s from %s", snapshot.Manifest.Name, snapshot.Origin)
			}
		})
	}
}

package pluginpackage

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
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

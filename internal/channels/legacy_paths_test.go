package channels

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestListLegacyChannelJSONInBases_EnumeratesWithoutDedup(t *testing.T) {
	t.Parallel()

	baseA := t.TempDir()
	baseB := t.TempDir()
	dirA := filepath.Join(baseA, channelsSubdir)
	dirB := filepath.Join(baseB, channelsSubdir)
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "Telegram.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "notes.txt"), []byte(`x`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, ".json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dirA, "subdir.json"), 0755); err != nil {
		t.Fatal(err)
	}
	// Mesmo slug em outro base — helper NÃO faz dedup.
	if err := os.WriteFile(filepath.Join(dirB, "telegram.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "discord.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}

	files, errs := listLegacyChannelJSONInBases([]string{baseA, baseB}, listLegacyChannelJSONOptions{})
	if len(errs) != 0 {
		t.Fatalf("errs inesperados: %v", errs)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files %+v, want 3", len(files), files)
	}

	byPath := make(map[string]legacyChannelJSONFile, len(files))
	var fromA, fromB int
	for _, f := range files {
		byPath[f.Path] = f
		switch f.Dir {
		case dirA:
			fromA++
		case dirB:
			fromB++
		}
	}
	if fromA != 1 || fromB != 2 {
		t.Fatalf("distribuição por base: fromA=%d fromB=%d files=%+v", fromA, fromB, files)
	}

	tgA, ok := byPath[filepath.Join(dirA, "Telegram.json")]
	if !ok || tgA.Slug != "telegram" || tgA.Name != "Telegram.json" {
		t.Fatalf("Telegram.json em A: ok=%v %+v", ok, tgA)
	}
	tgB, ok := byPath[filepath.Join(dirB, "telegram.json")]
	if !ok || tgB.Slug != "telegram" {
		t.Fatalf("telegram.json em B (sem dedup): ok=%v %+v", ok, tgB)
	}
	if _, ok := byPath[filepath.Join(dirB, "discord.json")]; !ok {
		t.Fatalf("discord.json ausente: %+v", files)
	}
}

func TestListLegacyChannelJSONInBases_IgnoresReadDirErrorsByDefault(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist-base")
	files, errs := listLegacyChannelJSONInBases([]string{missing}, listLegacyChannelJSONOptions{})
	if len(files) != 0 {
		t.Fatalf("files=%v", files)
	}
	if len(errs) != 0 {
		t.Fatalf("import silencia ReadDir: errs=%v", errs)
	}
}

func TestListLegacyChannelJSONInBases_RequireRealDirReportsErrors(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	// channels/ ausente → NotExist silencioso mesmo com RequireRealDir.
	files, errs := listLegacyChannelJSONInBases([]string{base}, listLegacyChannelJSONOptions{RequireRealDir: true})
	if len(files) != 0 || len(errs) != 0 {
		t.Fatalf("NotExist deve ser silencioso: files=%v errs=%v", files, errs)
	}

	// channels como arquivo regular (não diretório).
	channelsAsFile := filepath.Join(base, channelsSubdir)
	if err := os.WriteFile(channelsAsFile, []byte(`x`), 0600); err != nil {
		t.Fatal(err)
	}
	files, errs = listLegacyChannelJSONInBases([]string{base}, listLegacyChannelJSONOptions{RequireRealDir: true})
	if len(files) != 0 {
		t.Fatalf("files=%v", files)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "não é diretório") {
		t.Fatalf("esperava erro de diretório: %v", errs)
	}
}

func TestListLegacyChannelJSONInBases_RequireRealDirRejectsSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup varia no Windows sem privilégio de desenvolvedor")
	}

	base := t.TempDir()
	realDir := filepath.Join(t.TempDir(), "real-channels")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "telegram.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, channelsSubdir)
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	files, errs := listLegacyChannelJSONInBases([]string{base}, listLegacyChannelJSONOptions{RequireRealDir: true})
	if len(files) != 0 {
		t.Fatalf("symlink dir não deve enumerar: %v", files)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "symlink") {
		t.Fatalf("esperava erro de symlink: %v", errs)
	}

	// Sem RequireRealDir, ReadDir segue o symlink (comportamento do import).
	files, errs = listLegacyChannelJSONInBases([]string{base}, listLegacyChannelJSONOptions{})
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(files) != 1 || files[0].Slug != "telegram" {
		t.Fatalf("import enumera via symlink: %+v", files)
	}
}

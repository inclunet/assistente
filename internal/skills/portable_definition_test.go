package skills

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"assistente/internal/configdir"
)

func readSelected(m *Manager, slug string) ([]byte, error) {
	definitions, _ := m.DiscoverPortableDefinitions()
	read := definitions[slug]
	if read == nil {
		return nil, os.ErrNotExist
	}
	return read()
}

func TestPortableDefinitionReadsOnlySelectedNativeSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notes")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const raw = "---\nname: notes\ndescription: Summarize\n---\nRead the list."
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{resolver: configdir.NewResolverWithBase(root)}
	data, err := readSelected(manager, "notes")
	if err != nil || string(data) != raw {
		t.Fatal(string(data), err)
	}
	created, err := manager.Create(&SkillMetadata{Name: "native", Version: "1.0.0", Description: "Native skill"}, "Read.")
	if err != nil {
		t.Fatal(err)
	}
	native, err := readSelected(manager, created)
	if err != nil || !strings.Contains(string(native), "version:") {
		t.Fatal(string(native), err)
	}
	if got, err := manager.DiscoverPortableDefinitions(); err != nil || !slices.Equal(slices.Sorted(maps.Keys(got)), []string{"native", "notes"}) {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), append([]byte(raw), 0xff), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSelected(manager, "notes"); err == nil {
		t.Fatal("invalid UTF-8 exposed")
	}
	if _, err := readSelected(manager, "../other"); err == nil {
		t.Fatal("unexpected path lookup")
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "private.env"), []byte("private data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSelected(manager, "notes"); err == nil || err.Error() != "SKILL_BUILDER_SUPPORTING_FILES" {
		t.Fatal("expected supporting-file error", err)
	}
}

func TestBuilderDistinguishesAbsentAndUnreadableRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	m := &Manager{resolver: configdir.NewResolverWithBase(root)}
	if names, err := m.DiscoverPortableDefinitions(); err != nil || len(names) != 0 {
		t.Fatal(names, err)
	}
	if err := os.WriteFile(root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DiscoverPortableDefinitions(); err == nil {
		t.Fatal("unreadable root reported as empty")
	}
}

func TestBuilderKeepsSkillsFromReadableRoots(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(work)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	dir := filepath.Join(home, ".assistente", "skills", "release-notes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte("notes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(work, ".assistente"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".assistente", "skills"), []byte("unreadable root"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	names, err := m.DiscoverPortableDefinitions()
	if err == nil || names["release-notes"] == nil {
		t.Fatal(names, err)
	}
	raw, err := readSelected(m, "release-notes")
	if err != nil || string(raw) != "notes" {
		t.Fatal(string(raw), err)
	}
}

func TestPortableDefinitionUsesNativeWorkdirPrecedence(t *testing.T) {
	home, workDir := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(workDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	for _, root := range []string{home, workDir} {
		dir := filepath.Join(root, ".assistente", "skills", "notes")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(root), 0600); err != nil {
			t.Fatal(err)
		}
	}
	contents, err := readSelected(NewManager(), "notes")
	if err != nil || string(contents) != workDir {
		t.Fatal(string(contents), err)
	}
	homeSkill := filepath.Join(home, ".assistente", "skills", "lower")
	upperSkill := filepath.Join(workDir, ".assistente", "skills", "Lower")
	for _, dir := range []string{homeSkill, upperSkill} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	contents, err = readSelected(NewManager(), "lower")
	if err != nil || string(contents) != homeSkill {
		t.Fatal(string(contents), err)
	}

}

func TestNativeSkillLinkRejectsDisplayNameCollision(t *testing.T) {
	root := t.TempDir()
	for _, slug := range []string{"notes", "other"} {
		dir := filepath.Join(root, slug)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, skillFile), []byte("---\nname: notes\ndescription: Notes\n---\nRead."), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "not-file", skillFile), 0700); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(root, "manual")
	if err := os.Mkdir(manual, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manual, skillFile), []byte("---\nname: guide\n---\n"+strings.Repeat("a", 1<<20)), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{resolver: configdir.NewResolverWithBase(root)}
	if err := manager.WithLinkableDefinitions(func(files PortableDefinitions, err error) error {
		if err != nil {
			return err
		}
		if _, err := files["notes"](); err == nil {
			t.Error("ambiguous selection offered")
		}
		if manager.definitionsMu.TryLock() {
			manager.definitionsMu.Unlock()
			t.Error("edits possible during acceptance")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

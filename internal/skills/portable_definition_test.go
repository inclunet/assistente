package skills

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"assistente/internal/configdir"
)

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
	data, err := manager.PortableDefinition("notes")
	if err != nil || string(data) != raw {
		t.Fatal(string(data), err)
	}
	created, err := manager.Create(&SkillMetadata{Name: "native", Version: "1.0.0", Description: "Native skill"}, "Read.")
	if err != nil {
		t.Fatal(err)
	}
	native, err := manager.PortableDefinition(created)
	if err != nil || !strings.Contains(string(native), "version:") {
		t.Fatal(string(native), err)
	}
	if got := manager.BuilderSlugs(); !slices.Equal(got, []string{"native", "notes"}) {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), append([]byte(raw), 0xff), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PortableDefinition("notes"); err == nil {
		t.Fatal("invalid UTF-8 exposed")
	}
	if _, err := manager.PortableDefinition("../other"); err == nil {
		t.Fatal("unexpected path lookup")
	}
	if err := os.WriteFile(filepath.Join(dir, "private.env"), []byte("private data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PortableDefinition("notes"); err == nil {
		t.Fatal("ignored supporting files")
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
	contents, err := NewManager().PortableDefinition("notes")
	if err != nil || string(contents) != workDir {
		t.Fatal(string(contents), err)
	}
}

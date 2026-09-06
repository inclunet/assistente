package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"assistente/internal/configdir"
)

func TestPublishedSkillFormatIsEquivalentFrom019Through050(t *testing.T) {
	raw := readPublishedSkillFixture(t, "0.1.9-0.5.0", "SKILL.md")
	var baseline *SkillMetadata
	var baselineContent string
	for _, release := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			meta, content, err := Parse(string(raw))
			if err != nil {
				t.Fatalf("parse do formato publicado em %s: %v", release, err)
			}
			if baseline == nil {
				baseline, baselineContent = meta, content
				return
			}
			if !reflect.DeepEqual(meta, baseline) || content != baselineContent {
				t.Fatalf("release %s não produziu resultado equivalente", release)
			}
		})
	}
}

func TestPublishedSkillsLoadRepeatedlyRejectInvalidAndPreserveSources(t *testing.T) {
	valid := readPublishedSkillFixture(t, "0.1.9-0.5.0", "SKILL.md")
	invalid := readPublishedSkillFixture(t, "invalid", "SKILL.md")
	home := t.TempDir()
	workdir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(workdir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	validPath := filepath.Join(configdir.GetHomeDir(), "skills", "corpus-skill", "SKILL.md")
	invalidPath := filepath.Join(configdir.GetHomeDir(), "skills", "invalid", "SKILL.md")
	writePublishedSkillFixture(t, validPath, valid)
	writePublishedSkillFixture(t, invalidPath, invalid)

	manager := NewManager()
	first, err := manager.List()
	if err != nil {
		t.Fatalf("primeiro carregamento: %v", err)
	}
	second, err := manager.List()
	if err != nil {
		t.Fatalf("segundo carregamento: %v", err)
	}
	if countSkill(first, "corpus-skill") != 1 || countSkill(second, "corpus-skill") != 1 {
		t.Fatalf("carregamento repetido duplicou/perdeu skill: first=%+v second=%+v", first, second)
	}
	if countSkill(first, "invalid") != 0 || countSkill(second, "invalid") != 0 {
		t.Fatal("skill sintaticamente inválida não foi rejeitada")
	}
	assertPublishedSkillUnchanged(t, validPath, valid)
	assertPublishedSkillUnchanged(t, invalidPath, invalid)
}

func readPublishedSkillFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	pathParts := append([]string{"testdata", "published"}, parts...)
	data, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("ler fixture %s: %v", filepath.Join(parts...), err)
	}
	return data
}

func writePublishedSkillFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("criar diretório da fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("escrever fixture %s: %v", path, err)
	}
}

func countSkill(skills []SkillInfo, slug string) int {
	count := 0
	for _, skill := range skills {
		if skill.Slug == slug {
			count++
		}
	}
	return count
}

func assertPublishedSkillUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fonte %s deixou de existir: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("fonte %s foi alterada", path)
	}
}

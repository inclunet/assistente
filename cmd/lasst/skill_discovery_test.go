package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkill(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".assistente/skills", ".github/skills", ".agents/skills", ".claude/skills", ".cursor/skills"} {
		t.Run(dir, func(t *testing.T) {
			base := filepath.Join(root, filepath.FromSlash(dir), "review")
			if err := os.MkdirAll(base, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(base, "SKILL.md")
			if err := os.WriteFile(path, []byte("skill"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := resolveSkill(root, "review")
			if err != nil || got != path {
				t.Fatalf("got=%q err=%v", got, err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestResolveSkillAmbiguous(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".assistente/skills", ".github/skills"} {
		path := filepath.Join(root, filepath.FromSlash(dir), "review")
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("skill"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := resolveSkill(root, "review")
	if err == nil || !strings.Contains(err.Error(), "ambígua") {
		t.Fatalf("err=%v", err)
	}
}
func TestResolveSkillExplicitAndMissing(t *testing.T) {
	root := t.TempDir()
	if got, err := resolveSkill(root, "docs/review.md"); err != nil || got != "docs/review.md" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := resolveSkill(root, "ausente"); err == nil {
		t.Fatal("esperava erro")
	}
}
func TestReadSkillRejectsExternalSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	source := filepath.Join(outside, "SKILL.md")
	if err := os.WriteFile(source, []byte("segredo"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".agents", "skills", "review")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skipf("symlink indisponível: %v", err)
	}
	if _, err := readSkill(root, "review"); err == nil {
		t.Fatal("leitura de destino externo deve falhar")
	}
}

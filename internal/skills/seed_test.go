package skills

import (
	"testing"
	"testing/fstest"
)

func buildSkillMD(name, version, desc, body string) string {
	return "---\nname: " + name + "\nversion: " + version + "\ndescription: " + desc + "\n---\n\n" + body
}

func TestSeedBuiltinSkills(t *testing.T) {
	repo, ctx := setupRepo(t)

	fsys := fstest.MapFS{
		"builtin/skills/coding/SKILL.md": &fstest.MapFile{Data: []byte(buildSkillMD("coding", "1.0.0", "skill de coding para teste de seed", "corpo coding"))},
		"builtin/skills/memory/SKILL.md": &fstest.MapFile{Data: []byte(buildSkillMD("memory", "2.0.0", "skill de memory para teste de seed", "corpo memory"))},
		"builtin/skills/ignored.txt":     &fstest.MapFile{Data: []byte("not a skill")},
		"builtin/skills/empty/README.md": &fstest.MapFile{Data: []byte("dir sem SKILL.md")},
	}

	res, err := SeedBuiltinSkills(ctx, repo, fsys, "builtin/skills")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if res.Seeded != 2 {
		t.Errorf("seeded: got %d want 2", res.Seeded)
	}
	if res.Failed != 0 {
		t.Errorf("failed: got %d want 0", res.Failed)
	}

	coding, err := repo.Get(ctx, "coding")
	if err != nil {
		t.Fatalf("get coding: %v", err)
	}
	if coding.Content != "corpo coding" {
		t.Errorf("coding content: %q", coding.Content)
	}

	var row = mustRow(t, repo, "coding")
	if !row.IsBuiltin || row.BuiltinVersion != "1.0.0" {
		t.Errorf("coding builtin flags: isBuiltin=%v version=%q", row.IsBuiltin, row.BuiltinVersion)
	}
}

func TestSeedBuiltinSkillsIdempotentAndUpgrade(t *testing.T) {
	repo, ctx := setupRepo(t)

	v1 := fstest.MapFS{
		"b/coding/SKILL.md": &fstest.MapFile{Data: []byte(buildSkillMD("coding", "1.0.0", "skill de coding versao um teste", "v1"))},
	}
	if _, err := SeedBuiltinSkills(ctx, repo, v1, "b"); err != nil {
		t.Fatalf("seed v1: %v", err)
	}

	// Re-seed mesma versão — sem mudança.
	if _, err := SeedBuiltinSkills(ctx, repo, v1, "b"); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	got, _ := repo.Get(ctx, "coding")
	if got.Content != "v1" {
		t.Errorf("re-seed changed content: %q", got.Content)
	}

	// Versão maior — atualiza.
	v2 := fstest.MapFS{
		"b/coding/SKILL.md": &fstest.MapFile{Data: []byte(buildSkillMD("coding", "1.1.0", "skill de coding versao dois teste", "v2"))},
	}
	if _, err := SeedBuiltinSkills(ctx, repo, v2, "b"); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	got, _ = repo.Get(ctx, "coding")
	if got.Content != "v2" {
		t.Errorf("upgrade not applied: %q", got.Content)
	}
}

func TestManagerDelegatesToRepository(t *testing.T) {
	repo, _ := setupRepo(t)
	m := NewManager()
	if m.HasRepository() {
		t.Fatal("manager deveria iniciar em modo filesystem")
	}
	m.SetRepository(repo)
	if !m.HasRepository() {
		t.Fatal("manager deveria estar em modo banco após SetRepository")
	}

	meta := &SkillMetadata{Name: "dele", Version: "1.0.0", Description: "skill via manager modo repo teste"}
	slug, err := m.Create(meta, "corpo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if slug != "dele" {
		t.Errorf("slug: got %q", slug)
	}

	got, err := m.Get("dele")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "corpo" {
		t.Errorf("content: %q", got.Content)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len: got %d want 1", len(list))
	}
}

func mustRow(t *testing.T, repo *DBRepository, slug string) struct {
	IsBuiltin      bool
	BuiltinVersion string
} {
	t.Helper()
	var out struct {
		IsBuiltin      bool
		BuiltinVersion string
	}
	if err := repo.db.Table("skills").Select("is_builtin", "builtin_version").
		Where("slug = ?", slug).Scan(&out).Error; err != nil {
		t.Fatalf("scan row: %v", err)
	}
	return out
}

package skills

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRepo(t *testing.T) (*DBRepository, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.Skill{}, &database.SkillTool{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewDBRepository(db), context.Background()
}

func newSkill(slug string, mutate func(*Skill)) *Skill {
	s := &Skill{
		Slug:    slug,
		Content: "corpo de " + slug,
		SkillMetadata: SkillMetadata{
			Name:        slug,
			Version:     "1.0.0",
			Description: "descricao de teste valida para skill",
		},
	}
	if mutate != nil {
		mutate(s)
	}
	return s
}

func countTools(t *testing.T, repo *DBRepository, slug string) int {
	t.Helper()
	var skill database.Skill
	if err := repo.db.Where("slug = ?", slug).First(&skill).Error; err != nil {
		t.Fatalf("find skill %s: %v", slug, err)
	}
	var c int64
	if err := repo.db.Model(&database.SkillTool{}).Where("skill_id = ?", skill.ID).Count(&c).Error; err != nil {
		t.Fatalf("count tools: %v", err)
	}
	return int(c)
}

func TestRepoCreateAndGet(t *testing.T) {
	repo, ctx := setupRepo(t)
	orig := fullSkill()

	slug, err := repo.Create(ctx, orig)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if slug != "coding-helper" {
		t.Errorf("slug: got %q", slug)
	}

	got, err := repo.Get(ctx, slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != orig.Content {
		t.Errorf("content mismatch")
	}
	if !reflect.DeepEqual(got.SkillMetadata, orig.SkillMetadata) {
		t.Errorf("metadata mismatch:\n got: %+v\nwant: %+v", got.SkillMetadata, orig.SkillMetadata)
	}
	// junction: 3 allowed + 1 denied
	if n := countTools(t, repo, slug); n != 4 {
		t.Errorf("skill_tools rows: got %d want 4", n)
	}
}

func TestRepoCreateDuplicateSlugFails(t *testing.T) {
	repo, ctx := setupRepo(t)
	if _, err := repo.Create(ctx, newSkill("dup", nil)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, newSkill("dup", nil)); err == nil {
		t.Errorf("expected error creating duplicate slug")
	}
}

func TestRepoGetNotFound(t *testing.T) {
	repo, ctx := setupRepo(t)
	_, err := repo.Get(ctx, "missing")
	if !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestRepoUpdateReplacesToolsAndPreservesBuiltin(t *testing.T) {
	repo, ctx := setupRepo(t)

	// Seed um builtin para validar preservação de is_builtin/builtin_version no Update.
	if err := repo.SeedBuiltin(ctx, newSkill("editor", func(s *Skill) {
		s.Tools = &ToolPermissions{Allowed: []string{"Read", "Write"}}
	}), "1.0.0"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	upd := newSkill("editor", func(s *Skill) {
		s.Description = "descricao atualizada do editor de skill"
		s.Content = "novo corpo"
		s.Tools = &ToolPermissions{Allowed: []string{"Read"}, Denied: []string{"Delete"}}
	})
	if err := repo.Update(ctx, "editor", upd); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.Get(ctx, "editor")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "novo corpo" {
		t.Errorf("content not updated: %q", got.Content)
	}
	if n := countTools(t, repo, "editor"); n != 2 {
		t.Errorf("skill_tools after update: got %d want 2", n)
	}

	// is_builtin e builtin_version devem ser preservados pelo Update.
	var row database.Skill
	if err := repo.db.Where("slug = ?", "editor").First(&row).Error; err != nil {
		t.Fatalf("find row: %v", err)
	}
	if !row.IsBuiltin || row.BuiltinVersion != "1.0.0" {
		t.Errorf("builtin fields not preserved: isBuiltin=%v version=%q", row.IsBuiltin, row.BuiltinVersion)
	}
}

func TestRepoUpdateNotFound(t *testing.T) {
	repo, ctx := setupRepo(t)
	err := repo.Update(ctx, "missing", newSkill("missing", nil))
	if !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestRepoDelete(t *testing.T) {
	repo, ctx := setupRepo(t)
	if _, err := repo.Create(ctx, fullSkill()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Delete(ctx, "coding-helper"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, "coding-helper"); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("expected not found after delete, got %v", err)
	}
	var c int64
	repo.db.Model(&database.SkillTool{}).Count(&c)
	if c != 0 {
		t.Errorf("skill_tools should be empty after delete, got %d", c)
	}
}

func TestRepoDuplicate(t *testing.T) {
	repo, ctx := setupRepo(t)
	if _, err := repo.Create(ctx, newSkill("memory", nil)); err != nil {
		t.Fatalf("create: %v", err)
	}
	newSlug, err := repo.Duplicate(ctx, "memory")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if newSlug != "memory-copia" {
		t.Errorf("duplicate slug: got %q want memory-copia", newSlug)
	}
	if _, err := repo.Get(ctx, "memory-copia"); err != nil {
		t.Errorf("duplicated skill not found: %v", err)
	}
}

func TestRepoFilters(t *testing.T) {
	repo, ctx := setupRepo(t)

	mustCreate(t, repo, ctx, newSkill("auto-on", func(s *Skill) { s.AutoLoad = true }))
	mustCreate(t, repo, ctx, newSkill("auto-disabled", func(s *Skill) {
		s.AutoLoad = true
		s.DisableModelInvocation = true
	}))
	mustCreate(t, repo, ctx, newSkill("available", func(s *Skill) { s.AutoLoad = false }))
	mustCreate(t, repo, ctx, newSkill("hidden", func(s *Skill) {
		f := false
		s.UserInvocable = &f
	}))

	auto, err := repo.GetAutoSkills(ctx)
	if err != nil {
		t.Fatalf("auto: %v", err)
	}
	if len(auto) != 1 || auto[0].Slug != "auto-on" {
		t.Errorf("GetAutoSkills: got %v", slugsOf(auto))
	}

	avail, err := repo.GetAvailableSkills(ctx)
	if err != nil {
		t.Fatalf("avail: %v", err)
	}
	// available, hidden, auto-disabled têm auto_load=false
	if got := slugsOf(avail); len(got) != 3 {
		t.Errorf("GetAvailableSkills: got %v want 3", got)
	}

	invocable, err := repo.GetUserInvocableSkills(ctx)
	if err != nil {
		t.Fatalf("invocable: %v", err)
	}
	for _, info := range invocable {
		if info.Slug == "hidden" {
			t.Errorf("GetUserInvocableSkills should exclude hidden")
		}
	}
	if len(invocable) != 3 {
		t.Errorf("GetUserInvocableSkills: got %d want 3", len(invocable))
	}
}

func TestRepoSeedBuiltinVersioning(t *testing.T) {
	repo, ctx := setupRepo(t)

	v1 := newSkill("coding", func(s *Skill) { s.Content = "v1 corpo" })
	if err := repo.SeedBuiltin(ctx, v1, "1.0.0"); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	got, _ := repo.Get(ctx, "coding")
	if got.Content != "v1 corpo" {
		t.Errorf("seed v1 content: %q", got.Content)
	}

	// Mesmo seed novamente — sem efeito.
	v1again := newSkill("coding", func(s *Skill) { s.Content = "ignorado" })
	if err := repo.SeedBuiltin(ctx, v1again, "1.0.0"); err != nil {
		t.Fatalf("seed v1 again: %v", err)
	}
	got, _ = repo.Get(ctx, "coding")
	if got.Content != "v1 corpo" {
		t.Errorf("re-seed same version should not change content, got %q", got.Content)
	}

	// Versão maior — atualiza.
	v2 := newSkill("coding", func(s *Skill) { s.Content = "v2 corpo" })
	if err := repo.SeedBuiltin(ctx, v2, "1.1.0"); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	got, _ = repo.Get(ctx, "coding")
	if got.Content != "v2 corpo" {
		t.Errorf("seed v2 should update content, got %q", got.Content)
	}

	// Marca como customizado — seed futuro deve pular.
	if err := repo.db.Model(&database.Skill{}).Where("slug = ?", "coding").Update("is_customized", true).Error; err != nil {
		t.Fatalf("set customized: %v", err)
	}
	v3 := newSkill("coding", func(s *Skill) { s.Content = "v3 corpo" })
	if err := repo.SeedBuiltin(ctx, v3, "2.0.0"); err != nil {
		t.Fatalf("seed v3: %v", err)
	}
	got, _ = repo.Get(ctx, "coding")
	if got.Content != "v2 corpo" {
		t.Errorf("seed should skip customized skill, content changed to %q", got.Content)
	}
}

func TestRepoExistsBySlug(t *testing.T) {
	repo, ctx := setupRepo(t)
	if ok, _ := repo.ExistsBySlug(ctx, "x"); ok {
		t.Errorf("should not exist")
	}
	mustCreate(t, repo, ctx, newSkill("x", nil))
	if ok, _ := repo.ExistsBySlug(ctx, "x"); !ok {
		t.Errorf("should exist after create")
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.1", "1.0.0", 1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q,%q) = %d want %d", c.a, c.b, got, c.want)
		}
	}
}

func mustCreate(t *testing.T, repo *DBRepository, ctx context.Context, s *Skill) {
	t.Helper()
	if _, err := repo.Create(ctx, s); err != nil {
		t.Fatalf("create %s: %v", s.Slug, err)
	}
}

func slugsOf[T any](items []T) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := any(it).(type) {
		case Skill:
			out = append(out, v.Slug)
		case SkillInfo:
			out = append(out, v.Slug)
		}
	}
	return out
}

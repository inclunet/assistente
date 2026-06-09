package skills

import (
	"testing"
)

func findCatalogEntry(entries []SkillCatalogEntry, slug string) (SkillCatalogEntry, bool) {
	for _, e := range entries {
		if e.Slug == slug {
			return e, true
		}
	}
	return SkillCatalogEntry{}, false
}

func TestRebuildCatalogProjectsSkills(t *testing.T) {
	repo, ctx := setupRepo(t)

	// Skill autoload com reason e dependência de tools inferida.
	auto := newSkill("auto-skill", func(s *Skill) {
		s.AutoLoad = true
		s.AutoloadReason = "precisa estar sempre no prompt"
		s.Tools = &ToolPermissions{Allowed: []string{"read_file"}}
		s.ContextBudget = 123
	})
	if _, err := repo.Create(ctx, auto); err != nil {
		t.Fatalf("create auto: %v", err)
	}
	// Skill sob demanda simples.
	if _, err := repo.Create(ctx, newSkill("plain-skill", nil)); err != nil {
		t.Fatalf("create plain: %v", err)
	}

	if err := repo.RebuildCatalog(ctx); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	entries, err := repo.ListCatalog(ctx)
	if err != nil {
		t.Fatalf("list catalog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperava 2 entradas, got %d", len(entries))
	}

	a, ok := findCatalogEntry(entries, "auto-skill")
	if !ok {
		t.Fatal("auto-skill ausente do catálogo")
	}
	if !a.AutoLoad || a.AutoloadReason == "" {
		t.Errorf("auto-skill deveria ter autoload+reason: %+v", a)
	}
	if !a.RequiresTools {
		t.Errorf("auto-skill deveria inferir requires_tools: %+v", a)
	}
	if a.ContextBudget != 123 {
		t.Errorf("context_budget declarado deveria ser preservado: got %d", a.ContextBudget)
	}

	p, ok := findCatalogEntry(entries, "plain-skill")
	if !ok {
		t.Fatal("plain-skill ausente do catálogo")
	}
	if p.AutoLoad {
		t.Errorf("plain-skill não deveria ser autoload: %+v", p)
	}
	if p.ContextBudget == 0 {
		t.Errorf("plain-skill deveria estimar budget pelo corpo: got %d", p.ContextBudget)
	}
}

func TestRebuildCatalogIsIdempotent(t *testing.T) {
	repo, ctx := setupRepo(t)
	if _, err := repo.Create(ctx, newSkill("a", nil)); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := repo.RebuildCatalog(ctx); err != nil {
			t.Fatalf("rebuild %d: %v", i, err)
		}
	}
	entries, err := repo.ListCatalog(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("rebuild não deveria duplicar: got %d entradas", len(entries))
	}
}

func TestManagerCRUDKeepsCatalogInSync(t *testing.T) {
	repo, ctx := setupRepo(t)
	mgr := NewManager()
	mgr.SetRepository(repo)

	// Create → catálogo deve ganhar a entrada.
	slug, err := mgr.Create(&SkillMetadata{Name: "sync-skill", Version: "1.0.0", Description: "descricao valida para sincronizar catalogo"}, "corpo inicial")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries, _ := repo.ListCatalog(ctx)
	if _, ok := findCatalogEntry(entries, slug); !ok {
		t.Fatalf("catálogo deveria conter %q após create", slug)
	}

	// Update → descrição atualizada deve refletir no catálogo.
	newDesc := "descricao atualizada e suficientemente longa para o catalogo"
	if err := mgr.Update(slug, &SkillMetadata{Name: "sync-skill", Version: "1.0.0", Description: newDesc}, "corpo novo"); err != nil {
		t.Fatalf("update: %v", err)
	}
	entries, _ = repo.ListCatalog(ctx)
	updated, ok := findCatalogEntry(entries, slug)
	if !ok || updated.Description != newDesc {
		t.Fatalf("catálogo deveria refletir a descrição atualizada: %+v", updated)
	}

	// Delete → entrada deve sumir do catálogo.
	if err := mgr.Delete(slug); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = repo.ListCatalog(ctx)
	if _, ok := findCatalogEntry(entries, slug); ok {
		t.Fatalf("catálogo deveria remover %q após delete", slug)
	}
}

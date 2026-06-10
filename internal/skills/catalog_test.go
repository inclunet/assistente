package skills

import "testing"

func TestEstimateContextBudget(t *testing.T) {
	if got := EstimateContextBudget(""); got != 0 {
		t.Errorf("vazio: got %d want 0", got)
	}
	if got := EstimateContextBudget("abc"); got != 1 {
		t.Errorf("curto: got %d want 1 (mínimo)", got)
	}
	body := make([]byte, 400)
	for i := range body {
		body[i] = 'x'
	}
	if got := EstimateContextBudget(string(body)); got != 100 {
		t.Errorf("400 chars: got %d want 100", got)
	}
	// Runes multibyte contam como 1 caractere cada (não por bytes): 8 runes "é"
	// (2 bytes cada = 16 bytes) -> 8/4 = 2 tokens, não 16/4 = 4.
	if got := EstimateContextBudget("ééééééék"); got != 2 {
		t.Errorf("multibyte: got %d want 2 (contagem por runes)", got)
	}
}

func TestCatalogEntryFromSkill(t *testing.T) {
	s := &Skill{
		Slug:    "deploy",
		Source:  "exe",
		Content: "corpo",
		SkillMetadata: SkillMetadata{
			Name:           "deploy",
			Description:    "Use when deploying",
			AutoLoad:       true,
			AutoloadReason: "always",
			Tools:          &ToolPermissions{Allowed: []string{"run_command"}},
			Network:        &NetworkPermissions{AllowedHosts: []string{"x.com"}},
		},
	}
	e := CatalogEntryFromSkill(s)
	if e.Slug != "deploy" || !e.IsBuiltin {
		t.Errorf("slug/builtin: %+v", e)
	}
	if !e.RequiresTools || !e.RequiresNetwork {
		t.Errorf("requires inferido: %+v", e)
	}
	if !e.AutoLoad || e.AutoloadReason != "always" {
		t.Errorf("autoload: %+v", e)
	}
	// context_budget não declarado -> estimado do corpo ("corpo" = 5 chars -> 1).
	if e.ContextBudget != 1 {
		t.Errorf("budget estimado: got %d want 1", e.ContextBudget)
	}
}

func TestCatalogEntryUsesDeclaredBudget(t *testing.T) {
	s := &Skill{
		Slug:          "x",
		Content:       "corpo grande aqui muito maior que budget declarado",
		SkillMetadata: SkillMetadata{Name: "x", ContextBudget: 42},
	}
	if e := CatalogEntryFromSkill(s); e.ContextBudget != 42 {
		t.Errorf("budget declarado deveria prevalecer: got %d want 42", e.ContextBudget)
	}
}

func TestCatalogEntryFromInfo(t *testing.T) {
	info := SkillInfo{
		Slug:      "y",
		IsBuiltin: false,
		SkillMetadata: SkillMetadata{
			Name:          "y",
			Description:   "desc",
			ContextBudget: 10,
			RequiresMCP:   true,
		},
	}
	e := CatalogEntryFromInfo(info)
	if e.Slug != "y" || e.IsBuiltin || e.ContextBudget != 10 || !e.RequiresMCP {
		t.Errorf("entry from info: %+v", e)
	}
}

func TestCatalogByNamesOrderedResolvesSlugBeforeNameOnCollision(t *testing.T) {
	// Colisão: o slug de uma entrada é igual ao nome de outra. A resolução deve
	// ser determinística e priorizar o slug — sem depender da ordem de iteração.
	a := SkillCatalogEntry{Slug: "shared", Name: "Alpha"}
	b := SkillCatalogEntry{Slug: "beta", Name: "shared"}
	all := []SkillCatalogEntry{a, b}

	got := CatalogByNamesOrdered(all, []string{"shared"})
	if len(got) != 1 || got[0].Slug != "shared" {
		t.Fatalf("colisão deveria resolver pelo slug (entrada a), got %+v", got)
	}

	// Resolução por nome continua funcionando quando não há slug correspondente.
	got = CatalogByNamesOrdered(all, []string{"Alpha", "beta"})
	if len(got) != 2 || got[0].Slug != "shared" || got[1].Slug != "beta" {
		t.Fatalf("esperava [shared(Alpha), beta], got %+v", got)
	}
}

func TestCatalogByNamesOrderedNilVsEmpty(t *testing.T) {
	all := []SkillCatalogEntry{{Slug: "a", Name: "A"}}
	if got := CatalogByNamesOrdered(all, nil); len(got) != 1 {
		t.Errorf("nil names = todas, got %+v", got)
	}
	if got := CatalogByNamesOrdered(all, []string{}); got != nil {
		t.Errorf("names vazio = nenhuma, got %+v", got)
	}
}

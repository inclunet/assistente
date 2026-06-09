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

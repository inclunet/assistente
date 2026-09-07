package skills

import "testing"

func policySkill(slug string, content string, autoLoad bool) Skill {
	s := Skill{Slug: slug, Content: content}
	s.Name = slug
	s.Description = slug + " description"
	s.AutoLoad = autoLoad
	return s
}

func TestResolveSelectionPolicyEnabledSkillsFirstBaseRestOnDemand(t *testing.T) {
	all := []Skill{
		policySkill("coding", "coding body", false),
		policySkill("job-manager", "job body", false),
		policySkill("editor-texto", "editor body", false),
	}
	policy := ResolveSelectionPolicy(all, []string{"job-manager", "coding"}, false, false)
	if got := policy.ModeFor("job-manager"); got != SkillModeBase {
		t.Fatalf("first enabled skill should be base, got %s", got)
	}
	if got := policy.ModeFor("coding"); got != SkillModeOnDemand {
		t.Fatalf("second enabled skill should be on_demand, got %s", got)
	}
	if got := policy.ModeFor("editor-texto"); got != SkillModeDisabled {
		t.Fatalf("unlisted skill should be disabled, got %s", got)
	}
}

func TestResolveSelectionPolicyDisableOnDemandKeepsOnlyFirstBase(t *testing.T) {
	all := []Skill{
		policySkill("base", "base body", false),
		policySkill("later", "later body", false),
	}
	policy := ResolveSelectionPolicy(all, []string{"base", "later"}, false, true)
	if got := policy.ModeFor("base"); got != SkillModeBase {
		t.Fatalf("first enabled skill should remain base, got %s", got)
	}
	if got := policy.ModeFor("later"); got != SkillModeDisabled {
		t.Fatalf("on-demand skill should be disabled when on-demand is off, got %s", got)
	}
}

func TestResolveSelectionPolicyLegacyAutoLoadMapsToBase(t *testing.T) {
	all := []Skill{
		policySkill("legacy", "legacy body", true),
		policySkill("manual", "manual body", false),
	}
	policy := ResolveSelectionPolicy(all, nil, false, false)
	if got := policy.ModeFor("legacy"); got != SkillModeBase {
		t.Fatalf("legacy auto_load skill should be base, got %s", got)
	}
	if got := policy.ModeFor("manual"); got != SkillModeOnDemand {
		t.Fatalf("non-auto legacy skill should be on_demand, got %s", got)
	}
}

func TestResolveSelectionPolicyLegacyMultipleAutoLoadOnlyFirstBase(t *testing.T) {
	all := []Skill{
		policySkill("first", "first body", true),
		policySkill("second", "second body", true),
		policySkill("manual", "manual body", false),
	}
	policy := ResolveSelectionPolicy(all, nil, false, false)
	if got := policy.ModeFor("first"); got != SkillModeBase {
		t.Fatalf("first legacy auto_load skill should be base, got %s", got)
	}
	if got := policy.ModeFor("second"); got != SkillModeOnDemand {
		t.Fatalf("second legacy auto_load skill should be on_demand, got %s", got)
	}
	if got := policy.ModeFor("manual"); got != SkillModeOnDemand {
		t.Fatalf("manual legacy skill should be on_demand, got %s", got)
	}
}

func TestResolveSelectionPolicyEmptyEnabledSkillsDisablesAll(t *testing.T) {
	all := []Skill{
		policySkill("coding", "coding body", true),
		policySkill("job-manager", "job body", false),
	}
	policy := ResolveSelectionPolicy(all, []string{}, false, false)
	if got := policy.ModeFor("coding"); got != SkillModeDisabled {
		t.Fatalf("empty enabled_skills should disable skills, got %s", got)
	}
	if got := policy.ModeFor("job-manager"); got != SkillModeDisabled {
		t.Fatalf("empty enabled_skills should disable non-auto skills, got %s", got)
	}
}

func TestSelectionPolicyInvocableUserSkillsIncludesBaseAndOnDemand(t *testing.T) {
	all := []Skill{
		policySkill("base", "base body", false),
		policySkill("later", "later body", false),
	}
	policy := ResolveSelectionPolicy(all, []string{"base", "later"}, false, false)
	infos := policy.InvocableUserSkills()
	if len(infos) != 2 {
		t.Fatalf("expected base and on-demand skills in slash menu, got %d", len(infos))
	}
	if infos[0].Slug != "base" || infos[1].Slug != "later" {
		t.Fatalf("unexpected slash menu order: %+v", infos)
	}
}

func TestResolveSelectionPolicyKeepsSkillsWithTemplateExamples(t *testing.T) {
	all := []Skill{
		policySkill("templated", "{{ .ToolCallingEnabled }}", false),
		policySkill("plain", "plain body", false),
	}
	policy := ResolveSelectionPolicy(all, []string{"templated", "plain"}, false, false)
	if got := policy.ModeFor("templated"); got != SkillModeBase {
		t.Fatalf("skill content with template examples should remain loadable, got %s", got)
	}
	if got := policy.ModeFor("plain"); got != SkillModeOnDemand {
		t.Fatalf("next enabled skill should remain on-demand, got %s", got)
	}
}

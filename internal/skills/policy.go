package skills

import "strings"

type SkillMode string

const (
	SkillModeBase     SkillMode = "base"
	SkillModeOnDemand SkillMode = "on_demand"
	SkillModeDisabled SkillMode = "disabled"
)

type SelectionPolicy struct {
	Base     []Skill
	OnDemand []Skill
	Disabled []Skill
}

func (p SelectionPolicy) IsEnabled(slug string) bool {
	mode := p.ModeFor(slug)
	return mode == SkillModeBase || mode == SkillModeOnDemand
}

func (p SelectionPolicy) ModeFor(slug string) SkillMode {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return SkillModeDisabled
	}
	for _, s := range p.Base {
		if skillMatches(s, slug) {
			return SkillModeBase
		}
	}
	for _, s := range p.OnDemand {
		if skillMatches(s, slug) {
			return SkillModeOnDemand
		}
	}
	return SkillModeDisabled
}

func (p SelectionPolicy) InvocableUserSkills() []SkillInfo {
	enabled := append(append([]Skill{}, p.Base...), p.OnDemand...)
	infos := make([]SkillInfo, 0, len(enabled))
	for _, s := range enabled {
		if !s.IsUserInvocable() {
			continue
		}
		infos = append(infos, SkillInfo{
			SkillMetadata: s.SkillMetadata,
			Slug:          s.Slug,
			Source:        s.Source,
			AutoLoad:      s.IsAutoLoad(),
		})
	}
	return infos
}

func ResolveSelectionPolicy(allSkills []Skill, enabledSkills []string, disableSkills bool, disableOnDemand bool) SelectionPolicy {
	if disableSkills {
		return SelectionPolicy{Disabled: append([]Skill{}, allSkills...)}
	}
	if enabledSkills == nil {
		return resolveLegacyAutoLoadPolicy(allSkills, disableOnDemand)
	}
	if len(enabledSkills) == 0 {
		return SelectionPolicy{Disabled: append([]Skill{}, allSkills...)}
	}

	ordered := FilterByNamesOrdered(allSkills, enabledSkills)
	enabled := make(map[string]bool, len(ordered)*2)
	for _, s := range ordered {
		enabled[s.Slug] = true
		enabled[s.Name] = true
	}

	policy := SelectionPolicy{}
	for _, s := range ordered {
		if len(policy.Base) == 0 {
			policy.Base = append(policy.Base, s)
			continue
		}
		if disableOnDemand {
			policy.Disabled = append(policy.Disabled, s)
			continue
		}
		policy.OnDemand = append(policy.OnDemand, s)
	}

	for _, s := range allSkills {
		if enabled[s.Slug] || enabled[s.Name] {
			continue
		}
		policy.Disabled = append(policy.Disabled, s)
	}
	return policy
}

func resolveLegacyAutoLoadPolicy(allSkills []Skill, disableOnDemand bool) SelectionPolicy {
	policy := SelectionPolicy{}
	for _, s := range allSkills {
		if s.IsAutoLoad() {
			if len(policy.Base) == 0 {
				policy.Base = append(policy.Base, s)
				continue
			}
			if disableOnDemand {
				policy.Disabled = append(policy.Disabled, s)
				continue
			}
			policy.OnDemand = append(policy.OnDemand, s)
			continue
		}
		if disableOnDemand {
			policy.Disabled = append(policy.Disabled, s)
			continue
		}
		policy.OnDemand = append(policy.OnDemand, s)
	}
	return policy
}

func skillMatches(skill Skill, name string) bool {
	return skill.Slug == name || skill.Name == name
}

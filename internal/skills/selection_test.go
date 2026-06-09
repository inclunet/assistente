package skills

import "testing"

func mdAutoload(reason string) *SkillMetadata {
	return &SkillMetadata{Name: "a", Version: "1.0.0", Description: "x", AutoLoad: true, AutoloadReason: reason}
}

func TestPolicySkillsDisabled(t *testing.T) {
	p := NewSkillSelectionPolicy()
	d := p.Decide(mdAutoload("r"), "a", SkillSelectionContext{SkillsDisabled: true, ToolsEnabled: true})
	if d.Visibility != VisibilityHidden || d.Reason != ReasonSkillsDisabled {
		t.Errorf("esperava hidden/skills_disabled, got %+v", d)
	}
}

func TestPolicyAutoload(t *testing.T) {
	p := NewSkillSelectionPolicy()
	d := p.Decide(mdAutoload("r"), "a", SkillSelectionContext{ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true})
	if d.Visibility != VisibilityAutoload || d.Reason != ReasonAutoload {
		t.Errorf("esperava autoload, got %+v", d)
	}
}

func TestPolicyOnDemandDefault(t *testing.T) {
	p := NewSkillSelectionPolicy()
	m := &SkillMetadata{Name: "a", Version: "1.0.0", Description: "x"} // não autoload, model-invocable
	d := p.Decide(m, "a", SkillSelectionContext{ToolsEnabled: true})
	if d.Visibility != VisibilityOnDemand || d.Reason != ReasonOnDemand {
		t.Errorf("esperava on_demand, got %+v", d)
	}
}

func TestPolicyCapabilityGating(t *testing.T) {
	p := NewSkillSelectionPolicy()
	cases := []struct {
		name   string
		meta   *SkillMetadata
		ctx    SkillSelectionContext
		reason string
	}{
		{
			"tools",
			&SkillMetadata{RequiresTools: true},
			SkillSelectionContext{ToolsEnabled: false, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true},
			ReasonRequiresTools,
		},
		{
			"filesystem",
			&SkillMetadata{Filesystem: &FilesystemPermissions{Read: []string{"/a"}}},
			SkillSelectionContext{ToolsEnabled: true, FilesystemEnabled: false, NetworkEnabled: true, MCPEnabled: true},
			ReasonRequiresFilesystem,
		},
		{
			"network",
			&SkillMetadata{RequiresNetwork: true},
			SkillSelectionContext{ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: false, MCPEnabled: true},
			ReasonRequiresNetwork,
		},
		{
			"mcp",
			&SkillMetadata{MCP: &MCPConfig{Server: &MCPServerConfig{Command: "node"}}},
			SkillSelectionContext{ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: false},
			ReasonRequiresMCP,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := p.Decide(c.meta, "s", c.ctx)
			if d.Visibility != VisibilityHidden || d.Reason != c.reason {
				t.Errorf("esperava hidden/%s, got %+v", c.reason, d)
			}
		})
	}
}

func TestPolicyModelInvocationDisabledHidesOnDemand(t *testing.T) {
	p := NewSkillSelectionPolicy()
	m := &SkillMetadata{Name: "a", Version: "1.0.0", DisableModelInvocation: true} // só slash
	d := p.Decide(m, "a", SkillSelectionContext{ToolsEnabled: true})
	if d.Visibility != VisibilityHidden || d.Reason != ReasonModelInvocationOff {
		t.Errorf("esperava hidden/model_invocation_disabled, got %+v", d)
	}
}

func TestPolicyDisableOnDemand(t *testing.T) {
	p := NewSkillSelectionPolicy()
	m := &SkillMetadata{Name: "a", Version: "1.0.0"}
	d := p.Decide(m, "a", SkillSelectionContext{ToolsEnabled: true, DisableOnDemand: true})
	if d.Visibility != VisibilityHidden || d.Reason != ReasonOnDemandDisabled {
		t.Errorf("esperava hidden/on_demand_disabled, got %+v", d)
	}
}

func TestPolicyAutoloadAllowlistOverrides(t *testing.T) {
	p := NewSkillSelectionPolicy()
	// Skill NÃO é autoload no metadado, mas o perfil força via allowlist.
	m := &SkillMetadata{Name: "a", Version: "1.0.0"}
	ctx := SkillSelectionContext{ToolsEnabled: true, AutoloadAllowlist: []string{"a"}}
	if d := p.Decide(m, "a", ctx); d.Visibility != VisibilityAutoload {
		t.Errorf("allowlist deveria forçar autoload, got %+v", d)
	}
	// Skill autoload no metadado, mas allowlist não a inclui -> sob demanda.
	ctx2 := SkillSelectionContext{ToolsEnabled: true, AutoloadAllowlist: []string{"outra"}}
	if d := p.Decide(mdAutoload("r"), "a", ctx2); d.Visibility != VisibilityOnDemand {
		t.Errorf("fora da allowlist deveria virar on_demand, got %+v", d)
	}
}

func TestPolicyDecideAllGrouping(t *testing.T) {
	p := NewSkillSelectionPolicy()
	list := []Skill{
		{Slug: "auto", SkillMetadata: SkillMetadata{AutoLoad: true, AutoloadReason: "r"}},
		{Slug: "ondemand", SkillMetadata: SkillMetadata{}},
		{Slug: "needs-net", SkillMetadata: SkillMetadata{RequiresNetwork: true}},
	}
	ctx := SkillSelectionContext{ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: false, MCPEnabled: true}
	sel := p.DecideAll(list, ctx)
	if len(sel.Autoload) != 1 || sel.Autoload[0].Slug != "auto" {
		t.Errorf("autoload group: %+v", sel.Autoload)
	}
	if len(sel.OnDemand) != 1 || sel.OnDemand[0].Slug != "ondemand" {
		t.Errorf("on_demand group: %+v", sel.OnDemand)
	}
	if len(sel.Hidden) != 1 || sel.Hidden[0].Slug != "needs-net" {
		t.Errorf("hidden group: %+v", sel.Hidden)
	}
}

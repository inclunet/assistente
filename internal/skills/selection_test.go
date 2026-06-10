package skills

import "testing"

func mdAutoload(reason string) *SkillMetadata {
	return &SkillMetadata{Name: "a", Version: "1.0.0", Description: "x", AutoLoad: true, AutoloadReason: reason}
}

func TestPolicyDecideCatalogParityWithDecide(t *testing.T) {
	// AEP-0072 Fase 4b: DecideCatalog (sem corpo) deve ser semanticamente idêntico
	// a Decide (com metadata) — mesma visibilidade e motivo em todos os contextos.
	p := NewSkillSelectionPolicy()

	mkSkill := func(slug string, mut func(*Skill)) Skill {
		s := Skill{Slug: slug}
		s.Name = slug
		s.Version = "1.0.0"
		s.Description = "descricao para " + slug
		if mut != nil {
			mut(&s)
		}
		return s
	}

	skillsUnderTest := []Skill{
		mkSkill("plain", nil),
		mkSkill("autoload-reason", func(s *Skill) { s.AutoLoad = true; s.AutoloadReason = "porque sim" }),
		mkSkill("autoload-noreason", func(s *Skill) { s.AutoLoad = true }),
		mkSkill("needs-tools", func(s *Skill) { s.Tools = &ToolPermissions{Allowed: []string{"read_file"}} }),
		mkSkill("needs-network", func(s *Skill) { s.RequiresNetwork = true }),
		mkSkill("needs-fs", func(s *Skill) { s.Filesystem = &FilesystemPermissions{Read: []string{"~/x/**"}} }),
		mkSkill("no-model-invocation", func(s *Skill) { s.DisableModelInvocation = true }),
	}

	contexts := map[string]SkillSelectionContext{
		"all-enabled-metadata":      {ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true, RequireAutoloadReason: true},
		"tools-off":                 {RequireAutoloadReason: true},
		"skills-disabled":           {SkillsDisabled: true, ToolsEnabled: true},
		"disable-on-demand":         {ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true, DisableOnDemand: true, RequireAutoloadReason: true},
		"allowlist-autoload-reason": {ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true, AutoloadAllowlist: []string{"plain", "autoload-noreason"}},
	}

	for _, s := range skillsUnderTest {
		entry := CatalogEntryFromSkill(&s)
		for name, ctx := range contexts {
			viaMeta := p.Decide(&s.SkillMetadata, s.Slug, ctx)
			viaCatalog := p.DecideCatalog(entry, ctx)
			if viaMeta.Visibility != viaCatalog.Visibility || viaMeta.Reason != viaCatalog.Reason {
				t.Errorf("paridade quebrada em skill=%q ctx=%q: Decide=%+v DecideCatalog=%+v", s.Slug, name, viaMeta, viaCatalog)
			}
		}
	}
}

func TestPolicyAllowlistMatchesBySlugOrName(t *testing.T) {
	// Allowlist de perfil aceita slug OU nome (mesma semântica de
	// CatalogByNamesOrdered); Decide e DecideCatalog devem concordar.
	p := NewSkillSelectionPolicy()
	s := Skill{Slug: "my-slug"}
	s.Name = "My Skill"
	s.Version = "1.0.0"
	s.Description = "descricao da skill"
	entry := CatalogEntryFromSkill(&s)

	for _, allow := range [][]string{{"my-slug"}, {"My Skill"}} {
		ctx := SkillSelectionContext{
			ToolsEnabled: true, FilesystemEnabled: true, NetworkEnabled: true, MCPEnabled: true,
			AutoloadAllowlist: allow,
		}
		viaMeta := p.Decide(&s.SkillMetadata, s.Slug, ctx)
		viaCatalog := p.DecideCatalog(entry, ctx)
		if viaMeta.Visibility != VisibilityAutoload {
			t.Errorf("Decide deveria autoloadar com allowlist=%v, got %+v", allow, viaMeta)
		}
		if viaCatalog.Visibility != VisibilityAutoload {
			t.Errorf("DecideCatalog deveria autoloadar com allowlist=%v, got %+v", allow, viaCatalog)
		}
	}
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

func TestPolicyAutoloadDemotedWithoutReason(t *testing.T) {
	p := NewSkillSelectionPolicy()
	// Modo metadata-driven (sem allowlist) com RequireAutoloadReason: autoload
	// sem reason cai para sob demanda.
	m := &SkillMetadata{Name: "a", Version: "1.0.0", Description: "x", AutoLoad: true}
	d := p.Decide(m, "a", SkillSelectionContext{ToolsEnabled: true, RequireAutoloadReason: true})
	if d.Visibility != VisibilityOnDemand {
		t.Errorf("esperava on_demand (rebaixada), got %+v", d)
	}
	// O rebaixamento por falta de reason deve ser observável no motivo (AEP-0072 D5).
	if d.Reason != ReasonAutoloadNoReason {
		t.Errorf("esperava motivo %q (rebaixamento observável), got %q", ReasonAutoloadNoReason, d.Reason)
	}
	// Paridade: DecideCatalog reporta o mesmo motivo de rebaixamento.
	entry := CatalogEntryFromSkill(&Skill{SkillMetadata: *m, Slug: "a"})
	if dc := p.DecideCatalog(entry, SkillSelectionContext{ToolsEnabled: true, RequireAutoloadReason: true}); dc.Reason != ReasonAutoloadNoReason {
		t.Errorf("DecideCatalog deveria reportar %q, got %q", ReasonAutoloadNoReason, dc.Reason)
	}
	// Com reason permanece autoload.
	d = p.Decide(mdAutoload("porque sim"), "a", SkillSelectionContext{ToolsEnabled: true, RequireAutoloadReason: true})
	if d.Visibility != VisibilityAutoload {
		t.Errorf("esperava autoload com reason, got %+v", d)
	}
}

func TestPolicyExplicitAllowlistIgnoresReason(t *testing.T) {
	p := NewSkillSelectionPolicy()
	// Allowlist explícita: respeita a escolha do perfil mesmo sem reason e com
	// RequireAutoloadReason setado (que só vale no modo metadata-driven).
	m := &SkillMetadata{Name: "a", Version: "1.0.0", Description: "x", AutoLoad: true}
	d := p.Decide(m, "a", SkillSelectionContext{ToolsEnabled: true, RequireAutoloadReason: true, AutoloadAllowlist: []string{"a"}})
	if d.Visibility != VisibilityAutoload {
		t.Errorf("esperava autoload via allowlist, got %+v", d)
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

package chat

import (
	"testing"

	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// Estes testes garantem que ToolSelectionPolicy produz EXATAMENTE os mesmos
// resultados que a composição legada (capturada em
// tool_selection_characterization_test.go), provando a paridade exigida pela
// Fase 3 do AEP-0077 (#119). Reusam os mesmos cenários canônicos.

func TestToolSelectionPolicy_InitialToolDefs_MatchesLegacy(t *testing.T) {
	for _, tc := range initialSelectionCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			policy := NewToolSelectionPolicy(tc.registry)
			cfg := ProfileToolConfig{EnabledTools: tc.enabled, DisableTools: tc.disable, RuntimeTools: tc.runtime}

			gotDefs := defNames(policy.InitialToolDefs(cfg))
			assertNames(t, tc.name+" defs", gotDefs, tc.want)

			// InitialEnabledToolNames + buildLLMToolDefs deve coincidir com InitialToolDefs.
			legacy := defNames(legacyInitialDefs(tc.registry, tc.enabled, tc.disable, tc.runtime))
			assertNames(t, tc.name+" legacy", gotDefs, legacy)
		})
	}
}

func TestToolSelectionPolicy_ResolveExpandedToolDefs_MatchesLegacy(t *testing.T) {
	r := charRegistry(t)
	for _, tc := range dynamicExpansionCases() {
		t.Run(tc.name, func(t *testing.T) {
			streamer, mgr := expansionStreamerAndMgr(tc.native)
			policy := NewToolSelectionPolicy(r)
			cfg := ProfileToolConfig{EnabledTools: tc.enabled, DisableTools: tc.disable, NativeMCP: tc.override}

			got := defNames(policy.ResolveExpandedToolDefs(streamer, mgr, nil, tc.names, cfg))
			assertNames(t, tc.name, got, tc.want)

			legacy := defNames(legacyExpandedDefs(r, streamer, mgr, tc.names, tc.enabled, tc.disable, tc.override))
			assertNames(t, tc.name+" legacy", got, legacy)
		})
	}
}

func TestToolSelectionPolicy_ApplyNativeMCP_RemovesNativeBridge(t *testing.T) {
	r := charRegistry(t)
	capable := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", ToolNames: []string{"mcp_srv__do"}},
	}}
	enabled := []string{"read_file", "mcp_srv__do"}

	policy := NewToolSelectionPolicy(r)
	defs := policy.InitialToolDefs(ProfileToolConfig{EnabledTools: enabled})
	_, out := policy.ApplyNativeMCP(capable, defs, mgr, ProfileToolConfig{EnabledTools: enabled, NativeMCP: boolPtr(true)})
	assertNames(t, "policy", defNames(out), []string{"read_file"})
}

func TestToolSelectionPolicy_InitialEnabledToolNames_NilRegistrySafe(t *testing.T) {
	policy := NewToolSelectionPolicy(nil)
	if got := policy.InitialEnabledToolNames(ProfileToolConfig{}); got != nil {
		t.Fatalf("registry nil deveria devolver nil, got %#v", got)
	}
	if got := policy.InitialToolDefs(ProfileToolConfig{}); got != nil {
		t.Fatalf("registry nil deveria devolver nil defs, got %#v", got)
	}
}

func TestToolSelectionPolicy_InitialEnabledToolNames_MatchesResolution(t *testing.T) {
	r := charRegistry(t)
	policy := NewToolSelectionPolicy(r)
	got := policy.InitialEnabledToolNames(ProfileToolConfig{RuntimeTools: []string{tools.LoadSkillName}})
	assertNames(t, "names", got, []string{tools.ToolCatalogName, tools.LoadSkillName})
}

func TestToolSelectionPolicy_EffectiveTriStateFromLegacyEnabledTools(t *testing.T) {
	r := charRegistry(t)
	policy := NewToolSelectionPolicy(r)

	open := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{})
	if open.State(tools.ToolCatalogName) != ToolPolicyPreloaded {
		t.Fatalf("tool_catalog deveria ser preloaded em perfil legado aberto, got %s", open.State(tools.ToolCatalogName))
	}
	if open.State("read_file") != ToolPolicyOnDemand {
		t.Fatalf("read_file deveria ser on_demand em perfil legado aberto, got %s", open.State("read_file"))
	}
	if open.State("text_edit") != ToolPolicyDisabled {
		t.Fatalf("opt-in ausente deveria ficar disabled em perfil legado aberto, got %s", open.State("text_edit"))
	}

	empty := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{EnabledTools: []string{}})
	if empty.State(tools.ToolCatalogName) != ToolPolicyDisabled || len(empty.PreloadedNames()) != 0 {
		t.Fatalf("enabled_tools vazio deveria desabilitar tools iniciais, states=%#v preloaded=%#v", empty.states, empty.PreloadedNames())
	}

	explicit := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{EnabledTools: []string{"read_file"}})
	if explicit.State("read_file") != ToolPolicyPreloaded {
		t.Fatalf("read_file deveria ser preloaded em allowlist explícita, got %s", explicit.State("read_file"))
	}
	if explicit.State("grep_search") != ToolPolicyDisabled {
		t.Fatalf("grep_search ausente deveria ficar disabled em allowlist explícita, got %s", explicit.State("grep_search"))
	}

	disabled := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{EnabledTools: []string{"read_file"}, DisableTools: true})
	if disabled.State("read_file") != ToolPolicyDisabled || len(disabled.PreloadedNames()) != 0 {
		t.Fatalf("disable_tools deveria vencer todos os estados, got %s preloaded=%#v", disabled.State("read_file"), disabled.PreloadedNames())
	}
}

func TestToolSelectionPolicy_ExplicitToolPolicyOverridesLegacyEnabledTools(t *testing.T) {
	r := charRegistry(t)
	policy := NewToolSelectionPolicy(r)
	effective := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{
		EnabledTools: []string{"read_file"},
		ToolPolicy: map[string]string{
			"read_file":   string(ToolPolicyOnDemand),
			"grep_search": string(ToolPolicyPreloaded),
			"write_file":  string(ToolPolicyDisabled),
		},
	})

	if effective.State("read_file") != ToolPolicyOnDemand {
		t.Fatalf("tool_policy deveria sobrescrever enabled_tools para read_file, got %s", effective.State("read_file"))
	}
	if effective.State("grep_search") != ToolPolicyPreloaded {
		t.Fatalf("grep_search deveria ser preloaded pelo tool_policy, got %s", effective.State("grep_search"))
	}
	if effective.State("write_file") != ToolPolicyDisabled {
		t.Fatalf("write_file deveria ficar disabled pelo tool_policy, got %s", effective.State("write_file"))
	}
	assertNames(t, "preloaded", effective.PreloadedNames(), []string{"grep_search"})
}

func TestToolSelectionPolicy_CatalogVisibleNamesHideDisabledTools(t *testing.T) {
	r := charRegistry(t)
	policy := NewToolSelectionPolicy(r)

	open := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{})
	visibleOpen := map[string]bool{}
	for _, name := range open.CatalogVisibleNames() {
		visibleOpen[name] = true
	}
	if !visibleOpen["read_file"] || visibleOpen["text_edit"] {
		t.Fatalf("perfil legado aberto deveria revelar read_file e ocultar opt-in: %#v", open.CatalogVisibleNames())
	}

	explicit := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{EnabledTools: []string{"read_file"}})
	assertNames(t, "visible explicit", explicit.CatalogVisibleNames(), []string{"read_file"})
}

func TestToolSelectionPolicy_NativeMCPUsesPreloadedAllowlist(t *testing.T) {
	r := charRegistry(t)
	capable := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", ToolNames: []string{"mcp_srv__do"}},
	}}
	policy := NewToolSelectionPolicy(r)

	streamer, nativeDefs, _ := policy.PlanTurnToolDefs(capable, mgr, ProfileToolConfig{})
	result, ok := streamer.(*mockChatProvider)
	if !ok {
		t.Fatalf("esperava mock provider, got %#v", streamer)
	}
	if len(result.calledWith) != 0 {
		t.Fatalf("MCP nativo não deve expor tools on_demand no início do turno: %+v", result.calledWith)
	}
	assertNames(t, "native defs", defNames(nativeDefs), []string{tools.ToolCatalogName})

	streamer, nativeDefs, _ = policy.PlanTurnToolDefs(capable, mgr, ProfileToolConfig{EnabledTools: []string{"mcp_srv__do"}})
	result, ok = streamer.(*mockChatProvider)
	if !ok || len(result.calledWith) != 1 || len(result.calledWith[0].AllowedTools) != 1 || result.calledWith[0].AllowedTools[0] != "do" {
		t.Fatalf("allowlist explícita deveria expor só mcp_srv__do nativa, provider=%#v", streamer)
	}
	if len(nativeDefs) != 0 {
		t.Fatalf("bridge explicitamente preloaded deveria ser removida no caminho nativo, got %#v", nativeDefs)
	}
}

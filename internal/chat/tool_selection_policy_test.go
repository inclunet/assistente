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

			got := defNames(policy.ResolveExpandedToolDefs(streamer, mgr, tc.names, cfg))
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

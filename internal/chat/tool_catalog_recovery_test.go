package chat

import (
	"assistente/internal/tools"
	"testing"
)

func TestCatalogRecoveryRespectsProfileAndSkill(t *testing.T) {
	entry := tools.ToolCatalogEntry{Name: "mcp_remote__search", Package: "mcp:remote", Origin: tools.ToolOriginMCPBridge}
	for _, tc := range []struct {
		name  string
		cfg   ProfileToolConfig
		state ToolPolicyState
	}{
		{"default", ProfileToolConfig{}, ToolPolicyOnDemand},
		{"allowlist", ProfileToolConfig{EnabledTools: []string{entry.Name}}, ToolPolicyPreloaded},
		{"empty", ProfileToolConfig{EnabledTools: []string{}}, ToolPolicyDisabled},
		{"disabled", ProfileToolConfig{DisableTools: true}, ToolPolicyDisabled},
		{"selector", ProfileToolConfig{ToolPolicy: map[string]string{"mcp/remote/*": "on_demand"}}, ToolPolicyOnDemand},
		{"skill", ProfileToolConfig{SkillDeniedTools: []string{entry.Name}}, ToolPolicyDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := charRegistry(t)
			tc.cfg.CatalogTools = []tools.ToolCatalogEntry{entry}
			p := NewToolSelectionPolicy(r).ResolveEffectiveToolPolicy(tc.cfg)
			if got := p.State(entry.Name); got != tc.state {
				t.Fatalf("state=%s want=%s", got, tc.state)
			}
			if r.Has(entry.Name) {
				t.Fatal("metadata registered executable tool")
			}
			if tc.state == ToolPolicyOnDemand && !p.IsVisibleInCatalog(entry.Name) {
				t.Fatal("recoverable tool hidden")
			}
		})
	}
}

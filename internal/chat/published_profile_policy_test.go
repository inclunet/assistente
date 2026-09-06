package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/profiles"
)

func TestPublishedProfileToolPoliciesKeepTheirScope(t *testing.T) {
	for _, tc := range []struct {
		version string
		assert  func(*testing.T, EffectiveToolPolicy)
	}{
		{
			version: "0.1.9",
			assert: func(t *testing.T, effective EffectiveToolPolicy) {
				if effective.State("read_file") != ToolPolicyOnDemand ||
					effective.State("text_edit") != ToolPolicyDisabled {
					t.Fatalf("fallback legado com catálogo mudou de escopo")
				}
			},
		},
		{
			version: "0.2.0",
			assert: func(t *testing.T, effective EffectiveToolPolicy) {
				if effective.State("read_file") != ToolPolicyDisabled ||
					effective.State("grep_search") != ToolPolicyDisabled ||
					effective.State("text_edit") != ToolPolicyDisabled {
					t.Fatalf("lista vazia 0.2.0 abriu capability")
				}
			},
		},
		{
			version: "0.3.0",
			assert: func(t *testing.T, effective EffectiveToolPolicy) {
				if effective.State("read_file") != ToolPolicyPreloaded ||
					effective.State("grep_search") != ToolPolicyDisabled ||
					effective.State("text_edit") != ToolPolicyDisabled {
					t.Fatalf("default 0.3.0 abriu a allowlist legada")
				}
			},
		},
		{
			version: "0.4.0",
			assert: func(t *testing.T, effective EffectiveToolPolicy) {
				if effective.State("read_file") != ToolPolicyPreloaded ||
					effective.State("grep_search") != ToolPolicyOnDemand ||
					effective.State("write_file") != ToolPolicyDisabled {
					t.Fatalf("default disabled 0.4.0 abriu capability ausente")
				}
			},
		},
		{
			version: "0.5.0",
			assert: func(t *testing.T, effective EffectiveToolPolicy) {
				if effective.State("read_file") != ToolPolicyPreloaded ||
					effective.State("mcp_srv__do") != ToolPolicyOnDemand ||
					effective.State("text_edit") != ToolPolicyDisabled {
					t.Fatalf("wildcard MCP 0.5.0 perdeu escopo")
				}
			},
		},
	} {
		t.Run(tc.version, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "profiles", "testdata", "published", tc.version+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var profile profiles.Profile
			if err := json.Unmarshal(raw, &profile); err != nil {
				t.Fatal(err)
			}
			effective := NewToolSelectionPolicy(charRegistry(t)).ResolveEffectiveToolPolicy(ProfileToolConfig{
				EnabledTools:      profile.Chat.EnabledTools,
				ToolPolicy:        profile.Chat.ToolPolicy,
				ToolPolicyDefault: profile.Chat.ToolPolicyDefault,
				DisableTools:      profile.Chat.DisableTools,
			})
			tc.assert(t, effective)
		})
	}
}

func TestPublishedProfile019WithoutCatalogKeepsLegacyAllPreloaded(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "profiles", "testdata", "published", "0.1.9.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile profiles.Profile
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	effective := NewToolSelectionPolicy(charRegistryNoCatalog(t)).ResolveEffectiveToolPolicy(ProfileToolConfig{
		EnabledTools: profile.Chat.EnabledTools,
	})
	if got := effective.PreloadedNames(); got != nil ||
		effective.State("read_file") != ToolPolicyPreloaded ||
		effective.State("grep_search") != ToolPolicyPreloaded {
		t.Fatalf("legacyAllPreloaded 0.1.9 não preservado: names=%#v", got)
	}
}

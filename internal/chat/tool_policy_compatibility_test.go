package chat

import (
	"encoding/json"
	"testing"

	"assistente/internal/profiles"
	"assistente/internal/tools"
)

func TestToolPolicyUpgradeLegacyEnabledToolsPreservesCatalogDegradation(t *testing.T) {
	withCatalog := charRegistry(t)
	withoutCatalog := tools.NewRegistry()
	withoutCatalog.MustRegister(newToolDef("read_file"))
	withoutCatalog.MustRegister(newToolDef("grep_search"))
	withoutCatalog.MustRegisterOptIn(newToolDef("text_edit"))

	tests := []struct {
		name               string
		json               string
		wantNil            bool
		wantWithCatalog    []string
		wantWithoutCatalog []string
	}{
		{
			name:               "campo ausente",
			json:               `{}`,
			wantNil:            true,
			wantWithCatalog:    []string{tools.ToolCatalogName},
			wantWithoutCatalog: []string{"grep_search", "read_file"},
		},
		{
			name:               "null",
			json:               `{"enabled_tools":null}`,
			wantNil:            true,
			wantWithCatalog:    []string{tools.ToolCatalogName},
			wantWithoutCatalog: []string{"grep_search", "read_file"},
		},
		{
			name:               "lista vazia",
			json:               `{"enabled_tools":[]}`,
			wantNil:            false,
			wantWithCatalog:    []string{},
			wantWithoutCatalog: []string{},
		},
		{
			name:               "allowlist",
			json:               `{"enabled_tools":["read_file"]}`,
			wantNil:            false,
			wantWithCatalog:    []string{"read_file"},
			wantWithoutCatalog: []string{"read_file"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var persisted profiles.ChatConfig
			if err := json.Unmarshal([]byte(tc.json), &persisted); err != nil {
				t.Fatalf("decode do profile legado: %v", err)
			}
			if gotNil := persisted.EnabledTools == nil; gotNil != tc.wantNil {
				t.Fatalf("EnabledTools nil = %v, esperado %v", gotNil, tc.wantNil)
			}

			cfg := ProfileToolConfig{EnabledTools: persisted.EnabledTools}
			gotWithCatalog := defNames(NewToolSelectionPolicy(withCatalog).InitialToolDefs(cfg))
			assertNames(t, "com catálogo", gotWithCatalog, tc.wantWithCatalog)

			policyWithoutCatalog := NewToolSelectionPolicy(withoutCatalog)
			gotWithoutCatalog := defNames(policyWithoutCatalog.InitialToolDefs(cfg))
			assertNames(t, "sem catálogo", gotWithoutCatalog, tc.wantWithoutCatalog)
			if policyWithoutCatalog.ResolveEffectiveToolPolicy(cfg).State("text_edit") != ToolPolicyDisabled {
				t.Fatal("degradação sem catálogo não deve elevar tool opt-in")
			}
		})
	}
}

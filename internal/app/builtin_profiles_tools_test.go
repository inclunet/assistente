package app

import (
	"encoding/json"
	"io/fs"
	"testing"

	"assistente/internal/profiles"
)

func TestBuiltinProfilesDeclareOperationalToolBaselines(t *testing.T) {
	tests := []struct {
		filename       string
		defaultState   string
		expectedPolicy map[string]string
	}{
		{
			filename:     "programacao.json",
			defaultState: "on_demand",
			expectedPolicy: map[string]string{
				"read_file":    "preloaded",
				"search_files": "preloaded",
				"grep_search":  "preloaded",
				"edit_file":    "preloaded",
				"write_file":   "preloaded",
				"run_command":  "preloaded",
				"text_edit":    "disabled",
			},
		},
		{
			filename:     "padrao.json",
			defaultState: "on_demand",
			expectedPolicy: map[string]string{
				"read_file":         "preloaded",
				"search_files":      "preloaded",
				"grep_search":       "preloaded",
				"web_search":        "preloaded",
				"web_fetch":         "preloaded",
				"collect_responses": "preloaded",
				"text_edit":         "disabled",
			},
		},
		{
			filename:     "editor-texto.json",
			defaultState: "disabled",
			expectedPolicy: map[string]string{
				"text_edit": "preloaded",
				"edit_file": "preloaded",
			},
		},
		{
			filename:       "canais-comunicacao.json",
			defaultState:   "disabled",
			expectedPolicy: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+tc.filename)
			if err != nil {
				t.Fatalf("ler profile builtin: %v", err)
			}

			var raw struct {
				Chat map[string]json.RawMessage `json:"chat"`
			}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("decodificar profile bruto: %v", err)
			}
			if _, legacy := raw.Chat["enabled_tools"]; legacy {
				t.Fatal("profile builtin não deve depender de enabled_tools")
			}

			var profile profiles.Profile
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decodificar profile: %v", err)
			}
			if profile.Chat.ToolPolicyDefault != tc.defaultState {
				t.Fatalf("tool_policy_default = %q, esperado %q", profile.Chat.ToolPolicyDefault, tc.defaultState)
			}
			if len(profile.Chat.ToolPolicy) != len(tc.expectedPolicy) {
				t.Fatalf("tool_policy = %#v, esperado %#v", profile.Chat.ToolPolicy, tc.expectedPolicy)
			}
			for name, expectedState := range tc.expectedPolicy {
				if state := profile.Chat.ToolPolicy[name]; state != expectedState {
					t.Fatalf("tool_policy[%q] = %q, esperado %q", name, state, expectedState)
				}
			}
		})
	}
}

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
				"apply_patch":  "preloaded",
				"edit_file":    "preloaded",
				"write_file":   "preloaded",
				"run_command":  "preloaded",
				"update_plan":  "preloaded",
				"profile":      "preloaded",
				"subagent":     "preloaded",
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
				"profile":           "preloaded",
				"subagent":          "preloaded",
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

			// A instalação regrava o profile com MarshalIndent, e enabled_tools
			// não tem omitempty: o arquivo em disco ganha "enabled_tools": null.
			// Isso precisa continuar sendo nil na volta, senão o perfil viraria
			// allowlist legada e a tool_policy perderia a soberania.
			written, err := json.MarshalIndent(profile, "", "  ")
			if err != nil {
				t.Fatalf("serializar profile instalado: %v", err)
			}
			var reloaded profiles.Profile
			if err := json.Unmarshal(written, &reloaded); err != nil {
				t.Fatalf("decodificar profile instalado: %v", err)
			}
			if reloaded.Chat.EnabledTools != nil {
				t.Fatalf("enabled_tools = %#v após instalar, esperado nil", reloaded.Chat.EnabledTools)
			}
			if reloaded.Chat.ToolPolicyDefault != tc.defaultState {
				t.Fatalf("tool_policy_default = %q após instalar, esperado %q", reloaded.Chat.ToolPolicyDefault, tc.defaultState)
			}
			if len(reloaded.Chat.ToolPolicy) != len(tc.expectedPolicy) {
				t.Fatalf("tool_policy = %#v após instalar, esperado %#v", reloaded.Chat.ToolPolicy, tc.expectedPolicy)
			}
		})
	}
}

// Quando nenhum arquivo de perfil pôde ser lido, o Manager cai no
// DefaultProfile() em vez do padrao.json. Se os dois divergirem, justamente a
// instalação degradada perde o baseline da AEP-0096.
func TestDefaultProfileEspelhaOBaselineDoPadraoBuiltin(t *testing.T) {
	data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/padrao.json")
	if err != nil {
		t.Fatalf("ler profile builtin: %v", err)
	}
	var builtin profiles.Profile
	if err := json.Unmarshal(data, &builtin); err != nil {
		t.Fatalf("decodificar profile: %v", err)
	}

	fallback := profiles.DefaultProfile()
	if fallback.Chat.ToolPolicyDefault != builtin.Chat.ToolPolicyDefault {
		t.Fatalf("tool_policy_default do fallback = %q, esperado %q",
			fallback.Chat.ToolPolicyDefault, builtin.Chat.ToolPolicyDefault)
	}
	if len(fallback.Chat.ToolPolicy) != len(builtin.Chat.ToolPolicy) {
		t.Fatalf("tool_policy do fallback = %#v, esperado %#v",
			fallback.Chat.ToolPolicy, builtin.Chat.ToolPolicy)
	}
	for name, state := range builtin.Chat.ToolPolicy {
		if got := fallback.Chat.ToolPolicy[name]; got != state {
			t.Fatalf("tool_policy[%q] do fallback = %q, esperado %q", name, got, state)
		}
	}
}

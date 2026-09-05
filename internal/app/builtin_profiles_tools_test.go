package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/profileaccess"
	"assistente/internal/profiles"
)

type builtinRoutingProfileStore struct {
	infos  []profiles.ProfileInfo
	bySlug map[string]*profiles.Profile
}

func (s builtinRoutingProfileStore) List() ([]profiles.ProfileInfo, error) {
	return s.infos, nil
}

func (s builtinRoutingProfileStore) Get(slug string) (*profiles.Profile, error) {
	profile := s.bySlug[slug]
	if profile == nil {
		return nil, errors.New("profile não encontrado")
	}
	return profile, nil
}

func TestBuiltinProfilesDeclareOperationalToolBaselines(t *testing.T) {
	tests := []struct {
		filename       string
		version        string
		defaultState   string
		expectedPolicy map[string]string
	}{
		{
			filename:     "programacao.json",
			version:      "4.6.0",
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
				"mcp/*":        "on_demand",
				"text_edit":    "disabled",
			},
		},
		{
			filename:     "padrao.json",
			version:      "4.5.0",
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
				"mcp/*":             "on_demand",
				"text_edit":         "disabled",
			},
		},
		{
			filename:     "editor-texto.json",
			version:      "4.3.0",
			defaultState: "disabled",
			expectedPolicy: map[string]string{
				"text_edit": "preloaded",
				"edit_file": "preloaded",
			},
		},
		{
			filename:       "canais-comunicacao.json",
			version:        "4.2.0",
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
			if profile.BuiltinVersion != tc.version {
				t.Fatalf("_builtin_version = %q, esperado %q", profile.BuiltinVersion, tc.version)
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

func TestBuiltinProfilesMCPWildcardCobreToolsFuturasSemPreload(t *testing.T) {
	for _, filename := range []string{"padrao.json", "programacao.json"} {
		t.Run(filename, func(t *testing.T) {
			data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+filename)
			if err != nil {
				t.Fatalf("ler profile builtin: %v", err)
			}
			var profile profiles.Profile
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decodificar profile: %v", err)
			}

			matcher := chat.NewToolPolicyMatcher(profile.Chat.ToolPolicy, profile.Chat.ToolPolicyDefault)
			futureMCP := matcher.Resolve(chat.ToolPolicyTarget{Name: "mcp_future_server__new_tool"})
			if futureMCP.State != chat.ToolPolicyOnDemand {
				t.Fatalf("MCP futura = %s, esperado on_demand", futureMCP.State)
			}
			if futureMCP.State == chat.ToolPolicyPreloaded {
				t.Fatal("MCP futura não deve entrar no payload inicial")
			}
			optIn := matcher.Resolve(chat.ToolPolicyTarget{Name: "future_opt_in", OptIn: true})
			if optIn.State != chat.ToolPolicyDisabled {
				t.Fatalf("opt-in futura = %s, esperado disabled", optIn.State)
			}
		})
	}
}

func TestUpgradeBuiltinAplicaWildcardMCPPreservandoRuntime(t *testing.T) {
	tests := []struct {
		filename        string
		installedBefore string
	}{
		{filename: "padrao.json", installedBefore: "4.3.0"},
		{filename: "programacao.json", installedBefore: "4.4.0"},
	}
	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+tc.filename)
			if err != nil {
				t.Fatalf("ler profile builtin: %v", err)
			}
			var embedded profiles.Profile
			if err := json.Unmarshal(data, &embedded); err != nil {
				t.Fatalf("decodificar profile: %v", err)
			}
			if !isVersionNewer(embedded.BuiltinVersion, tc.installedBefore) {
				t.Fatalf("versão %s deve atualizar instalação %s", embedded.BuiltinVersion, tc.installedBefore)
			}

			existing := embedded
			existing.BuiltinVersion = tc.installedBefore
			existing.Active = true
			existing.Chat.ToolPolicy = make(map[string]string, len(embedded.Chat.ToolPolicy))
			for name, state := range embedded.Chat.ToolPolicy {
				existing.Chat.ToolPolicy[name] = state
			}
			delete(existing.Chat.ToolPolicy, "mcp/*")

			upgraded := mergeBuiltinPreservingRuntime(embedded, &existing)
			if upgraded.Chat.ToolPolicy["mcp/*"] != string(chat.ToolPolicyOnDemand) {
				t.Fatalf("upgrade não aplicou mcp/*: %#v", upgraded.Chat.ToolPolicy)
			}
			if !upgraded.Active {
				t.Fatal("upgrade deve preservar estado runtime Active")
			}
		})
	}
}

func TestProfileListExpoeDescricoesBuiltinAcionaveisParaRoteamento(t *testing.T) {
	store := builtinRoutingProfileStore{bySlug: make(map[string]*profiles.Profile)}
	for _, slug := range []string{"padrao", "programacao"} {
		data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+slug+".json")
		if err != nil {
			t.Fatalf("ler profile builtin %s: %v", slug, err)
		}
		var profile profiles.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			t.Fatalf("decodificar profile builtin %s: %v", slug, err)
		}
		store.infos = append(store.infos, profiles.ProfileInfo{
			Slug: slug, Name: profile.Name, Description: profile.Description,
		})
		store.bySlug[slug] = &profile
	}

	items, err := profileaccess.NewService(store, nil, nil, nil).List(t.Context(), "padrao")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("profile list = %#v, esperados Padrão e Programação", items)
	}

	bySlug := make(map[string]profileaccess.ProfileSummary, len(items))
	for _, item := range items {
		bySlug[item.Slug] = item
		for _, section := range []string{"Use quando", "Não use", "Exemplos:"} {
			if !strings.Contains(item.Description, section) {
				t.Errorf("descrição de %s não contém %q: %q", item.Slug, section, item.Description)
			}
		}
	}
	if !bySlug["padrao"].Current {
		t.Fatal("profile list não marcou Padrão como atual")
	}
	if !strings.Contains(bySlug["padrao"].Description, "escolha Programação") {
		t.Errorf("Padrão não orienta a especialização em código: %q", bySlug["padrao"].Description)
	}
	if !strings.Contains(bySlug["programacao"].Description, "escolha Padrão") {
		t.Errorf("Programação não orienta o retorno a tarefas gerais: %q", bySlug["programacao"].Description)
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
	if fallback.Description != builtin.Description {
		t.Fatalf("description do fallback = %q, esperado %q", fallback.Description, builtin.Description)
	}
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

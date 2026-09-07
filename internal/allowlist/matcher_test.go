package allowlist

import (
	"encoding/json"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		command string
		pattern string
		want    bool
	}{
		// Match exato
		{"exato simples", "ls", "ls", true},
		{"exato com args", "git status", "git status", true},
		{"exato não casa", "git push", "git status", false},

		// Pattern como base command (sem wildcard, mas comando tem args)
		{"base com args", "ls -la", "ls", true},
		{"base com args 2", "git status --short", "git status", true},
		{"base não casa", "lsof", "ls", false},

		// Wildcard no final com espaço: "git diff *"
		{"wildcard com espaço", "git diff HEAD", "git diff *", true},
		{"wildcard com espaço exato", "git diff", "git diff *", false}, // "git diff" não começa com "git diff "
		{"wildcard com espaço no casa", "git status", "git diff *", false},

		// Wildcard solo: "npm*"
		{"wildcard solo", "npm install", "npm*", true},
		{"wildcard solo exato", "npm", "npm*", true},
		{"wildcard solo prefixo", "npmrc", "npm*", true},
		{"wildcard solo não casa", "node", "npm*", false},

		// Edge cases
		{"pattern vazio", "ls", "", false},
		{"comando vazio", "", "ls", false},
		{"ambos vazios", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchPattern(tt.command, tt.pattern)
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.command, tt.pattern, got, tt.want)
			}
		})
	}
}

// Os testes que cobriam Allowlist.Evaluate (removido no AEP-0060) foram
// migrados para internal/commandpolicy/evaluator_test.go: a semantica geral
// (deny vence approve, default_action, DefaultAllowlist end-to-end, nil
// allowlist, comando vazio) e responsabilidade do commandpolicy, que agora
// e o unico avaliador de comandos. Aqui mantemos apenas os testes
// relacionados ao MatchPattern, ao schema JSON da Allowlist e a utilitarios
// internos do pacote.

func TestAllowlist_CommandRulesJSONCompatibility(t *testing.T) {
	raw := []byte(`{
		"name": "Kubernetes",
		"auto_approve": ["git status"],
		"always_deny": ["rm -rf *"],
		"command_rules": [
			{
				"program": "kubectl",
				"subcommands": ["get"],
				"args": ["*"],
				"decision": "approve",
				"description": "leitura"
			}
		],
		"default_action": "confirm"
	}`)

	var al Allowlist
	if err := json.Unmarshal(raw, &al); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(al.CommandRules) != 1 {
		t.Fatalf("command rule count = %d, want 1", len(al.CommandRules))
	}
	rule := al.CommandRules[0]
	if rule.Program != "kubectl" || rule.Subcommands[0] != "get" || rule.Args[0] != "*" || rule.Decision != "approve" {
		t.Fatalf("unexpected command rule: %#v", rule)
	}

	encoded, err := json.Marshal(&al)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("encoded allowlist is not valid JSON: %s", string(encoded))
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Padrão", "padrao"},
		{"Desenvolvimento Web", "desenvolvimento-web"},
		{"Admin (Root)", "admin-root"},
		{"Café & Código", "cafe--codigo"},
		{"simple", "simple"},
		{"  spaces  ", "spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugify(tt.name)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

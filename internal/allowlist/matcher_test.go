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

func TestEvaluate_NilAllowlist(t *testing.T) {
	var al *Allowlist
	if al.Evaluate("ls") != DecisionConfirm {
		t.Error("nil allowlist should return DecisionConfirm")
	}
}

func TestEvaluate_EmptyCommand(t *testing.T) {
	al := &Allowlist{}
	if al.Evaluate("") != DecisionDeny {
		t.Error("empty command should be denied")
	}
	if al.Evaluate("  ") != DecisionDeny {
		t.Error("whitespace command should be denied")
	}
}

func TestEvaluate_DenyTakesPriority(t *testing.T) {
	al := &Allowlist{
		AutoApprove: []string{"rm"},
		AlwaysDeny:  []string{"rm -rf /"},
	}

	// "rm -rf /" está em deny E em approve (via base match) — deny ganha
	if al.Evaluate("rm -rf /") != DecisionDeny {
		t.Error("deny should take priority over approve")
	}

	// "rm file.txt" está apenas em approve
	if al.Evaluate("rm file.txt") != DecisionApprove {
		t.Error("'rm file.txt' should be approved")
	}
}

func TestEvaluate_AutoApprove(t *testing.T) {
	al := &Allowlist{
		AutoApprove:   []string{"ls", "git status", "git diff *"},
		DefaultAction: "confirm",
	}

	tests := []struct {
		command  string
		expected Decision
	}{
		{"ls", DecisionApprove},
		{"ls -la", DecisionApprove},
		{"git status", DecisionApprove},
		{"git status --short", DecisionApprove},
		{"git diff HEAD~1", DecisionApprove},
		{"git push", DecisionConfirm}, // não está na allowlist
		{"rm -rf /", DecisionConfirm}, // não está em deny nem approve
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := al.Evaluate(tt.command)
			if got != tt.expected {
				t.Errorf("Evaluate(%q) = %v, want %v", tt.command, got, tt.expected)
			}
		})
	}
}

func TestEvaluate_DefaultActionDeny(t *testing.T) {
	al := &Allowlist{
		AutoApprove:   []string{"ls"},
		DefaultAction: "deny",
	}

	if al.Evaluate("ls") != DecisionApprove {
		t.Error("'ls' should be approved")
	}
	if al.Evaluate("rm -rf /") != DecisionDeny {
		t.Error("unknown command with default 'deny' should be denied")
	}
}

func TestEvaluate_DefaultAllowlist(t *testing.T) {
	al := DefaultAllowlist()

	// Comandos que devem ser aprovados
	approved := []string{
		"ls", "ls -la",
		"pwd",
		"git status", "git diff main",
		"echo hello",
		"go version",
		"go test ./...",
	}
	for _, cmd := range approved {
		if al.Evaluate(cmd) != DecisionApprove {
			t.Errorf("default allowlist should approve %q", cmd)
		}
	}

	// Comandos que devem ser negados
	denied := []string{
		"rm -rf /",
		"shutdown",
		"reboot",
	}
	for _, cmd := range denied {
		if al.Evaluate(cmd) != DecisionDeny {
			t.Errorf("default allowlist should deny %q", cmd)
		}
	}

	// Comandos que devem pedir confirmação
	confirm := []string{
		"rm temp.txt",
		"curl https://example.com",
		"docker rm container",
	}
	for _, cmd := range confirm {
		if al.Evaluate(cmd) != DecisionConfirm {
			t.Errorf("default allowlist should confirm %q", cmd)
		}
	}
}

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

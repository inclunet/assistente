package allowlist

import (
	"strings"
	"testing"
)

func TestAllowlist_Validate_AcceptsValid(t *testing.T) {
	al := &Allowlist{
		Name:          "Padrao",
		AutoApprove:   []string{"ls"},
		AlwaysDeny:    []string{"rm -rf /"},
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "approve"},
			{Program: "kubectl", Subcommands: []string{"delete", "*"}, Decision: "deny"},
			{Program: "psql", Decision: "Confirm"},
		},
	}
	if err := al.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestAllowlist_Validate_RejectsUnknownDecision(t *testing.T) {
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "kubectl", Decision: "approv"},
		},
	}
	err := al.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for unknown decision")
	}
	if !strings.Contains(err.Error(), "decision") {
		t.Fatalf("error %q does not mention decision", err.Error())
	}
}

func TestAllowlist_Validate_RejectsEmptyDecision(t *testing.T) {
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "kubectl", Decision: ""},
		},
	}
	if err := al.Validate(); err == nil {
		t.Fatal("Validate() = nil, want error for empty decision")
	}
}

func TestAllowlist_Validate_RejectsEmptyProgram(t *testing.T) {
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "  ", Decision: "approve"},
		},
	}
	err := al.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for empty program")
	}
	if !strings.Contains(err.Error(), "program") {
		t.Fatalf("error %q does not mention program", err.Error())
	}
}

func TestAllowlist_Validate_RejectsWildcardOutOfTail(t *testing.T) {
	cases := []struct {
		name string
		rule CommandRule
		want string
	}{
		{
			name: "wildcard no meio de subcommands",
			rule: CommandRule{Program: "kubectl", Subcommands: []string{"pod", "*", "--force"}, Decision: "approve"},
			want: "subcommands",
		},
		{
			name: "wildcard no meio de args",
			rule: CommandRule{Program: "kubectl", Args: []string{"a", "*", "b"}, Decision: "approve"},
			want: "args",
		},
		{
			name: "wildcard primeiro de varios em subcommands",
			rule: CommandRule{Program: "kubectl", Subcommands: []string{"*", "x"}, Decision: "approve"},
			want: "subcommands",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &Allowlist{Name: "x", DefaultAction: "confirm", CommandRules: []CommandRule{tc.rule}}
			err := al.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestAllowlist_Validate_AcceptsWildcardAtTail(t *testing.T) {
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "kubectl", Subcommands: []string{"get", "*"}, Args: []string{"*"}, Decision: "approve"},
			{Program: "kubectl", Subcommands: []string{"delete"}, Args: []string{"pod", "*"}, Decision: "deny"},
		},
	}
	if err := al.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestAllowlist_Validate_RejectsInvalidDefaultAction(t *testing.T) {
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "block",
	}
	err := al.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for invalid default_action")
	}
	if !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("error %q does not mention default_action", err.Error())
	}
}

func TestAllowlist_Validate_RejectsTrailingWildcardSubcommandsWithArgs(t *testing.T) {
	// Subcommands terminando em "*" consome todo o restante de cmd.Args, entao
	// declarar Args nao-vazio depois e ambiguo: a regra ficaria silenciosamente
	// inerte porque rule.Args nunca chegaria a ser testado.
	cases := []struct {
		name string
		rule CommandRule
	}{
		{
			name: "trailing * in subcommands + args literal",
			rule: CommandRule{Program: "kubectl", Subcommands: []string{"get", "*"}, Args: []string{"--force"}, Decision: "deny"},
		},
		{
			name: "trailing * in subcommands + args composto",
			rule: CommandRule{Program: "kubectl", Subcommands: []string{"*"}, Args: []string{"-n", "*"}, Decision: "approve"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &Allowlist{Name: "x", DefaultAction: "confirm", CommandRules: []CommandRule{tc.rule}}
			err := al.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), "subcommands termina com") {
				t.Fatalf("error %q nao menciona o conflito subcommands+args", err.Error())
			}
		})
	}
}

func TestAllowlist_Validate_AcceptsTrailingWildcardSubcommandsWithEmptyArgs(t *testing.T) {
	// Args vazio ou Args=["*"] sao no-op em termos de matching, entao nao
	// conflitam com Subcommands terminando em "*".
	cases := []struct {
		name string
		rule CommandRule
	}{
		{name: "args vazio", rule: CommandRule{Program: "kubectl", Subcommands: []string{"get", "*"}, Decision: "approve"}},
		{name: "args wildcard solo", rule: CommandRule{Program: "kubectl", Subcommands: []string{"get", "*"}, Args: []string{"*"}, Decision: "approve"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &Allowlist{Name: "x", DefaultAction: "confirm", CommandRules: []CommandRule{tc.rule}}
			if err := al.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestAllowlist_Validate_RejectsEmptyOrWhitespaceTokens(t *testing.T) {
	// Copilot review thread (PR #117): tokens vazios ou whitespace-only em
	// Subcommands/Args passavam na validacao mas tornavam a regra
	// silenciosamente inerte porque matchSequence faz comparacao literal
	// case-insensitive sem TrimSpace ("get " jamais casa "get").
	cases := []struct {
		name     string
		rule     CommandRule
		wantPart string
	}{
		{
			name:     "subcommand vazio",
			rule:     CommandRule{Program: "kubectl", Subcommands: []string{"get", ""}, Decision: "approve"},
			wantPart: "subcommands[1]",
		},
		{
			name:     "subcommand com whitespace adjacente",
			rule:     CommandRule{Program: "kubectl", Subcommands: []string{"get "}, Decision: "approve"},
			wantPart: "subcommands[0]",
		},
		{
			name:     "subcommand apenas whitespace",
			rule:     CommandRule{Program: "kubectl", Subcommands: []string{"   "}, Decision: "approve"},
			wantPart: "subcommands[0]",
		},
		{
			name:     "subcommand apenas tab",
			rule:     CommandRule{Program: "kubectl", Subcommands: []string{"\t"}, Decision: "approve"},
			wantPart: "subcommands[0]",
		},
		{
			name:     "arg vazio",
			rule:     CommandRule{Program: "kubectl", Args: []string{""}, Decision: "approve"},
			wantPart: "args[0]",
		},
		{
			name:     "arg apenas whitespace",
			rule:     CommandRule{Program: "kubectl", Args: []string{"  "}, Decision: "approve"},
			wantPart: "args[0]",
		},
		{
			name:     "arg com whitespace no fim",
			rule:     CommandRule{Program: "kubectl", Args: []string{"--force "}, Decision: "approve"},
			wantPart: "args[0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &Allowlist{Name: "x", DefaultAction: "confirm", CommandRules: []CommandRule{tc.rule}}
			err := al.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantPart)
			}
		})
	}
}

func TestAllowlist_Validate_RejectsWildcardWithWhitespace(t *testing.T) {
	// "* " ou " *" jamais casa o wildcard real (matchSequence compara contra
	// "*" exato) e tambem nao casa um arg literal "*", entao a regra ficaria
	// silenciosamente inerte. Forcamos o usuario a escrever "*" exato.
	cases := []struct {
		name string
		rule CommandRule
	}{
		{name: "subcommand wildcard com espaco no fim", rule: CommandRule{Program: "kubectl", Subcommands: []string{"* "}, Decision: "approve"}},
		{name: "subcommand wildcard com espaco no inicio", rule: CommandRule{Program: "kubectl", Subcommands: []string{" *"}, Decision: "approve"}},
		{name: "args wildcard com espacos dos dois lados", rule: CommandRule{Program: "kubectl", Args: []string{" * "}, Decision: "approve"}},
		{name: "args wildcard com tab", rule: CommandRule{Program: "kubectl", Args: []string{"*\t"}, Decision: "approve"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &Allowlist{Name: "x", DefaultAction: "confirm", CommandRules: []CommandRule{tc.rule}}
			err := al.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), "wildcard") {
				t.Fatalf("error %q does not mention wildcard", err.Error())
			}
		})
	}
}

func TestAllowlist_Validate_AcceptsTokensWithInternalWhitespace(t *testing.T) {
	// Tokens com espaco interno (vindos de args quotados pelo parser, ex.:
	// echo "a b" produz Args=["a b"]) sao casos legitimos e devem ser
	// aceitos. So whitespace adjacente cria ambiguidade.
	al := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "echo", Args: []string{"a b"}, Decision: "approve"},
			{Program: "git", Subcommands: []string{"commit -m"}, Decision: "approve"},
		},
	}
	if err := al.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil (tokens com espaco interno sao validos)", err)
	}
}

func TestAllowlist_Validate_AggregatesMultipleErrors(t *testing.T) {
	al := &Allowlist{
		Name:          "",
		DefaultAction: "block",
		CommandRules: []CommandRule{
			{Program: "", Decision: ""},
		},
	}
	err := al.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want aggregated errors")
	}
	for _, want := range []string{"name", "default_action", "program", "decision"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

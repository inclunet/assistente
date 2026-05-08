package commandpolicy

import (
	"strings"
	"testing"

	"assistente/internal/allowlist"
)

func TestEvaluate_ApprovesCompoundCommandsWhenAllAtomsAllowed(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status", "git diff"},
		DefaultAction: "confirm",
	}

	got := Evaluate("git status && git diff", al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_DenyInCompoundCommandWins(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status"},
		AlwaysDeny:    []string{"rm -rf *"},
		DefaultAction: "confirm",
	}

	got := Evaluate("git status && rm -rf dist", al)
	if got.Decision != allowlist.DecisionDeny {
		t.Fatalf("decision = %s, want deny; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_ConfirmInCompoundCommandWinsApprove(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status"},
		DefaultAction: "confirm",
	}

	got := Evaluate("git status && docker ps", al)
	if got.Decision != allowlist.DecisionConfirm {
		t.Fatalf("decision = %s, want confirm; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_KubectlStructuredRules(t *testing.T) {
	al := &allowlist.Allowlist{
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "approve"},
			{Program: "kubectl", Subcommands: []string{"delete"}, Args: []string{"*"}, Decision: "deny"},
			{Program: "kubectl", Subcommands: []string{"patch"}, Args: []string{"*"}, Decision: "confirm"},
		},
		DefaultAction: "confirm",
	}

	tests := []struct {
		command string
		want    allowlist.Decision
	}{
		{"kubectl get pods", allowlist.DecisionApprove},
		{"kubectl delete pod x", allowlist.DecisionDeny},
		{"kubectl patch deployment x", allowlist.DecisionConfirm},
		{"kubectl get pods && kubectl delete pod x", allowlist.DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := Evaluate(tt.command, al)
			if got.Decision != tt.want {
				t.Fatalf("decision = %s, want %s; reasons=%v", got.Decision, tt.want, got.Reasons)
			}
		})
	}
}

func TestEvaluate_StructuredDenyOverridesLegacyAutoApprove(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove: []string{"kubectl *"},
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"delete"}, Args: []string{"*"}, Decision: "deny"},
		},
		DefaultAction: "confirm",
	}

	got := Evaluate("kubectl delete pod x", al)
	if got.Decision != allowlist.DecisionDeny {
		t.Fatalf("decision = %s, want deny; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_StructuredConfirmOverridesLegacyAutoApprove(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove: []string{"kubectl *"},
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"patch"}, Args: []string{"*"}, Decision: "confirm"},
		},
		DefaultAction: "confirm",
	}

	got := Evaluate("kubectl patch deployment x", al)
	if got.Decision != allowlist.DecisionConfirm {
		t.Fatalf("decision = %s, want confirm; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_ConservativeFeaturesForceConfirmation(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"echo", "git status", "cat"},
		DefaultAction: "confirm",
	}

	tests := []string{
		"echo ok > out.txt",
		"echo ok >> out.txt",
		"echo ok 2> err.log",
		"cat < in.txt",
		"cat << EOF\nhello\nEOF",
		"git status | cat",
		"echo $(pwd)",
		"echo `pwd`",
	}

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			got := Evaluate(command, al)
			if got.Decision != allowlist.DecisionConfirm {
				t.Fatalf("decision = %s, want confirm; reasons=%v", got.Decision, got.Reasons)
			}
		})
	}
}

func TestEvaluate_ReasonsDoNotLeakCommandArguments(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"kubectl get *"},
		AlwaysDeny:    []string{"rm *"},
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{Program: "psql", Subcommands: []string{"-h"}, Args: []string{"*"}, Decision: "confirm"},
		},
	}

	commands := []string{
		"kubectl get secret api-token-prod-supersecret",
		"rm /tmp/secret-token-XYZ",
		"psql -h db.prod.internal -U admin -W hunter2",
		"docker run -e PASSWORD=hunter2 alpine",
	}

	leakedFragments := []string{
		"api-token-prod-supersecret",
		"secret-token-XYZ",
		"db.prod.internal",
		"hunter2",
		"PASSWORD",
	}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			got := Evaluate(command, al)
			joined := strings.Join(got.Reasons, " | ")
			for _, fragment := range leakedFragments {
				if strings.Contains(joined, fragment) {
					t.Fatalf("reasons %q leaked argument fragment %q", joined, fragment)
				}
			}
		})
	}
}

func TestEvaluate_NoDuplicateReasonForUnrecognizedCommand(t *testing.T) {
	al := &allowlist.Allowlist{DefaultAction: "confirm"}

	got := Evaluate(";", al)
	count := 0
	for _, reason := range got.Reasons {
		if reason == "nenhum comando atomico reconhecido" {
			count++
		}
	}
	if count > 1 {
		t.Fatalf("reason 'nenhum comando atomico reconhecido' duplicada %d vezes: %v", count, got.Reasons)
	}
}

func TestEvaluate_WildcardInMiddleIsLiteral(t *testing.T) {
	// Regra estruturada com "*" no meio jamais deve casar (defesa em
	// profundidade — a validacao de save tambem rejeita esse caso, mas o
	// runtime deve se proteger contra perfis legados/manualmente editados).
	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"pod", "*", "--force"}, Decision: "approve"},
		},
	}

	got := Evaluate("kubectl pod something --force", al)
	if got.Decision == allowlist.DecisionApprove {
		t.Fatalf("regra com * no meio nao deveria aprovar: reasons=%v", got.Reasons)
	}
}

func TestEvaluate_WildcardAtTailMatches(t *testing.T) {
	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "approve"},
		},
	}

	got := Evaluate("kubectl get pods -n production", al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_StructuredConfirmBeatsStructuredApproveForSameAtom(t *testing.T) {
	// Documenta a precedencia interna: quando duas regras estruturadas casam
	// o mesmo atomo, "confirm" vence "approve" (fail-closed).
	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "approve"},
			{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "confirm"},
		},
	}

	got := Evaluate("kubectl get secret api-token", al)
	if got.Decision != allowlist.DecisionConfirm {
		t.Fatalf("decision = %s, want confirm (precedencia interna)", got.Decision)
	}
}

func TestEvaluate_LegacyPatternWithQuotesStillMatches(t *testing.T) {
	// Patterns legados (pre-AEP-0060) podem ter sido escritos com aspas
	// envolvendo args com espaco. O parser remove as aspas em Args, entao
	// precisamos garantir que o matching tente tambem a forma re-quotada para
	// nao quebrar perfis existentes.
	al := &allowlist.Allowlist{
		AutoApprove:   []string{`echo "a b"`},
		DefaultAction: "confirm",
	}

	got := Evaluate(`echo "a b"`, al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve (legacy quoted pattern); reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_LegacyPatternWithoutQuotesStillMatches(t *testing.T) {
	// O matching duplo nao pode quebrar quem tinha pattern sem aspas casando
	// argumentos com espaco (Args:["a b"] devolvia String() "echo a b").
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"echo a b"},
		DefaultAction: "confirm",
	}

	got := Evaluate(`echo "a b"`, al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve (legacy unquoted pattern); reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_LegacyPatternWithSingleQuotesStillMatches(t *testing.T) {
	// Patterns legados podem ter sido escritos com aspas simples
	// (ex.: `echo 'a b'`). O parser remove ambas as aspas em Args, entao o
	// matching precisa tentar a forma com aspas simples re-aplicadas alem
	// das outras duas formas.
	al := &allowlist.Allowlist{
		AutoApprove:   []string{`echo 'a b'`},
		DefaultAction: "confirm",
	}

	got := Evaluate(`echo "a b"`, al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve (legacy single-quoted pattern); reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluate_StructuredRuleConsumesAllArgsWhenSubcommandsEndWithWildcard(t *testing.T) {
	// Antes: matchSequence retornava true sem indicar tokens consumidos e o
	// chamador usava len(rule.Subcommands), que descartava args reais quando
	// o ultimo Subcommand era "*". Agora consumed reflete o consumo real.
	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{Program: "kubectl", Subcommands: []string{"get", "*"}, Decision: "approve"},
		},
	}

	got := Evaluate("kubectl get pods deployment -n production", al)
	if got.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve; reasons=%v", got.Decision, got.Reasons)
	}
}

func TestCommand_SingleQuotedString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want string
	}{
		{name: "no args", cmd: Command{Program: "ls"}, want: "ls"},
		{name: "simple args sem caractere especial", cmd: Command{Program: "git", Args: []string{"status"}}, want: "git status"},
		{name: "arg com espaco", cmd: Command{Program: "echo", Args: []string{"a b"}}, want: `echo 'a b'`},
		{name: "arg com aspas duplas usa aspas simples", cmd: Command{Program: "echo", Args: []string{`a"b`}}, want: `echo 'a"b'`},
		{name: "arg com aspa simples cai para sem aspas", cmd: Command{Program: "echo", Args: []string{"a'b"}}, want: `echo a'b`},
		{name: "arg vazio", cmd: Command{Program: "echo", Args: []string{""}}, want: `echo ''`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cmd.SingleQuotedString(); got != tt.want {
				t.Fatalf("SingleQuotedString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommand_QuotedString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want string
	}{
		{name: "no args", cmd: Command{Program: "ls"}, want: "ls"},
		{name: "simple args", cmd: Command{Program: "git", Args: []string{"status"}}, want: "git status"},
		{name: "arg com espaco", cmd: Command{Program: "echo", Args: []string{"a b"}}, want: `echo "a b"`},
		{name: "arg com aspas duplas", cmd: Command{Program: "echo", Args: []string{`a"b`}}, want: `echo "a\"b"`},
		{name: "arg vazio", cmd: Command{Program: "echo", Args: []string{""}}, want: `echo ""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cmd.QuotedString(); got != tt.want {
				t.Fatalf("QuotedString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvaluate_DefaultAllowlistKubectlPolicy(t *testing.T) {
	al := allowlist.DefaultAllowlist()

	tests := []struct {
		command string
		want    allowlist.Decision
	}{
		{"kubectl get pods", allowlist.DecisionApprove},
		{"kubectl describe pod x", allowlist.DecisionApprove},
		{"kubectl logs pod/x", allowlist.DecisionApprove},
		{"kubectl delete pod x", allowlist.DecisionConfirm},
		{"kubectl patch deployment x", allowlist.DecisionConfirm},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := Evaluate(tt.command, al)
			if got.Decision != tt.want {
				t.Fatalf("decision = %s, want %s; reasons=%v", got.Decision, tt.want, got.Reasons)
			}
		})
	}
}

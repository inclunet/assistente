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

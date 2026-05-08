package commandpolicy

import (
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

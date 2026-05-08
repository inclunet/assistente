package commandpolicy

import (
	"fmt"
	"strings"

	"assistente/internal/allowlist"
)

// EvaluationResult descreve a decisao final para uma linha de comando.
type EvaluationResult struct {
	Decision allowlist.Decision
	Parse    ParseResult
	Reasons  []string
}

// Evaluate parseia e avalia uma linha de comando contra a allowlist.
func Evaluate(commandLine string, al *allowlist.Allowlist) EvaluationResult {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" {
		return EvaluationResult{
			Decision: allowlist.DecisionDeny,
			Reasons:  []string{"comando vazio"},
		}
	}

	parsed := Parse(commandLine)
	result := EvaluationResult{
		Decision: allowlist.DecisionApprove,
		Parse:    parsed,
	}

	for _, cmd := range parsed.Commands {
		decision, reason := evaluateAtom(cmd, al)
		result.Reasons = append(result.Reasons, reason)
		result.Decision = combineDecision(result.Decision, decision)
	}

	if len(parsed.Commands) == 0 {
		result.Decision = combineDecision(result.Decision, allowlist.DecisionConfirm)
		result.Reasons = append(result.Reasons, "nenhum comando atomico reconhecido")
	}

	if parsed.RequiresConfirmation() && result.Decision != allowlist.DecisionDeny {
		result.Decision = combineDecision(result.Decision, allowlist.DecisionConfirm)
		for _, feature := range parsed.Features {
			result.Reasons = append(result.Reasons, fmt.Sprintf("feature conservadora detectada: %s", feature))
		}
		for _, err := range parsed.Errors {
			result.Reasons = append(result.Reasons, fmt.Sprintf("sintaxe ambigua: %s", err))
		}
	}

	return result
}

func evaluateAtom(cmd Command, al *allowlist.Allowlist) (allowlist.Decision, string) {
	if al == nil {
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao: sem allowlist ativa", cmd.String())
	}

	if cmd.Program == "" {
		return allowlist.DecisionDeny, "comando atomico vazio"
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionDeny); ok {
		return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por regra estruturada: %s", cmd.String(), describeRule(rule))
	}

	for _, pattern := range al.AlwaysDeny {
		if matchLegacyPattern(cmd.String(), pattern) {
			return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por always_deny: %s", cmd.String(), pattern)
		}
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionConfirm); ok {
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao por regra estruturada: %s", cmd.String(), describeRule(rule))
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionApprove); ok {
		return allowlist.DecisionApprove, fmt.Sprintf("%q aprovado por regra estruturada: %s", cmd.String(), describeRule(rule))
	}

	for _, pattern := range al.AutoApprove {
		if matchLegacyPattern(cmd.String(), pattern) {
			return allowlist.DecisionApprove, fmt.Sprintf("%q aprovado por auto_approve: %s", cmd.String(), pattern)
		}
	}

	switch strings.ToLower(al.DefaultAction) {
	case "deny":
		return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por default_action=deny", cmd.String())
	default:
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao por default_action=confirm", cmd.String())
	}
}

func combineDecision(current, next allowlist.Decision) allowlist.Decision {
	if current == allowlist.DecisionDeny || next == allowlist.DecisionDeny {
		return allowlist.DecisionDeny
	}
	if current == allowlist.DecisionConfirm || next == allowlist.DecisionConfirm {
		return allowlist.DecisionConfirm
	}
	return allowlist.DecisionApprove
}

func matchStructuredRule(cmd Command, rules []allowlist.CommandRule, decision allowlist.Decision) (allowlist.CommandRule, bool) {
	for _, rule := range rules {
		if parseRuleDecision(rule.Decision) != decision {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.Program), cmd.Program) {
			continue
		}
		if !matchSequence(cmd.Args, rule.Subcommands) {
			continue
		}
		remainingStart := len(rule.Subcommands)
		if remainingStart > len(cmd.Args) {
			remainingStart = len(cmd.Args)
		}
		remainingArgs := cmd.Args[remainingStart:]
		if !matchSequence(remainingArgs, rule.Args) {
			continue
		}
		return rule, true
	}
	return allowlist.CommandRule{}, false
}

func matchSequence(values, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	if len(patterns) == 1 && patterns[0] == "*" {
		return true
	}
	if len(values) < len(patterns) {
		return false
	}
	for i, pattern := range patterns {
		if pattern == "*" {
			return true
		}
		if !strings.EqualFold(values[i], pattern) {
			return false
		}
	}
	return true
}

func parseRuleDecision(value string) allowlist.Decision {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "deny":
		return allowlist.DecisionDeny
	case "approve":
		return allowlist.DecisionApprove
	default:
		return allowlist.DecisionConfirm
	}
}

func describeRule(rule allowlist.CommandRule) string {
	parts := []string{rule.Program}
	parts = append(parts, rule.Subcommands...)
	parts = append(parts, rule.Args...)
	if rule.Description != "" {
		return strings.Join(parts, " ") + " (" + rule.Description + ")"
	}
	return strings.Join(parts, " ")
}

func matchLegacyPattern(command, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}

	if command == pattern {
		return true
	}

	return strings.HasPrefix(command, pattern+" ")
}

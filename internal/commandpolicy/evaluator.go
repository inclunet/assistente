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
		// Evita duplicar com parsed.Errors, que ja contem a mesma mensagem quando
		// o parser nao reconhece nenhum atomo (e que sera reformatada como
		// "sintaxe ambigua" abaixo).
		if !containsString(parsed.Errors, "nenhum comando atomico reconhecido") {
			result.Reasons = append(result.Reasons, "nenhum comando atomico reconhecido")
		}
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

// evaluateAtom avalia um unico comando atomico contra a allowlist e devolve
// uma reason segura para usuario/log: usamos apenas cmd.Program (e nao
// cmd.String()) para evitar replicar args completos em logs ou no output da
// tool, que pode conter segredos (tokens, paths, credenciais).
//
// Precedencia interna (fail-closed) quando varias regras casam o mesmo atomo:
//
//  1. structured deny
//  2. legacy always_deny
//  3. structured confirm     // antes de approve para preservar excecoes restritivas
//  4. structured approve
//  5. legacy auto_approve
//  6. default_action da allowlist
//
// Esta ordem é diferente da agregacao entre atomos compostos
// (deny > confirm > approve, em ParseResult/Evaluate): aqui ela existe para
// que um usuario consiga adicionar uma regra restritiva (ex.:
// "kubectl get secret confirm") sobre uma regra mais permissiva
// ("kubectl get * approve") sem precisar reordenar. Documentado tambem em
// aep/0060-command-policy-parser.md.
func evaluateAtom(cmd Command, al *allowlist.Allowlist) (allowlist.Decision, string) {
	if al == nil {
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao: sem allowlist ativa", cmd.Program)
	}

	if cmd.Program == "" {
		return allowlist.DecisionDeny, "comando atomico vazio"
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionDeny); ok {
		return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por regra estruturada: %s", cmd.Program, describeRule(rule))
	}

	for _, pattern := range al.AlwaysDeny {
		if allowlist.MatchPattern(cmd.String(), pattern) {
			return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por always_deny: %s", cmd.Program, pattern)
		}
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionConfirm); ok {
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao por regra estruturada: %s", cmd.Program, describeRule(rule))
	}

	if rule, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionApprove); ok {
		return allowlist.DecisionApprove, fmt.Sprintf("%q aprovado por regra estruturada: %s", cmd.Program, describeRule(rule))
	}

	for _, pattern := range al.AutoApprove {
		if allowlist.MatchPattern(cmd.String(), pattern) {
			return allowlist.DecisionApprove, fmt.Sprintf("%q aprovado por auto_approve: %s", cmd.Program, pattern)
		}
	}

	switch strings.ToLower(al.DefaultAction) {
	case "deny":
		return allowlist.DecisionDeny, fmt.Sprintf("%q bloqueado por default_action=deny", cmd.Program)
	default:
		return allowlist.DecisionConfirm, fmt.Sprintf("%q exige confirmacao por default_action=confirm", cmd.Program)
	}
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
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

// matchSequence faz casamento posicional de patterns contra values.
//
// O coringa "*" so e tratado como wildcard quando aparece como ultimo
// elemento da lista de patterns (consome o restante de values). Em qualquer
// outra posicao, "*" e tratado como literal — um usuario que escreva
// Subcommands: ["pod", "*", "--force"] espera, intuitivamente, que --force
// seja obrigatorio. A validacao em allowlist.Validate() rejeita "*" fora da
// ultima posicao no momento do save, mas mantemos esse comportamento defensivo
// aqui para perfis legados ou regras editadas manualmente.
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
		if i == len(patterns)-1 && pattern == "*" {
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


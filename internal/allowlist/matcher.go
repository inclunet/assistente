package allowlist

import (
	"strings"
)

// Evaluate avalia um comando contra as regras da allowlist.
// Ordem de avaliação:
//  1. AlwaysDeny — se bater, retorna DecisionDeny
//  2. AutoApprove — se bater, retorna DecisionApprove
//  3. DefaultAction — "confirm" (padrão) ou "deny"
func (al *Allowlist) Evaluate(command string) Decision {
	if al == nil {
		return DecisionConfirm
	}

	// Normaliza o comando (remove espaços extras)
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return DecisionDeny
	}

	// 1. Verifica AlwaysDeny primeiro
	for _, pattern := range al.AlwaysDeny {
		if matchPattern(cmd, pattern) {
			return DecisionDeny
		}
	}

	// 2. Verifica AutoApprove
	for _, pattern := range al.AutoApprove {
		if matchPattern(cmd, pattern) {
			return DecisionApprove
		}
	}

	// 3. Decisão padrão
	switch strings.ToLower(al.DefaultAction) {
	case "deny":
		return DecisionDeny
	default:
		return DecisionConfirm
	}
}

// matchPattern verifica se um comando corresponde a um pattern.
//
// Regras de matching:
//   - Pattern exato: "git status" casa com "git status" apenas
//   - Pattern com wildcard no final: "git *" casa com qualquer comando que começa com "git "
//   - Pattern simples (sem wildcard): "ls" casa com "ls" ou "ls <args>"
//     (ou seja, o comando deve ser exatamente o pattern ou começar com pattern seguido de espaço)
func matchPattern(command, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	// Pattern com wildcard no final: "git diff *" → prefix match com "git diff "
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}

	// Pattern com wildcard solo: "npm*" → prefix match com "npm"
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}

	// Match exato ou command é pattern + argumentos
	if command == pattern {
		return true
	}

	// O comando começa com o pattern seguido de espaço (o pattern é o "base command")
	return strings.HasPrefix(command, pattern+" ")
}

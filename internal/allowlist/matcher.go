package allowlist

import (
	"strings"
)

// MatchPattern verifica se um comando corresponde a um pattern legado.
//
// Regras de matching:
//   - Pattern exato: "git status" casa com "git status" apenas
//   - Pattern com wildcard no final: "git *" casa com qualquer comando que começa com "git "
//   - Pattern simples (sem wildcard): "ls" casa com "ls" ou "ls <args>"
//     (ou seja, o comando deve ser exatamente o pattern ou começar com pattern seguido de espaço)
//
// Esta funcao e o ponto unico de matching de patterns legados (auto_approve /
// always_deny) e e consumida por commandpolicy.Evaluate, que e o avaliador
// efetivo (parser conservador + politica). O metodo Allowlist.Evaluate antigo
// foi removido (AEP-0060): toda decisao de comando passa por
// commandpolicy.Evaluate, evitando dois caminhos divergentes.
func MatchPattern(command, pattern string) bool {
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

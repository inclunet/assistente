package fstrust

import (
	"fmt"
	"strings"
)

// PathAllowlistDeepLink abre a tela de gestão da allowlist de path. Vai na
// mensagem de negação como link Markdown: quem lê a conversa consegue ir
// direto revisar o que já está autorizado (e revogar), em vez de caçar a tela
// nas configurações (AEP-0092 D6 / Fase 1b).
const PathAllowlistDeepLink = "assistente://navigate/settings/path-allowlist"

// DeniedPathError é o erro acionável devolvido quando um path fora do sandbox
// não foi autorizado (usuário negou ou não há prompter). Expõe path, operação
// e ações possíveis — inclusive deep link para a tela de gestão.
type DeniedPathError struct {
	Path        string
	Operation   string
	Reason      string
	Suggestions []string
}

func (e *DeniedPathError) Error() string {
	var b strings.Builder
	b.WriteString("acesso a path fora do sandbox bloqueado")
	if e.Path != "" {
		fmt.Fprintf(&b, "\n- Path: %s", e.Path)
	}
	if e.Operation != "" {
		fmt.Fprintf(&b, "\n- Operação: %s", e.Operation)
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, "\n- Motivo: %s", e.Reason)
	}
	if len(e.Suggestions) > 0 {
		label := "Ação possível"
		if len(e.Suggestions) > 1 {
			label = "Ações possíveis"
		}
		fmt.Fprintf(&b, "\n- %s: %s", label, strings.Join(e.Suggestions, " | "))
	}
	return b.String()
}

func newDeniedPathError(path, operation, reason string) *DeniedPathError {
	suggestions := make([]string, 0, 2)
	// Sem prompter não há diálogo: sugerir autorizar no diálogo seria mentira.
	if reason != "sem prompter de consentimento" {
		suggestions = append(suggestions, "autorizar esta tentativa no diálogo de consentimento")
	}
	suggestions = append(suggestions,
		"revisar ou revogar autorizações em [allowlist de paths]("+PathAllowlistDeepLink+")",
	)
	return &DeniedPathError{
		Path:        path,
		Operation:   operation,
		Reason:      reason,
		Suggestions: suggestions,
	}
}

package skills

import (
	"fmt"
	"strings"
)

// SubstituteArguments substitui variáveis de argumento no conteúdo de um skill.
// Compatível com a spec SKILL.md (Claude Code):
//   - $ARGUMENTS → string completa de argumentos
//   - $1, $2, ..., $N → argumentos posicionais
//
// Argumentos entre aspas (simples ou duplas) são tratados como um único argumento.
// Exemplo: `/fix-issue 42 "my description"` → $1=42, $2=my description, $ARGUMENTS=42 "my description"
func SubstituteArguments(content string, rawArgs string) string {
	if content == "" {
		return ""
	}

	// Substitui $ARGUMENTS pela string completa
	result := strings.ReplaceAll(content, "$ARGUMENTS", rawArgs)

	// Faz parse dos argumentos posicionais (respeitando aspas)
	args := parseArgs(rawArgs)

	// Substitui $N de trás para frente para evitar conflitos ($10 antes de $1)
	for i := len(args); i >= 1; i-- {
		placeholder := fmt.Sprintf("$%d", i)
		result = strings.ReplaceAll(result, placeholder, args[i-1])
	}

	return result
}

// parseArgs faz parse de uma string de argumentos, respeitando aspas.
// "hello world" → um argumento: hello world
// 'single quoted' → um argumento: single quoted
// arg1 arg2 → dois argumentos: arg1, arg2
func parseArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	inQuote := rune(0) // 0 = fora, '"' ou '\'' = dentro de aspas

	for _, ch := range raw {
		switch {
		case inQuote != 0:
			if ch == inQuote {
				// Fecha aspas — não inclui a aspa no valor
				inQuote = 0
			} else {
				current.WriteRune(ch)
			}
		case ch == '"' || ch == '\'':
			// Abre aspas
			inQuote = ch
		case ch == ' ' || ch == '\t':
			// Separador fora de aspas
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Último argumento
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

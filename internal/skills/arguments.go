package skills

import (
	"fmt"
	"strings"
)

// SubstituteArguments substitui variáveis de argumento no conteúdo de um skill.
// Compatível com a spec oficial do Claude Code:
//   - $ARGUMENTS → string completa de argumentos
//   - $ARGUMENTS[N] → argumento posicional 0-based (bracket syntax)
//   - $N → shorthand para $ARGUMENTS[N] (0-based: $0 = primeiro arg)
//   - ${CLAUDE_SESSION_ID} → ID da sessão corrente
//
// Se $ARGUMENTS NÃO está presente no conteúdo, os argumentos são appendados como "ARGUMENTS: <value>".
//
// vars: variáveis de sessão adicionais (ex: {"CLAUDE_SESSION_ID": "123"}).
//
// Argumentos entre aspas (simples ou duplas) são tratados como um único argumento.
// Exemplo: `/fix-issue 42 "my description"` → $0=42, $1=my description, $ARGUMENTS=42 "my description"
func SubstituteArguments(content string, rawArgs string, vars ...map[string]string) string {
	if content == "" {
		return ""
	}

	// Detecta se $ARGUMENTS (plain, não bracket) existe no conteúdo
	// Para o fallback: se NÃO existe e há args, appenda "ARGUMENTS: <value>"
	hasArgumentsPlaceholder := containsPlainArguments(content)

	// Faz parse dos argumentos posicionais (respeitando aspas)
	args := parseArgs(rawArgs)

	result := content

	// 1. Substitui $ARGUMENTS[N] PRIMEIRO (bracket syntax, 0-based, mais específico)
	for i := len(args) - 1; i >= 0; i-- {
		bracket := fmt.Sprintf("$ARGUMENTS[%d]", i)
		result = strings.ReplaceAll(result, bracket, args[i])
	}

	// 2. Substitui $ARGUMENTS pela string completa (depois dos brackets)
	result = strings.ReplaceAll(result, "$ARGUMENTS", rawArgs)

	// 3. Substitui $N (shorthand, 0-based) — de trás para frente para evitar conflitos ($10 antes de $1)
	for i := len(args) - 1; i >= 0; i-- {
		placeholder := fmt.Sprintf("$%d", i)
		result = strings.ReplaceAll(result, placeholder, args[i])
	}

	// 4. Substitui variáveis de sessão (${CLAUDE_SESSION_ID}, etc.)
	if len(vars) > 0 && vars[0] != nil {
		for k, v := range vars[0] {
			result = strings.ReplaceAll(result, "${"+k+"}", v)
		}
	}

	// 5. Fallback: se $ARGUMENTS não estava no conteúdo original e há args, appenda
	if !hasArgumentsPlaceholder && rawArgs != "" {
		result = result + "\n\nARGUMENTS: " + rawArgs
	}

	return result
}

// containsPlainArguments verifica se o conteúdo contém $ARGUMENTS (plain),
// excluindo $ARGUMENTS[N] (bracket syntax). Usado para decidir se o fallback é necessário.
func containsPlainArguments(content string) bool {
	idx := 0
	for {
		pos := strings.Index(content[idx:], "$ARGUMENTS")
		if pos == -1 {
			return false
		}
		pos += idx
		afterEnd := pos + len("$ARGUMENTS")
		// Se após "$ARGUMENTS" tem "[", é bracket syntax — ignora
		if afterEnd < len(content) && content[afterEnd] == '[' {
			idx = afterEnd
			continue
		}
		return true
	}
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

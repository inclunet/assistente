package jobs

import (
	"context"
	"regexp"
	"strings"

	"assistente/internal/tools/invocationctx"
)

const redactedValue = "[redacted]"

// Só um primeiro token literal e separado do template pode identificar a
// operação na pergunta. Nunca extraímos o programa do valor do cofre.
var shellTemplatePrefix = regexp.MustCompile(`^[ \t]*([a-zA-Z][a-zA-Z0-9._-]{0,63})[ \t]+`)

// RedactResolvedInputs keeps run history useful without persisting secrets
// resolved from templates or values entered directly into sensitive fields.
func RedactResolvedInputs(original, resolved map[string]any) map[string]any {
	if len(resolved) == 0 {
		return map[string]any{}
	}
	redacted := make(map[string]any, len(resolved))
	for key, value := range resolved {
		redacted[key] = redactValue(key, original[key], value)
	}
	return redacted
}

// O shell ecoa o comando nos metadados. Propague a redação antes que ele
// produza o resultado, inclusive para saídas parciais ou armazenadas à parte.
func withRedactedCommandMetadata(ctx context.Context, original, persisted map[string]any) context.Context {
	// encoding/json aceita variantes de caixa nas chaves de uma struct. Se
	// houver mais de uma, qualquer uma redigida deve proteger os metadados.
	matches := 0
	var protectedKey string
	for key, value := range persisted {
		if strings.EqualFold(key, "command") {
			matches++
			if value == redactedValue {
				protectedKey = key
			}
		}
	}
	if protectedKey != "" {
		var prefix string
		if template, ok := original[protectedKey].(string); ok && matches == 1 && containsSecretTemplate(template) {
			if match := shellTemplatePrefix.FindStringSubmatch(template); match != nil {
				prefix = match[1]
			}
		}
		return invocationctx.WithProtectedShellCommand(ctx, redactedValue, prefix)
	}
	return ctx
}

func ContainsRedactedValue(value any) bool {
	switch v := value.(type) {
	case string:
		return v == redactedValue
	case map[string]any:
		for _, child := range v {
			if ContainsRedactedValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if ContainsRedactedValue(child) {
				return true
			}
		}
	}
	return false
}

func redactValue(key string, original, resolved any) any {
	if isSensitiveInputKey(key) || containsSecretTemplate(original) {
		return redactedValue
	}
	switch value := resolved.(type) {
	case map[string]any:
		origMap, _ := original.(map[string]any)
		next := make(map[string]any, len(value))
		for childKey, childValue := range value {
			next[childKey] = redactValue(childKey, origMap[childKey], childValue)
		}
		return next
	case []any:
		origSlice, _ := original.([]any)
		next := make([]any, len(value))
		for i, childValue := range value {
			var childOriginal any
			if i < len(origSlice) {
				childOriginal = origSlice[i]
			}
			next[i] = redactValue("", childOriginal, childValue)
		}
		return next
	default:
		return resolved
	}
}

func containsSecretTemplate(value any) bool {
	switch v := value.(type) {
	case string:
		normalized := strings.ToLower(v)
		return strings.Contains(normalized, "{{") && strings.Contains(normalized, "secret")
	case map[string]any:
		for _, child := range v {
			if containsSecretTemplate(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsSecretTemplate(child) {
				return true
			}
		}
	}
	return false
}

func isSensitiveInputKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	compact := strings.NewReplacer("-", "", "_", "", " ", "", ".", "").Replace(normalized)
	for _, token := range []string{
		"api_key",
		"access_key",
		"authorization",
		"client_secret",
		"cookie",
		"credential",
		"jwt",
		"password",
		"private_key",
		"refresh_token",
		"secret",
		"session",
		"session_id",
		"token",
	} {
		tokenCompact := strings.ReplaceAll(token, "_", "")
		if strings.Contains(normalized, token) || strings.Contains(compact, tokenCompact) {
			return true
		}
	}
	return false
}

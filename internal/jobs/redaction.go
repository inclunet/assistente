package jobs

import "strings"

const redactedValue = "[redacted]"

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
		"credential",
		"password",
		"private_key",
		"refresh_token",
		"secret",
		"token",
	} {
		tokenCompact := strings.ReplaceAll(token, "_", "")
		if strings.Contains(normalized, token) || strings.Contains(compact, tokenCompact) {
			return true
		}
	}
	return false
}

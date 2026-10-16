package commandledger

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"assistente/internal/commandforeground"
	"assistente/internal/commandjson"
)

// O envelope recebe a projeção do adapter, não o snapshot nativo. Mesmo assim,
// a persistência aplica a allowlist D14 novamente: formatos desconhecidos ou
// inválidos ficam integralmente redigidos, inclusive em reservas negadas.
func redactedForegroundIfPresent(value *json.RawMessage) *string {
	if value == nil {
		return nil
	}
	redacted := envelopeStringPtr(redactedDocument)
	if len(*value) > 4096 {
		return redacted
	}
	canonical, err := commandjson.Canonicalize(*value)
	if err != nil {
		return redacted
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(canonical, &fields) != nil || len(fields) != 3 {
		return redacted
	}
	for _, key := range []string{"executable", "window_class", "provider_version"} {
		if _, ok := fields[key]; !ok {
			return redacted
		}
	}
	var summary commandforeground.Summary
	if json.Unmarshal(canonical, &summary) != nil || summary.ProviderVersion != commandforeground.ProviderVersion ||
		!validForegroundAuditText(summary.Executable) || !validForegroundAuditText(summary.WindowClass) ||
		strings.ContainsAny(summary.WindowClass, `/\`) || foregroundClassHasDrivePrefix(summary.WindowClass) ||
		strings.ContainsAny(summary.Executable, `/\:`) || summary.Executable == "." || summary.Executable == ".." ||
		summary.Executable != strings.ToLower(summary.Executable) {
		return redacted
	}
	encoded, err := commandjson.Marshal(summary)
	if err != nil {
		return redacted
	}
	return envelopeStringPtr(string(encoded))
}

func validForegroundAuditText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 256 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

func foregroundClassHasDrivePrefix(value string) bool {
	return len(value) >= 2 && value[1] == ':' && (value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z')
}

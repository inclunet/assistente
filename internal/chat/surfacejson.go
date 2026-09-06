package chat

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/logging"
)

// DecodeSurfaceJSONMap decodifica um JSON de objeto usado para surface state/context.
// Retorna nil para vazio, whitespace, objeto vazio ou payload inválido.
func DecodeSurfaceJSONMap(raw string, logPrefix string) map[string]any {
	return DecodeSurfaceJSONMapWithLogger(raw, logPrefix, func(format string, args ...any) {
		logging.Warnf(context.Background(), "chat.surfacejson", format, args...)
	})
}

// DecodeCanonicalSurfaceContextJSON aceita somente o envelope completo
// definido pela AEP-0080. Payloads antigos ou incompletos são ignorados.
func DecodeCanonicalSurfaceContextJSON(raw string, logPrefix string) map[string]any {
	decoded := DecodeSurfaceJSONMap(raw, logPrefix)
	if !hasNonEmptySurfaceString(decoded, "surfaceType") ||
		!hasNonEmptySurfaceString(decoded, "surfaceId") ||
		!hasNonEmptySurfaceString(decoded, "snapshotVersion") {
		return nil
	}
	return decoded
}

func hasNonEmptySurfaceString(values map[string]any, key string) bool {
	value, ok := values[key].(string)
	return ok && strings.TrimSpace(value) != ""
}

// DecodeSurfaceJSONMapWithLogger permite injetar o logger em testes sem tocar no logger global.
func DecodeSurfaceJSONMapWithLogger(raw string, logPrefix string, logf func(string, ...any)) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		if logf != nil {
			logf("%s inválido: %v", logPrefix, err)
		}
		return nil
	}
	if len(decoded) == 0 {
		return nil
	}
	return decoded
}

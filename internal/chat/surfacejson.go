package chat

import (
	"encoding/json"
	"log"
	"strings"
)

// DecodeSurfaceJSONMap decodifica um JSON de objeto usado para surface state/context.
// Retorna nil para vazio, whitespace, objeto vazio ou payload inválido.
func DecodeSurfaceJSONMap(raw string, logPrefix string) map[string]any {
	return DecodeSurfaceJSONMapWithLogger(raw, logPrefix, log.Printf)
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

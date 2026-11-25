package jobs

import (
	"encoding/json"
	"log/slog"

	"assistente/internal/tools"
)

// Objetos expõem seus campos aos templates. Arrays ficam em content. Escalares
// JSON (inclusive null) e texto conservam a representação recebida da ferramenta.
func jobToolOutput(result tools.ToolResult, debug *slog.Logger) map[string]any {
	output := make(map[string]any)
	if result.Content != "" {
		var value any
		if err := json.Unmarshal([]byte(result.Content), &value); err != nil {
			value = nil
		}
		switch value := value.(type) {
		case map[string]any:
			output = value
			if debug != nil {
				keys := make([]string, 0, len(output))
				for key := range output {
					keys = append(keys, key)
				}
				debug.Debug("tool output parsed as object", slog.Any("keys", keys))
			}
		case []any:
			output["content"] = value
			if debug != nil {
				debug.Debug("tool output parsed as array", slog.Int("elements", len(value)))
			}
		default:
			output["content"] = result.Content
			if debug != nil {
				debug.Debug("tool output stored as raw string", slog.Int("content_length", len(result.Content)))
			}
		}
	}
	for key, value := range result.Metadata {
		output["_meta_"+key] = value
	}
	return output
}

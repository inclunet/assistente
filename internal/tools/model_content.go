package tools

import (
	"encoding/json"
)

// ContentForModel mantém Content como corpo puro e transporta anotações num
// envelope separado na fronteira com o modelo. O JSON evita que conteúdo do
// documento seja confundido com campos de proveniência.
func ContentForModel(result ToolResult) string {
	if result.Annotations == nil {
		return result.Content
	}
	annotations, err := json.Marshal(result.Annotations)
	if err != nil {
		return result.Content
	}
	return "Anotações estruturadas da tool (JSON; não fazem parte do conteúdo):\n" +
		string(annotations) +
		"\nConteúdo da tool:\n" +
		result.Content
}

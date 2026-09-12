package tools

import (
	"encoding/json"
	"strings"
)

const (
	annotationsHeader = "Anotações estruturadas da tool (JSON; não fazem parte do conteúdo):\n"
	contentHeader     = "\nConteúdo da tool:\n"
)

// ContentForModel mantém Content como corpo puro e transporta anotações num
// envelope separado na fronteira com o modelo. O JSON evita que conteúdo do
// documento seja confundido com campos de proveniência.
//
// As anotações vêm antes do corpo porque o pre-check de janela de contexto
// trunca pelo fim: assim o que se perde é a cauda do documento, não a
// proveniência.
func ContentForModel(result ToolResult) string {
	// RawExact é o contrato textual exato da chamada. Anotações ainda podem ser
	// carregadas para UI/auditoria (por exemplo, proveniência de uma projeção),
	// mas não são prefixadas ao texto enviado ao modelo.
	if result.RawExact && !result.IsError &&
		(result.Annotations == nil ||
			(result.Annotations.OutputWindow == nil && result.Annotations.HTTPResponse == nil)) {
		return result.Content
	}
	if result.Annotations == nil {
		return result.Content
	}
	annotations, err := json.Marshal(result.Annotations)
	if err != nil {
		return result.Content
	}
	return annotationsHeader + string(annotations) + contentHeader + result.Content
}

// ContentForModelSize calcula o tamanho model-facing sem materializar o corpo.
// É útil para produtores streaming que já conhecem o número de bytes coletados.
func ContentForModelSize(contentBytes int, annotations *ResultAnnotations, rawExact bool) int {
	if annotations == nil || (rawExact && annotations.OutputWindow == nil && annotations.HTTPResponse == nil) {
		return contentBytes
	}
	encoded, err := json.Marshal(annotations)
	if err != nil {
		return contentBytes
	}
	return len(annotationsHeader) + len(encoded) + len(contentHeader) + contentBytes
}

// ContentForDurableHistory remove referências ao LRU efêmero antes de gravar
// mensagens role=tool. Uma prévia retomável não pode sobreviver como se o
// result_id continuasse válido depois de reinício ou eviction.
func ContentForDurableHistory(result ToolResult, modelContent string) string {
	if window := outputWindowOf(result); window != nil && window.ResultID != "" {
		if window.HasMore {
			return "[result_omitted_for_persistence] Resultado retomável omitido do histórico durável."
		}
		cloned := cloneMutableResultFields(result)
		cloned.Annotations.OutputWindow = cloneOutputWindow(window)
		cloned.Annotations.OutputWindow.ResultID = ""
		if cloned.Metadata != nil {
			delete(cloned.Metadata, "result_id")
		}
		return ContentForModel(cloned)
	}
	if strings.HasPrefix(modelContent, annotationsHeader) {
		parts := strings.SplitN(modelContent, contentHeader, 2)
		if len(parts) == 2 {
			var annotations ResultAnnotations
			rawAnnotations := strings.TrimPrefix(parts[0], annotationsHeader)
			if json.Unmarshal([]byte(rawAnnotations), &annotations) == nil {
				window := annotations.OutputWindow
				if window != nil && window.ResultID != "" {
					if window.HasMore {
						return "[result_omitted_for_persistence] Resultado retomável omitido do histórico durável."
					}
					window.ResultID = ""
					encoded, err := json.Marshal(annotations)
					if err == nil {
						return annotationsHeader + string(encoded) + contentHeader + parts[1]
					}
				}
			}
		}
	}
	return modelContent
}

// SanitizeTruncatedEnvelope descarta um envelope cortado no meio das anotações.
// Quando o orçamento de contexto é menor que o próprio cabeçalho, a truncagem
// deixa um cabeçalho ou um JSON pela metade que o modelo leria como
// proveniência legítima — entregar nada é mais honesto que entregar metade.
//
// A decisão usa result como fonte da verdade em vez de adivinhar pelo texto:
// só quem tinha anotações precisa exibir o envelope inteiro.
func SanitizeTruncatedEnvelope(result ToolResult, content string) string {
	if result.Annotations == nil {
		return content
	}
	if strings.HasPrefix(content, annotationsHeader) && strings.Contains(content, contentHeader) {
		return content
	}
	return ""
}

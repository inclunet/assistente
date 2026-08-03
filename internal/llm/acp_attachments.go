package llm

import (
	"strings"

	"assistente/internal/acp"
)

// Anexos no turno do agente (AEP-0084). A mensagem da pessoa pode ser
// multimodal e o session/prompt recebe uma lista de blocos, então o mapeamento
// é direto: cada parte de texto vira bloco de texto, cada imagem vira bloco de
// imagem, na ordem original.
//
// O que o agente aceita vem do initialize. Anexo que ele não aceita não é
// descartado em silêncio: o turno segue com o texto e a pessoa é avisada do que
// ficou de fora.

// dataURLPrefix e base64Marker delimitam a imagem embutida que o pipeline
// monta. O bloco do protocolo é base64 mais tipo MIME, então uma imagem que
// esteja em outro lugar — um endereço remoto, por exemplo — não tem como ser
// embutida no pedido.
const (
	dataURLPrefix = "data:"
	base64Marker  = ";base64,"
)

// turnContent monta os blocos da mensagem da pessoa e devolve quantos anexos
// ficaram de fora.
func turnContent(msg Message, acceptsImage bool) (content []acp.Content, notSent int) {
	for _, part := range messageParts(msg) {
		switch part.Type {
		case "text":
			if text := strings.TrimSpace(part.Text); text != "" {
				content = append(content, acp.TextContent(text))
			}
		case "image_url":
			data, mime, inline := inlineImage(part)
			if !acceptsImage || !inline {
				notSent++
				continue
			}
			content = append(content, acp.ImageContent(data, mime))
		}
	}
	return content, notSent
}

// messageParts normaliza o conteúdo da mensagem nas partes que interessam ao
// turno. A lista chega nos dois formatos que o pipeline produz: tipada, quando
// o builder a montou, e destipada, quando ela veio de JSON.
func messageParts(msg Message) []ContentPart {
	switch parts := msg.Content.(type) {
	case []ContentPart:
		return parts
	case []interface{}:
		out := make([]ContentPart, 0, len(parts))
		for _, item := range parts {
			partMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			kind, _ := partMap["type"].(string)
			switch kind {
			case "text":
				text, _ := partMap["text"].(string)
				out = append(out, ContentPart{Type: kind, Text: text})
			case "image_url":
				part := ContentPart{Type: kind}
				if image, ok := partMap["image_url"].(map[string]interface{}); ok {
					url, _ := image["url"].(string)
					part.ImageURL = &ImageURL{URL: url}
				}
				out = append(out, part)
			}
		}
		return out
	default:
		// Conteúdo simples: o turno inteiro é uma parte de texto.
		return []ContentPart{{Type: "text", Text: msg.GetContentAsString()}}
	}
}

// inlineImage separa a imagem embutida em dados e tipo. Só a data URL em base64
// serve: é o formato que o bloco do protocolo carrega.
func inlineImage(part ContentPart) (data, mime string, ok bool) {
	if part.ImageURL == nil {
		return "", "", false
	}
	url := strings.TrimSpace(part.ImageURL.URL)
	if !strings.HasPrefix(url, dataURLPrefix) {
		return "", "", false
	}
	marker := strings.Index(url, base64Marker)
	if marker < 0 {
		return "", "", false
	}
	mime = strings.TrimSpace(url[len(dataURLPrefix):marker])
	data = url[marker+len(base64Marker):]
	if mime == "" || data == "" {
		return "", "", false
	}
	return data, mime, true
}

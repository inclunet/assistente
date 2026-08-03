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
//
// acceptsImage é função porque a resposta vem do agente: perguntar num turno de
// texto puro seria conversa à toa, e uma falha nela viraria aviso de anexo numa
// mensagem que não tem anexo nenhum.
func turnContent(msg Message, acceptsImage func() bool) (content []acp.Content, notSent int) {
	accepts, asked := false, false
	for _, part := range messageParts(msg) {
		switch part.Type {
		case "":
			// Sem tipo não há como saber o que é: não é conteúdo que o app
			// tenha deixado de enviar, e contá-lo daria alarme falso.
			continue
		case "text":
			if text := strings.TrimSpace(part.Text); text != "" {
				content = append(content, acp.TextContent(text))
			}
		case "image_url":
			if !asked {
				accepts, asked = acceptsImage(), true
			}
			data, mime, inline := inlineImage(part)
			if !accepts || !inline {
				notSent++
				continue
			}
			content = append(content, acp.ImageContent(data, mime))
		default:
			// Áudio, documento e vídeo chegam aqui: o pipeline os monta quando
			// o modelo os recebe nativamente, e o bloco do ACP só carrega texto
			// e imagem. Some deles em silêncio e a pessoa espera uma resposta
			// sobre o que o agente nunca recebeu.
			notSent++
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
			default:
				// O tipo desconhecido aqui é anexo que o app monta para outros
				// modelos (áudio, documento, vídeo). Filtrá-lo nesta passagem o
				// esconderia de quem conta o que não foi enviado.
				out = append(out, ContentPart{Type: kind})
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

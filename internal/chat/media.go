package chat

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/llm"
)

// SupportedAudioFormats lista os formatos de áudio aceitos nativamente pela API OpenAI input_audio.
// Formatos fora desta lista requerem transcrição via Whisper antes de enviar ao LLM.
var SupportedAudioFormats = map[string]bool{
	"wav": true, "mp3": true,
}

// whisperExtensionMap mapeia extensões/formatos para extensões aceitas pelo Whisper.
// Whisper aceita: flac, m4a, mp3, mp4, mpeg, mpga, oga, ogg, wav, webm.
// AAC é o codec dentro de M4A, então mapeamos aac → m4a.
var whisperExtensionMap = map[string]string{
	"aac":  "m4a",
	"opus": "ogg",
}

// WhisperFilename retorna um nome de arquivo com extensão compatível com Whisper.
func WhisperFilename(format string) string {
	if mapped, ok := whisperExtensionMap[format]; ok {
		return fmt.Sprintf("audio.%s", mapped)
	}
	return fmt.Sprintf("audio.%s", format)
}

// truncate encurta uma string para uso em logs.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ExtractAudio examina um mediaJSON e retorna o base64 e MIME do primeiro áudio encontrado.
// Retorna ("", "") se não houver áudio ou o JSON for inválido.
func ExtractAudio(mediaJSON string) (audioBase64, mimeType string) {
	var parts []map[string]interface{}
	if err := json.Unmarshal([]byte(mediaJSON), &parts); err != nil {
		return "", ""
	}
	for _, p := range parts {
		t, _ := p["type"].(string)
		d, _ := p["data"].(string)
		if strings.HasPrefix(t, "audio/") && d != "" {
			return d, t
		}
	}
	return "", ""
}

// TranscribeFunc abstrai a transcrição de áudio sem acoplar este pacote ao internal/speech.
// Retorna o texto transcrito ou string vazia em caso de falha (nunca erro fatal).
type TranscribeFunc func(ctx context.Context, audioBase64, filename string) (string, error)

// MediaHistoryLoader carrega o histórico de conversa convertendo mídias para o formato LLM.
type MediaHistoryLoader struct {
	Repo       MessageRepository
	Transcribe TranscribeFunc // nil = transcrição indisponível
	MaxMsgs    int
}

// Load retorna as mensagens formatadas para o LLM e o resumo existente da conversa.
func (l *MediaHistoryLoader) Load(ctx context.Context, conversationID string) ([]llm.Message, string, error) {
	h := HistoryLoader{Repo: l.Repo, MaxMsgs: l.MaxMsgs}
	dbMessages, existingSummary, err := h.Load(ctx, conversationID)
	if err != nil {
		return nil, "", err
	}

	messages := make([]llm.Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		// Otimização de contexto: omitir mensagens intermediárias de tool calling
		// de turnos anteriores. O modelo já processou esses resultados e produziu
		// uma resposta final com a informação sintetizada — reenviar a cadeia
		// tool_call→tool_result desperdiça tokens sem valor.
		if m.Role == "tool" {
			continue
		}
		if m.Role == "assistant" && strings.TrimSpace(m.ToolCalls) != "" && strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Reasoning) == "" {
			continue
		}

		msg := llm.Message{
			MessageID:  m.ID,
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}

		if m.Media != "" {
			var mediaParts []map[string]interface{}
			if err := json.Unmarshal([]byte(m.Media), &mediaParts); err == nil {
				msg.Content = l.convertMediaParts(ctx, mediaParts, m.Content)
			} else {
				msg.Content = m.Content
			}
		} else {
			msg.Content = m.Content
		}

		messages = append(messages, msg)
	}

	return messages, existingSummary, nil
}

// convertMediaParts converte os mediaParts do banco para o formato multimodal do LLM.
func (l *MediaHistoryLoader) convertMediaParts(ctx context.Context, mediaParts []map[string]interface{}, textContent string) []interface{} {
	var content []interface{}

	// Se já existe transcrição de áudio no Content, inclui como texto inicial
	// e pula qualquer parte de áudio do media (evita duplicação)
	hasTextContent := textContent != ""
	if hasTextContent {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": textContent,
		})
	}

	for _, mp := range mediaParts {
		mediaType, _ := mp["type"].(string)
		data, _ := mp["data"].(string)
		name, _ := mp["name"].(string)

		switch {
		case strings.HasPrefix(mediaType, "image/"):
			content = append(content, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
				},
			})

		case strings.HasPrefix(mediaType, "audio/"):
			// Se já temos transcrição no Content, não re-transcreve o áudio
			if hasTextContent {
				logging.Infof(ctx, "chat.media", "[Media] Áudio ignorado no histórico — já temos transcrição no content")
				continue
			}
			content = append(content, l.convertAudioPart(ctx, data, mediaType)...)

		case mediaType == "application/pdf" || strings.HasPrefix(mediaType, "text/"):
			content = append(content, map[string]interface{}{
				"type": "file",
				"file": map[string]interface{}{
					"filename":  name,
					"data":      data,
					"mime_type": mediaType,
				},
			})

		case strings.HasPrefix(mediaType, "video/"):
			content = append(content, map[string]interface{}{
				"type": "video",
				"video": map[string]interface{}{
					"data":      data,
					"mime_type": mediaType,
				},
			})

		default:
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("[Arquivo anexado: %s (%s)]", name, mediaType),
			})
		}
	}

	return content
}

// convertAudioPart converte um áudio para formato LLM, transcrevendo via Whisper se necessário.
func (l *MediaHistoryLoader) convertAudioPart(ctx context.Context, data, mediaType string) []interface{} {
	audioFmt := strings.TrimPrefix(mediaType, "audio/")

	if SupportedAudioFormats[audioFmt] {
		return []interface{}{map[string]interface{}{
			"type": "input_audio",
			"input_audio": map[string]interface{}{
				"data":   data,
				"format": audioFmt,
			},
		}}
	}

	// Formato não suportado: tenta transcrever com Whisper
	if l.Transcribe != nil {
		filename := WhisperFilename(audioFmt)
		logging.Infof(ctx, "chat.media", "[Media] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
		text, err := l.Transcribe(ctx, data, filename)
		if err != nil {
			logging.Errorf(ctx, "chat.media", "[Media] Erro ao transcrever %s via Whisper: %v", audioFmt, err)
		} else if text != "" {
			logging.Infof(ctx, "chat.media", "[Media] Áudio %s transcrito via Whisper ao carregar histórico: %s", audioFmt, truncate(text, 100))
			return []interface{}{map[string]interface{}{
				"type": "text",
				"text": text,
			}}
		}
	}

	// NUNCA enviar formato não suportado como input_audio — placeholder textual
	logging.Infof(ctx, "chat.media", "[Media] Áudio %s não transcrito — adicionando placeholder textual", audioFmt)
	return []interface{}{map[string]interface{}{
		"type": "text",
		"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
	}}
}

// PreprocessMessages percorre as mensagens e:
//   - Converte formatos de áudio não suportados (aac, ogg, webm, etc.) para texto via Whisper
//   - Se audioSupported é false, transcreve todo áudio com Whisper
//   - Se docSupported é false, converte documentos em texto placeholder
func PreprocessMessages(ctx context.Context, messages []llm.Message, transcribe TranscribeFunc, audioSupported *bool, docSupported *bool) []llm.Message {
	for i, msg := range messages {
		content, ok := msg.Content.([]interface{})
		if !ok {
			continue
		}

		var newContent []interface{}
		for _, part := range content {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				newContent = append(newContent, part)
				continue
			}

			partType, _ := partMap["type"].(string)

			if partType == "input_audio" {
				audioMap, _ := partMap["input_audio"].(map[string]interface{})
				if audioMap != nil {
					audioData, _ := audioMap["data"].(string)
					audioFmt, _ := audioMap["format"].(string)

					needsWhisper := !SupportedAudioFormats[audioFmt]
					if audioSupported != nil && !*audioSupported {
						needsWhisper = true
					}

					if needsWhisper {
						transcribed := false
						if audioData != "" && transcribe != nil {
							filename := WhisperFilename(audioFmt)
							logging.Infof(ctx, "chat.media", "[Preprocess] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
							text, err := transcribe(ctx, audioData, filename)
							if err != nil {
								logging.Errorf(ctx, "chat.media", "[Preprocess] Erro ao transcrever áudio %s: %v", audioFmt, err)
							} else if text != "" {
								logging.Infof(ctx, "chat.media", "[Preprocess] Áudio %s transcrito via Whisper: %s", audioFmt, truncate(text, 100))
								newContent = append(newContent, map[string]interface{}{
									"type": "text",
									"text": text,
								})
								transcribed = true
							}
						}
						if !transcribed {
							// NUNCA enviar formato não suportado — placeholder textual
							logging.Infof(ctx, "chat.media", "[Preprocess] Áudio %s não transcrito — placeholder textual", audioFmt)
							newContent = append(newContent, map[string]interface{}{
								"type": "text",
								"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
							})
						}
						continue
					}
				}
			}

			if partType == "file" && docSupported != nil && !*docSupported {
				fileMap, _ := partMap["file"].(map[string]interface{})
				if fileMap != nil {
					fname, _ := fileMap["filename"].(string)
					mime, _ := fileMap["mime_type"].(string)
					newContent = append(newContent, map[string]interface{}{
						"type": "text",
						"text": fmt.Sprintf("[Documento anexado: %s (%s) — modelo não suporta documentos nativamente]", fname, mime),
					})
					continue
				}
			}

			newContent = append(newContent, part)
		}
		messages[i].Content = newContent
	}

	return messages
}

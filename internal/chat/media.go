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

// NormalizeAudioMIME remove parâmetros opcionais (ex.: codecs) e normaliza o
// tipo antes de derivar extensões ou conteúdo persistido.
func NormalizeAudioMIME(mimeType string) string {
	base, _, _ := strings.Cut(mimeType, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

// AudioTranscriptionFallback devolve conteúdo persistível no idioma de STT do
// perfil. Inglês é o fallback quando o idioma está vazio ou não é suportado.
func AudioTranscriptionFallback(language, mimeType string) string {
	format := strings.TrimPrefix(NormalizeAudioMIME(mimeType), "audio/")
	switch strings.ToLower(strings.SplitN(strings.ReplaceAll(language, "_", "-"), "-", 2)[0]) {
	case "pt":
		return fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", format)
	case "es":
		return fmt.Sprintf("[Mensaje de audio recibido (%s) — no se pudo transcribir]", format)
	default:
		return fmt.Sprintf("[Audio message received (%s) — could not be transcribed]", format)
	}
}

// AudioSTTNotConfiguredFallback explica, no idioma de STT do perfil, por que
// um canal externo não conseguiu processar a mensagem de voz.
func AudioSTTNotConfiguredFallback(language string) string {
	switch strings.ToLower(strings.SplitN(strings.ReplaceAll(language, "_", "-"), "-", 2)[0]) {
	case "pt":
		return "[Mensagem de áudio recebida, mas a transcrição automática não está configurada. Configure o Whisper no perfil deste canal para processar mensagens de voz.]"
	case "es":
		return "[Mensaje de audio recibido, pero la transcripción automática no está configurada. Configura Whisper en el perfil de este canal para procesar mensajes de voz.]"
	default:
		return "[Audio message received, but automatic transcription is not configured. Configure Whisper in this channel profile to process voice messages.]"
	}
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

// TranscribeFunc abstrai a transcrição única do áudio do turno sem acoplar este
// pacote ao internal/speech. Histórico e pré-processamento nunca a invocam.
type TranscribeFunc func(ctx context.Context, audioBase64, filename string) (string, error)

// MediaHistoryLoader carrega o histórico de conversa convertendo mídias para o formato LLM.
type MediaHistoryLoader struct {
	Repo    MessageRepository
	MaxMsgs int
}

// Load retorna as mensagens formatadas para o LLM e o resumo existente da conversa.
func (l *MediaHistoryLoader) Load(ctx context.Context, conversationID string) ([]llm.Message, string, error) {
	h := HistoryLoader{Repo: l.Repo, MaxMsgs: l.MaxMsgs}
	dbMessages, existingSummary, err := h.Load(ctx, conversationID)
	if err != nil {
		return nil, "", err
	}
	return l.format(ctx, dbMessages, existingSummary)
}

// LoadWindow reutiliza o mesmo HistoryLoader e a mesma conversão de mídia sobre
// uma janela já carregada pela transação de persistência.
func (l *MediaHistoryLoader) LoadWindow(ctx context.Context, conversationID string, window *HistoryWindow) ([]llm.Message, string, error) {
	if window == nil {
		return nil, "", fmt.Errorf("janela de histórico indisponível")
	}
	summary := window.Summary
	if window.SummaryUpToMessageID != "" && !window.SummaryBoundaryAvailable {
		summary = ""
	}
	h := HistoryLoader{Repo: l.Repo, MaxMsgs: l.MaxMsgs}
	dbMessages, summary, err := h.filter(ctx, conversationID, window.Messages, summary)
	if err != nil {
		return nil, "", err
	}
	return l.format(ctx, dbMessages, summary)
}

func (l *MediaHistoryLoader) format(ctx context.Context, dbMessages []Message, existingSummary string) ([]llm.Message, string, error) {
	messages := make([]llm.Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		// Otimização de contexto: omitir mensagens intermediárias de tool calling
		// de turnos anteriores. O modelo já processou esses resultados e produziu
		// uma resposta final com a informação sintetizada — reenviar a cadeia
		// tool_call→tool_result desperdiça tokens sem valor.
		if m.Role == "tool" {
			continue
		}
		if m.Role == "assistant" && strings.TrimSpace(m.ToolCalls) != "" && strings.TrimSpace(m.Content) == "" {
			// Tool calling de turnos anteriores não é reenviado. Reasoning
			// persistido também não vira extensão de protocolo (AEP-0097), então
			// mantê-lo aqui produziria uma assistant vazia no payload.
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
			content = append(content, convertPersistedAudioPart(ctx, data, mediaType)...)

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

// convertPersistedAudioPart converte áudio histórico sem executar I/O. Áudio
// incompatível só pode chegar ao LLM como a transcrição já persistida em
// Message.Content; sem ela, usamos um placeholder determinístico.
func convertPersistedAudioPart(ctx context.Context, data, mediaType string) []interface{} {
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

	logging.Infof(ctx, "chat.media", "[Media] Áudio histórico %s sem transcrição persistida — adicionando placeholder textual", audioFmt)
	return []interface{}{map[string]interface{}{
		"type": "text",
		"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
	}}
}

// PreprocessMessages percorre as mensagens sem I/O e:
//   - impede formatos de áudio não suportados de chegar ao provider;
//   - usa placeholder se o provider não suporta áudio e não há transcrição persistida;
//   - Se docSupported é false, converte documentos em texto placeholder
func PreprocessMessages(ctx context.Context, messages []llm.Message, audioSupported *bool, docSupported *bool) []llm.Message {
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
					audioFmt, _ := audioMap["format"].(string)

					needsWhisper := !SupportedAudioFormats[audioFmt]
					if audioSupported != nil && !*audioSupported {
						needsWhisper = true
					}

					if needsWhisper {
						logging.Infof(ctx, "chat.media", "[Preprocess] Áudio %s sem transcrição persistida — placeholder textual", audioFmt)
						newContent = append(newContent, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
						})
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

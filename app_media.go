package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"assistente/internal/profiles"
)

// extractAudioFromMedia extrai o primeiro áudio base64 e MIME do mediaJSON.
// Retorna ("", "") se não houver áudio.
func extractAudioFromMedia(mediaJSON string) (string, string) {
	var mediaParts []map[string]interface{}
	if err := json.Unmarshal([]byte(mediaJSON), &mediaParts); err != nil {
		return "", ""
	}
	for _, mp := range mediaParts {
		mediaType, _ := mp["type"].(string)
		data, _ := mp["data"].(string)
		if strings.HasPrefix(mediaType, "audio/") && data != "" {
			return data, mediaType
		}
	}
	return "", ""
}

// transcribeAudioFromMedia extrai áudio do mediaJSON e transcreve via Whisper.
// Retorna o texto transcrito, ou "" se não houver áudio ou falhar.
func (a *App) transcribeAudioFromMedia(mediaJSON string) string {
	var mediaParts []map[string]interface{}
	if err := json.Unmarshal([]byte(mediaJSON), &mediaParts); err != nil {
		return ""
	}

	var transcriptions []string
	for _, mp := range mediaParts {
		mediaType, _ := mp["type"].(string)
		data, _ := mp["data"].(string)

		if !strings.HasPrefix(mediaType, "audio/") || data == "" {
			continue
		}

		audioFmt := strings.TrimPrefix(mediaType, "audio/")
		if !a.ensureSpeechManager() {
			log.Printf("[Transcribe] speechManager indisponível para transcrever áudio %s", audioFmt)
			continue
		}

		filename := whisperFilename(audioFmt)
		log.Printf("[Transcribe] Transcrevendo áudio %s antes de salvar (filename=%s)", audioFmt, filename)
		result, err := a.speechManager.Transcribe(data, filename)
		if err != nil {
			log.Printf("[Transcribe] Erro ao transcrever áudio %s: %v", audioFmt, err)
			continue
		}
		if result.Text != "" {
			log.Printf("[Transcribe] Áudio transcrito: %s", truncateStr(result.Text, 100))
			transcriptions = append(transcriptions, result.Text)
		}
	}

	return strings.Join(transcriptions, "\n")
}

// Formatos de áudio suportados nativamente pela API OpenAI input_audio.
var supportedAudioFormats = map[string]bool{
	"wav": true, "mp3": true,
}

// whisperExtensionMap mapeia extensões/formatos para extensões aceitas pelo Whisper.
// Whisper aceita: flac, m4a, mp3, mp4, mpeg, mpga, oga, ogg, wav, webm.
// AAC é o codec dentro de M4A, então mapeamos aac → m4a.
var whisperExtensionMap = map[string]string{
	"aac":  "m4a",
	"opus": "ogg",
}

// whisperFilename retorna um nome de arquivo com extensão compatível com Whisper.
func whisperFilename(format string) string {
	if mapped, ok := whisperExtensionMap[format]; ok {
		return fmt.Sprintf("audio.%s", mapped)
	}
	return fmt.Sprintf("audio.%s", format)
}

// preprocessMediaMessages percorre as mensagens e:
//   - Converte formatos de áudio não suportados (aac, ogg, webm, etc.) para texto via Whisper
//   - Se MediaSupport.Audio = false, transcreve todo áudio com Whisper
//   - Se MediaSupport.Document = false, converte documentos em texto placeholder
func (a *App) preprocessMediaMessages(messages []Message, profile *profiles.Profile) []Message {
	var ms *profiles.MediaSupport
	if profile != nil {
		ms = profile.MediaSupport
	}

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

			// Áudio: verifica formato e suporte do modelo
			if partType == "input_audio" {
				audioMap, _ := partMap["input_audio"].(map[string]interface{})
				if audioMap != nil {
					audioData, _ := audioMap["data"].(string)
					audioFmt, _ := audioMap["format"].(string)

					// Decide se precisa transcrever com Whisper:
					// 1. Formato não suportado pela API (aac, ogg, webm, m4a, etc.)
					// 2. Perfil explicitamente marca que modelo não suporta áudio
					needsWhisper := !supportedAudioFormats[audioFmt]
					if ms != nil && ms.Audio != nil && !*ms.Audio {
						needsWhisper = true
					}

					if needsWhisper {
						transcribed := false
						if audioData != "" && a.ensureSpeechManager() {
							filename := whisperFilename(audioFmt)
							log.Printf("[Preprocess] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
							result, err := a.speechManager.Transcribe(audioData, filename)
							if err == nil && result.Text != "" {
								log.Printf("[Preprocess] Áudio %s transcrito via Whisper: %s", audioFmt, truncateStr(result.Text, 100))
								newContent = append(newContent, map[string]interface{}{
									"type": "text",
									"text": result.Text,
								})
								transcribed = true
							} else if err != nil {
								log.Printf("[Preprocess] Erro ao transcrever áudio %s: %v", audioFmt, err)
							}
						}
						if !transcribed {
							// NUNCA enviar formato não suportado — placeholder textual
							log.Printf("[Preprocess] Áudio %s não transcrito — placeholder textual", audioFmt)
							newContent = append(newContent, map[string]interface{}{
								"type": "text",
								"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
							})
						}
						continue
					}
				}
			}

			// Documento: verifica suporte do modelo
			if partType == "file" && ms != nil && ms.Document != nil && !*ms.Document {
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

// UpdateProfileMediaSupport atualiza o MediaSupport de um perfil e salva.
// Chamado quando detectamos que um modelo não suporta determinado tipo de mídia.
func (a *App) UpdateProfileMediaSupport(mediaType string, supported bool) {
	if a.profileManager == nil {
		return
	}

	profile, err := a.profileManager.GetActive()
	if err != nil || profile == nil {
		return
	}

	if profile.MediaSupport == nil {
		profile.MediaSupport = &profiles.MediaSupport{}
	}

	switch mediaType {
	case "audio":
		profile.MediaSupport.Audio = &supported
	case "image":
		profile.MediaSupport.Image = &supported
	case "document":
		profile.MediaSupport.Document = &supported
	case "video":
		profile.MediaSupport.Video = &supported
	}

	slug := a.profileManager.GetActiveSlug()
	if slug == "" {
		return
	}
	if err := a.profileManager.Update(slug, profile); err != nil {
		log.Printf("[MediaSupport] Erro ao salvar perfil: %v", err)
	} else {
		log.Printf("[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
	}
}

// truncateStr encurta uma string para exibição em logs.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

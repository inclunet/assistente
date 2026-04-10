package main

import (
	"encoding/base64"
	"log"
	"strings"

	"assistente/internal/channels"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/speech"
)

// onResponseSaved é o callback invocado pelo agentSvc e appStreamHandler após
// salvar a resposta do assistente no DB. Resolve o perfil da conversa, decide a
// estratégia de TTS e emite o evento tts:ready para que consumidores (frontend,
// SIP pipeline) reproduzam a fala sem duplicação.
func (a *App) onResponseSaved(conversationID, messageID uint, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}

	profile := a.resolveConversationProfile(conversationID)
	if profile == nil {
		return
	}

	voiceCfg := profile.Voice.Assistant
	if !voiceCfg.Enabled || voiceCfg.Provider == "" || voiceCfg.Provider == "disabled" {
		return
	}

	// Decide estratégia: providers locais (webspeech, sapi5) não geram áudio no backend.
	// Backend TTS gera e salva no DB para cache.
	switch voiceCfg.Provider {
	case "webspeech":
		a.emitter.Emit("tts:ready", map[string]interface{}{
			"messageId":      messageID,
			"conversationId": conversationID,
			"text":           stripMarkdownForTTS(text),
			"strategy":       "webspeech",
			"webspeech": map[string]interface{}{
				"voice":  voiceCfg.VoiceID,
				"rate":   voiceCfg.Rate,
				"pitch":  voiceCfg.Pitch,
				"volume": voiceCfg.Volume,
			},
		})

	case "sapi5":
		a.emitter.Emit("tts:ready", map[string]interface{}{
			"messageId":      messageID,
			"conversationId": conversationID,
			"text":           stripMarkdownForTTS(text),
			"strategy":       "sapi5",
			"sapi5": map[string]interface{}{
				"voice":  voiceCfg.VoiceID,
				"rate":   voiceCfg.Rate,
				"volume": voiceCfg.Volume,
			},
		})

	default:
		// Backend TTS (OpenAI, LocalAI, etc.): gera áudio WAV (lossless) e salva
		// no DB. WAV preserva qualidade máxima para SIP (PCM 8kHz direto) e para
		// canais que precisam de MP3, a conversão é feita no ponto de envio.
		cleanText := stripMarkdownForTTS(text)
		client := a.speechSvc.CreateTTSClient(voiceCfg.LLMProviderID, voiceCfg.Model)
		if client == nil {
			log.Printf("[TTS Proactive] Provider TTS %q não encontrado para msg %d", voiceCfg.LLMProviderID, messageID)
			return
		}
		client.SetVoice(speech.TTSVoice(voiceCfg.VoiceID))
		if voiceCfg.Rate >= 0.25 {
			client.SetSpeed(voiceCfg.Rate)
		}
		client.SetFormat(speech.FormatWAV)

		audioData, err := client.Synthesize(cleanText)
		if err != nil {
			log.Printf("[TTS Proactive] Erro ao gerar WAV para msg %d: %v", messageID, err)
			return
		}

		audioBase64 := base64.StdEncoding.EncodeToString(audioData)
		if saveErr := a.speechSvc.GetAudioRepo().SaveMessageAudio(messageID, audioBase64, "audio/wav"); saveErr != nil {
			log.Printf("[TTS Proactive] Erro ao salvar WAV no DB para msg %d: %v", messageID, saveErr)
			return
		}

		log.Printf("[TTS Proactive] WAV gerado e salvo para msg %d (%d bytes, provider=%s, voice=%s)",
			messageID, len(audioData), voiceCfg.LLMProviderID, voiceCfg.VoiceID)

		a.emitter.Emit("tts:ready", map[string]interface{}{
			"messageId":      messageID,
			"conversationId": conversationID,
			"strategy":       "backend",
		})
	}
}

// resolveConversationProfile resolve o perfil de voz para uma conversa.
// Prioridade: perfil do canal > perfil ativo global.
func (a *App) resolveConversationProfile(conversationID uint) *profiles.Profile {
	// Tenta resolver perfil pelo canal da conversa
	conv, err := database.GetConversationInfo(conversationID)
	if err == nil && conv.Channel != "" {
		if chCfg, _ := channels.Load(conv.Channel); chCfg != nil && chCfg.Profile != "" {
			if p, err := a.profileManager.Get(chCfg.Profile); err == nil {
				return p
			}
		}
	}

	// Fallback: perfil ativo global
	if p, err := a.profileManager.GetActive(); err == nil {
		return p
	}

	return nil
}

// stripMarkdownForTTS remove formatação markdown básica para TTS mais natural.
func stripMarkdownForTTS(text string) string {
	r := strings.NewReplacer(
		"**", "",
		"__", "",
		"*", "",
		"_", "",
		"`", "",
		"```", "",
		"# ", "",
		"## ", "",
		"### ", "",
		"#### ", "",
		"- ", "",
		"> ", "",
	)
	return strings.TrimSpace(r.Replace(text))
}

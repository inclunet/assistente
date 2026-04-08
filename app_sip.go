package main

import (
	"fmt"
	"log"

	"assistente/internal/channels"
	"assistente/internal/database"
	"assistente/internal/messaging/sip"
)

// cancelStreamingForContact cancela o streaming LLM em andamento para uma conversa
// de um contato em um canal. Usado pelo SIP adapter durante barge-in.
func (a *App) cancelStreamingForContact(channel, contactID string) {
	conv, _, err := database.FindOrCreateChannelConversation(channel, contactID, "")
	if err != nil || conv == nil {
		return
	}
	a.CancelStreamingForConversation(conv.ID)
}

// injectSIPSpeechManager cria e injeta um SpeechManager no SIP adapter.
// Usa o perfil do canal SIP quando configurado; caso contrário, usa o speechManager global.
func (a *App) injectSIPSpeechManager() {
	sipAdapter, err := a.getSIPAdapter()
	if err != nil {
		return // SIP não configurado — nada a fazer
	}

	// Tenta carregar o perfil do canal SIP
	if chCfg, _ := channels.Load("sip"); chCfg != nil && chCfg.Profile != "" {
		if p, err := a.profileManager.Get(chCfg.Profile); err == nil {
			pCopy := *p

			// SIP é server-side: webspeech não funciona, força whisper
			if pCopy.Input.STTProvider == "webspeech" || pCopy.Input.STTProvider == "" {
				pCopy.Input.STTProvider = "whisper"
			}

			// Whisper é API OpenAI — sempre usar o LLMProviderID do assistant voice
			// (o Input.LLMProviderID pode apontar para Google ou outro provider sem Whisper)
			isWhisper := pCopy.Input.STTProvider == "whisper" || pCopy.Input.STTProvider == "whisper_api"
			if isWhisper && pCopy.Voice.Assistant.LLMProviderID != "" {
				pCopy.Input.LLMProviderID = pCopy.Voice.Assistant.LLMProviderID
				log.Printf("[Speech] SIP override: STT=%s, LLMProviderID=%s (do assistant voice)",
					pCopy.Input.STTProvider, pCopy.Input.LLMProviderID)
			}

			p = &pCopy
			sm := a.createSpeechManagerForProfile(p)
			if sm != nil {
				sipAdapter.SetSpeechManager(sm)
				if p.Voice.Assistant.VoiceID != "" {
					sipAdapter.SetVoiceID(p.Voice.Assistant.VoiceID)
				}
				log.Printf("[Speech] SpeechManager do perfil '%s' injetado no SIP adapter", chCfg.Profile)
				return
			}
		} else {
			log.Printf("[Speech] Perfil '%s' do canal SIP não encontrado: %v", chCfg.Profile, err)
		}
	}

	// Fallback: usa o speechManager global
	if a.speechManager != nil {
		sipAdapter.SetSpeechManager(a.speechManager)
		log.Printf("[Speech] SpeechManager global injetado no SIP adapter (fallback)")
	}
}

// SIPCall inicia uma chamada SIP de saída para o número/ramal especificado.
// O número pode ser um ramal ("200"), user@host ("200@pbx"), ou SIP URI.
// Requer que o canal SIP esteja conectado.
func (a *App) SIPCall(number string) (sip.CallInfo, error) {
	if number == "" {
		return sip.CallInfo{}, fmt.Errorf("número não pode ser vazio")
	}

	adapter, err := a.getSIPAdapter()
	if err != nil {
		return sip.CallInfo{}, err
	}

	call, err := adapter.Dial(a.ctx, number)
	if err != nil {
		return sip.CallInfo{}, err
	}

	return sip.CallInfo{
		ID:        call.ID,
		CallerID:  call.CallerID,
		State:     string(call.GetState()),
		Duration:  "0s",
		StartedAt: call.StartedAt,
	}, nil
}

// SIPHangup encerra uma chamada SIP ativa pelo seu ID.
func (a *App) SIPHangup(callID string) error {
	adapter, err := a.getSIPAdapter()
	if err != nil {
		return err
	}
	return adapter.HangupCall(a.ctx, callID)
}

// SIPActiveCalls retorna as chamadas SIP ativas.
func (a *App) SIPActiveCalls() []sip.CallInfo {
	adapter, err := a.getSIPAdapter()
	if err != nil {
		return nil
	}
	return adapter.ActiveCalls()
}

// getSIPAdapter retorna o SIPAdapter do gateway, se conectado.
func (a *App) getSIPAdapter() (*sip.SIPAdapter, error) {
	if a.msgGateway == nil {
		return nil, fmt.Errorf("sip: gateway não inicializado")
	}
	m, ok := a.msgGateway.GetMessenger("sip")
	if !ok {
		return nil, fmt.Errorf("sip: canal SIP não configurado")
	}
	adapter, ok := m.(*sip.SIPAdapter)
	if !ok {
		return nil, fmt.Errorf("sip: adapter inválido")
	}
	return adapter, nil
}

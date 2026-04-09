package main

import (
	"fmt"
	"log"

	"assistente/internal/channels"
	"assistente/internal/database"
	"assistente/internal/messaging/sip"
	"assistente/internal/speech"
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

// injectSIPSpeechManager cria e injeta um SpeechManager no SIP adapter existente.
// Chamado após initMessaging na startup.
func (a *App) injectSIPSpeechManager() {
	sipAdapter, err := a.getSIPAdapter()
	if err != nil {
		return // SIP não configurado — nada a fazer
	}

	sm, voiceID := a.createSIPSpeechManager()
	if sm != nil {
		sipAdapter.SetSpeechManager(sm)
		if voiceID != "" {
			sipAdapter.SetVoiceID(voiceID)
		}
	}
}

// createSIPSpeechManager cria um SpeechManager adequado para o canal SIP.
// SIP é server-side, então webspeech não funciona — força whisper quando necessário.
// Retorna (speechManager, voiceID). Se não conseguir criar, retorna (nil, "").
func (a *App) createSIPSpeechManager() (*speech.SpeechManager, string) {
	log.Printf("[SIP-Speech] Criando SpeechManager para canal SIP...")

	// Tenta carregar o perfil do canal SIP
	chCfg, chErr := channels.Load("sip")
	if chErr != nil {
		log.Printf("[SIP-Speech] Erro ao carregar config do canal SIP: %v", chErr)
	}

	if chCfg != nil && chCfg.Profile != "" {
		log.Printf("[SIP-Speech] Canal SIP tem perfil configurado: '%s'", chCfg.Profile)
		if p, err := a.profileManager.Get(chCfg.Profile); err == nil {
			pCopy := *p
			log.Printf("[SIP-Speech] Perfil carregado: STTProvider='%s', Input.LLMProviderID='%s', Voice.Assistant.LLMProviderID='%s'",
				pCopy.Input.STTProvider, pCopy.Input.LLMProviderID, pCopy.Voice.Assistant.LLMProviderID)

			// SIP é server-side: webspeech não funciona, força whisper
			if pCopy.Input.STTProvider == "webspeech" || pCopy.Input.STTProvider == "" {
				pCopy.Input.STTProvider = "whisper"
				log.Printf("[SIP-Speech] STTProvider forçado para 'whisper' (server-side)")
			}

			// Whisper sem idioma → força "pt" para evitar alucinações ("[Music]" etc.)
			if pCopy.Input.Language == "" {
				pCopy.Input.Language = "pt"
				log.Printf("[SIP-Speech] Language forçado para 'pt' (idioma padrão SIP)")
			}

			// Whisper é API OpenAI — sempre usar o LLMProviderID do assistant voice
			// (o Input.LLMProviderID pode apontar para Google ou outro provider sem Whisper)
			isWhisper := pCopy.Input.STTProvider == "whisper" || pCopy.Input.STTProvider == "whisper_api"
			if isWhisper && pCopy.Voice.Assistant.LLMProviderID != "" {
				pCopy.Input.LLMProviderID = pCopy.Voice.Assistant.LLMProviderID
				log.Printf("[SIP-Speech] SIP override: STT=%s, LLMProviderID=%s (do assistant voice)",
					pCopy.Input.STTProvider, pCopy.Input.LLMProviderID)
			}

			sm := a.speechSvc.CreateManagerForProfile(&pCopy)
			if sm != nil {
				log.Printf("[SIP-Speech] SpeechManager do perfil '%s' criado com sucesso", chCfg.Profile)
				return sm, pCopy.Voice.Assistant.VoiceID
			}
			log.Printf("[SIP-Speech] FALHA ao criar SpeechManager do perfil '%s'", chCfg.Profile)
		} else {
			log.Printf("[SIP-Speech] Perfil '%s' do canal SIP não encontrado: %v", chCfg.Profile, err)
		}
	} else {
		log.Printf("[SIP-Speech] Canal SIP sem perfil configurado (chCfg=%v), tentando fallback global...", chCfg != nil)
	}

	// Fallback: tenta inicializar o speechManager global (pode ser nil na startup)
	a.speechSvc.EnsureSpeechManager()
	if sm := a.speechSvc.GetSpeechManager(); sm != nil {
		log.Printf("[SIP-Speech] SpeechManager global usado para SIP (fallback)")
		return sm, ""
	}

	log.Printf("[SIP-Speech] ERRO: Nenhum SpeechManager disponível para SIP — STT não funcionará!")
	return nil, ""
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

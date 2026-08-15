package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/llm"
	"assistente/internal/speech"
	"context"
	"sync"
)

// SpeechDispatcher resolve DispatchSpeech via helpers do *App (perfil, strip,
// emit chat:speak). Implementado pelo App via adapter privado — não entra no Bind.
type SpeechDispatcher interface {
	DispatchSpeech(req apidto.ChatSpeakRequest) error
}

// Speech é o bind Wails do domínio speech / TTS/STT (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Helpers lowercase (dispatchSpeechEvent, resolveSpeechProfile, …) permanecem no *App.
type Speech struct {
	mu         sync.RWMutex
	session    Session
	ctrl       *controllers.SpeechController
	dispatcher SpeechDispatcher
}

// NewSpeech cria o bind vazio; AttachSpeech preenche deps no startup.
func NewSpeech() *Speech {
	return &Speech{}
}

// AttachSpeech associa Session, controller e dispatcher após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachSpeech(api *Speech, session Session, ctrl *controllers.SpeechController, dispatcher SpeechDispatcher) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
	api.dispatcher = dispatcher
}

func (api *Speech) deps() (Session, *controllers.SpeechController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrSpeechNotWired
	}
	return api.session, api.ctrl, nil
}

func (api *Speech) dispatchDeps() (Session, SpeechDispatcher, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.dispatcher == nil {
		return nil, nil, ErrSpeechNotWired
	}
	return api.session, api.dispatcher, nil
}

func synthesisResultInfo(r *speech.SynthesisResult) *apidto.SynthesisResultInfo {
	if r == nil {
		return nil
	}
	return &apidto.SynthesisResultInfo{
		AudioBase64: r.AudioBase64,
		Format:      r.Format,
		Provider:    r.Provider,
	}
}

// InitSpeechManagerFromProfile inicializa o speech manager a partir do perfil ativo.
func (api *Speech) InitSpeechManagerFromProfile() error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.InitFromProfile(ctx)
	})
	return err
}

// TranscribeWhisper transcreve áudio via Whisper/STT.
func (api *Speech) TranscribeWhisper(audioBase64, filename string) (*speech.TranscriptionResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*speech.TranscriptionResult, error) {
		return ctrl.Transcribe(ctx, audioBase64, filename)
	})
}

// SynthesizeOpenAI sintetiza fala via TTS OpenAI (voz padrão do manager).
func (api *Speech) SynthesizeOpenAI(text string) (*apidto.SynthesisResultInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.SynthesisResultInfo, error) {
		r, err := ctrl.Synthesize(ctx, text)
		if err != nil {
			return nil, err
		}
		return synthesisResultInfo(r), nil
	})
}

// SynthesizeOpenAIWithVoice sintetiza fala com voz explícita.
func (api *Speech) SynthesizeOpenAIWithVoice(text, voice string) (*apidto.SynthesisResultInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.SynthesisResultInfo, error) {
		r, err := ctrl.SynthesizeWithVoice(ctx, text, voice)
		if err != nil {
			return nil, err
		}
		return synthesisResultInfo(r), nil
	})
}

// SynthesizeOpenAIStream inicia síntese TTS em streaming.
func (api *Speech) SynthesizeOpenAIStream(text, voice, sessionID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SynthesizeStream(ctx, text, voice, sessionID)
	})
	return err
}

// GetOpenAITTSVoices lista vozes TTS OpenAI disponíveis.
func (api *Speech) GetOpenAITTSVoices() ([]speech.TTSVoiceInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]speech.TTSVoiceInfo, error) {
		return ctrl.GetAvailableVoices(), nil
	})
}

// SetOpenAITTSVoice define a voz TTS OpenAI do manager.
func (api *Speech) SetOpenAITTSVoice(voice string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		ctrl.SetTTSVoice(voice)
		return struct{}{}, nil
	})
	return err
}

// SetOpenAITTSSpeed define a velocidade TTS OpenAI do manager.
func (api *Speech) SetOpenAITTSSpeed(rate int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		ctrl.SetTTSSpeed(rate)
		return struct{}{}, nil
	})
	return err
}

// GetMessageAudio retorna áudio persistido de uma mensagem.
func (api *Speech) GetMessageAudio(messageID string) (*speech.AudioResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*speech.AudioResult, error) {
		return ctrl.GetMessageAudio(ctx, messageID)
	})
}

// SaveMessageAudio persiste áudio de uma mensagem.
func (api *Speech) SaveMessageAudio(messageID string, audioBase64, mimeType string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveMessageAudio(ctx, messageID, audioBase64, mimeType)
	})
	return err
}

// GenerateAndSaveMessageAudio gera TTS e persiste o áudio da mensagem.
func (api *Speech) GenerateAndSaveMessageAudio(messageID string, text string) (*speech.AudioResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*speech.AudioResult, error) {
		return ctrl.GenerateAndSaveMessageAudio(ctx, messageID, text)
	})
}

// SpeakMessage sintetiza (ou recupera do cache) o áudio de uma mensagem.
func (api *Speech) SpeakMessage(messageID string, providerID, model, voiceID string, rate float64, language string) (*speech.AudioResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*speech.AudioResult, error) {
		return ctrl.SpeakMessage(ctx, messageID, providerID, model, voiceID, rate, language)
	})
}

// GetSpeechProviders lista provedores LLM usáveis para TTS/STT.
func (api *Speech) GetSpeechProviders() ([]*llm.ProviderConfig, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]*llm.ProviderConfig, error) {
		return ctrl.GetSpeechProviders(), nil
	})
}

// GetTTSModels lista modelos TTS de um provedor.
func (api *Speech) GetTTSModels(providerID string) ([]speech.TTSModelInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]speech.TTSModelInfo, error) {
		return ctrl.GetTTSModels(ctx, providerID), nil
	})
}

// GetTTSVoices lista vozes TTS de um provedor/modelo.
func (api *Speech) GetTTSVoices(providerID, modelID string) ([]speech.TTSVoiceInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]speech.TTSVoiceInfo, error) {
		return ctrl.GetTTSVoices(ctx, providerID, modelID), nil
	})
}

// GetSTTModels lista modelos STT de um provedor.
func (api *Speech) GetSTTModels(providerID string) ([]speech.SpeechModelInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]speech.SpeechModelInfo, error) {
		return ctrl.GetSTTModels(ctx, providerID), nil
	})
}

// SpeakPreview reproduz preview de voz (SAPI5/LLM) sem persistir.
func (api *Speech) SpeakPreview(providerID, model, voiceID string, rate, volume float64, language, text, sessionID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SpeakPreview(ctx, providerID, model, voiceID, rate, volume, language, text, sessionID)
	})
	return err
}

// DispatchSpeech resolve perfil/estratégia e emite chat:speak (helpers no *App).
func (api *Speech) DispatchSpeech(req apidto.ChatSpeakRequest) error {
	session, dispatcher, err := api.dispatchDeps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, dispatcher.DispatchSpeech(req)
	})
	return err
}

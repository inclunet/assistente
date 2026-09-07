package speech

import (
	"assistente/internal/logging"
	"context"

	"assistente/internal/credentials"
	"assistente/internal/profiles"
)

// NewSpeechManagerFromProfile cria um SpeechManager configurado a partir de um Profile.
//
// O perfil deve ter os sentinelas "$default" já resolvidos para IDs reais (use
// providers.Service.ResolveProfileDefaults antes de chamar esta função).
//
// Retorna nil se p for nil.
func NewSpeechManagerFromProfile(ctx context.Context, p *profiles.Profile, registry ProviderRegistry, credMgr *credentials.Manager) *SpeechManager {
	if p == nil {
		return nil
	}

	// Cache de credenciais por providerID para evitar resolver o mesmo provider múltiplas vezes.
	type resolvedCreds struct {
		apiKey, baseURL, credPattern string
	}
	credsCache := map[string]*resolvedCreds{}

	resolveAPICreds := func(llmProviderID string) (apiKey, baseURL, credPattern string) {
		if llmProviderID == "" {
			return "", "", ""
		}
		if cached, ok := credsCache[llmProviderID]; ok {
			return cached.apiKey, cached.baseURL, cached.credPattern
		}
		cfg := registry.Get(llmProviderID)
		if cfg == nil {
			logging.Infof(ctx, "speech.profile-factory", "[Speech] Provider '%s' não encontrado no registry", llmProviderID)
			credsCache[llmProviderID] = &resolvedCreds{}
			return "", "", ""
		}
		baseURL = cfg.BaseURL
		credPattern = cfg.CredentialPattern
		if cfg.CredentialPattern != "" {
			if auth, err := credMgr.GetByPatternWithContext(ctx, cfg.CredentialPattern); err == nil && auth != nil {
				apiKey = auth.Token
			} else if err != nil {
				logging.Errorf(ctx, "speech.profile-factory", "[Speech] ERRO ao resolver credencial para pattern '%s' (provider=%s): %v",
					cfg.CredentialPattern, llmProviderID, err)
			} else {
				logging.Warnf(ctx, "speech.profile-factory", "[Speech] AVISO: credencial não encontrada para pattern '%s' (provider=%s) — TTS pode falhar",
					cfg.CredentialPattern, llmProviderID)
			}
		} else {
			logging.Infof(ctx, "speech.profile-factory", "[Speech] Provider '%s' não tem CredentialPattern configurado", llmProviderID)
		}
		credsCache[llmProviderID] = &resolvedCreds{apiKey, baseURL, credPattern}
		return apiKey, baseURL, credPattern
	}

	buildRoleConfig := func(role profiles.VoiceRoleConfig) RoleVoiceConfig {
		apiKey, baseURL, credPattern := resolveAPICreds(role.LLMProviderID)
		return RoleVoiceConfig{
			Provider:          role.Provider,
			APIKey:            apiKey,
			BaseURL:           baseURL,
			CredentialPattern: credPattern,
			Voice:             role.VoiceID,
			Model:             role.Model,
			SelectionMode:     role.SelectionMode,
			Rate:              role.Rate,
			Pitch:             role.Pitch,
			Volume:            role.Volume,
		}
	}

	assistantCfg := buildRoleConfig(p.Voice.Assistant)

	if p.Voice.Assistant.Provider == "openai" && assistantCfg.APIKey == "" {
		logging.Warnf(ctx, "speech.profile-factory", "[Speech] AVISO: Provider assistant é 'openai' mas API key está vazia. "+
			"LLMProviderID='%s', Voice=%+v",
			p.Voice.Assistant.LLMProviderID,
			p.Voice.Assistant)
	}

	_, sttURL, sttCredPattern := resolveAPICreds(p.Input.LLMProviderID)

	speechCfg := SpeechConfig{
		STTProvider:          STTProvider(p.Input.STTProvider),
		STTAPIBaseURL:        sttURL,
		STTCredentialPattern: sttCredPattern,
		WhisperModel:         p.Input.STTModel,
		WhisperLanguage:      p.Input.Language,
		Assistant:            assistantCfg,
		User:                 buildRoleConfig(p.Voice.User),
		System:               buildRoleConfig(p.Voice.System),
	}

	sm := NewSpeechManager(speechCfg, credMgr)

	logging.Infof(ctx, "speech.profile-factory", "[Speech] Manager inicializado | Assistant: %s | User: %s | System: %s | STT: %s",
		p.Voice.Assistant.Provider,
		p.Voice.User.Provider,
		p.Voice.System.Provider,
		p.Input.STTProvider)

	return sm
}

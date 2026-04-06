package main

import (
	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"context"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ============================================================================
// LLM Provider API
// ============================================================================

// GetLLMProviders retorna todos os provedores LLM disponíveis
func (a *App) GetLLMProviders() []*llm.ProviderConfig {
	if a.llmRegistry == nil {
		return []*llm.ProviderConfig{}
	}
	return a.llmRegistry.List()
}

// GetLLMProvider retorna um provedor LLM pelo ID
func (a *App) GetLLMProvider(id string) *llm.ProviderConfig {
	if a.llmRegistry == nil {
		return nil
	}
	return a.llmRegistry.Get(id)
}

// GetActiveProviderInfo retorna informações sobre o provedor LLM ativo
// (baseado no perfil ativo)
func (a *App) GetActiveProviderInfo() map[string]interface{} {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		return map[string]interface{}{
			"error": "perfil ativo não encontrado",
		}
	}

	activeProfile = a.resolveProfileDefaults(activeProfile)

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		return map[string]interface{}{
			"error":      "provedor não encontrado",
			"providerID": activeProfile.Chat.LLMProvider,
		}
	}

	return map[string]interface{}{
		"id":       provider.ID,
		"name":     provider.Name,
		"type":     provider.Type,
		"base_url": provider.BaseURL,
		"model":    provider.Model,
	}
}

// TestLLMProvider testa a conexão com um provider LLM.
// Quando provider_id é informado e api_key está vazio, busca a credencial existente
// no credential manager.
func (a *App) TestLLMProvider(req TestLLMProviderRequest) (bool, error) {
	return a.providerSvc.TestConnection(a.ctx, providers.TestRequest{
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		ProviderID: req.ProviderID,
	})
}

// ListModelsRaw lista modelos de um provedor usando credenciais ad-hoc (sem exigir provider salvo).
// Usado pelo formulário de criação/edição de providers para validar e selecionar modelo.
// Se provider_id é informado e api_key está vazio, busca credencial existente.
func (a *App) ListModelsRaw(req TestLLMProviderRequest) ([]string, error) {
	if req.BaseURL == "" {
		return nil, fmt.Errorf("base_url é obrigatório")
	}

	parsedURL, err := url.Parse(req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("URL deve começar com http:// ou https://")
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL deve conter um endereço de servidor válido")
	}

	apiKey := req.APIKey

	// Se não tem API key mas tem provider_id, busca credencial existente
	if apiKey == "" && req.ProviderID != "" && a.llmRegistry != nil && a.credMgr != nil {
		if provider := a.llmRegistry.Get(req.ProviderID); provider != nil && provider.CredentialPattern != "" {
			if auth, err := a.credMgr.GetByPattern(provider.CredentialPattern); err == nil && auth.Token != "" {
				apiKey = auth.Token
			}
		}
	}

	hostname := parsedURL.Hostname()
	tempProvider := &llm.ProviderConfig{
		ID:                "temp-form",
		Name:              "temp",
		Type:              llm.ProviderType(req.Type),
		BaseURL:           req.BaseURL,
		CredentialPattern: hostname,
		Timeout:           15,
	}

	// Se tem API key ad-hoc, registra temporariamente para o credential manager achar
	if apiKey != "" && a.credMgr != nil {
		_ = a.credMgr.RegisterPatternWithContext(a.ctx, hostname, &credentials.AuthConfig{
			Type: "bearer", Token: apiKey,
		})
		defer func() {
			if req.ProviderID == "" {
				_ = a.credMgr.DeletePattern(a.ctx, hostname)
			}
		}()
	}

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()

	// Usa ChatProvider se o tipo tem api_format inferível
	cp := llm.NewChatProvider(tempProvider, a.credMgr)
	models, err := cp.GetModels(ctx)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// CreateLLMProvider cria um novo provider com auto-salvamento de credenciais.
func (a *App) CreateLLMProvider(req CreateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := a.providerSvc.Create(a.ctx, providers.CreateRequest{
		ID:           req.ID,
		Name:         req.Name,
		Type:         req.Type,
		APIFormat:    req.APIFormat,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		DefaultModel: req.DefaultModel,
	})
	if err != nil {
		return nil, err
	}
	p := res.Provider
	return map[string]interface{}{
		"id":                    p.ID,
		"name":                  p.Name,
		"type":                  string(p.Type),
		"base_url":              p.BaseURL,
		"model":                 p.Model,
		"default_model":         p.DefaultModel,
		"is_default":            p.IsDefault,
		"timeout":               p.Timeout,
		"credential_pattern":    res.CredentialPattern,
		"credential_configured": res.CredentialConfigured,
	}, nil
}

// UpdateLLMProvider atualiza um provider existente.
func (a *App) UpdateLLMProvider(id string, req UpdateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := a.providerSvc.Update(a.ctx, id, providers.UpdateRequest{
		Name:         req.Name,
		Type:         req.Type,
		APIFormat:    req.APIFormat,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		DefaultModel: req.DefaultModel,
	})
	if err != nil {
		return nil, err
	}
	p := res.Provider
	return map[string]interface{}{
		"id":                    p.ID,
		"name":                  p.Name,
		"type":                  string(p.Type),
		"base_url":              p.BaseURL,
		"model":                 p.Model,
		"default_model":         p.DefaultModel,
		"is_default":            p.IsDefault,
		"timeout":               p.Timeout,
		"credential_pattern":    p.CredentialPattern,
		"credential_configured": res.CredentialConfigured,
	}, nil
}

// SetDefaultProvider marca um provedor como o default do sistema.
func (a *App) SetDefaultProvider(id string) error {
	if err := a.providerSvc.SetDefault(id); err != nil {
		return err
	}
	a.initLLMClient()
	return nil
}

// DeleteLLMProvider remove um provider do registry.
func (a *App) DeleteLLMProvider(ctx context.Context, id string) error {
	return a.providerSvc.Delete(id)
}

// GetLLMProvidersWithStatus retorna todos os providers com status de credencial.
func (a *App) GetLLMProvidersWithStatus() []map[string]interface{} {
	statuses := a.providerSvc.ListWithStatus()
	result := make([]map[string]interface{}, 0, len(statuses))
	for _, s := range statuses {
		p := s.Provider
		result = append(result, map[string]interface{}{
			"id":                    p.ID,
			"name":                  p.Name,
			"type":                  string(p.Type),
			"api_format":            string(p.APIFormat),
			"base_url":              p.BaseURL,
			"model":                 p.Model,
			"default_model":         p.DefaultModel,
			"is_default":            p.IsDefault,
			"timeout":               p.Timeout,
			"credential_pattern":    p.CredentialPattern,
			"credential_configured": s.CredentialConfigured,
		})
	}
	return result
}

// PreviewVoiceSettings reproduz um texto de teste com configurações ad-hoc
func (a *App) PreviewVoiceSettings(provider, voiceID string, rate, pitch, volume float64, sampleText string) error {
	if sampleText == "" {
		sampleText = "Este é um teste das configurações de voz"
	}

	log.Printf("[PreviewVoiceSettings] provider=%s, voiceID=%s, rate=%.2f", provider, voiceID, rate)

	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("erro ao carregar config: %w", err)
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API key não configurada")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voiceID, "tts-1")
	}

	if provider == "openai" {
		a.speechManager.SetTTSVoice(voiceID)
	}

	result, err := a.speechManager.SynthesizeWithVoice(sampleText, voiceID)
	if err != nil {
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}

	runtime.EventsEmit(a.ctx, "voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})

	return nil
}

// saveLLMProviders persiste todos os provedores do registry no store.
func (a *App) saveLLMProviders() error {
	return a.providerSvc.Save()
}

// loadLLMProviders carrega provedores do store para o registry.
func (a *App) loadLLMProviders() error {
	return a.providerSvc.Load()
}

// ensureDefaultProvider delega ao service para garantir que há um provedor default.
func (a *App) ensureDefaultProvider() {
	a.providerSvc.EnsureDefault()
}

// ============================================================================
// LLM Client / Provider Init
// ============================================================================

// initLLMClient inicializa o cliente LLM usando o provider do perfil ativo
func (a *App) initLLMClient() {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		log.Printf("[initLLMClient] Perfil ativo não encontrado: %v", err)
		return
	}
	activeProfile = a.resolveProfileDefaults(activeProfile)

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("[initLLMClient] Provedor LLM não encontrado: %s", activeProfile.Chat.LLMProvider)
		return
	}

	log.Printf("[initLLMClient] Provedor ativo: %s (api_format=%s)", provider.Name, provider.GetAPIFormat())
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// getChatProviderForProvider retorna um ChatProvider para o provedor especificado.
func (a *App) getChatProviderForProvider(providerID string) (llm.ChatProvider, error) {
	if a.llmRegistry == nil {
		return nil, fmt.Errorf("registro de provedores não inicializado")
	}

	provider := a.llmRegistry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provedor LLM não encontrado: %s", providerID)
	}

	return llm.NewChatProvider(provider, a.credMgr), nil
}

// resolveProfileDefaults substitui sentinelas "$default" no profile pelo provedor/modelo padrão.
func (a *App) resolveProfileDefaults(p *profiles.Profile) *profiles.Profile {
	if a.providerSvc == nil {
		return p
	}
	return a.providerSvc.ResolveProfileDefaults(p)
}

// initLLMProviders inicializa o registro de provedores LLM a partir do store.
func (a *App) initLLMProviders() {
	if err := a.loadLLMProviders(); err != nil {
		count, countErr := a.providerSvc.Count()
		if countErr != nil || count == 0 {
			log.Printf("Nenhum provedor encontrado. Configure um provedor nas configurações ou crie um perfil.")
		}
	}
}

// CreateDefaultLLMProvider cria o primeiro provedor durante o wizard.
func (a *App) CreateDefaultLLMProvider(providerType, apiKey string) error {
	return a.providerSvc.CreateFromTemplate(a.ctx, providerType, apiKey)
}

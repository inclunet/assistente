package main

import (
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"context"
	"fmt"
	"log"
	"time"
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
		return map[string]interface{}{"error": "perfil ativo não encontrado"}
	}
	info := a.providerSvc.GetActiveProviderInfo(activeProfile)
	if info.Error != "" {
		return map[string]interface{}{
			"error":      info.Error,
			"providerID": activeProfile.Chat.LLMProvider,
		}
	}
	return map[string]interface{}{
		"id":       info.ID,
		"name":     info.Name,
		"type":     info.Type,
		"base_url": info.BaseURL,
		"model":    info.Model,
	}
}

// TestLLMProvider testa a conexão com um provider LLM.
// Quando provider_id é informado e api_key está vazio, busca a credencial existente
// no credential manager.
func (a *App) TestLLMProvider(req TestLLMProviderRequest) (ok bool, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("erro interno ao testar provider: %v", r)
		}
	}()

	if a.ctx == nil {
		return false, fmt.Errorf("aplicação ainda não está pronta, aguarde")
	}

	return a.providerSvc.TestConnection(a.ctx, providers.TestRequest{
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		ProviderID: req.ProviderID,
	})
}

// ListModelsRaw lista modelos de um provedor usando credenciais ad-hoc (sem exigir provider salvo).
// Usado pelo formulário de criação/edição de providers para validar e selecionar modelo.
// Se provider_id é informado e api_key está vazio, busca credencial existente.
func (a *App) ListModelsRaw(req TestLLMProviderRequest) (models []string, retErr error) {
	// Protege contra panic inesperado em camadas inferiores
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ListModelsRaw] PANIC: %v", r)
			retErr = fmt.Errorf("erro interno ao listar modelos: %v", r)
		}
	}()

	if a.ctx == nil {
		return nil, fmt.Errorf("aplicação ainda não está pronta, aguarde")
	}

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()

	return a.providerSvc.ListModelsRaw(ctx, providers.ListModelsRawRequest{
		Type:       req.Type,
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		ProviderID: req.ProviderID,
	})
}

// providerToMap serializa um ProviderConfig para o formato map[string]interface{}
// esperado pelo frontend, incluindo status de credencial.
func providerToMap(p *llm.ProviderConfig, credentialPattern string, credentialConfigured bool) map[string]interface{} {
	return map[string]interface{}{
		"id":                    p.ID,
		"name":                  p.Name,
		"type":                  string(p.Type),
		"api_format":            string(p.APIFormat),
		"base_url":              p.BaseURL,
		"model":                 p.Model,
		"default_model":         p.DefaultModel,
		"is_default":            p.IsDefault,
		"timeout":               p.Timeout,
		"credential_pattern":    credentialPattern,
		"credential_configured": credentialConfigured,
	}
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
	return providerToMap(res.Provider, res.CredentialPattern, res.CredentialConfigured), nil
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
	return providerToMap(p, p.CredentialPattern, res.CredentialConfigured), nil
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
		result = append(result, providerToMap(s.Provider, s.Provider.CredentialPattern, s.CredentialConfigured))
	}
	return result
}

// PreviewVoiceSettings reproduz um texto de teste com configurações ad-hoc
func (a *App) PreviewVoiceSettings(provider, voiceID string, rate, pitch, volume float64, sampleText string) error {
	return a.speechSvc.PreviewVoiceSettings(provider, voiceID, rate, volume, sampleText)
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

// getChatProviderForProvider é uma fina camada de delegação para providerSvc.GetChatProvider.
// Mantida para uso em testes de integração que combinam resolveProfileDefaults com routing.
func (a *App) getChatProviderForProvider(providerID string) (llm.ChatProvider, error) {
	if a.providerSvc == nil {
		return nil, fmt.Errorf("provider service not initialized")
	}
	return a.providerSvc.GetChatProvider(providerID)
}

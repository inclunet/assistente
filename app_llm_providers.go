package main

import (
	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// extractDomainPattern extrai o pattern de domínio de uma base URL
func extractHostname(baseURL string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("base_url vazio")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("base_url inválido: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", fmt.Errorf("host não encontrado no base_url")
	}

	return host, nil
}

// TestLLMProvider testa a conexão com um provider LLM.
// Quando provider_id é informado e api_key está vazio, busca a credencial existente
// no credential manager (resolve o problema de testes falharem durante edição).
func (a *App) TestLLMProvider(req TestLLMProviderRequest) (bool, error) {
	if req.BaseURL == "" {
		return false, fmt.Errorf("base_url é obrigatório")
	}

	parsedURL, err := url.Parse(req.BaseURL)
	if err != nil {
		return false, fmt.Errorf("URL inválida: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false, fmt.Errorf("URL deve começar com http:// ou https://")
	}

	if parsedURL.Host == "" {
		return false, fmt.Errorf("URL deve conter um endereço de servidor válido")
	}

	apiKey := req.APIKey

	// Se não tem API key mas tem provider_id, busca credencial existente no credManager
	if apiKey == "" && req.ProviderID != "" && a.llmRegistry != nil && a.credMgr != nil {
		if provider := a.llmRegistry.Get(req.ProviderID); provider != nil && provider.CredentialPattern != "" {
			if auth, err := a.credMgr.GetByPattern(provider.CredentialPattern); err == nil && auth.Token != "" {
				apiKey = auth.Token
				log.Printf("[TestLLMProvider] Usando credencial existente para provider '%s' (pattern: %s)", req.ProviderID, provider.CredentialPattern)
			}
		}
	}

	modelsEndpoint := strings.TrimSuffix(req.BaseURL, "/") + "/models"

	client := &http.Client{Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("erro ao conectar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, fmt.Errorf("servidor retornou erro: %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return false, fmt.Errorf("API Key inválida ou não autorizada")
	}

	if resp.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("acesso negado (403). A API Key pode não ter permissões suficientes")
	}

	return true, nil
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

// CreateLLMProvider cria um novo provider com auto-salvamento de credenciais
func (a *App) CreateLLMProvider(req CreateLLMProviderRequest) (map[string]interface{}, error) {
	// Validação
	if req.ID == "" || req.Name == "" || req.BaseURL == "" {
		return nil, fmt.Errorf("campos obrigatórios faltando (id, name, base_url)")
	}

	// Verificar se já existe
	if a.llmRegistry.Get(req.ID) != nil {
		return nil, fmt.Errorf("provider com ID '%s' já existe", req.ID)
	}

	// Extrair hostname exato do base_url
	hostname, err := extractHostname(req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
	}

	// Salvar API key se fornecida
	credConfigured := false
	if req.APIKey != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}
		err = a.credMgr.RegisterPatternWithContext(a.ctx, hostname, authCfg)
		if err != nil {
			return nil, fmt.Errorf("erro ao salvar credencial: %w", err)
		}
		credConfigured = true
	}

	// Timeout default
	timeout := 180

	// Se é o primeiro provedor, será marcado como default automaticamente
	isFirstProvider := len(a.llmRegistry.List()) == 0

	// Criar provider config
	provider := &llm.ProviderConfig{
		ID:                req.ID,
		Name:              req.Name,
		Type:              llm.ProviderType(req.Type),
		APIFormat:         llm.APIFormat(req.APIFormat),
		BaseURL:           req.BaseURL,
		Model:             "",
		DefaultModel:      req.DefaultModel,
		IsDefault:         isFirstProvider,
		Timeout:           timeout,
		CredentialPattern: hostname,
	}

	// Registrar no registry
	err = a.llmRegistry.Register(provider)
	if err != nil {
		return nil, fmt.Errorf("erro ao registrar provider: %w", err)
	}

	// Salvar provedores em disco
	if err := a.saveLLMProviders(); err != nil {
		log.Printf("[ProviderManager] Erro ao salvar provedores: %v", err)
	}

	if isFirstProvider {
		if err := database.SetDefaultProvider(req.ID); err != nil {
			log.Printf("[ProviderManager] Aviso: erro ao marcar como default: %v", err)
		}
	}

	log.Printf("[ProviderManager] Provider '%s' criado com hostname '%s', default=%v", req.ID, hostname, isFirstProvider)

	return map[string]interface{}{
		"id":                    provider.ID,
		"name":                  provider.Name,
		"type":                  string(provider.Type),
		"base_url":              provider.BaseURL,
		"model":                 provider.Model,
		"default_model":         provider.DefaultModel,
		"is_default":            provider.IsDefault,
		"timeout":               provider.Timeout,
		"credential_pattern":    hostname,
		"credential_configured": credConfigured,
	}, nil
}

// UpdateLLMProvider atualiza um provider existente
func (a *App) UpdateLLMProvider(id string, req UpdateLLMProviderRequest) (map[string]interface{}, error) {
	// Buscar provider existente
	existing := a.llmRegistry.Get(id)
	if existing == nil {
		return nil, fmt.Errorf("provider '%s' não encontrado", id)
	}

	// Atualizar campos fornecidos
	updated := &llm.ProviderConfig{
		ID:                existing.ID,
		Name:              existing.Name,
		Type:              existing.Type,
		APIFormat:         existing.APIFormat,
		BaseURL:           existing.BaseURL,
		Model:             existing.Model,
		DefaultModel:      existing.DefaultModel,
		IsDefault:         existing.IsDefault,
		Timeout:           existing.Timeout,
		CredentialPattern: existing.CredentialPattern,
	}

	if req.Name != "" {
		updated.Name = req.Name
	}
	if req.Type != "" {
		updated.Type = llm.ProviderType(req.Type)
	}
	if req.APIFormat != "" {
		updated.APIFormat = llm.APIFormat(req.APIFormat)
	}
	if req.DefaultModel != "" {
		updated.DefaultModel = req.DefaultModel
	}
	if req.BaseURL != "" {
		updated.BaseURL = req.BaseURL
		// Re-extrair hostname se base_url mudou
		hostname, err := extractHostname(req.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
		}
		updated.CredentialPattern = hostname
		log.Printf("[UpdateLLMProvider] Base URL mudou, novo hostname: '%s'", hostname)
	}

	// Atualizar credencial se fornecida
	credConfigured := false
	if req.APIKey != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}
		err := a.credMgr.RegisterPatternWithContext(a.ctx, updated.CredentialPattern, authCfg)
		if err != nil {
			return nil, fmt.Errorf("erro ao atualizar credencial: %w", err)
		}
		credConfigured = true
	} else {
		// Verificar se credencial já existe (GetByPattern retorna nil,nil se não encontrada)
		auth, err := a.credMgr.GetByPattern(updated.CredentialPattern)
		credConfigured = (err == nil && auth != nil)
	}

	// Remover provider antigo e registrar atualizado
	a.llmRegistry.Remove(id)
	err := a.llmRegistry.Register(updated)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar provider: %w", err)
	}

	// Salvar provedores em disco
	if err := a.saveLLMProviders(); err != nil {
		log.Printf("[ProviderManager] Erro ao salvar provedores: %v", err)
	}

	log.Printf("[ProviderManager] Provider '%s' atualizado", id)

	return map[string]interface{}{
		"id":                    updated.ID,
		"name":                  updated.Name,
		"type":                  string(updated.Type),
		"base_url":              updated.BaseURL,
		"model":                 updated.Model,
		"default_model":         updated.DefaultModel,
		"is_default":            updated.IsDefault,
		"timeout":               updated.Timeout,
		"credential_pattern":    updated.CredentialPattern,
		"credential_configured": credConfigured,
	}, nil
}

// SetDefaultProvider marca um provedor como o default do sistema.
func (a *App) SetDefaultProvider(id string) error {
	provider := a.llmRegistry.Get(id)
	if provider == nil {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}

	if err := database.SetDefaultProvider(id); err != nil {
		return fmt.Errorf("erro ao definir provider default: %w", err)
	}

	// Atualizar flag no registry
	for _, p := range a.llmRegistry.List() {
		p.IsDefault = (p.ID == id)
	}

	// Reinicializar client LLM (perfil ativo pode usar $default)
	a.initLLMClient()

	log.Printf("[ProviderManager] Provider '%s' definido como default", id)
	return nil
}

// DeleteLLMProvider remove um provider do registry
func (a *App) DeleteLLMProvider(ctx context.Context, id string) error {
	provider := a.llmRegistry.Get(id)
	if provider == nil {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}

	// Remover do registry
	err := a.llmRegistry.Remove(id)
	if err != nil {
		return fmt.Errorf("erro ao remover provider: %w", err)
	}

	// Nota: Não removemos a credencial do credentials.Manager pois pode ser usada por outros providers
	// Se quiser remover, adicionar: a.credMgr.DeletePattern(provider.CredentialPattern)

	log.Printf("[ProviderManager] Provider '%s' removido", id)
	return nil
}

// GetLLMProvidersWithStatus retorna todos os providers com status de credencial
func (a *App) GetLLMProvidersWithStatus() []map[string]interface{} {
	providers := a.GetLLMProviders()
	result := make([]map[string]interface{}, 0, len(providers))

	for _, p := range providers {
		// Verificar se credencial está configurada (GetByPattern retorna nil,nil se não encontrada)
		credConfigured := false
		if p.CredentialPattern != "" {
			auth, err := a.credMgr.GetByPattern(p.CredentialPattern)
			credConfigured = (err == nil && auth != nil)
		}

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
			"credential_configured": credConfigured,
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

// saveLLMProviders salva os provedores no SQLite
func (a *App) saveLLMProviders() error {
	providers := a.llmRegistry.List()

	for _, p := range providers {
		dbProvider := &database.LLMProvider{
			ID:                p.ID,
			Name:              p.Name,
			Type:              string(p.Type),
			APIFormat:         string(p.APIFormat),
			BaseURL:           p.BaseURL,
			Model:             p.Model,
			DefaultModel:      p.DefaultModel,
			IsDefault:         p.IsDefault,
			Timeout:           p.Timeout,
			CredentialPattern: p.CredentialPattern,
		}
		if err := database.SaveLLMProvider(dbProvider); err != nil {
			log.Printf("Erro ao salvar provedor %s: %v", p.ID, err)
			return err
		}
	}

	return nil
}

// loadLLMProviders carrega provedores do SQLite
func (a *App) loadLLMProviders() error {
	providers, err := database.GetLLMProviders()
	if err != nil {
		return err
	}

	if len(providers) == 0 {
		return fmt.Errorf("nenhum provedor encontrado")
	}

	for _, dbProvider := range providers {
		p := &llm.ProviderConfig{
			ID:                dbProvider.ID,
			Name:              dbProvider.Name,
			Type:              llm.ProviderType(dbProvider.Type),
			APIFormat:         llm.APIFormat(dbProvider.APIFormat),
			BaseURL:           dbProvider.BaseURL,
			Model:             dbProvider.Model,
			DefaultModel:      dbProvider.DefaultModel,
			IsDefault:         dbProvider.IsDefault,
			Timeout:           dbProvider.Timeout,
			CredentialPattern: dbProvider.CredentialPattern,
		}
		if err := a.llmRegistry.Register(p); err != nil {
			log.Printf("Erro ao registrar provedor %s: %v", p.ID, err)
		}
	}

	log.Printf("Provedores LLM carregados do SQLite: %d", len(providers))

	a.ensureDefaultProvider()

	return nil
}

// ensureDefaultProvider marks the first provider as default when none is.
// Handles migration for providers created before the IsDefault feature.
func (a *App) ensureDefaultProvider() {
	defaultProv, err := database.GetDefaultProvider()
	if err == nil && defaultProv != nil {
		return
	}

	allProviders := a.llmRegistry.List()
	if len(allProviders) == 0 {
		return
	}

	first := allProviders[0]
	log.Printf("[ProviderManager] Nenhum provedor default — marcando '%s' como default", first.Name)

	if err := database.SetDefaultProvider(first.ID); err != nil {
		log.Printf("[ProviderManager] Erro ao definir default: %v", err)
		return
	}
	first.IsDefault = true

	if first.DefaultModel == "" && first.Model != "" {
		first.DefaultModel = first.Model
		if dbProv, err := database.GetLLMProvider(first.ID); err == nil {
			dbProv.DefaultModel = first.Model
			database.SaveLLMProvider(dbProv)
		}
	}
}

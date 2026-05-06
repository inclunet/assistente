package controllers

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
)

// LLMProviderRequest types — movidos de app.go para o pacote controllers.

// CreateLLMProviderRequest é o payload para criar um provedor LLM.
type CreateLLMProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
}

// TestLLMProviderRequest é o payload para testar um provedor LLM.
type TestLLMProviderRequest struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
}

// UpdateLLMProviderRequest é o payload para atualizar um provedor LLM.
type UpdateLLMProviderRequest struct {
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
}

// LLMControllerConfig agrupa as dependências do LLMController.
type LLMControllerConfig struct {
	LLMRegistry      *llm.ProviderRegistry
	ProfileMgr       *profiles.Manager
	ProviderSvc      *providers.Service
	Emitter          ports.Emitter
	OnProviderChange func() // ex: reinicializar cliente LLM ativo
}

// LLMController é o adapter primário (Inbound) para operações de provedores LLM.
type LLMController struct {
	llmRegistry      *llm.ProviderRegistry
	profileMgr       *profiles.Manager
	providerSvc      *providers.Service
	emitter          ports.Emitter
	onProviderChange func()
}

// NewLLMController cria um LLMController com suas dependências.
func NewLLMController(cfg LLMControllerConfig) *LLMController {
	return &LLMController{
		llmRegistry:      cfg.LLMRegistry,
		profileMgr:       cfg.ProfileMgr,
		providerSvc:      cfg.ProviderSvc,
		emitter:          cfg.Emitter,
		onProviderChange: cfg.OnProviderChange,
	}
}

func (c *LLMController) GetLLMProviders() []*llm.ProviderConfig {
	if c.llmRegistry == nil {
		return []*llm.ProviderConfig{}
	}
	return c.llmRegistry.List()
}

func (c *LLMController) GetLLMProvider(id string) *llm.ProviderConfig {
	if c.llmRegistry == nil {
		return nil
	}
	return c.llmRegistry.Get(id)
}

func (c *LLMController) GetActiveProviderInfo() map[string]interface{} {
	activeProfile, err := c.profileMgr.GetActive()
	if err != nil || activeProfile == nil {
		return map[string]interface{}{"error": "perfil ativo não encontrado"}
	}
	info := c.providerSvc.GetActiveProviderInfo(activeProfile)
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

func (c *LLMController) TestLLMProvider(ctx context.Context, req TestLLMProviderRequest) (ok bool, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("erro interno ao testar provider: %v", r)
		}
	}()
	return c.providerSvc.TestConnection(ctx, providers.TestRequest{
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		ProviderID: req.ProviderID,
	})
}

func (c *LLMController) ListModelsRaw(ctx context.Context, req TestLLMProviderRequest) (models []string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LLMController.ListModelsRaw] PANIC: %v", r)
			retErr = fmt.Errorf("erro interno ao listar modelos: %v", r)
		}
	}()
	return c.providerSvc.ListModelsRaw(ctx, providers.ListModelsRawRequest{
		Type:       req.Type,
		BaseURL:    req.BaseURL,
		APIKey:     req.APIKey,
		ProviderID: req.ProviderID,
	})
}

// providerToMap serializa um ProviderConfig para o formato esperado pelo frontend.
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

func (c *LLMController) CreateLLMProvider(ctx context.Context, req CreateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := c.providerSvc.Create(ctx, providers.CreateRequest{
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

func (c *LLMController) UpdateLLMProvider(ctx context.Context, id string, req UpdateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := c.providerSvc.Update(ctx, id, providers.UpdateRequest{
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

func (c *LLMController) SetDefaultProvider(id string) error {
	if err := c.providerSvc.SetDefault(id); err != nil {
		return err
	}
	if c.onProviderChange != nil {
		c.onProviderChange()
	}
	return nil
}

func (c *LLMController) DeleteLLMProvider(id string) error {
	return c.providerSvc.Delete(id)
}

func (c *LLMController) GetLLMProvidersWithStatus(ctx context.Context) []map[string]interface{} {
	statuses := c.providerSvc.ListWithStatus(ctx)
	result := make([]map[string]interface{}, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, providerToMap(s.Provider, s.Provider.CredentialPattern, s.CredentialConfigured))
	}
	return result
}

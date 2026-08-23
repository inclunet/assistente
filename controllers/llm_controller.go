package controllers

import (
	"assistente/internal/apidto"
	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"context"
	"fmt"
)

// CreateLLMProviderRequest — alias estável durante a migração Strangler (AEP-0088 D5).
type CreateLLMProviderRequest = apidto.CreateLLMProviderRequest

// TestLLMProviderRequest — alias estável durante a migração Strangler (AEP-0088 D5).
type TestLLMProviderRequest = apidto.TestLLMProviderRequest

// UpdateLLMProviderRequest — alias estável durante a migração Strangler (AEP-0088 D5).
type UpdateLLMProviderRequest = apidto.UpdateLLMProviderRequest

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

func (c *LLMController) GetActiveProviderInfo(ctx context.Context) map[string]interface{} {
	activeProfile, err := c.profileMgr.GetActive()
	if err != nil || activeProfile == nil {
		return map[string]interface{}{"error": "perfil ativo não encontrado"}
	}
	info := c.providerSvc.GetActiveProviderInfo(ctx, activeProfile)
	if info.Error != "" {
		return map[string]interface{}{
			"error":      info.Error,
			"providerID": activeProfile.Chat.LLMProvider,
		}
	}
	return map[string]interface{}{
		"id":                         info.ID,
		"name":                       info.Name,
		"type":                       info.Type,
		"base_url":                   info.BaseURL,
		"model":                      info.Model,
		"supports_assistant_prefill": info.SupportsAssistantPrefill,
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
			logging.Errorf(ctx, "controllers.llm-controller", "[LLMController.ListModelsRaw] PANIC: %v", r)
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
	// Lista sempre presente: `null` faria a tela distinguir "sem argumentos" de
	// "campo ausente" antes de conseguir preencher o formulário de edição.
	acpArgs := p.ACPArgs
	if acpArgs == nil {
		acpArgs = []string{}
	}
	// Pela mesma razão, o par variável/entrada do cofre sai sempre como objeto.
	// Ele volta para a tela porque é referência, e é o que permite editar o que
	// está ligado sem reconfigurar do zero; o segredo não vem junto.
	acpCredentialEnv := p.ACPCredentialEnv
	if acpCredentialEnv == nil {
		acpCredentialEnv = map[string]string{}
	}
	// ACPEnv NÃO volta na leitura. É o mesmo motivo da exportação: a coluna pode
	// guardar token colado à mão (AEP-0086), e a tela não edita esse mapa — o
	// env{} do binário o app aplica a partir do installed.json. Devolver {}
	// evita vazar segredo para o frontend sem perder o runtime.
	acpEnv := map[string]string{}
	return map[string]interface{}{
		"id":                     p.ID,
		"name":                   p.Name,
		"type":                   string(p.Type),
		"api_format":             string(p.GetAPIFormat()),
		"base_url":               p.BaseURL,
		"model":                  p.Model,
		"default_model":          p.DefaultModel,
		"is_default":             p.IsDefault,
		"timeout":                p.Timeout,
		"credential_pattern":     credentialPattern,
		"credential_configured":  credentialConfigured,
		"auth_mode":              string(p.EffectiveAuthMode()),
		"reasoning_content_mode": string(p.EffectiveReasoningContentMode()),
		"acp_command":            p.ACPCommand,
		"acp_args":               acpArgs,
		"acp_env":                acpEnv,
		"acp_agent_id":           p.ACPAgentID,
		"acp_credential_env":     acpCredentialEnv,
	}
}

func (c *LLMController) CreateLLMProvider(ctx context.Context, req CreateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := c.providerSvc.Create(ctx, providers.CreateRequest{
		ID:                   req.ID,
		Name:                 req.Name,
		Type:                 req.Type,
		APIFormat:            req.APIFormat,
		BaseURL:              req.BaseURL,
		APIKey:               req.APIKey,
		DefaultModel:         req.DefaultModel,
		ReasoningContentMode: req.ReasoningContentMode,
		ACPCommand:           req.ACPCommand,
		ACPArgs:              req.ACPArgs,
		ACPAgentID:           req.ACPAgentID,

		ACPCredentialEnv: req.ACPCredentialEnv,
	})
	if err != nil {
		return nil, err
	}
	return providerToMap(res.Provider, res.CredentialPattern, res.CredentialConfigured), nil
}

func (c *LLMController) UpdateLLMProvider(ctx context.Context, id string, req UpdateLLMProviderRequest) (map[string]interface{}, error) {
	res, err := c.providerSvc.Update(ctx, id, providers.UpdateRequest{
		Name:                 req.Name,
		Type:                 req.Type,
		APIFormat:            req.APIFormat,
		BaseURL:              req.BaseURL,
		APIKey:               req.APIKey,
		DefaultModel:         req.DefaultModel,
		ReasoningContentMode: req.ReasoningContentMode,
		ACPCommand:           req.ACPCommand,
		ACPArgs:              req.ACPArgs,
		ACPAgentID:           req.ACPAgentID,

		ACPCredentialEnv: req.ACPCredentialEnv,
	})
	if err != nil {
		return nil, err
	}
	p := res.Provider
	return providerToMap(p, p.CredentialPattern, res.CredentialConfigured), nil
}

func (c *LLMController) SetDefaultProvider(ctx context.Context, id string) error {
	if err := c.providerSvc.SetDefault(ctx, id); err != nil {
		return err
	}
	if c.onProviderChange != nil {
		c.onProviderChange()
	}
	return nil
}

func (c *LLMController) DeleteLLMProvider(ctx context.Context, id string) error {
	return c.providerSvc.Delete(ctx, id)
}

func (c *LLMController) GetLLMProvidersWithStatus(ctx context.Context) []map[string]interface{} {
	statuses := c.providerSvc.ListWithStatus(ctx)
	result := make([]map[string]interface{}, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, providerToMap(s.Provider, s.Provider.CredentialPattern, s.CredentialConfigured))
	}
	return result
}

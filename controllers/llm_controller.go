package controllers

import (
	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"context"
	"fmt"
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
	// ACPCommand e ACPArgs endereçam o agente de código quando APIFormat é
	// acp: é o que substitui BaseURL e APIKey, que ali não existem
	// (AEP-0084 D12).
	//
	// ACPEnv fica de fora de propósito. Variável de ambiente de processo é onde
	// token costuma parar, e não há tela que a edite: expô-la na fronteira só
	// criaria um caminho para segredo entrar sem que ninguém o veja. Quem
	// precisa dela usa a importação de configuração, que já a aceita.
	ACPCommand string   `json:"acp_command,omitempty"`
	ACPArgs    []string `json:"acp_args,omitempty"`
	// ACPAgentID é o agente do registro que a tela escolheu no catálogo
	// (AEP-0086 D11). Vazio é agente apontado à mão, que segue valendo.
	ACPAgentID string `json:"acp_agent_id,omitempty"`
	// ACPCredentialEnv diz quais variáveis do ambiente do agente recebem uma
	// credencial do cofre, e de qual entrada dele (AEP-0086 D12). Ao contrário
	// do ACPEnv, isto atravessa a fronteira: o que passa aqui é o nome da
	// entrada, não o segredo — ele continua saindo do cofre só na hora de subir
	// o processo, e nunca volta para a tela.
	ACPCredentialEnv map[string]string `json:"acp_credential_env,omitempty"`
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
	// ACPCommand segue a convenção dos demais campos daqui: vazio é "não
	// mexer".
	ACPCommand string `json:"acp_command,omitempty"`
	// ACPArgs é ponteiro porque, aqui, lista vazia é edição legítima — tirar
	// todos os argumentos do agente —, e "vazio é não mexer" tornaria isso
	// impossível.
	ACPArgs *[]string `json:"acp_args,omitempty"`
	// ACPAgentID troca qual agente do registro este provedor é. É ponteiro
	// pela razão do ACPArgs: vazio aqui é edição de verdade, porque agente
	// apontado à mão é caminho válido (AEP-0086 D3) e é para onde volta quem
	// precisa desvincular o provedor do catálogo. Nulo é "não mexer".
	ACPAgentID *string `json:"acp_agent_id,omitempty"`
	// ACPCredentialEnv troca as variáveis que recebem credencial do cofre
	// (AEP-0086 D12). É ponteiro pela razão do ACPArgs: mapa vazio aqui é
	// desligar a passagem, que é a edição que alguém faz ao tirar a credencial
	// de um agente; nulo é "não mexer".
	ACPCredentialEnv *map[string]string `json:"acp_credential_env,omitempty"`
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
		"id":                    p.ID,
		"name":                  p.Name,
		"type":                  string(p.Type),
		"api_format":            string(p.GetAPIFormat()),
		"base_url":              p.BaseURL,
		"model":                 p.Model,
		"default_model":         p.DefaultModel,
		"is_default":            p.IsDefault,
		"timeout":               p.Timeout,
		"credential_pattern":    credentialPattern,
		"credential_configured": credentialConfigured,
		"auth_mode":             string(p.EffectiveAuthMode()),
		"acp_command":           p.ACPCommand,
		"acp_args":              acpArgs,
		"acp_env":               acpEnv,
		"acp_agent_id":          p.ACPAgentID,
		"acp_credential_env":    acpCredentialEnv,
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
		ACPCommand:   req.ACPCommand,
		ACPArgs:      req.ACPArgs,
		ACPAgentID:   req.ACPAgentID,

		ACPCredentialEnv: req.ACPCredentialEnv,
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
		ACPCommand:   req.ACPCommand,
		ACPArgs:      req.ACPArgs,
		ACPAgentID:   req.ACPAgentID,

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

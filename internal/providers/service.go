package providers

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// CredentialManager abstrai o gerenciador de credenciais para o service.
// Permite testar sem o credentials.Manager concreto se necessário.
type CredentialManager interface {
	RegisterPatternWithContext(ctx context.Context, pattern string, auth *credentials.AuthConfig) error
	GetByPattern(pattern string) (*credentials.AuthConfig, error)
	GetByPatternWithContext(ctx context.Context, pattern string) (*credentials.AuthConfig, error)
	DeletePattern(ctx context.Context, pattern string) error
}

// ServiceConfig contém as dependências externas do Service.
type ServiceConfig struct {
	Registry *llm.ProviderRegistry
	CredMgr  CredentialManager
	Store    ProviderStore
	// RateLimiter aplica rate limiting por usuário nas chamadas de geração ao
	// provedor LLM (Issue #27 / AEP-0065). Opcional: nil = sem limite.
	RateLimiter *llm.RateLimiter
	// RateLimitKeyFunc extrai a chave de limite (tipicamente o userID) do
	// contexto. Opcional: nil cai na chave global do limitador.
	RateLimitKeyFunc func(context.Context) string
}

// Service encapsula a lógica de negócio de gerenciamento de provedores LLM.
// Não depende de Wails — é testável de forma isolada.
type Service struct {
	registry         *llm.ProviderRegistry
	credMgr          CredentialManager
	store            ProviderStore
	rateLimiter      *llm.RateLimiter
	rateLimitKeyFunc func(context.Context) string
}

// Count retorna o número de provedores no store.
func (s *Service) Count(ctx context.Context) (int, error) {
	return s.store.Count(ctx)
}

// NewService cria um Service com as dependências injetadas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		registry:         cfg.Registry,
		credMgr:          cfg.CredMgr,
		store:            cfg.Store,
		rateLimiter:      cfg.RateLimiter,
		rateLimitKeyFunc: cfg.RateLimitKeyFunc,
	}
}

// ============================================================================
// URL / Hostname helpers
// ============================================================================

// ExtractHostname extrai o hostname de uma base URL para uso como padrão de credencial.
func ExtractHostname(baseURL string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("base_url vazio")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("base_url inválido: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("host não encontrado no base_url")
	}
	return host, nil
}

// ============================================================================
// Persistence
// ============================================================================

// Save persiste todos os provedores do registry no store.
func (s *Service) Save(ctx context.Context) error {
	providers := s.registry.List()
	return s.store.Save(ctx, providers)
}

// Load carrega provedores do store para o registry.
func (s *Service) Load(ctx context.Context) error {
	providers, err := s.store.Load(ctx)
	if err != nil {
		return err
	}
	if len(providers) == 0 {
		return fmt.Errorf("nenhum provedor encontrado")
	}

	needsSave := false
	for _, p := range providers {
		// Materializa api_format inferido para providers existentes sem valor explícito.
		// Evita log repetitivo de inferência a cada chamada de GetAPIFormat().
		if p.APIFormat == "" {
			inferred := p.GetAPIFormat()
			p.APIFormat = inferred
			needsSave = true
			logging.Infof(ctx, "providers.service", "[providers] api_format de '%s' materializado como %q", p.Name, inferred)
		}
		if err := s.registry.Register(p); err != nil {
			logging.Errorf(ctx, "providers.service", "[providers] Erro ao registrar provedor '%s': %v", p.ID, err)
		}
	}
	logging.Infof(ctx, "providers.service", "[providers] %d provedor(es) carregado(s) do store", len(providers))
	s.EnsureDefault(ctx)

	// Persistir api_format materializado para não repetir inferência no próximo boot
	if needsSave {
		if err := s.Save(ctx); err != nil {
			logging.Errorf(ctx, "providers.service", "[providers] Erro ao persistir api_format materializado: %v", err)
		}
	}
	return nil
}

// EnsureDefault garante que pelo menos um provedor está marcado como padrão.
// Chamado automaticamente após Load. Seguro executar múltiplas vezes.
func (s *Service) EnsureDefault(ctx context.Context) {
	defaultProv, err := s.store.GetDefault(ctx)
	if err == nil && defaultProv != nil {
		return
	}

	all := s.registry.List()
	if len(all) == 0 {
		return
	}

	first := all[0]
	logging.Warnf(ctx, "providers.service", "[providers] Nenhum provedor default — marcando '%s' como default", first.Name)

	if err := s.store.SetDefault(ctx, first.ID); err != nil {
		logging.Errorf(ctx, "providers.service", "[providers] Erro ao definir default: %v", err)
		return
	}
	first.IsDefault = true

	if first.DefaultModel == "" && first.Model != "" {
		first.DefaultModel = first.Model
		// Persiste o DefaultModel preenchido
		if err := s.store.Save(ctx, []*llm.ProviderConfig{first}); err != nil {
			logging.Errorf(ctx, "providers.service", "[providers] Erro ao salvar DefaultModel: %v", err)
		}
	}
}

// ============================================================================
// CRUD
// ============================================================================

// CreateRequest contém os dados para criar um provedor.
type CreateRequest struct {
	ID           string
	Name         string
	Type         string
	APIFormat    string
	BaseURL      string
	APIKey       string
	DefaultModel string
}

// CreateResult contém os dados retornados após criar um provedor.
type CreateResult struct {
	Provider             *llm.ProviderConfig
	CredentialPattern    string
	CredentialConfigured bool
}

func defaultAuthModeForProviderType(providerType llm.ProviderType) llm.AuthMode {
	switch providerType {
	case llm.ProviderLocalAI:
		return llm.AuthModeOptional
	case llm.ProviderOllama, llm.ProviderLlamaCPP:
		return llm.AuthModeNone
	default:
		return ""
	}
}

func normalizeProviderAuthMode(p *llm.ProviderConfig) {
	if p == nil || p.AuthMode != "" {
		return
	}
	p.AuthMode = defaultAuthModeForProviderType(p.Type)
}

func normalizeProviderAPIFormat(p *llm.ProviderConfig) {
	if p == nil {
		return
	}
	switch p.Type {
	case llm.ProviderLocalAI, llm.ProviderOllama, llm.ProviderLlamaCPP:
		if p.APIFormat == "" || p.APIFormat == llm.APIFormatOpenAIResponses {
			p.APIFormat = llm.APIFormatOpenAI
		}
	}
}

func normalizeProviderRuntimeDefaults(p *llm.ProviderConfig) {
	normalizeProviderAuthMode(p)
	normalizeProviderAPIFormat(p)
}

// Create cria e registra um novo provedor LLM.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*CreateResult, error) {
	if req.ID == "" || req.Name == "" || req.BaseURL == "" {
		return nil, fmt.Errorf("campos obrigatórios faltando (id, name, base_url)")
	}
	if s.registry.Get(req.ID) != nil {
		return nil, fmt.Errorf("provider com ID '%s' já existe", req.ID)
	}

	hostname, err := ExtractHostname(req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
	}

	credConfigured := false
	if req.APIKey != "" {
		if err := s.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}); err != nil {
			return nil, fmt.Errorf("erro ao salvar credencial: %w", err)
		}
		credConfigured = true
	}

	isFirst := len(s.registry.List()) == 0
	provider := &llm.ProviderConfig{
		ID:                req.ID,
		Name:              req.Name,
		Type:              llm.ProviderType(req.Type),
		APIFormat:         llm.APIFormat(req.APIFormat),
		BaseURL:           req.BaseURL,
		DefaultModel:      req.DefaultModel,
		IsDefault:         isFirst,
		Timeout:           180,
		CredentialPattern: hostname,
	}
	normalizeProviderRuntimeDefaults(provider)

	if err := s.registry.Register(provider); err != nil {
		return nil, fmt.Errorf("erro ao registrar provider: %w", err)
	}
	if err := s.Save(ctx); err != nil {
		logging.Errorf(ctx, "providers.service", "[providers] Erro ao salvar após criação: %v", err)
	}
	if isFirst {
		if err := s.store.SetDefault(ctx, req.ID); err != nil {
			logging.Warnf(ctx, "providers.service", "[providers] Aviso: erro ao marcar como default: %v", err)
		}
	}

	logging.Infof(ctx, "providers.service", "[providers] Provider '%s' criado (hostname=%s, default=%v)", req.ID, hostname, isFirst)
	return &CreateResult{
		Provider:             provider,
		CredentialPattern:    hostname,
		CredentialConfigured: credConfigured,
	}, nil
}

// UpdateRequest contém os campos opcionais para atualizar um provedor.
type UpdateRequest struct {
	Name         string
	Type         string
	APIFormat    string
	BaseURL      string
	APIKey       string
	DefaultModel string
}

// UpdateResult contém os dados após atualização.
type UpdateResult struct {
	Provider             *llm.ProviderConfig
	CredentialConfigured bool
}

// Update atualiza um provedor LLM existente.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*UpdateResult, error) {
	existing := s.registry.Get(id)
	if existing == nil {
		return nil, fmt.Errorf("provider '%s' não encontrado", id)
	}

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
		AuthMode:          existing.AuthMode,
	}

	if req.Name != "" {
		updated.Name = req.Name
	}
	if req.Type != "" {
		updated.Type = llm.ProviderType(req.Type)
		updated.AuthMode = defaultAuthModeForProviderType(updated.Type)
	}
	if req.APIFormat != "" {
		updated.APIFormat = llm.APIFormat(req.APIFormat)
	}
	if req.DefaultModel != "" {
		updated.DefaultModel = req.DefaultModel
	}
	if req.BaseURL != "" {
		hostname, err := ExtractHostname(req.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
		}
		updated.BaseURL = req.BaseURL
		updated.CredentialPattern = hostname
	}
	normalizeProviderRuntimeDefaults(updated)

	credConfigured := false
	if req.APIKey != "" {
		if err := s.credMgr.RegisterPatternWithContext(ctx, updated.CredentialPattern, &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}); err != nil {
			return nil, fmt.Errorf("erro ao atualizar credencial: %w", err)
		}
		credConfigured = true
	} else if updated.CredentialPattern != "" {
		auth, err := s.credMgr.GetByPatternWithContext(ctx, updated.CredentialPattern)
		credConfigured = err == nil && auth != nil
	}

	if err := s.registry.Remove(id); err != nil {
		logging.Warnf(ctx, "providers.service", "[providers] Aviso: falha ao remover provider antigo '%s': %v", id, err)
	}
	if err := s.registry.Register(updated); err != nil {
		return nil, fmt.Errorf("erro ao atualizar provider: %w", err)
	}
	if err := s.Save(ctx); err != nil {
		logging.Errorf(ctx, "providers.service", "[providers] Erro ao salvar após atualização: %v", err)
	}

	logging.Infof(ctx, "providers.service", "[providers] Provider '%s' atualizado", id)
	return &UpdateResult{Provider: updated, CredentialConfigured: credConfigured}, nil
}

// Delete remove um provedor do registry.
func (s *Service) Delete(ctx context.Context, id string) error {
	if s.registry.Get(id) == nil {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}
	if err := s.registry.Remove(id); err != nil {
		return fmt.Errorf("erro ao remover provider: %w", err)
	}
	logging.Infof(ctx, "providers.service", "[providers] Provider '%s' removido", id)
	return nil
}

// SetDefault marca um provedor como padrão do sistema.
func (s *Service) SetDefault(ctx context.Context, id string) error {
	if s.registry.Get(id) == nil {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}
	if err := s.store.SetDefault(ctx, id); err != nil {
		return fmt.Errorf("erro ao definir provider default: %w", err)
	}
	for _, p := range s.registry.List() {
		p.IsDefault = (p.ID == id)
	}
	logging.Infof(ctx, "providers.service", "[providers] Provider '%s' definido como default", id)
	return nil
}

// ============================================================================
// Provider Status
// ============================================================================

// ProviderStatus combina o ProviderConfig com status de credencial.
type ProviderStatus struct {
	Provider             *llm.ProviderConfig
	CredentialConfigured bool
}

// ListWithStatus retorna todos os provedores com flag de credencial configurada.
func (s *Service) ListWithStatus(ctx context.Context) []ProviderStatus {
	providers := s.registry.List()
	result := make([]ProviderStatus, 0, len(providers))
	for _, p := range providers {
		credConfigured := false
		if p.CredentialPattern != "" {
			auth, err := s.credMgr.GetByPatternWithContext(ctx, p.CredentialPattern)
			if err != nil {
				logging.Infof(ctx, "providers.service", "[providers] Credencial '%s' do provider '%s' não pode ser usada: %v", p.CredentialPattern, p.ID, err)
			}
			credConfigured = err == nil && auth != nil
		}
		result = append(result, ProviderStatus{Provider: p, CredentialConfigured: credConfigured})
	}
	return result
}

// ============================================================================
// Profile defaults resolution
// ============================================================================

// ResolveProfileDefaults substitui sentinelas "$default" no perfil pelo
// provedor/modelo correspondente. Retorna uma cópia modificada — não altera
// o perfil em disco.
//
// Regra do modelo: `Model == $default` significa "use o modelo padrão **do
// provider escolhido**", não o modelo do provider default global. Se o
// profile fixou `LLMProvider="ollama-local"` mas deixou `Model=""` (que
// `normalizeRoutingFields` transformou em $default), o modelo resolvido
// vem de `ollama-local.DefaultModel`. Antes esse caminho usava o
// `defaultProvider.DefaultModel`, o que misturava providers — um profile
// `Modelo Local` acabava enviando o modelo padrão da OpenAI para o
// servidor local. Esse cross-provider leak gerava o sintoma "troquei o
// perfil mas continua usando OpenAI".
func (s *Service) ResolveProfileDefaults(ctx context.Context, p *profiles.Profile) *profiles.Profile {
	if p == nil {
		return nil
	}
	needsResolve := p.Chat.LLMProvider == profiles.DefaultProviderSentinel ||
		p.Chat.Model == profiles.DefaultProviderSentinel ||
		p.Voice.Assistant.LLMProviderID == profiles.DefaultProviderSentinel ||
		p.Input.LLMProviderID == profiles.DefaultProviderSentinel
	if !needsResolve {
		return p
	}

	resolved := *p
	resolved.Chat = p.Chat
	resolved.Voice = p.Voice
	resolved.Input = p.Input

	// Resolve o provider default só se for realmente necessário: se algum
	// dos campos *Provider* do profile estiver com $default, ou se Model
	// estiver $default e Chat.LLMProvider também (caso em que precisamos
	// herdar provider+modelo do default global).
	var defaultProvider *llm.ProviderConfig
	needsDefaultProvider := resolved.Chat.LLMProvider == profiles.DefaultProviderSentinel ||
		resolved.Voice.Assistant.LLMProviderID == profiles.DefaultProviderSentinel ||
		resolved.Input.LLMProviderID == profiles.DefaultProviderSentinel ||
		(resolved.Chat.Model == profiles.DefaultProviderSentinel && resolved.Chat.LLMProvider == profiles.DefaultProviderSentinel)

	if needsDefaultProvider {
		dp, err := s.store.GetDefault(ctx)
		if err != nil || dp == nil {
			logging.Infof(ctx, "providers.service", "[providers] Nenhum provedor default encontrado para resolução: %v", err)
			return p
		}
		defaultProvider = dp
	}

	if resolved.Chat.LLMProvider == profiles.DefaultProviderSentinel {
		resolved.Chat.LLMProvider = defaultProvider.ID
	}
	if resolved.Voice.Assistant.LLMProviderID == profiles.DefaultProviderSentinel {
		resolved.Voice.Assistant.LLMProviderID = defaultProvider.ID
	}
	if resolved.Input.LLMProviderID == profiles.DefaultProviderSentinel {
		resolved.Input.LLMProviderID = defaultProvider.ID
	}

	if resolved.Chat.Model == profiles.DefaultProviderSentinel {
		// IMPORTANTE: o modelo é resolvido a partir do provider que o
		// profile (já resolvido acima) acabou de fixar. Evita pegar o
		// modelo do provider global quando o profile escolheu outro.
		var modelSourceProvider *llm.ProviderConfig
		if s.registry != nil {
			modelSourceProvider = s.registry.Get(resolved.Chat.LLMProvider)
		}
		if modelSourceProvider == nil {
			modelSourceProvider = defaultProvider
		}
		resolvedModel := ""
		if modelSourceProvider != nil {
			if modelSourceProvider.DefaultModel != "" {
				resolvedModel = modelSourceProvider.DefaultModel
			} else if modelSourceProvider.Model != "" {
				resolvedModel = modelSourceProvider.Model
			}
		}
		resolved.Chat.Model = resolvedModel
		if modelSourceProvider != nil {
			logging.Infof(ctx, "providers.service", "[providers] Resolvido $default model → provider=%s, model=%s", modelSourceProvider.ID, resolvedModel)
		}
	} else if defaultProvider != nil {
		logging.Infof(ctx, "providers.service", "[providers] Resolvido $default → provider=%s", defaultProvider.ID)
	}
	return &resolved
}

// ============================================================================
// Connection testing
// ============================================================================

// TestRequest contém os parâmetros para testar uma conexão.
type TestRequest struct {
	BaseURL    string
	APIKey     string
	ProviderID string // Se informado e APIKey vazio, busca credencial existente
}

// TestConnection verifica se um endpoint LLM está acessível.
func (s *Service) TestConnection(ctx context.Context, req TestRequest) (bool, error) {
	if req.BaseURL == "" {
		return false, fmt.Errorf("base_url é obrigatório")
	}

	parsed, err := url.Parse(req.BaseURL)
	if err != nil {
		return false, fmt.Errorf("URL inválida: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false, fmt.Errorf("URL deve começar com http:// ou https://")
	}
	if parsed.Host == "" {
		return false, fmt.Errorf("URL deve conter um endereço de servidor válido")
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && req.ProviderID != "" && s.registry != nil && s.credMgr != nil {
		if provider := s.registry.Get(req.ProviderID); provider != nil && provider.CredentialPattern != "" {
			if auth, err := s.credMgr.GetByPatternWithContext(ctx, provider.CredentialPattern); err == nil && auth != nil && auth.Token != "" {
				apiKey = auth.Token
			}
		}
	}

	modelsEndpoint := strings.TrimSuffix(req.BaseURL, "/") + "/models"
	client := &http.Client{Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
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
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 500:
		return false, fmt.Errorf("servidor retornou erro: %d", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized:
		return false, fmt.Errorf("API Key inválida ou não autorizada")
	case resp.StatusCode == http.StatusForbidden:
		return false, fmt.Errorf("acesso negado (403). A API Key pode não ter permissões suficientes")
	}

	return true, nil
}

// ListModels lista os modelos disponíveis em um endpoint LLM.
func (s *Service) ListModels(ctx context.Context, req TestRequest) ([]string, error) {
	if req.BaseURL == "" {
		return nil, fmt.Errorf("base_url é obrigatório")
	}

	parsed, err := url.Parse(req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL deve começar com http:// ou https://")
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && req.ProviderID != "" && s.registry != nil && s.credMgr != nil {
		if provider := s.registry.Get(req.ProviderID); provider != nil && provider.CredentialPattern != "" {
			if auth, err := s.credMgr.GetByPatternWithContext(ctx, provider.CredentialPattern); err == nil && auth != nil && auth.Token != "" {
				apiKey = auth.Token
			}
		}
	}

	modelsEndpoint := strings.TrimSuffix(req.BaseURL, "/") + "/models"
	client := &http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("API Key inválida ou não autorizada")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("erro do servidor: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return nil, fmt.Errorf("erro ao processar resposta: %w", err)
	}

	models := make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// GetChatProvider returns a ready-to-use ChatProvider for the given provider ID.
// Looks up the provider config in the registry and wraps it with the credential manager.
func (s *Service) GetChatProvider(ctx context.Context, providerID string) (llm.ChatProvider, error) {
	if s.registry == nil {
		return nil, fmt.Errorf("registro de provedores não inicializado")
	}
	provider := s.registry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provedor LLM não encontrado: %s", providerID)
	}
	cm, _ := s.credMgr.(*credentials.Manager)
	// Aplica rate limiting por usuário de forma central (Issue #27). Quando
	// rateLimiter é nil, NewRateLimitedProvider devolve o provider inalterado.
	return llm.NewRateLimitedProvider(llm.NewChatProvider(provider, cm), s.rateLimiter, s.rateLimitKeyFunc), nil
}

// ListModelsRawRequest contém os parâmetros para listagem de modelos via credenciais ad-hoc.
type ListModelsRawRequest struct {
	Type       string
	BaseURL    string
	APIKey     string // se vazio e ProviderID preenchido, busca credencial existente
	ProviderID string // opcional; usado para recuperar credencial existente
}

// buildTempProviderForListModels monta o ProviderConfig efêmero usado em
// `ListModelsRaw`. Espelha campos críticos do provider persistido (quando
// disponível) para que a rota usada no teste de chave coincida com a rota
// usada em produção. Sem o espelhamento de `APIFormat`, o teste cairia no
// client default (Chat Completions) enquanto o uso real bateria em
// Responses API — divergência que mascarava o motivo real do 400.
//
// Extraído como função pura para permitir teste unitário sem precisar
// rodar o pipeline HTTP completo. Não toca `s.credMgr` nem o registry.
func buildTempProviderForListModels(req ListModelsRawRequest, hostname string, existing *llm.ProviderConfig) *llm.ProviderConfig {
	temp := &llm.ProviderConfig{
		ID:                "temp-form",
		Name:              "temp",
		Type:              llm.ProviderType(req.Type),
		BaseURL:           req.BaseURL,
		CredentialPattern: hostname,
		Timeout:           15,
	}
	if existing != nil {
		temp.APIFormat = existing.APIFormat
	}
	return temp
}

// ListModelsRaw lista modelos de um provedor usando credenciais ad-hoc ou existentes.
// Não requer que o provedor já esteja persistido — usado pelo formulário de criação/edição.
func (s *Service) ListModelsRaw(ctx context.Context, req ListModelsRawRequest) ([]string, error) {
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

	apiKey := strings.TrimSpace(req.APIKey)
	// Fallback: busca credencial existente quando provider_id informado e api_key ausente
	var existingProvider *llm.ProviderConfig
	if req.ProviderID != "" && s.registry != nil {
		existingProvider = s.registry.Get(req.ProviderID)
	}
	if apiKey == "" && existingProvider != nil && existingProvider.CredentialPattern != "" && s.credMgr != nil {
		if auth, err := s.credMgr.GetByPatternWithContext(ctx, existingProvider.CredentialPattern); err == nil && auth != nil && auth.Token != "" {
			apiKey = auth.Token
		}
	}

	hostname := parsedURL.Hostname()
	tempProvider := buildTempProviderForListModels(req, hostname, existingProvider)

	cm, _ := s.credMgr.(*credentials.Manager)

	// Registra credencial ad-hoc temporariamente para o provider encontrá-la
	if apiKey != "" && s.credMgr != nil {
		_ = s.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{
			Type: "bearer", Token: apiKey,
		})
		// Remove credencial temporária ao término (somente se não é um provider existente)
		if req.ProviderID == "" {
			defer s.credMgr.DeletePattern(ctx, hostname) //nolint:errcheck
		}
	}

	cp := llm.NewChatProvider(tempProvider, cm)
	return cp.GetModels(ctx)
}

// GetModels retorna os modelos disponíveis para o provedor do perfil ativo.
// Resolve sentinelas $default antes de consultar o provider.
func (s *Service) GetModels(ctx context.Context, activeProfile *profiles.Profile) ([]string, error) {
	activeProfile = s.ResolveProfileDefaults(ctx, activeProfile)
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		return nil, fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	cp, err := s.GetChatProvider(ctx, activeProfile.Chat.LLMProvider)
	if err != nil {
		return nil, err
	}
	return cp.GetModels(ctx)
}

// GetModelsByProvider retorna os modelos disponíveis para um provider específico pelo ID.
func (s *Service) GetModelsByProvider(ctx context.Context, providerID string) ([]string, error) {
	if providerID == "" {
		return []string{}, nil
	}
	cp, err := s.GetChatProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	return cp.GetModels(ctx)
}

// ActiveProviderInfo contém campos informativos sobre o provedor ativo.
type ActiveProviderInfo struct {
	ID                       string           `json:"id"`
	Name                     string           `json:"name"`
	Type                     llm.ProviderType `json:"type"`
	BaseURL                  string           `json:"base_url"`
	Model                    string           `json:"model"`
	SupportsAssistantPrefill bool             `json:"supports_assistant_prefill"`
	Error                    string           `json:"error,omitempty"`
}

// GetActiveProviderInfo retorna informações sobre o provedor do perfil ativo.
func (s *Service) GetActiveProviderInfo(ctx context.Context, activeProfile *profiles.Profile) ActiveProviderInfo {
	if activeProfile == nil {
		return ActiveProviderInfo{Error: "perfil ativo não encontrado"}
	}
	activeProfile = s.ResolveProfileDefaults(ctx, activeProfile)

	if s.registry == nil {
		return ActiveProviderInfo{Error: "registro de provedores não inicializado"}
	}
	provider := s.registry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		return ActiveProviderInfo{
			Error: fmt.Sprintf("provedor não encontrado: %s", activeProfile.Chat.LLMProvider),
		}
	}
	return ActiveProviderInfo{
		ID:                       provider.ID,
		Name:                     provider.Name,
		Type:                     provider.Type,
		BaseURL:                  provider.BaseURL,
		Model:                    provider.Model,
		SupportsAssistantPrefill: llm.SupportsAssistantPrefill(provider),
	}
}

// SupportsAssistantPrefill aplica a mesma resolução de perfil usada em runtime
// para decidir se a continuação explícita pode enviar assistant prefill.
func (s *Service) SupportsAssistantPrefill(ctx context.Context, activeProfile *profiles.Profile) bool {
	if activeProfile == nil || s.registry == nil {
		return false
	}
	activeProfile = s.ResolveProfileDefaults(ctx, activeProfile)
	if activeProfile == nil {
		return false
	}
	return llm.SupportsAssistantPrefill(s.registry.Get(activeProfile.Chat.LLMProvider))
}

// SupportsExplicitCacheControl aplica a resolução de perfil usada em runtime
// para decidir se o provider ativo aceita cache_control explícito no payload.
func (s *Service) SupportsExplicitCacheControl(ctx context.Context, activeProfile *profiles.Profile) bool {
	if activeProfile == nil || s.registry == nil {
		return false
	}
	activeProfile = s.ResolveProfileDefaults(ctx, activeProfile)
	if activeProfile == nil {
		return false
	}
	return llm.SupportsExplicitCacheControl(s.registry.Get(activeProfile.Chat.LLMProvider))
}

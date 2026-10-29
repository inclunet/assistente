package providers

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
)

// TemplateFromCatalogServer converte somente identificadores declarativos
// conhecidos pelo runtime. Os valores do catálogo nunca escolhem código ou
// adaptadores: família, formato e autenticação passam por allowlists locais.
func TemplateFromCatalogServer(server llmcatalog.Server) (*llm.ProviderConfig, error) {
	if server.Status != "active" {
		return nil, fmt.Errorf("perfil de servidor %q não está ativo", server.ID)
	}
	apiFormatValue := CatalogFieldDefault(server, "api_format", server.APIFormat)
	profile, err := catalogRuntimeProfileFor(server, apiFormatValue)
	if err != nil {
		return nil, err
	}
	apiFormat, err := catalogAPIFormat(apiFormatValue)
	if err != nil {
		return nil, err
	}
	authModeValue := CatalogFieldDefault(server, "auth.mode", server.Auth.Mode)
	authMode, err := CatalogAuthMode(authModeValue)
	if err != nil {
		return nil, err
	}

	baseURL := CatalogFieldDefault(server, "base_url", server.BaseURL)
	credentialPattern := ""
	if baseURL != "" {
		origin, _, originErr := catalogCredentialOrigin(baseURL)
		if originErr != nil {
			return nil, fmt.Errorf("base_url do perfil %q: %w", server.ID, originErr)
		}
		if origin != profile.credentialOrigin {
			return nil, fmt.Errorf("base_url do perfil %q aponta para origem %q fora da allowlist", server.ID, origin)
		}
	}

	defaultModel := CatalogFieldDefault(server, "default_model_id", server.DefaultModelID)
	id, err := catalogProviderID(server.ID)
	if err != nil {
		return nil, err
	}
	if authMode != llm.AuthModeNone {
		credentialPattern = "provider:" + id
	}
	if apiFormat == llm.APIFormatGoogle && authMode != llm.AuthModeRequired {
		return nil, fmt.Errorf("perfil Google exige autentica\u00e7\u00e3o obrigat\u00f3ria")
	}
	return &llm.ProviderConfig{
		ID: id, Name: server.Name, Type: profile.providerType,
		ErrorFamily:     catalogErrorFamily(server.ProviderFamily),
		ServerProfileID: server.ID, APIFormat: apiFormat, BaseURL: baseURL,
		Model: defaultModel, DefaultModel: defaultModel, Timeout: 180,
		CredentialPattern: credentialPattern, AuthMode: authMode,
		ReasoningContentMode: profile.reasoningContentMode,
	}, nil
}

// CatalogFieldDefault aplica a precedência de valores do contrato AEP-0115.
func CatalogFieldDefault(server llmcatalog.Server, field, fallback string) string {
	policy, ok := server.Fields[field]
	if !ok {
		return fallback
	}
	value, ok := policy.Default.(string)
	if policy.Policy == "fixed" || policy.Policy == "hidden" || policy.Policy == "choice" {
		if ok {
			return value
		}
		return fallback
	}
	if fallback != "" {
		return fallback
	}
	if ok {
		return value
	}
	return fallback
}

func catalogCredentialOrigin(rawURL string) (origin, hostname string, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname = strings.ToLower(parsed.Hostname())
	if (scheme != "http" && scheme != "https") || hostname == "" || parsed.User != nil {
		return "", "", fmt.Errorf("origem inválida")
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	origin = scheme + "://" + hostname
	if port != "" {
		origin += ":" + port
	}
	return origin, hostname, nil
}

// CatalogTemplate resolve um perfil explicitamente identificado no snapshot
// atual. Snapshot vazio/inexistente retorna erro; presets de provedores não são
// duplicados localmente como fallback.
func (s *Service) CatalogTemplate(profileID string) (*llm.ProviderConfig, error) {
	if s.catalogSnapshot == nil {
		return nil, fmt.Errorf("catálogo de provedores indisponível")
	}
	snapshot := s.catalogSnapshot()
	for _, server := range snapshot.Document.Servers {
		if server.ID == profileID {
			return TemplateFromCatalogServer(server)
		}
	}
	return nil, fmt.Errorf("perfil de servidor %q não encontrado no catálogo", profileID)
}

// CreateFromCatalogProfile cria um provedor a partir do ID de perfil escolhido
// explicitamente no catálogo atual. O snapshot não contém credenciais; a chave
// fornecida é salva no gerenciador local de credenciais.
func (s *Service) CreateFromCatalogProfile(ctx context.Context, profileID, apiKey string) error {
	p, err := s.CatalogTemplate(profileID)
	if err != nil {
		return err
	}
	return s.createFromTemplate(ctx, p, apiKey)
}

var legacyTemplateProfiles = map[string]string{
	"openai": "openai", "claude": "anthropic", "google": "google-gemini",
	"openrouter": "openrouter", "mistral": "mistral", "groq": "groq",
	"together": "together-ai", "fireworks": "fireworks-ai", "perplexity": "perplexity",
	"deepseek": "deepseek", "grok": "xai", "localai": "localai-openai-compatible",
	"llamacpp": "llamacpp", "ollama": "ollama",
}

type catalogRuntimeProfile struct {
	credentialOrigin     string
	providerFamily       string
	apiFormat            string
	providerID           string
	providerType         llm.ProviderType
	reasoningContentMode llm.ReasoningContentMode
}

// A identidade do perfil, sua família e seu formato formam uma allowlist
// indivisível: um perfil não pode assumir a família/formato de outro.
var catalogRuntimeProfiles = map[string]catalogRuntimeProfile{
	"openai":                    {credentialOrigin: "https://api.openai.com", providerFamily: "openai", apiFormat: "openai_responses", providerID: "openai-default", providerType: llm.ProviderOpenAI},
	"anthropic":                 {credentialOrigin: "https://api.anthropic.com", providerFamily: "anthropic", apiFormat: "anthropic", providerID: "anthropic-claude", providerType: llm.ProviderClaude},
	"google-gemini":             {credentialOrigin: "https://generativelanguage.googleapis.com", providerFamily: "google", apiFormat: "google", providerID: "google-gemini", providerType: llm.ProviderOpenAI},
	"xai":                       {credentialOrigin: "https://api.x.ai", providerFamily: "xai", apiFormat: "openai", providerID: "xai-grok", providerType: llm.ProviderGrok},
	"deepseek":                  {credentialOrigin: "https://api.deepseek.com", providerFamily: "deepseek", apiFormat: "openai", providerID: "deepseek-default", providerType: llm.ProviderDeepSeek, reasoningContentMode: llm.ReasoningContentReplayWithTools},
	"mistral":                   {credentialOrigin: "https://api.mistral.ai", providerFamily: "mistral", apiFormat: "openai", providerID: "mistral-default", providerType: llm.ProviderMistral},
	"groq":                      {credentialOrigin: "https://api.groq.com", providerFamily: "groq", apiFormat: "openai", providerID: "groq-default", providerType: llm.ProviderGroq},
	"together-ai":               {credentialOrigin: "https://api.together.xyz", providerFamily: "together", apiFormat: "openai", providerID: "together-default", providerType: llm.ProviderTogether},
	"fireworks-ai":              {credentialOrigin: "https://api.fireworks.ai", providerFamily: "fireworks", apiFormat: "openai", providerID: "fireworks-default", providerType: llm.ProviderFireworks},
	"perplexity":                {credentialOrigin: "https://api.perplexity.ai", providerFamily: "perplexity", apiFormat: "openai", providerID: "perplexity-default", providerType: llm.ProviderPerplexity},
	"zai-global":                {credentialOrigin: "https://api.z.ai", providerFamily: "zai", apiFormat: "openai", providerID: "zai-global-default", providerType: llm.ProviderOpenAI},
	"zai-coding-plan":           {credentialOrigin: "https://api.z.ai", providerFamily: "zai", apiFormat: "openai", providerID: "zai-coding-plan-default", providerType: llm.ProviderOpenAI},
	"openrouter":                {credentialOrigin: "https://openrouter.ai", providerFamily: "openrouter", apiFormat: "openai", providerID: "openrouter-default", providerType: llm.ProviderOpenAI},
	"litellm-openai-proxy":      {credentialOrigin: "http://localhost:4000", providerFamily: "litellm", apiFormat: "openai", providerID: "litellm-default", providerType: llm.ProviderOpenAI},
	"localai-openai-compatible": {credentialOrigin: "http://localhost:8080", providerFamily: "localai", apiFormat: "openai", providerID: "localai-local", providerType: llm.ProviderLocalAI},
	"llamacpp":                  {credentialOrigin: "http://localhost:8080", providerFamily: "llamacpp", apiFormat: "openai", providerID: "llamacpp-local", providerType: llm.ProviderLlamaCPP},
}

func catalogRuntimeProfileFor(server llmcatalog.Server, apiFormat string) (catalogRuntimeProfile, error) {
	profile, ok := catalogRuntimeProfiles[server.ID]
	if !ok {
		switch server.ID {
		case "cohere":
			return catalogRuntimeProfile{}, fmt.Errorf("perfil Cohere ainda não é suportado: o runtime não representa o endpoint catalogado /compatibility/v1/chat/completions")
		case "ollama":
			return catalogRuntimeProfile{}, fmt.Errorf("o formato ollama ainda não é suportado pelo runtime de provedores")
		default:
			return catalogRuntimeProfile{}, fmt.Errorf("perfil de servidor %q não tem combinação de runtime allowlisted", server.ID)
		}
	}
	if server.ProviderFamily != profile.providerFamily || apiFormat != profile.apiFormat {
		return catalogRuntimeProfile{}, fmt.Errorf("perfil %q requer provider_family=%q e api_format=%q", server.ID, profile.providerFamily, profile.apiFormat)
	}
	return profile, nil
}

func catalogProviderID(profileID string) (string, error) {
	profile, ok := catalogRuntimeProfiles[profileID]
	if !ok {
		return "", fmt.Errorf("perfil de servidor %q não tem identidade de runtime allowlisted", profileID)
	}
	return profile.providerID, nil
}

func catalogAPIFormat(value string) (llm.APIFormat, error) {
	switch value {
	case "openai":
		return llm.APIFormatOpenAI, nil
	case "openai_responses":
		return llm.APIFormatOpenAIResponses, nil
	case "anthropic":
		return llm.APIFormatAnthropic, nil
	case "google":
		return llm.APIFormatGoogle, nil
	default:
		return "", fmt.Errorf("api_format %q não está na allowlist do runtime", value)
	}
}

// CatalogAuthMode traduz autenticação declarativa para o modo do runtime.
func CatalogAuthMode(mode string) (llm.AuthMode, error) {
	switch mode {
	case "bearer_api_key":
		return llm.AuthModeRequired, nil
	case "api_key_header":
		return llm.AuthModeRequired, nil
	case "optional_api_key":
		return llm.AuthModeOptional, nil
	case "none":
		return llm.AuthModeNone, nil
	}
	return "", fmt.Errorf("modo de autenticação %q não está na allowlist do runtime", mode)
}

func catalogErrorFamily(family string) llm.ProviderErrorFamily {
	switch family {
	case "openai":
		return llm.ProviderErrorFamilyOpenAI
	case "openrouter":
		return llm.ProviderErrorFamilyOpenRouter
	case "litellm":
		return llm.ProviderErrorFamilyLiteLLM
	default:
		return ""
	}
}

// AgentProvider monta o provedor de um agente de código.
//
// Ele é o mesmo para os 38 agentes do catálogo: o que muda entre eles é o
// identificador, o nome e o comando, e nada disso é código (AEP-0086 D11).
//
// Nada de URL, credencial ou modelo: um agente não tem endereço, o login é
// feito fora do app (AEP-0084 D12) e a lista de modelos vem da sessão dele.
func AgentProvider(id, name, agentID string, install acp.Install) *llm.ProviderConfig {
	return &llm.ProviderConfig{
		ID:         id,
		Name:       name,
		Type:       llm.ProviderACP,
		APIFormat:  llm.APIFormatACP,
		ACPAgentID: agentID,
		// Um turno de agente de código roda ferramentas e edita arquivos: ele
		// demora mais que uma resposta de LLM, e o teto dos provedores locais é
		// o que mais se aproxima disso.
		Timeout:  300,
		AuthMode: llm.AuthModeNone,
		// A cópia dos argumentos é o que impede o provider de seguir a lista de
		// quem detectou, que é reaproveitada por quem chamou.
		ACPCommand: install.Command,
		ACPArgs:    slices.Clone(install.Args),
	}
}

// CreateFromTemplate preserva a entrada legada da CLI, mas resolve os dados
// exclusivamente do perfil correspondente no catálogo atual.
//
// Só provedor HTTP passa por aqui: agente de código não tem template, e quem
// cria um é o formulário, com o agente escolhido no catálogo (AEP-0086 D11).
func (s *Service) CreateFromTemplate(ctx context.Context, providerType, apiKey string) error {
	p, err := s.TemplateForProviderType(providerType)
	if err != nil {
		return err
	}
	return s.createFromTemplate(ctx, p, apiKey)
}

// TemplateForProviderType resolves a legacy type using the current catalog.
func (s *Service) TemplateForProviderType(providerType string) (*llm.ProviderConfig, error) {
	profileID, ok := legacyTemplateProfiles[providerType]
	if !ok {
		return nil, fmt.Errorf("tipo de provedor %q não possui perfil no catálogo", providerType)
	}
	return s.CatalogTemplate(profileID)
}

func (s *Service) createFromTemplate(ctx context.Context, p *llm.ProviderConfig, apiKey string) error {
	generation := s.registry.Generation()
	var draft *CredentialSpec
	if apiKey != "" && p.EffectiveAuthMode() != llm.AuthModeNone {
		auth := &credentials.AuthConfig{Source: "static", Type: "bearer", Token: apiKey}
		if p.GetAPIFormat() == llm.APIFormatAnthropic {
			auth = &credentials.AuthConfig{Source: "static", Type: "custom", Headers: map[string]string{"x-api-key": apiKey}}
		}
		draft = &CredentialSpec{Pattern: p.CredentialPattern, Auth: auth}
	}
	if draft == nil {
		s.publicationMu.Lock()
		defer s.publicationMu.Unlock()
	}
	// Creating a preset never replaces an existing connection.
	if s.registry.Get(p.ID) != nil {
		return fmt.Errorf("provider com ID '%s' já existe", p.ID)
	}
	if err := validateCredentialMode(p, draft); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if draft != nil {
		scope, err := s.captureCredentialSave(ctx, draft, generation)
		if err != nil {
			return err
		}
		return s.saveWithCredential(ctx, p, nil, draft, scope)
	}
	p.IsDefault = len(s.registry.List()) == 0
	return s.saveWithoutCredentialDraft(ctx, p, generation, "", p.IsDefault)
}

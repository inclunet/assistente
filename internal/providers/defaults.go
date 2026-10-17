package providers

import (
	"context"
	"fmt"
	"slices"

	"assistente/internal/acp"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
	"assistente/internal/logging"
)

// BuiltinTemplate retorna um ProviderConfig pré-configurado para um tipo conhecido.
// providerType deve ser um dos: "openai", "claude", "google", "openrouter", "mistral",
// "groq", "together", "fireworks", "perplexity", "deepseek", "grok", "ollama",
// "localai", "llamacpp".
// Retorna (nil, ErrUnknownProviderType) se o tipo não for reconhecido.
//
// Agente de código não tem template aqui, e é de propósito (AEP-0086 D11). Um
// template descreve um serviço que existe no mesmo lugar para todo mundo; um
// agente é um processo que mora onde a máquina o instalou, e qual agente subir
// é escolha de quem configura, feita no catálogo. Enumerá-los aqui seria
// reconstruir a lista escrita à mão que esta fase apagou.
func BuiltinTemplate(providerType string) (*llm.ProviderConfig, error) {
	switch providerType {
	case "openai":
		return &llm.ProviderConfig{
			ID:                "openai-default",
			Name:              "OpenAI",
			Type:              llm.ProviderOpenAI,
			ErrorFamily:       llm.ProviderErrorFamilyOpenAI,
			APIFormat:         llm.APIFormatOpenAIResponses,
			BaseURL:           "https://api.openai.com/v1",
			Model:             "gpt-4o-mini",
			DefaultModel:      "gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "api.openai.com",
		}, nil
	case "claude":
		return &llm.ProviderConfig{
			ID:                "anthropic-claude",
			Name:              "Claude (Anthropic)",
			Type:              llm.ProviderClaude,
			BaseURL:           "https://api.anthropic.com",
			Model:             "claude-3-7-sonnet-20250219",
			DefaultModel:      "claude-3-7-sonnet-20250219",
			Timeout:           180,
			CredentialPattern: "api.anthropic.com",
		}, nil
	case "google":
		return &llm.ProviderConfig{
			ID:                "google-gemini",
			Name:              "Google (Gemini)",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai/",
			Model:             "gemini-2.0-flash",
			DefaultModel:      "gemini-2.0-flash",
			Timeout:           180,
			CredentialPattern: "generativelanguage.googleapis.com",
		}, nil
	case "openrouter":
		return &llm.ProviderConfig{
			ID:                "openrouter-default",
			Name:              "OpenRouter",
			Type:              llm.ProviderOpenAI,
			ErrorFamily:       llm.ProviderErrorFamilyOpenRouter,
			BaseURL:           "https://openrouter.ai/api/v1",
			Model:             "openai/gpt-4o-mini",
			DefaultModel:      "openai/gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "openrouter.ai",
		}, nil
	case "mistral":
		return &llm.ProviderConfig{
			ID:                "mistral-default",
			Name:              "Mistral AI",
			Type:              llm.ProviderMistral,
			BaseURL:           "https://api.mistral.ai/v1",
			Model:             "mistral-large-latest",
			DefaultModel:      "mistral-large-latest",
			Timeout:           180,
			CredentialPattern: "api.mistral.ai",
		}, nil
	case "groq":
		return &llm.ProviderConfig{
			ID:                "groq-default",
			Name:              "Groq",
			Type:              llm.ProviderGroq,
			BaseURL:           "https://api.groq.com/openai/v1",
			Model:             "llama-3.3-70b-versatile",
			DefaultModel:      "llama-3.3-70b-versatile",
			Timeout:           180,
			CredentialPattern: "api.groq.com",
		}, nil
	case "together":
		return &llm.ProviderConfig{
			ID:                "together-default",
			Name:              "Together AI",
			Type:              llm.ProviderTogether,
			BaseURL:           "https://api.together.xyz/v1",
			Model:             "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			DefaultModel:      "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Timeout:           180,
			CredentialPattern: "api.together.xyz",
		}, nil
	case "fireworks":
		return &llm.ProviderConfig{
			ID:                "fireworks-default",
			Name:              "Fireworks AI",
			Type:              llm.ProviderFireworks,
			BaseURL:           "https://api.fireworks.ai/inference/v1",
			Model:             "accounts/fireworks/models/llama-v3p3-70b-instruct",
			DefaultModel:      "accounts/fireworks/models/llama-v3p3-70b-instruct",
			Timeout:           180,
			CredentialPattern: "api.fireworks.ai",
		}, nil
	case "perplexity":
		return &llm.ProviderConfig{
			ID:                "perplexity-default",
			Name:              "Perplexity",
			Type:              llm.ProviderPerplexity,
			BaseURL:           "https://api.perplexity.ai",
			Model:             "sonar",
			DefaultModel:      "sonar",
			Timeout:           180,
			CredentialPattern: "api.perplexity.ai",
		}, nil
	case "deepseek":
		return &llm.ProviderConfig{
			ID:                   "deepseek-default",
			Name:                 "DeepSeek",
			Type:                 llm.ProviderDeepSeek,
			BaseURL:              "https://api.deepseek.com/v1",
			Model:                "deepseek-chat",
			DefaultModel:         "deepseek-chat",
			Timeout:              180,
			CredentialPattern:    "api.deepseek.com",
			ReasoningContentMode: llm.ReasoningContentReplayWithTools,
		}, nil
	case "grok":
		return &llm.ProviderConfig{
			ID:                "xai-grok",
			Name:              "xAI (Grok)",
			Type:              llm.ProviderGrok,
			BaseURL:           "https://api.x.ai/v1",
			Model:             "grok-2",
			DefaultModel:      "grok-2",
			Timeout:           180,
			CredentialPattern: "api.x.ai",
		}, nil
	case "ollama":
		return &llm.ProviderConfig{
			ID:                "ollama-local",
			Name:              "Ollama (Local)",
			Type:              llm.ProviderOllama,
			BaseURL:           "http://localhost:11434/api",
			Model:             "llama2",
			DefaultModel:      "llama2",
			Timeout:           300,
			CredentialPattern: "",
			AuthMode:          llm.AuthModeNone,
		}, nil
	case "localai":
		return &llm.ProviderConfig{
			ID:                "localai-local",
			Name:              "LocalAI",
			Type:              llm.ProviderLocalAI,
			APIFormat:         llm.APIFormatOpenAICompatible,
			BaseURL:           "http://localhost:8080/v1",
			Timeout:           180,
			CredentialPattern: "",
			AuthMode:          llm.AuthModeOptional,
		}, nil
	case "llamacpp":
		return &llm.ProviderConfig{
			ID:                "llamacpp-local",
			Name:              "llama.cpp (server)",
			Type:              llm.ProviderLlamaCPP,
			APIFormat:         llm.APIFormatOpenAICompatible,
			BaseURL:           "http://localhost:8080/v1",
			Timeout:           300,
			CredentialPattern: "",
			AuthMode:          llm.AuthModeNone,
		}, nil
	default:
		return nil, fmt.Errorf("tipo de provedor inválido: %s", providerType)
	}
}

// TemplateFromCatalogServer converte somente identificadores declarativos
// conhecidos pelo runtime. Os valores do catálogo nunca escolhem código ou
// adaptadores: família, formato e autenticação passam por allowlists locais.
func TemplateFromCatalogServer(server llmcatalog.Server) (*llm.ProviderConfig, error) {
	if server.Status != "active" {
		return nil, fmt.Errorf("perfil de servidor %q não está ativo", server.ID)
	}
	profile, err := catalogRuntimeProfileFor(server)
	if err != nil {
		return nil, err
	}
	apiFormat, err := catalogAPIFormat(server.APIFormat)
	if err != nil {
		return nil, err
	}
	authMode, err := catalogAuthMode(server.Auth.Mode, server.Auth.Required)
	if err != nil {
		return nil, err
	}

	baseURL := server.BaseURL
	if baseURL == "" {
		if value, ok := server.Fields["base_url"].Default.(string); ok {
			baseURL = value
		}
	}
	credentialPattern := ""
	if authMode != llm.AuthModeNone && baseURL != "" {
		credentialPattern, err = ExtractHostname(baseURL)
		if err != nil {
			return nil, fmt.Errorf("base_url do perfil %q: %w", server.ID, err)
		}
	}
	defaultModel := server.DefaultModelID
	if defaultModel == "" {
		if value, ok := server.Fields["default_model_id"].Default.(string); ok {
			defaultModel = value
		}
	}
	id, err := catalogProviderID(server.ID)
	if err != nil {
		return nil, err
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

// CatalogTemplate resolve um perfil explicitamente identificado no snapshot
// atual. Snapshot vazio/inexistente retorna erro para permitir fallback local.
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
	providerFamily       string
	apiFormat            string
	providerID           string
	providerType         llm.ProviderType
	reasoningContentMode llm.ReasoningContentMode
}

// A identidade do perfil, sua família e seu formato formam uma allowlist
// indivisível: um perfil não pode assumir a família/formato de outro.
var catalogRuntimeProfiles = map[string]catalogRuntimeProfile{
	"openai":                    {providerFamily: "openai", apiFormat: "openai_responses", providerID: "openai-default", providerType: llm.ProviderOpenAI},
	"anthropic":                 {providerFamily: "anthropic", apiFormat: "anthropic", providerID: "anthropic-claude", providerType: llm.ProviderClaude},
	"google-gemini":             {providerFamily: "google", apiFormat: "google", providerID: "google-gemini", providerType: llm.ProviderOpenAI},
	"xai":                       {providerFamily: "xai", apiFormat: "openai", providerID: "xai-grok", providerType: llm.ProviderGrok},
	"deepseek":                  {providerFamily: "deepseek", apiFormat: "openai", providerID: "deepseek-default", providerType: llm.ProviderDeepSeek, reasoningContentMode: llm.ReasoningContentReplayWithTools},
	"mistral":                   {providerFamily: "mistral", apiFormat: "openai", providerID: "mistral-default", providerType: llm.ProviderMistral},
	"groq":                      {providerFamily: "groq", apiFormat: "openai", providerID: "groq-default", providerType: llm.ProviderGroq},
	"together-ai":               {providerFamily: "together", apiFormat: "openai", providerID: "together-default", providerType: llm.ProviderTogether},
	"fireworks-ai":              {providerFamily: "fireworks", apiFormat: "openai", providerID: "fireworks-default", providerType: llm.ProviderFireworks},
	"perplexity":                {providerFamily: "perplexity", apiFormat: "openai", providerID: "perplexity-default", providerType: llm.ProviderPerplexity},
	"zai-global":                {providerFamily: "zai", apiFormat: "openai", providerID: "zai-global-default", providerType: llm.ProviderOpenAI},
	"zai-coding-plan":           {providerFamily: "zai", apiFormat: "openai", providerID: "zai-coding-plan-default", providerType: llm.ProviderOpenAI},
	"openrouter":                {providerFamily: "openrouter", apiFormat: "openai", providerID: "openrouter-default", providerType: llm.ProviderOpenAI},
	"litellm-openai-proxy":      {providerFamily: "litellm", apiFormat: "openai", providerID: "litellm-default", providerType: llm.ProviderOpenAI},
	"localai-openai-compatible": {providerFamily: "localai", apiFormat: "openai", providerID: "localai-local", providerType: llm.ProviderLocalAI},
	"llamacpp":                  {providerFamily: "llamacpp", apiFormat: "openai", providerID: "llamacpp-local", providerType: llm.ProviderLlamaCPP},
}

func catalogRuntimeProfileFor(server llmcatalog.Server) (catalogRuntimeProfile, error) {
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
	if server.ProviderFamily != profile.providerFamily || server.APIFormat != profile.apiFormat {
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

func catalogAuthMode(mode string, required bool) (llm.AuthMode, error) {
	switch mode {
	case "bearer_api_key":
		if required {
			return llm.AuthModeRequired, nil
		}
	case "api_key_header":
		if required {
			return llm.AuthModeRequired, nil
		}
	case "optional_api_key":
		if !required {
			return llm.AuthModeOptional, nil
		}
	case "none":
		if !required {
			return llm.AuthModeNone, nil
		}
	}
	return "", fmt.Errorf("modo de autenticação %q incompatível com required=%t", mode, required)
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

// CreateFromTemplate cria um provedor a partir de um template builtin, registra-o no
// registry, salva a credencial (se apiKey fornecida) e persiste no store.
// É equivalente ao antigo CreateDefaultLLMProvider no app layer.
//
// Só provedor HTTP passa por aqui: agente de código não tem template, e quem
// cria um é o formulário, com o agente escolhido no catálogo (AEP-0086 D11).
func (s *Service) CreateFromTemplate(ctx context.Context, providerType, apiKey string) error {
	var p *llm.ProviderConfig
	var err error
	if profileID, ok := legacyTemplateProfiles[providerType]; ok {
		p, err = s.CatalogTemplate(profileID)
	}
	if p == nil {
		// Catálogo vazio, offline ou perfil ainda não suportado pelo runtime:
		// preserva o primeiro setup e a compatibilidade da CLI.
		p, err = BuiltinTemplate(providerType)
	}
	if err != nil {
		return err
	}
	return s.createFromTemplate(ctx, p, apiKey)
}

func (s *Service) createFromTemplate(ctx context.Context, p *llm.ProviderConfig, apiKey string) error {
	if err := s.registry.Register(p); err != nil {
		return fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	if apiKey != "" && p.CredentialPattern != "" {
		if err := s.credMgr.RegisterPatternWithContext(ctx, p.CredentialPattern, &credentials.AuthConfig{Source: "static",
			Type:  "bearer",
			Token: apiKey,
		}); err != nil {
			return fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	if err := s.Save(ctx); err != nil {
		return fmt.Errorf("erro ao salvar provedor: %w", err)
	}

	logging.Infof(ctx, "providers.defaults", "[providers] Provedor '%s' criado a partir do template", p.ID)
	return nil
}

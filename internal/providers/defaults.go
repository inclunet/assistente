package providers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"assistente/internal/acp"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/logging"
)

// ErrAgentNotFound diz que o agente de código do template não está instalado
// nesta máquina. É estado de instalação, não de configuração: quem chamou
// precisa distinguir isso de "tipo inválido" para dar a instrução certa —
// instalar o CLI resolve, mudar o formulário não (AEP-0084 Fase 3).
var ErrAgentNotFound = errors.New("agente de código não encontrado nesta máquina")

// BuiltinTemplate retorna um ProviderConfig pré-configurado para um tipo conhecido.
// providerType deve ser um dos: "openai", "claude", "google", "openrouter", "mistral",
// "groq", "together", "fireworks", "perplexity", "deepseek", "grok", "ollama",
// "localai", "llamacpp", "cursor".
// Retorna (nil, ErrUnknownProviderType) se o tipo não for reconhecido.
//
// O template de agente de código ("cursor") é o único que consulta a máquina:
// o que endereça um agente é o comando dele, e o comando depende de onde o CLI
// foi instalado (AEP-0084 D15). Sem instalação ele devolve ErrAgentNotFound.
func BuiltinTemplate(providerType string) (*llm.ProviderConfig, error) {
	switch providerType {
	case "cursor":
		install, err := acp.DetectAgent(acp.AgentKindCursor)
		if err != nil {
			return nil, err
		}
		if !install.Found {
			return nil, fmt.Errorf("cursor: %w", ErrAgentNotFound)
		}
		return CursorTemplate(install), nil
	case "openai":
		return &llm.ProviderConfig{
			ID:                "openai-default",
			Name:              "OpenAI",
			Type:              llm.ProviderOpenAI,
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
			BaseURL:           "https://api.anthropic.com/v1",
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
			ID:                "deepseek-default",
			Name:              "DeepSeek",
			Type:              llm.ProviderDeepSeek,
			BaseURL:           "https://api.deepseek.com/v1",
			Model:             "deepseek-chat",
			DefaultModel:      "deepseek-chat",
			Timeout:           180,
			CredentialPattern: "api.deepseek.com",
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

// CursorTemplate monta o provider do Cursor a partir de uma instalação já
// encontrada. Fica separado da detecção para poder ser testado sem depender de
// o CLI estar instalado em quem roda o teste.
//
// Nada de URL, credencial ou modelo: um agente não tem endereço, o login é
// feito no CLI dele (AEP-0084 D12) e a lista de modelos vem da sessão, que é
// assunto da fase seguinte.
func CursorTemplate(install acp.Install) *llm.ProviderConfig {
	return &llm.ProviderConfig{
		ID:        "cursor-agent",
		Name:      "Cursor (agente de código)",
		Type:      llm.ProviderCursor,
		APIFormat: llm.APIFormatACP,
		// Um turno de agente de código roda ferramentas e edita arquivos: ele
		// demora mais que uma resposta de LLM, e o teto dos provedores locais é
		// o que mais se aproxima disso.
		Timeout:    300,
		AuthMode:   llm.AuthModeNone,
		ACPCommand: install.Command,
		ACPArgs:    slices.Clone(install.Args),
	}
}

// CreateFromTemplate cria um provedor a partir de um template builtin, registra-o no
// registry, salva a credencial (se apiKey fornecida) e persiste no store.
// É equivalente ao antigo CreateDefaultLLMProvider no app layer.
func (s *Service) CreateFromTemplate(ctx context.Context, providerType, apiKey string) error {
	p, err := BuiltinTemplate(providerType)
	if err != nil {
		return err
	}

	// Recusar em vez de ignorar: quem informou uma chave espera que ela
	// autentique alguma coisa, e num agente de código o login é feito no CLI
	// dele, fora do app (AEP-0084 D12).
	if p.IsACP() && apiKey != "" {
		return fmt.Errorf("provedor acp não guarda credencial no app; autentique o agente pelo CLI dele")
	}

	if err := s.registry.Register(p); err != nil {
		return fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	if apiKey != "" && p.CredentialPattern != "" {
		if err := s.credMgr.RegisterPatternWithContext(ctx, p.CredentialPattern, &credentials.AuthConfig{
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

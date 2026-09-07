package providers

import (
	"context"
	"fmt"
	"slices"

	"assistente/internal/acp"
	"assistente/internal/credentials"
	"assistente/internal/llm"
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
	p, err := BuiltinTemplate(providerType)
	if err != nil {
		return err
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

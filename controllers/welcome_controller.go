package controllers

import (
	"context"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
)

// ============================================================================
// Wizard Provider Metadata
// ============================================================================

// WizardProviderInfo mapeia a escolha do wizard para configuração do provedor.
type WizardProviderInfo struct {
	ID                   string
	Name                 string
	Type                 llm.ProviderType
	ErrorFamily          llm.ProviderErrorFamily
	APIFormat            llm.APIFormat
	DefaultModel         string
	ReasoningContentMode llm.ReasoningContentMode
}

// WizardLabelToProviderType mapeia o rótulo exibido no wizard para o type ID
// usado pela entrada legada de perfis do catálogo. Retorna "" para provedores sem template canônico.
func WizardLabelToProviderType(label string) string {
	switch label {
	case "OpenAI":
		return "openai"
	case "Anthropic (Claude)":
		return "claude"
	case "Google (Gemini)":
		return "google"
	case "DeepSeek":
		return "deepseek"
	case "xAI (Grok)":
		return "grok"
	case "OpenRouter":
		return "openrouter"
	case "Mistral AI":
		return "mistral"
	case "Groq":
		return "groq"
	case "Together AI":
		return "together"
	case "Fireworks AI":
		return "fireworks"
	case "Perplexity":
		return "perplexity"
	case "Ollama (Local)":
		return "ollama"
	default:
		return ""
	}
}

// GetWizardProviderInfo retorna ID, nome, tipo e modelo default sugerido para a escolha do wizard.
func GetWizardProviderInfo(providerChoice string) WizardProviderInfo {
	switch providerChoice {
	case "OpenAI":
		return WizardProviderInfo{ID: "openai-default", Name: "OpenAI", Type: llm.ProviderOpenAI, ErrorFamily: llm.ProviderErrorFamilyOpenAI, APIFormat: llm.APIFormatOpenAIResponses, DefaultModel: "gpt-4o-mini"}
	case "Anthropic (Claude)":
		return WizardProviderInfo{ID: "anthropic-claude", Name: "Claude (Anthropic)", Type: llm.ProviderClaude, APIFormat: llm.APIFormatAnthropic, DefaultModel: "claude-sonnet-4-20250514"}
	case "Google (Gemini)":
		return WizardProviderInfo{ID: "google-gemini", Name: "Google (Gemini)", Type: llm.ProviderOpenAI, DefaultModel: "gemini-2.0-flash"}
	case "OpenRouter":
		return WizardProviderInfo{ID: "openrouter-default", Name: "OpenRouter", Type: llm.ProviderOpenAI, ErrorFamily: llm.ProviderErrorFamilyOpenRouter}
	case "Mistral AI":
		return WizardProviderInfo{ID: "mistral-default", Name: "Mistral AI", Type: llm.ProviderMistral, DefaultModel: "mistral-small-latest"}
	case "Groq":
		return WizardProviderInfo{ID: "groq-default", Name: "Groq", Type: llm.ProviderGroq, DefaultModel: "llama-3.3-70b-versatile"}
	case "Together AI":
		return WizardProviderInfo{ID: "together-default", Name: "Together AI", Type: llm.ProviderTogether}
	case "Fireworks AI":
		return WizardProviderInfo{ID: "fireworks-default", Name: "Fireworks AI", Type: llm.ProviderFireworks}
	case "Perplexity":
		return WizardProviderInfo{ID: "perplexity-default", Name: "Perplexity", Type: llm.ProviderPerplexity, DefaultModel: "sonar"}
	case "DeepSeek":
		return WizardProviderInfo{
			ID: "deepseek-default", Name: "DeepSeek", Type: llm.ProviderDeepSeek,
			DefaultModel: "deepseek-chat", ReasoningContentMode: llm.ReasoningContentReplayWithTools,
		}
	case "xAI (Grok)":
		return WizardProviderInfo{ID: "xai-grok", Name: "xAI (Grok)", Type: llm.ProviderGrok, DefaultModel: "grok-3-mini"}
	case "Azure OpenAI":
		return WizardProviderInfo{ID: "azure-openai", Name: "Azure OpenAI", Type: llm.ProviderOpenAI}
	case "Ollama (Local)":
		return WizardProviderInfo{ID: "ollama-local", Name: "Ollama (Local)", Type: llm.ProviderOllama}
	case "LiteLLM":
		return WizardProviderInfo{ID: "litellm", Name: "LiteLLM", Type: llm.ProviderOpenAI, ErrorFamily: llm.ProviderErrorFamilyLiteLLM}
	default:
		return WizardProviderInfo{ID: "custom", Name: "Custom Provider", Type: llm.ProviderCustom}
	}
}

// ============================================================================
// WelcomeController
// ============================================================================

// WelcomeControllerConfig agrupa dependências do WelcomeController.
type WelcomeControllerConfig struct {
	QuestionnaireMgr           *questionnaire.Manager
	ProviderSvc                *providers.Service
	ConfigureCredentialManager func(dek []byte, persist bool)
}

type WelcomeController struct {
	questionnaireMgr           *questionnaire.Manager
	providerSvc                *providers.Service
	configureCredentialManager func(dek []byte, persist bool)
}

func NewWelcomeController(cfg WelcomeControllerConfig) *WelcomeController {
	return &WelcomeController{questionnaireMgr: cfg.QuestionnaireMgr, providerSvc: cfg.ProviderSvc, configureCredentialManager: cfg.ConfigureCredentialManager}
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas.
// Retorna true se não houver chave mestra ou provedor configurado.
func (c *WelcomeController) NeedsWelcomeWizard(ctx context.Context) bool {
	store := credentials.NewDBStore()
	hasMasterKey, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster)
	if err != nil {
		return true
	}

	hasProviders := false
	if c.providerSvc != nil {
		count, _ := c.providerSvc.Count(ctx)
		hasProviders = count > 0
	}

	return !hasProviders || !hasMasterKey
}

// RunWelcomeWizard prepara o cofre para o onboarding. O retorno true significa
// que senha e recuperacao foram configuradas; o frontend conclui o onboarding
// com o ProviderForm compartilhado, pela API autenticada de provedores.
func (c *WelcomeController) RunWelcomeWizard(ctx context.Context) (bool, error) {
	store := credentials.NewDBStore()
	configured, err := store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)
	if err != nil {
		return false, err
	}
	if configured {
		return true, nil
	}
	var passwordError questionnaire.Text
	for {
		response, err := c.questionnaireMgr.RequestQuestionnaire(ctx, welcomeMasterPasswordPayload(passwordError))
		if err != nil || response.Cancelled {
			return false, err
		}
		password, _ := response.Answers["masterPassword"].(string)
		confirmation, _ := response.Answers["confirmPassword"].(string)
		if strings.TrimSpace(password) == "" || password != confirmation {
			passwordError = welcomePasswordMismatch()
			continue
		}
		result, err := credentials.SetupMasterKeyAdoptingKeychain(store, password)
		if err != nil {
			return false, err
		}
		if c.configureCredentialManager != nil {
			c.configureCredentialManager(result.DEK, true)
		}
		response, err = c.questionnaireMgr.RequestQuestionnaire(ctx, welcomeRecoveryCodePayload(result.RecoveryKey))
		if err != nil || response.Cancelled {
			return false, err
		}
		return true, nil
	}
}

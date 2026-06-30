package controllers

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/updater"
)

// ============================================================================
// Wizard Provider Metadata
// ============================================================================

// WizardProviderInfo mapeia a escolha do wizard para configuração do provedor.
type WizardProviderInfo struct {
	ID           string
	Name         string
	Type         llm.ProviderType
	APIFormat    llm.APIFormat
	DefaultModel string
}

// WizardLabelToProviderType mapeia o rótulo exibido no wizard para o type ID
// usado por providers.BuiltinTemplate. Retorna "" para provedores sem template canônico.
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
		return WizardProviderInfo{ID: "openai-default", Name: "OpenAI", Type: llm.ProviderOpenAI, APIFormat: llm.APIFormatOpenAIResponses, DefaultModel: "gpt-4o-mini"}
	case "Anthropic (Claude)":
		return WizardProviderInfo{ID: "anthropic-claude", Name: "Claude (Anthropic)", Type: llm.ProviderClaude, APIFormat: llm.APIFormatAnthropic, DefaultModel: "claude-sonnet-4-20250514"}
	case "Google (Gemini)":
		return WizardProviderInfo{ID: "google-gemini", Name: "Google (Gemini)", Type: llm.ProviderOpenAI, DefaultModel: "gemini-2.0-flash"}
	case "OpenRouter":
		return WizardProviderInfo{ID: "openrouter-default", Name: "OpenRouter", Type: llm.ProviderOpenAI}
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
		return WizardProviderInfo{ID: "deepseek-default", Name: "DeepSeek", Type: llm.ProviderDeepSeek, DefaultModel: "deepseek-chat"}
	case "xAI (Grok)":
		return WizardProviderInfo{ID: "xai-grok", Name: "xAI (Grok)", Type: llm.ProviderGrok, DefaultModel: "grok-3-mini"}
	case "Azure OpenAI":
		return WizardProviderInfo{ID: "azure-openai", Name: "Azure OpenAI", Type: llm.ProviderOpenAI}
	case "Ollama (Local)":
		return WizardProviderInfo{ID: "ollama-local", Name: "Ollama (Local)", Type: llm.ProviderOllama}
	case "LiteLLM":
		return WizardProviderInfo{ID: "litellm", Name: "LiteLLM", Type: llm.ProviderOpenAI}
	default:
		return WizardProviderInfo{ID: "custom", Name: "Custom Provider", Type: llm.ProviderCustom}
	}
}

// ============================================================================
// WelcomeController
// ============================================================================

// WelcomeControllerConfig agrupa dependências do WelcomeController.
type WelcomeControllerConfig struct {
	QuestionnaireMgr *questionnaire.Manager
	CredMgr          *credentials.Manager
	ProviderSvc      *providers.Service
	LLMRegistry      *llm.ProviderRegistry
	Updater          *updater.Updater
	UpdaterCtrl      *UpdaterController

	// Callbacks para operações que pertencem à camada App (infra).
	ConfigureCredentialManager func(dek []byte, persist bool)
	InitLLMClient              func()
	SaveLLMProviders           func() error
}

// WelcomeController expõe o wizard de boas-vindas e verificações iniciais.
type WelcomeController struct {
	questionnaireMgr           *questionnaire.Manager
	credMgr                    *credentials.Manager
	providerSvc                *providers.Service
	llmRegistry                *llm.ProviderRegistry
	updater                    *updater.Updater
	updaterCtrl                *UpdaterController
	configureCredentialManager func(dek []byte, persist bool)
	initLLMClient              func()
	saveLLMProviders           func() error
}

// NewWelcomeController cria um WelcomeController com as dependências fornecidas.
func NewWelcomeController(cfg WelcomeControllerConfig) *WelcomeController {
	return &WelcomeController{
		questionnaireMgr:           cfg.QuestionnaireMgr,
		credMgr:                    cfg.CredMgr,
		providerSvc:                cfg.ProviderSvc,
		llmRegistry:                cfg.LLMRegistry,
		updater:                    cfg.Updater,
		updaterCtrl:                cfg.UpdaterCtrl,
		configureCredentialManager: cfg.ConfigureCredentialManager,
		initLLMClient:              cfg.InitLLMClient,
		saveLLMProviders:           cfg.SaveLLMProviders,
	}
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

// RunWelcomeWizard executa o wizard de boas-vindas.
// Retorna true se completou com sucesso, false se cancelado.
func (c *WelcomeController) RunWelcomeWizard(ctx context.Context) (bool, error) {
	var provider string
	var baseURL string
	var apiKey string
	var defaultModel string
	var recoveryKey string
	var passwordError string
	var urlError string
	var keyError string
	var validatedModels []string

	store := credentials.NewDBStore()
	masterKeyConfigured, _ := store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)

	currentStep := 0
	if masterKeyConfigured {
		currentStep = 2
	}

	for currentStep >= 0 {
		switch currentStep {
		case 0: // Etapa 0: Senha mestre
			description := "Defina uma senha mestre para criptografar credenciais locais. Guarde com cuidado."
			if passwordError != "" {
				description = passwordError
			}

			passwordResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Segurança: senha mestre",
				Description: description,
				Questions: []questionnaire.Question{
					{
						ID:          "masterPassword",
						Type:        "password",
						Prompt:      "Senha mestre",
						Required:    true,
						Placeholder: "Digite uma senha forte",
					},
					{
						ID:          "confirmPassword",
						Type:        "password",
						Prompt:      "Confirmar senha mestre",
						Required:    true,
						Placeholder: "Repita a senha",
					},
				},
				AllowCancel: true,
				SubmitLabel: "Continuar",
				CancelLabel: "Cancelar",
			})

			if err != nil || passwordResp.Cancelled {
				return false, err
			}

			masterPassword, _ := passwordResp.Answers["masterPassword"].(string)
			confirmPassword, _ := passwordResp.Answers["confirmPassword"].(string)
			if strings.TrimSpace(masterPassword) == "" || masterPassword != confirmPassword {
				passwordError = "As senhas não conferem. Tente novamente."
				currentStep = 0
				continue
			}

			setupResult, err := credentials.SetupMasterKeyAdoptingKeychain(store, masterPassword)
			if err != nil {
				return false, err
			}

			recoveryKey = setupResult.RecoveryKey
			if c.configureCredentialManager != nil {
				c.configureCredentialManager(setupResult.DEK, true)
			}
			passwordError = ""
			currentStep = 1

		case 1: // Etapa 1: Código de recuperação
			_, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Código de recuperação",
				Description: "Guarde este código em local seguro. Ele permite recuperar suas credenciais se você esquecer a senha mestre.",
				Questions: []questionnaire.Question{
					{
						ID:      "recoveryCode",
						Type:    "readonly_code",
						Prompt:  "Código de recuperação",
						Content: recoveryKey,
					},
					{
						ID:       "confirmed",
						Type:     "boolean",
						Prompt:   "Eu salvei o código de recuperação em local seguro",
						Required: true,
					},
				},
				AllowCancel: false,
				SubmitLabel: "Continuar",
			})
			if err != nil {
				return false, err
			}
			currentStep = 2

		case 2: // Etapa 2: Escolher provedor
			providerResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Bem-vindo ao Assistente!",
				Description: "Vamos configurar seu assistente em alguns passos simples.",
				Questions: []questionnaire.Question{
					{
						ID:       "provider",
						Type:     "single_choice",
						Prompt:   "Qual provedor de IA você deseja usar?",
						Required: true,
						Options: []string{
							"OpenAI",
							"Anthropic (Claude)",
							"Google (Gemini)",
							"DeepSeek",
							"xAI (Grok)",
							"OpenRouter",
							"Mistral AI",
							"Groq",
							"Together AI",
							"Fireworks AI",
							"Perplexity",
							"Azure OpenAI",
							"Ollama (Local)",
							"LiteLLM",
							"Outro (URL personalizada)",
						},
						Default: provider,
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Cancelar",
			})

			if err != nil || providerResp.Cancelled {
				return false, err
			}

			provider = providerResp.Answers["provider"].(string)

			if tmpl, err := providers.BuiltinTemplate(WizardLabelToProviderType(provider)); err == nil {
				baseURL = tmpl.BaseURL
			} else {
				baseURL = ""
			}

			currentStep = 3

		case 3: // Etapa 3: URL personalizada (se necessário)
			needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"

			if !needsCustomURL {
				currentStep = 4
				continue
			}

			placeholderURL := "http://localhost:11434/v1"
			switch provider {
			case "LiteLLM":
				placeholderURL = "http://localhost:4000"
			case "Azure OpenAI":
				placeholderURL = "https://your-resource.openai.azure.com"
			}

			urlDescription := "Informe a URL do servidor OpenAI-compatible."
			if urlError != "" {
				urlDescription = urlError
			}

			urlResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Configuração do Servidor",
				Description: urlDescription,
				Questions: []questionnaire.Question{
					{
						ID:          "baseURL",
						Type:        "text",
						Prompt:      "URL do servidor",
						Required:    true,
						Placeholder: placeholderURL,
						Default:     baseURL,
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Voltar",
			})

			if err != nil {
				return false, err
			}

			if urlResp.Cancelled {
				currentStep = 2
				continue
			}

			baseURL = urlResp.Answers["baseURL"].(string)
			urlError = ""

			if err := c.ValidateWizardURL(ctx, baseURL); err != nil {
				urlError = fmt.Sprintf("⚠️ %v\n\nCorreija a URL e tente novamente.", err)
				currentStep = 3
				continue
			}

			currentStep = 4

		case 4: // Etapa 4: API Key + validação de conexão
			keyDescription := "Informe sua chave de API. Deixe em branco se o servidor não requer autenticação."
			if provider == "Ollama (Local)" {
				keyDescription = "Ollama local geralmente não precisa de chave. Você pode deixar em branco."
			}
			if keyError != "" {
				keyDescription = keyError
			}

			keyResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Chave de API",
				Description: keyDescription,
				Questions: []questionnaire.Question{
					{
						ID:          "apiKey",
						Type:        "text",
						Prompt:      "Chave de API (opcional)",
						Required:    false,
						Placeholder: "sk-...",
						Default:     apiKey,
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Voltar",
			})

			if err != nil {
				return false, err
			}

			if keyResp.Cancelled {
				keyError = ""
				needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"
				if needsCustomURL {
					currentStep = 3
				} else {
					currentStep = 2
				}
				continue
			}

			if keyResp.Answers["apiKey"] != nil {
				apiKey = keyResp.Answers["apiKey"].(string)
			}
			keyError = ""

			logging.Errorf(ctx, "controllers.welcome-controller", "[Wizard] Validando conexão: %s (com key: %v)", baseURL, apiKey != "")
			validation := c.ValidateWizardConnection(ctx, baseURL, apiKey)

			needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"

			switch validation.ErrorType {
			case "url_invalid", "url_unreachable":
				if needsCustomURL {
					urlError = fmt.Sprintf("⚠️ %s", validation.ErrorDetail)
					currentStep = 3
				} else {
					keyError = fmt.Sprintf("⚠️ Não foi possível conectar ao servidor do %s (%s).\n\n%s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para escolher outro provedor.", provider, baseURL, validation.ErrorDetail)
					currentStep = 4
				}
				continue

			case "auth_required":
				keyError = fmt.Sprintf("⚠️ %s\n\nInforme uma API Key válida para continuar.", validation.ErrorDetail)
				currentStep = 4
				continue

			case "auth_invalid":
				keyError = fmt.Sprintf("⚠️ %s\n\nVerifique sua chave e tente novamente.", validation.ErrorDetail)
				currentStep = 4
				continue

			case "server_error":
				keyError = fmt.Sprintf("⚠️ %s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para alterar configurações.", validation.ErrorDetail)
				currentStep = 4
				continue
			}

			validatedModels = validation.Models
			logging.Infof(ctx, "controllers.welcome-controller", "[Wizard] Conexão validada com sucesso. Modelos disponíveis: %d", len(validatedModels))
			currentStep = 5

		case 5: // Etapa 5: Escolher modelo (conexão já validada)
			if len(validatedModels) > 0 {
				modelDefault := defaultModel
				if modelDefault == "" {
					modelDefault = validatedModels[0]
				}

				modelResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
					Title:       "Escolha o Modelo Padrão",
					Description: fmt.Sprintf("Conexão validada com sucesso! %d modelo(s) disponível(is).\n\nSelecione o modelo padrão. Você pode alterar depois nas configurações.", len(validatedModels)),
					Questions: []questionnaire.Question{
						{
							ID:       "model",
							Type:     "single_choice",
							Prompt:   "Modelo padrão:",
							Required: true,
							Options:  validatedModels,
							Default:  modelDefault,
						},
					},
					AllowCancel: true,
					SubmitLabel: "Finalizar",
					CancelLabel: "Voltar",
				})

				if err != nil {
					return false, err
				}

				if modelResp.Cancelled {
					currentStep = 4
					continue
				}

				defaultModel = modelResp.Answers["model"].(string)
			} else {
				manualResp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
					Title:       "Configurar Modelo",
					Description: "Conexão validada! O servidor não suporta listagem automática de modelos.\n\nInforme o nome do modelo que deseja usar.",
					Questions: []questionnaire.Question{
						{
							ID:          "defaultModel",
							Type:        "text",
							Prompt:      "Nome do modelo",
							Required:    true,
							Placeholder: "gpt-4o-mini",
							Default:     defaultModel,
						},
					},
					AllowCancel: true,
					SubmitLabel: "Finalizar",
					CancelLabel: "Voltar",
				})

				if err != nil {
					return false, err
				}

				if manualResp.Cancelled {
					currentStep = 4
					continue
				}

				defaultModel = manualResp.Answers["defaultModel"].(string)
			}

			// Registra credencial temporária para o CreateWizardProvider
			wizardHostname, _ := providers.ExtractHostname(baseURL)
			if apiKey != "" && wizardHostname != "" {
				wizardAuth := &credentials.AuthConfig{
					Type:  "bearer",
					Token: apiKey,
				}
				if err := c.credMgr.RegisterPatternWithContext(ctx, wizardHostname, wizardAuth); err != nil {
					logging.Errorf(ctx, "controllers.welcome-controller", "[Wizard] Erro ao registrar credencial temporária: %v", err)
				}
			}

			providerID, err := c.CreateWizardProvider(ctx, provider, baseURL, apiKey, defaultModel)
			if err != nil {
				return false, fmt.Errorf("erro ao criar provedor: %w", err)
			}

			if c.initLLMClient != nil {
				c.initLLMClient()
			}

			// Verificação final: confirma que o provider funciona com o modelo escolhido
			finalModels, finalErr := c.providerSvc.GetModelsByProvider(ctx, providerID)
			if finalErr != nil {
				logging.Infof(ctx, "controllers.welcome-controller", "[Wizard] Verificação final: %v (provider pode não suportar /models)", finalErr)
			} else {
				logging.Infof(ctx, "controllers.welcome-controller", "[Wizard] Verificação final OK: %d modelos via provider '%s'", len(finalModels), providerID)
				modelFound := false
				for _, m := range finalModels {
					if m == defaultModel {
						modelFound = true
						break
					}
				}
				if !modelFound && len(finalModels) > 0 {
					logging.Warnf(ctx, "controllers.welcome-controller", "[Wizard] Aviso: modelo '%s' não encontrado na lista do provider (%d modelos)", defaultModel, len(finalModels))
				}
			}

			go c.checkForUpdatesAfterWizard()
			return true, nil
		}
	}

	return false, nil
}

// ValidateWizardConnection testa URL, autenticação e lista modelos de um provedor.
func (c *WelcomeController) ValidateWizardConnection(ctx context.Context, baseURL, apiKey string) providers.ConnectionProbeResult {
	return c.providerSvc.ProbeConnection(ctx, baseURL, apiKey)
}

// ValidateWizardURL valida formato e alcançabilidade básica de uma URL personalizada.
func (c *WelcomeController) ValidateWizardURL(ctx context.Context, baseURL string) error {
	return providers.ValidateURL(ctx, baseURL)
}

// CreateWizardProvider cria o provedor LLM escolhido durante o wizard,
// registrando-o no registry, salvando a credencial e persistindo no SQLite.
func (c *WelcomeController) CreateWizardProvider(ctx context.Context, providerChoice, baseURL, apiKey, model string) (string, error) {
	info := GetWizardProviderInfo(providerChoice)

	hostname, err := providers.ExtractHostname(baseURL)
	if err != nil {
		return "", fmt.Errorf("erro ao extrair hostname de %s: %w", baseURL, err)
	}

	timeout := 180
	if info.Type == llm.ProviderOllama {
		timeout = 300
	}

	defaultModel := model
	if defaultModel == "" && info.DefaultModel != "" {
		defaultModel = info.DefaultModel
	}

	provider := &llm.ProviderConfig{
		ID:                info.ID,
		Name:              info.Name,
		Type:              info.Type,
		APIFormat:         info.APIFormat,
		BaseURL:           baseURL,
		Model:             model,
		DefaultModel:      defaultModel,
		IsDefault:         true,
		Timeout:           timeout,
		CredentialPattern: hostname,
	}

	if err := c.llmRegistry.Register(provider); err != nil {
		return "", fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	if apiKey != "" && hostname != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: apiKey,
		}
		if err := c.credMgr.RegisterPatternWithContext(ctx, hostname, authCfg); err != nil {
			return "", fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	if c.saveLLMProviders != nil {
		if err := c.saveLLMProviders(); err != nil {
			return "", fmt.Errorf("erro ao persistir provedor: %w", err)
		}
	}

	if err := c.providerSvc.SetDefault(ctx, info.ID); err != nil {
		logging.Warnf(ctx, "controllers.welcome-controller", "[Wizard] Aviso: erro ao marcar provedor como default: %v", err)
	}

	logging.Infof(ctx, "controllers.welcome-controller", "[Wizard] Provedor '%s' (%s) criado como default, modelo padrão: %s", info.ID, info.Name, defaultModel)
	return info.ID, nil
}

// checkForUpdatesAfterWizard verifica atualizações após o wizard de configuração.
func (c *WelcomeController) checkForUpdatesAfterWizard() {
	time.Sleep(2 * time.Second)

	if c.updater == nil {
		logging.Warnf(context.Background(), "controllers.welcome-controller", "[Wizard] Updater não inicializado, pulando verificação de atualizações")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logging.Errorf(context.Background(), "controllers.welcome-controller", "[Wizard] Verificando atualizações disponíveis...")

	info, err := c.updater.CheckForUpdates(ctx)
	if err != nil {
		logging.Errorf(context.Background(), "controllers.welcome-controller", "[Wizard] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		logging.Infof(context.Background(), "controllers.welcome-controller", "[Wizard] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	logging.Infof(context.Background(), "controllers.welcome-controller", "[Wizard] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	if c.updaterCtrl != nil {
		c.updaterCtrl.PromptForUpdate(ctx, info)
	}
}

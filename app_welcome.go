package main

import (
	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ==================== Welcome Wizard ====================

// wizardValidationResult é um alias local para providers.ConnectionProbeResult.
// Mantido como type alias para não alterar o código que o consome dentro deste arquivo.
type wizardValidationResult = providers.ConnectionProbeResult

// validateWizardConnection testa URL, autenticação e lista modelos de um provedor.
func (a *App) validateWizardConnection(baseURL, apiKey string) wizardValidationResult {
	return a.providerSvc.ProbeConnection(a.ctx, baseURL, apiKey)
}

// validateWizardURL valida formato e alcançabilidade básica de uma URL personalizada.
func (a *App) validateWizardURL(baseURL string) error {
	return providers.ValidateURL(a.ctx, baseURL)
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas
// Retorna true se não houver chave mestra ou provedor configurado
func (a *App) NeedsWelcomeWizard() bool {
	cfg, err := config.Load()
	if err != nil {
		return true
	}

	// Verifica se tem API key e base URL configurados
	hasConfig := cfg.APIKey != "" && cfg.APIBaseURL != ""

	store := credentials.NewDBStore()
	hasMasterKey, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster)
	if err != nil {
		return true
	}

	return !hasConfig || !hasMasterKey
}

// RunWelcomeWizard executa o wizard de boas-vindas
// Retorna true se completou com sucesso, false se cancelado
func (a *App) RunWelcomeWizard() (bool, error) {
	ctx := a.ctx

	// Variáveis para armazenar dados entre etapas
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

	// Controle de navegação entre etapas
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

			passwordResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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

			setupResult, err := credentials.SetupMasterKey(store, masterPassword)
			if err != nil {
				return false, err
			}

			recoveryKey = setupResult.RecoveryKey
			a.configureCredentialManager(setupResult.DEK, true)
			passwordError = ""
			currentStep = 1

		case 1: // Etapa 1: Código de recuperação
			_, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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
			providerResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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
						Default: provider, // Mantém seleção anterior se voltar
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

			// Obtém baseURL do template canônico; provedores que precisam de URL
			// personalizada (Azure, LiteLLM, Outro) deixam baseURL vazio.
			if tmpl, err := providers.BuiltinTemplate(wizardLabelToProviderType(provider)); err == nil {
				baseURL = tmpl.BaseURL
			} else {
				baseURL = "" // Azure OpenAI, LiteLLM, Outro (URL personalizada)
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

			urlResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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

			if err := a.validateWizardURL(baseURL); err != nil {
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

			keyResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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

			log.Printf("[Wizard] Validando conexão: %s (com key: %v)", baseURL, apiKey != "")
			validation := a.validateWizardConnection(baseURL, apiKey)

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
			log.Printf("[Wizard] Conexão validada com sucesso. Modelos disponíveis: %d", len(validatedModels))
			currentStep = 5

		case 5: // Etapa 5: Escolher modelo (conexão já validada)
			if len(validatedModels) > 0 {
				modelDefault := ""
				if defaultModel != "" {
					modelDefault = defaultModel
				} else {
					modelDefault = validatedModels[0]
				}

				modelResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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
				manualResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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

			// Registra credencial temporária para o createWizardProvider
			wizardHostname, _ := providers.ExtractHostname(baseURL)
			if apiKey != "" && wizardHostname != "" {
				wizardAuth := &credentials.AuthConfig{
					Type:  "bearer",
					Token: apiKey,
				}
				if err := a.credMgr.RegisterPatternWithContext(ctx, wizardHostname, wizardAuth); err != nil {
					log.Printf("[Wizard] Erro ao registrar credencial temporária: %v", err)
				}
			}

			providerID, err := a.createWizardProvider(provider, baseURL, apiKey, defaultModel)
			if err != nil {
				return false, fmt.Errorf("erro ao criar provedor: %w", err)
			}

			if err := a.saveWelcomeConfig(baseURL, apiKey, defaultModel); err != nil {
				return false, err
			}

			// Profiles com $default resolvem automaticamente para o provedor marcado como default.
			// Não é mais necessário reescrever todos os perfis.

			a.initLLMClient()

			// Verificação final: confirma que o provider funciona com o modelo escolhido
			finalModels, finalErr := a.GetModelsByProvider(providerID)
			if finalErr != nil {
				log.Printf("[Wizard] Verificação final: %v (provider pode não suportar /models)", finalErr)
			} else {
				log.Printf("[Wizard] Verificação final OK: %d modelos via provider '%s'", len(finalModels), providerID)
				modelFound := false
				for _, m := range finalModels {
					if m == defaultModel {
						modelFound = true
						break
					}
				}
				if !modelFound && len(finalModels) > 0 {
					log.Printf("[Wizard] Aviso: modelo '%s' não encontrado na lista do provider (%d modelos)", defaultModel, len(finalModels))
				}
			}

			go a.checkForUpdatesAfterWizard()
			return true, nil
		}
	}

	return false, nil
}

// wizardLabelToProviderType mapeia o rótulo exibido no wizard para o type ID
// usado por providers.BuiltinTemplate. Retorna "" para provedores sem template
// (Azure OpenAI, LiteLLM, Outro), que exigem URL personalizada.
func wizardLabelToProviderType(label string) string {
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

// wizardProviderInfo mapeia a escolha do wizard para configuração do provedor
type wizardProviderInfo struct {
	ID           string
	Name         string
	Type         llm.ProviderType
	APIFormat    llm.APIFormat // se vazio, será inferido por GetAPIFormat()
	DefaultModel string
}

// getWizardProviderInfo retorna ID, nome, tipo e modelo default sugerido para a escolha do wizard
func getWizardProviderInfo(providerChoice string) wizardProviderInfo {
	switch providerChoice {
	case "OpenAI":
		return wizardProviderInfo{ID: "openai-default", Name: "OpenAI", Type: llm.ProviderOpenAI, APIFormat: llm.APIFormatOpenAIResponses, DefaultModel: "gpt-4o-mini"}
	case "Anthropic (Claude)":
		return wizardProviderInfo{ID: "anthropic-claude", Name: "Claude (Anthropic)", Type: llm.ProviderClaude, APIFormat: llm.APIFormatAnthropic, DefaultModel: "claude-sonnet-4-20250514"}
	case "Google (Gemini)":
		return wizardProviderInfo{ID: "google-gemini", Name: "Google (Gemini)", Type: llm.ProviderOpenAI, DefaultModel: "gemini-2.0-flash"}
	case "OpenRouter":
		return wizardProviderInfo{ID: "openrouter-default", Name: "OpenRouter", Type: llm.ProviderOpenAI}
	case "Mistral AI":
		return wizardProviderInfo{ID: "mistral-default", Name: "Mistral AI", Type: llm.ProviderMistral, DefaultModel: "mistral-small-latest"}
	case "Groq":
		return wizardProviderInfo{ID: "groq-default", Name: "Groq", Type: llm.ProviderGroq, DefaultModel: "llama-3.3-70b-versatile"}
	case "Together AI":
		return wizardProviderInfo{ID: "together-default", Name: "Together AI", Type: llm.ProviderTogether}
	case "Fireworks AI":
		return wizardProviderInfo{ID: "fireworks-default", Name: "Fireworks AI", Type: llm.ProviderFireworks}
	case "Perplexity":
		return wizardProviderInfo{ID: "perplexity-default", Name: "Perplexity", Type: llm.ProviderPerplexity, DefaultModel: "sonar"}
	case "DeepSeek":
		return wizardProviderInfo{ID: "deepseek-default", Name: "DeepSeek", Type: llm.ProviderDeepSeek, DefaultModel: "deepseek-chat"}
	case "xAI (Grok)":
		return wizardProviderInfo{ID: "xai-grok", Name: "xAI (Grok)", Type: llm.ProviderGrok, DefaultModel: "grok-3-mini"}
	case "Azure OpenAI":
		return wizardProviderInfo{ID: "azure-openai", Name: "Azure OpenAI", Type: llm.ProviderOpenAI}
	case "Ollama (Local)":
		return wizardProviderInfo{ID: "ollama-local", Name: "Ollama (Local)", Type: llm.ProviderOllama}
	case "LiteLLM":
		return wizardProviderInfo{ID: "litellm", Name: "LiteLLM", Type: llm.ProviderOpenAI}
	default:
		return wizardProviderInfo{ID: "custom", Name: "Custom Provider", Type: llm.ProviderCustom}
	}
}

// createWizardProvider cria o provedor LLM escolhido durante o wizard,
// registrando-o no registry, salvando a credencial e persistindo no SQLite.
func (a *App) createWizardProvider(providerChoice, baseURL, apiKey, model string) (string, error) {
	info := getWizardProviderInfo(providerChoice)

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

	if err := a.llmRegistry.Register(provider); err != nil {
		return "", fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	if apiKey != "" && hostname != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: apiKey,
		}
		if err := a.credMgr.RegisterPatternWithContext(a.ctx, hostname, authCfg); err != nil {
			return "", fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	if err := a.saveLLMProviders(); err != nil {
		return "", fmt.Errorf("erro ao persistir provedor: %w", err)
	}

	if err := a.providerSvc.SetDefault(info.ID); err != nil {
		log.Printf("[Wizard] Aviso: erro ao marcar provedor como default: %v", err)
	}

	log.Printf("[Wizard] Provedor '%s' (%s) criado como default, modelo padrão: %s", info.ID, info.Name, defaultModel)
	return info.ID, nil
}

// saveWelcomeConfig salva a configuração legada do wizard (config.json)
func (a *App) saveWelcomeConfig(baseURL, apiKey, defaultModel string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	cfg.APIBaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.DefaultModel = defaultModel
	cfg.ChatParams.Model = defaultModel

	return config.Save(cfg)
}

// REMOVED: updateAllProfilesProviderAndModel — substituída pelo sistema de $default.
// Profiles com llm_provider="$default" resolvem automaticamente para o provedor marcado
// como IsDefault no banco. Não é mais necessário reescrever todos os perfis.

// checkForUpdatesAfterWizard verifica atualizações após o wizard de configuração
func (a *App) checkForUpdatesAfterWizard() {
	// Aguarda 2 segundos para não interferir com finalização do wizard
	time.Sleep(2 * time.Second)

	if a.updater == nil {
		log.Printf("[Wizard] Updater não inicializado, pulando verificação de atualizações")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("[Wizard] Verificando atualizações disponíveis...")

	info, err := a.updater.CheckForUpdates(ctx)
	if err != nil {
		log.Printf("[Wizard] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		log.Printf("[Wizard] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	log.Printf("[Wizard] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	// Pergunta ao usuário se deseja atualizar
	go a.promptForUpdate(info)
}

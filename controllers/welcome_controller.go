package controllers

import (
	"context"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
)

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
// que senha e recuperação foram configuradas; o frontend conclui o onboarding
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

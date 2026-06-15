package app

import (
	"assistente/controllers"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/providers"
	"fmt"
)

// ==================== Welcome Wizard ====================

// wizardValidationResult é um alias local para providers.ConnectionProbeResult.
type wizardValidationResult = providers.ConnectionProbeResult

// wizardProviderInfo é um alias para controllers.WizardProviderInfo (usado em testes).
type wizardProviderInfo = controllers.WizardProviderInfo

// getWizardProviderInfo retorna ID, nome, tipo e modelo default para a escolha do wizard.
// Mantida no pacote main para compatibilidade com testes.
func getWizardProviderInfo(choice string) wizardProviderInfo {
	return controllers.GetWizardProviderInfo(choice)
}

func (a *App) welcomeController() (*controllers.WelcomeController, error) {
	if a.welcomeCtrl == nil {
		return nil, fmt.Errorf("welcome controller not initialized")
	}
	return a.welcomeCtrl, nil
}

// validateWizardConnection testa URL, autenticação e lista modelos de um provedor.
func (a *App) validateWizardConnection(baseURL, apiKey string) wizardValidationResult {
	ctrl, err := a.welcomeController()
	if err != nil {
		return wizardValidationResult{
			ErrorType:   "app_initializing",
			ErrorDetail: err.Error(),
		}
	}
	return ctrl.ValidateWizardConnection(a.ctx, baseURL, apiKey)
}

// validateWizardURL valida formato e alcançabilidade básica de uma URL personalizada.
func (a *App) validateWizardURL(baseURL string) error {
	ctrl, err := a.welcomeController()
	if err != nil {
		return err
	}
	return ctrl.ValidateWizardURL(a.ctx, baseURL)
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas.
//
// Question 14 + Blocker C do re-review do AEP-0052: o wizard tem partes
// per-instance (master key, primeiro usuário) e parte per-user (provedores
// LLM). Sem distinguir os dois modos a função engolia ErrUserScopeRequired
// silenciosamente e dava certo "por acidente". A versão dual-mode explícita:
//
//   - Pré-login (CLI `assistente setup` ou primeira boot da UI antes do
//     AuthGate): wizard é puramente instance-wide. Decide só por (a)
//     existência de master key e (b) existência de algum usuário cadastrado.
//     NÃO consulta provedores — eles são per-user.
//   - Pós-login (UI rodando depois do AuthGate): ctx carrega o userID; o
//     check de provedores fica per-user via requireAuthenticatedContext.
//
// Esse é o único binding Wails que tolera "sem sessão", e mesmo assim só
// para devolver true/false consistentes — nada é lido de tabelas de usuário
// pré-login.
func (a *App) NeedsWelcomeWizard() bool {
	store := credentials.NewDBStore()
	hasMasterKey, err := store.HasKeyWrap(a.appContext(), credentials.KeyWrapKindMaster)
	if err != nil {
		return true
	}

	a.authMu.RLock()
	loggedIn := a.currentUserID != ""
	a.authMu.RUnlock()

	if !loggedIn {
		var userCount int64
		if database.DB() == nil {
			return true
		}
		if err := database.DB().Model(&database.User{}).Count(&userCount).Error; err != nil {
			return true
		}
		return !hasMasterKey || userCount == 0
	}

	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return true
	}
	if a.welcomeCtrl != nil {
		return a.welcomeCtrl.NeedsWelcomeWizard(ctx)
	}
	if a.providerSvc == nil {
		return true
	}
	count, err := a.providerSvc.Count(ctx)
	if err != nil {
		return true
	}
	return count == 0 || !hasMasterKey
}

// RunWelcomeWizard executa o wizard de boas-vindas.
// Retorna true se completou com sucesso, false se cancelado.
func (a *App) RunWelcomeWizard() (bool, error) {
	ctrl, err := a.welcomeController()
	if err != nil {
		return false, err
	}
	return ctrl.RunWelcomeWizard(a.ctx)
}

// createWizardProvider cria o provedor LLM escolhido durante o wizard (thin-wrap para testes).
func (a *App) createWizardProvider(providerChoice, baseURL, apiKey, model string) (string, error) {
	ctrl, err := a.welcomeController()
	if err != nil {
		return "", err
	}
	return ctrl.CreateWizardProvider(a.ctx, providerChoice, baseURL, apiKey, model)
}

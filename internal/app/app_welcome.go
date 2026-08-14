package app

import (
	"assistente/controllers"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/providers"
	"assistente/internal/wailsapi"
	"context"
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

// createWizardProvider cria o provedor LLM escolhido durante o wizard (thin-wrap para testes).
func (a *App) createWizardProvider(providerChoice, baseURL, apiKey, model string) (string, error) {
	ctrl, err := a.welcomeController()
	if err != nil {
		return "", err
	}
	return ctrl.CreateWizardProvider(a.ctx, providerChoice, baseURL, apiKey, model)
}

// welcomeRuntime adapta *App para wailsapi.WelcomeRuntime sem expor métodos no Bind.
type welcomeRuntime struct {
	app *App
}

func (r welcomeRuntime) AppContext() context.Context {
	if r.app == nil {
		return context.Background()
	}
	return r.app.appContext()
}

func (r welcomeRuntime) IsLoggedIn() bool {
	if r.app == nil {
		return false
	}
	r.app.authMu.RLock()
	defer r.app.authMu.RUnlock()
	return r.app.currentUserID != ""
}

func (r welcomeRuntime) HasMasterKey() (bool, error) {
	store := credentials.NewDBStore()
	return store.HasKeyWrap(r.AppContext(), credentials.KeyWrapKindMaster)
}

func (r welcomeRuntime) UserCount() (int64, error) {
	if database.DB() == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var userCount int64
	if err := database.DB().Model(&database.User{}).Count(&userCount).Error; err != nil {
		return 0, err
	}
	return userCount, nil
}

func (r welcomeRuntime) ProviderCount(ctx context.Context) (int64, error) {
	if r.app == nil || r.app.providerSvc == nil {
		return 0, fmt.Errorf("provider service not initialized")
	}
	count, err := r.app.providerSvc.Count(ctx)
	return int64(count), err
}

// NeedsWelcomeWizard avalia o wizard dual-mode para a CLI (cmd/asst).
// Função de pacote (não método) para não entrar na superfície Bind do Wails;
// a UI usa wailsapi.Welcome.
func NeedsWelcomeWizard(a *App) bool {
	if a == nil {
		return true
	}
	return wailsapi.EvaluateNeedsWelcomeWizard(wailsSession{app: a}, a.welcomeCtrl, welcomeRuntime{app: a})
}

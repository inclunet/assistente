package main

import (
	"assistente/controllers"
	"assistente/internal/providers"
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

// wizardLabelToProviderType mapeia rótulo do wizard para o type ID de providers.BuiltinTemplate.
// Mantida no pacote main para compatibilidade com testes.
func wizardLabelToProviderType(label string) string {
	return controllers.WizardLabelToProviderType(label)
}

// validateWizardConnection testa URL, autenticação e lista modelos de um provedor.
func (a *App) validateWizardConnection(baseURL, apiKey string) wizardValidationResult {
	return a.welcomeCtrl.ValidateWizardConnection(a.ctx, baseURL, apiKey)
}

// validateWizardURL valida formato e alcançabilidade básica de uma URL personalizada.
func (a *App) validateWizardURL(baseURL string) error {
	return a.welcomeCtrl.ValidateWizardURL(a.ctx, baseURL)
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas.
func (a *App) NeedsWelcomeWizard() bool {
	return a.welcomeCtrl.NeedsWelcomeWizard()
}

// RunWelcomeWizard executa o wizard de boas-vindas.
// Retorna true se completou com sucesso, false se cancelado.
func (a *App) RunWelcomeWizard() (bool, error) {
	return a.welcomeCtrl.RunWelcomeWizard(a.ctx)
}

// createWizardProvider cria o provedor LLM escolhido durante o wizard (thin-wrap para testes).
func (a *App) createWizardProvider(providerChoice, baseURL, apiKey, model string) (string, error) {
	return a.welcomeCtrl.CreateWizardProvider(a.ctx, providerChoice, baseURL, apiKey, model)
}

// saveWelcomeConfig salva a configuração do wizard (thin-wrap para testes).
func (a *App) saveWelcomeConfig(baseURL, apiKey, defaultModel string) error {
	return a.welcomeCtrl.SaveWelcomeConfig(baseURL, apiKey, defaultModel)
}

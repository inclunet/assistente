package main

import (
	"context"
	"log"
	"time"

	"assistente/internal/questionnaire"
	"assistente/internal/updater"
)

// ============================================================================
// Auto Update API
// ============================================================================

// AppVersion é a versão do aplicativo, injetada via ldflags no build.
// Em dev, permanece como "dev".
var AppVersion = "dev"

// initUpdater inicializa o gerenciador de atualizações e configura seus callbacks.
func (a *App) initUpdater() {
	a.updater = updater.New(AppVersion, a.credMgr)

	// Configura callback de progresso
	a.updater.SetProgressCallback(func(bytesDownloaded, totalBytes int64, phase string) {
		if a.ctx == nil {
			return
		}
		var percentage float64
		if totalBytes > 0 {
			percentage = float64(bytesDownloaded) / float64(totalBytes) * 100
		}
		a.emitter.Emit("update:progress", map[string]interface{}{
			"phase":           phase,
			"bytesDownloaded": bytesDownloaded,
			"totalBytes":      totalBytes,
			"percentage":      percentage,
		})
	})

	// Configura callback de elevação (solicita permissão ao usuário)
	a.updater.SetElevationCallback(func() bool {
		if a.questionnaireMgr == nil {
			log.Printf("[Updater] Questionnaire manager não disponível para solicitar elevação")
			return false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Permissão Necessária",
			Description: "Para atualizar o aplicativo, precisamos de permissões de administrador para substituir o arquivo executável.\n\nDeseja permitir?",
			Questions: []questionnaire.Question{
				{
					ID:       "allow",
					Type:     "boolean",
					Prompt:   "Permitir atualização com privilégios de administrador?",
					Required: true,
					Default:  true,
				},
			},
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Cancelar",
		})

		if err != nil {
			log.Printf("[Updater] Erro ao solicitar confirmação de elevação: %v", err)
			return false
		}
		if resp.Cancelled {
			log.Printf("[Updater] Usuário cancelou a solicitação de elevação")
			return false
		}
		if allow, ok := resp.Answers["allow"].(bool); ok && allow {
			log.Printf("[Updater] Usuário autorizou elevação")
			return true
		}
		return false
	})

	log.Printf("[Updater] Inicializado (versão atual: %s)", AppVersion)
}

// checkForUpdatesOnStartup verifica atualizações ao iniciar (não bloqueante).
func (a *App) checkForUpdatesOnStartup() {
	a.updaterCtrl.CheckForUpdatesOnStartup(a.ctx)
}

// promptForUpdate delega para o updaterCtrl (chamado por app_welcome.go).
func (a *App) promptForUpdate(info *updater.UpdateInfo) {
	a.updaterCtrl.PromptForUpdate(a.ctx, info)
}

// GetAppVersion retorna a versão atual do aplicativo.
func (a *App) GetAppVersion() string {
	return a.updaterCtrl.GetAppVersion()
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis.
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	return a.updaterCtrl.CheckForUpdates()
}

// ApplyUpdate aplica a atualização (chamado pelo frontend).
func (a *App) ApplyUpdate() error {
	return a.updaterCtrl.ApplyUpdate(a.ctx)
}

// StartUpdate inicia o processo de atualização (navega para página e inicia).
func (a *App) StartUpdate() error {
	return a.updaterCtrl.StartUpdate(a.ctx)
}

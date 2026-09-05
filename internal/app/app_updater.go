package app

import (
	"assistente/internal/logging"
	"context"
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

// updateElevationTextKey é o assunto deste diálogo nas chaves de tradução
// (AEP-0085 D7).
func updateElevationTextKey(field string) string {
	return "app.questionnaire.updateElevation." + field
}

// updateElevationPayload monta o pedido de privilégio de administrador para
// substituir o executável. É decisão de segurança, e das que não se avaliam sem
// entender: quem lê o pedido num idioma que não fala não tem como saber o que
// está autorizando (AEP-0085).
func updateElevationPayload() questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Kind:  questionnaire.KindDecision,
		Title: questionnaire.Keyed(updateElevationTextKey("title"), "Permissão Necessária"),
		Description: questionnaire.Keyed(
			updateElevationTextKey("description"),
			"Para atualizar o aplicativo, precisamos de permissões de administrador para substituir o arquivo executável.\n\nDeseja permitir?",
		),
		AllowCancel: true,
		Actions: []questionnaire.DecisionAction{
			{
				ID:      "allow",
				Label:   questionnaire.Keyed(updateElevationTextKey("submit"), "Permitir"),
				Variant: "primary",
				Primary: true,
			},
			{
				ID:      "deny",
				Label:   questionnaire.Keyed(updateElevationTextKey("cancel"), "Cancelar"),
				Variant: "outline",
			},
		},
	}
}

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
			logging.Warnf(context.Background(), "app.app-updater", "[Updater] Questionnaire manager não disponível para solicitar elevação")
			return false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, updateElevationPayload())

		if err != nil {
			logging.Errorf(context.Background(), "app.app-updater", "[Updater] Erro ao solicitar confirmação de elevação: %v", err)
			return false
		}
		if resp.Cancelled {
			logging.Infof(context.Background(), "app.app-updater", "[Updater] Usuário cancelou a solicitação de elevação")
			return false
		}
		id, ok := questionnaire.DecisionActionID(resp)
		if !ok || id != "allow" {
			return false
		}
		logging.Infof(context.Background(), "app.app-updater", "[Updater] Usuário autorizou elevação")
		return true
	})

	logging.Infof(context.Background(), "app.app-updater", "[Updater] Inicializado (versão atual: %s)", AppVersion)
}

// checkForUpdatesOnStartup executa o scheduler cancelável de atualizações.
func (a *App) checkForUpdatesOnStartup() {
	a.updaterCtrl.RunUpdateChecks(a.ctx)
}

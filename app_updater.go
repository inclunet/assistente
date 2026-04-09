package main

import (
	"context"
	"fmt"
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

// GetAppVersion retorna a versão atual do aplicativo.
func (a *App) GetAppVersion() string {
	return AppVersion
}

// initUpdater inicializa o gerenciador de atualizações
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

		a.emitter.Emit( "update:progress", map[string]interface{}{
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

// checkForUpdatesOnStartup verifica atualizações ao iniciar (não bloqueante)
func (a *App) checkForUpdatesOnStartup() {
	// Pula verificação de updates em modo desenvolvimento
	if AppVersion == "dev" {
		log.Printf("[Updater] Modo desenvolvimento detectado (AppVersion=%s): pulando verificação de updates", AppVersion)
		return
	}

	// Aguarda 5 segundos após startup para não interferir com inicialização
	time.Sleep(5 * time.Second)

	// Só verifica atualizações se há providers configurados
	provCount, countErr := a.providerSvc.Count()
	if countErr != nil || provCount == 0 {
		log.Printf("[Updater] Pulando verificação de atualizações: nenhum provider configurado")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := a.updater.CheckForUpdates(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		log.Printf("[Updater] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	log.Printf("[Updater] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	// Pergunta ao usuário se deseja atualizar usando o sistema de questionário
	go a.promptForUpdate(info)
}

// promptForUpdate pergunta ao usuário se deseja atualizar
func (a *App) promptForUpdate(info *updater.UpdateInfo) {
	if a.questionnaireMgr == nil {
		log.Printf("[Updater] Questionnaire manager não disponível")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	description := fmt.Sprintf("Versão atual: %s\nNova versão: %s", info.CurrentVersion, info.LatestVersion)
	if info.ReleaseNotes != "" {
		description += "\n\nNotas da versão:\n" + info.ReleaseNotes
	}
	if info.DownloadSize > 0 {
		sizeMB := float64(info.DownloadSize) / (1024 * 1024)
		description += fmt.Sprintf("\n\nTamanho do download: %.2f MB", sizeMB)
	}

	resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "Atualização Disponível",
		Description: description,
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   "Deseja atualizar agora?",
				Required: true,
				Default:  true,
			},
		},
		AllowCancel: true,
		SubmitLabel: "Atualizar",
		CancelLabel: "Mais Tarde",
	})

	if err != nil {
		log.Printf("[Updater] Erro ao solicitar confirmação: %v", err)
		return
	}

	if resp.Cancelled {
		log.Printf("[Updater] Usuário cancelou a atualização")
		return
	}

	if confirm, ok := resp.Answers["confirm"].(bool); ok && confirm {
		// Navega para página de atualização
		if a.ctx != nil {
			a.emitter.Emit( "navigate:update", nil)
		}
		go a.applyUpdateWithProgress()
	}
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis (chamado pelo frontend)
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("updater não inicializado")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.updater.CheckForUpdates(ctx)
}

// ApplyUpdate aplica a atualização (chamado pelo frontend)
func (a *App) ApplyUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	go a.applyUpdateWithProgress()
	return nil
}

// StartUpdate inicia o processo de atualização (navega para página e inicia)
func (a *App) StartUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	// Emite evento para navegar para página de atualização
	if a.ctx != nil {
		a.emitter.Emit( "navigate:update", nil)
	}

	// Aguarda um pouco para garantir que a navegação ocorreu
	time.Sleep(500 * time.Millisecond)

	go a.applyUpdateWithProgress()
	return nil
}

// applyUpdateWithProgress aplica a atualização com feedback de progresso
func (a *App) applyUpdateWithProgress() {
	// Emite evento de início
	a.emitter.Emit( "update:started", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Printf("[Updater] Iniciando download e aplicação da atualização...")

	err := a.updater.ApplyUpdate(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao aplicar atualização: %v", err)
		a.emitter.Emit( "update:error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	log.Printf("[Updater] Atualização aplicada com sucesso. Reinicie o aplicativo.")
	a.emitter.Emit( "update:completed", map[string]interface{}{
		"message": "Atualização instalada com sucesso! Feche e reabra o aplicativo para aplicar as mudanças.",
	})
}

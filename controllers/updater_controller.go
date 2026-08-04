package controllers

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/updater"
)

// UpdaterControllerConfig agrupa dependências do UpdaterController.
type UpdaterControllerConfig struct {
	Updater          *updater.Updater
	Emitter          ports.Emitter
	QuestionnaireMgr *questionnaire.Manager
	ProviderSvc      *providers.Service
	AppVersion       string
}

// UpdaterController expõe operações de verificação e aplicação de atualizações.
type UpdaterController struct {
	updater          *updater.Updater
	emitter          ports.Emitter
	questionnaireMgr *questionnaire.Manager
	providerSvc      *providers.Service
	appVersion       string
}

// NewUpdaterController cria um UpdaterController com as dependências fornecidas.
func NewUpdaterController(cfg UpdaterControllerConfig) *UpdaterController {
	return &UpdaterController{
		updater:          cfg.Updater,
		emitter:          cfg.Emitter,
		questionnaireMgr: cfg.QuestionnaireMgr,
		providerSvc:      cfg.ProviderSvc,
		appVersion:       cfg.AppVersion,
	}
}

// GetAppVersion retorna a versão atual do aplicativo.
func (c *UpdaterController) GetAppVersion() string {
	return c.appVersion
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis.
func (c *UpdaterController) CheckForUpdates() (*updater.UpdateInfo, error) {
	if c.updater == nil {
		return nil, fmt.Errorf("updater não inicializado")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return c.updater.CheckForUpdates(ctx)
}

// ApplyUpdate inicia a aplicação da atualização em background.
func (c *UpdaterController) ApplyUpdate(ctx context.Context) error {
	if c.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	go c.applyUpdateWithProgress(ctx)
	return nil
}

// StartUpdate emite evento de navegação e inicia atualização em background.
func (c *UpdaterController) StartUpdate(ctx context.Context) error {
	if c.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	c.emitter.Emit("navigate:update", nil)
	time.Sleep(500 * time.Millisecond)

	go c.applyUpdateWithProgress(ctx)
	return nil
}

// CheckForUpdatesOnStartup verifica atualizações ao iniciar (não bloqueante).
func (c *UpdaterController) CheckForUpdatesOnStartup(ctx context.Context) {
	if c.appVersion == "dev" {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Modo desenvolvimento detectado (AppVersion=%s): pulando verificação de updates", c.appVersion)
		return
	}

	time.Sleep(5 * time.Second)

	provCount, countErr := c.providerSvc.Count(ctx)
	if countErr != nil {
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao contar providers para verificação de atualizações: %v", countErr)
		return
	}
	if provCount == 0 {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Pulando verificação de atualizações: nenhum provider configurado")
		return
	}

	checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.updater.CheckForUpdates(checkCtx)
	if err != nil {
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	logging.Infof(ctx, "controllers.updater-controller", "[Updater] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)
	go c.promptForUpdate(ctx, info)
}

// PromptForUpdate pergunta ao usuário se deseja atualizar (público para uso em app_welcome.go).
func (c *UpdaterController) PromptForUpdate(ctx context.Context, info *updater.UpdateInfo) {
	go c.promptForUpdate(ctx, info)
}

// updateTextKey é o assunto deste diálogo nas chaves de tradução (AEP-0085 D7).
func updateTextKey(field string) string {
	return "app.questionnaire.update." + field
}

// updatePromptPayload monta o convite para atualizar. Versões, notas da release e
// tamanho do download são dados da release: vão como parâmetros da tradução, e o
// texto pronto já vai com eles no lugar (AEP-0085 D6).
//
// A descrição muda com o que a release traz, e as quatro formas dividem um campo
// só: é a chave que diz qual delas está na tela. Com uma chave só, quem traduz
// deixaria de fora as notas ou o tamanho — e é pelo tamanho que alguém em conexão
// limitada decide esperar.
func updatePromptPayload(info *updater.UpdateInfo) questionnaire.RequestPayload {
	campo := "description"
	params := map[string]any{
		"current": info.CurrentVersion,
		"latest":  info.LatestVersion,
	}
	fallback := fmt.Sprintf("Versão atual: %s\nNova versão: %s", info.CurrentVersion, info.LatestVersion)

	if info.ReleaseNotes != "" {
		campo += "Notes"
		params["notes"] = info.ReleaseNotes
		fallback += "\n\nNotas da versão:\n" + info.ReleaseNotes
	}
	if info.DownloadSize > 0 {
		campo += "Size"
		size := fmt.Sprintf("%.2f", float64(info.DownloadSize)/(1024*1024))
		params["size"] = size
		fallback += fmt.Sprintf("\n\nTamanho do download: %s MB", size)
	}

	return questionnaire.RequestPayload{
		Title:       questionnaire.Keyed(updateTextKey("title"), "Atualização Disponível"),
		Description: questionnaire.KeyedWith(updateTextKey(campo), params, fallback),
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   questionnaire.Keyed(updateTextKey("prompt"), "Deseja atualizar agora?"),
				Required: true,
				Default:  true,
			},
		},
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed(updateTextKey("submit"), "Atualizar"),
		CancelLabel: questionnaire.Keyed(updateTextKey("cancel"), "Mais Tarde"),
	}
}

// promptForUpdate pergunta ao usuário se deseja atualizar.
func (c *UpdaterController) promptForUpdate(ctx context.Context, info *updater.UpdateInfo) {
	if c.questionnaireMgr == nil {
		logging.Warnf(ctx, "controllers.updater-controller", "[Updater] Questionnaire manager não disponível")
		return
	}

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := c.questionnaireMgr.RequestQuestionnaire(qCtx, updatePromptPayload(info))

	if err != nil {
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao solicitar confirmação: %v", err)
		return
	}

	if resp.Cancelled {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Usuário cancelou a atualização")
		return
	}

	if confirm, ok := resp.Answers["confirm"].(bool); ok && confirm {
		c.emitter.Emit("navigate:update", nil)
		go c.applyUpdateWithProgress(ctx)
	}
}

// applyUpdateWithProgress aplica a atualização com feedback de progresso via eventos.
func (c *UpdaterController) applyUpdateWithProgress(ctx context.Context) {
	c.emitter.Emit("update:started", nil)

	applyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	logging.Infof(ctx, "controllers.updater-controller", "[Updater] Iniciando download e aplicação da atualização...")

	err := c.updater.ApplyUpdate(applyCtx)
	if err != nil {
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao aplicar atualização: %v", err)
		c.emitter.Emit("update:error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	logging.Infof(ctx, "controllers.updater-controller", "[Updater] Atualização aplicada com sucesso. Reinicie o aplicativo.")
	c.emitter.Emit("update:completed", map[string]interface{}{
		"message": "Atualização instalada com sucesso! Feche e reabra o aplicativo para aplicar as mudanças.",
	})
}

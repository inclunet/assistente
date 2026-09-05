package controllers

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/questionnaire"
	"assistente/internal/updater"
)

const updateCheckErrorEvent = "update:check-error"

type updaterService interface {
	CheckForUpdates(context.Context) (*updater.UpdateInfo, error)
	ApplyUpdate(context.Context) error
}

// UpdaterControllerConfig agrupa dependências do UpdaterController.
type UpdaterControllerConfig struct {
	Updater          updaterService
	Emitter          ports.Emitter
	QuestionnaireMgr *questionnaire.Manager
	AppVersion       string
}

// UpdaterController expõe operações de verificação e aplicação de atualizações.
type UpdaterController struct {
	updater          updaterService
	emitter          ports.Emitter
	questionnaireMgr *questionnaire.Manager
	appVersion       string
	startupDelay     time.Duration
	checkInterval    time.Duration
	checkTimeout     time.Duration
	checkRequests    chan struct{}

	stateMu         sync.Mutex
	promptedVersion string
	errorReported   bool
}

// NewUpdaterController cria um UpdaterController com as dependências fornecidas.
func NewUpdaterController(cfg UpdaterControllerConfig) *UpdaterController {
	return &UpdaterController{
		updater:          cfg.Updater,
		emitter:          cfg.Emitter,
		questionnaireMgr: cfg.QuestionnaireMgr,
		appVersion:       cfg.AppVersion,
		startupDelay:     5 * time.Second,
		checkInterval:    updater.CheckInterval,
		checkTimeout:     30 * time.Second,
		checkRequests:    make(chan struct{}, 1),
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

// RunUpdateChecks mantém a verificação de startup e as verificações periódicas
// no contexto raiz do app. O método bloqueia até o cancelamento para que o App
// possa rastreá-lo no bgWG e fazer join no Shutdown.
func (c *UpdaterController) RunUpdateChecks(ctx context.Context) {
	if c.appVersion == "dev" {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Modo desenvolvimento detectado (AppVersion=%s): pulando verificação de updates", c.appVersion)
		return
	}

	delay := c.startupDelay
	if delay < 0 {
		delay = 0
	}
	interval := c.checkInterval
	if interval <= 0 {
		interval = updater.CheckInterval
	}

	startupTimer := time.NewTimer(delay)
	defer startupTimer.Stop()
	startupC := startupTimer.C

	var ticker *time.Ticker
	var tickerC <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-startupC:
		case <-tickerC:
		case <-c.checkRequests:
		}

		c.checkAndPrompt(ctx)

		if startupC != nil {
			if !startupTimer.Stop() {
				select {
				case <-startupTimer.C:
				default:
				}
			}
			startupC = nil
			ticker = time.NewTicker(interval)
			tickerC = ticker.C
		}

		// Uma solicitação pós-wizard que chegou durante o fetch já foi atendida
		// pelo check em voo. Esvaziá-la evita um segundo fetch imediato.
		select {
		case <-c.checkRequests:
		default:
		}
	}
}

// RequestUpdateCheck antecipa a primeira verificação (por exemplo, ao terminar
// o wizard). O canal de capacidade 1 agrega solicitações concorrentes.
func (c *UpdaterController) RequestUpdateCheck() {
	select {
	case c.checkRequests <- struct{}{}:
	default:
	}
}

func (c *UpdaterController) checkAndPrompt(ctx context.Context) {
	if c.updater == nil {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, c.checkTimeout)
	defer cancel()

	info, err := c.updater.CheckForUpdates(checkCtx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao verificar atualizações: %v", err)
		c.reportCheckErrorOnce()
		return
	}

	c.stateMu.Lock()
	c.errorReported = false
	c.stateMu.Unlock()

	if !info.Available {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	c.stateMu.Lock()
	if c.promptedVersion == info.LatestVersion {
		c.stateMu.Unlock()
		return
	}
	c.stateMu.Unlock()

	logging.Infof(ctx, "controllers.updater-controller", "[Updater] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)
	if c.promptForUpdate(ctx, info) {
		c.stateMu.Lock()
		c.promptedVersion = info.LatestVersion
		c.stateMu.Unlock()
	}
}

func (c *UpdaterController) reportCheckErrorOnce() {
	c.stateMu.Lock()
	if c.errorReported {
		c.stateMu.Unlock()
		return
	}
	c.errorReported = true
	emitter := c.emitter
	c.stateMu.Unlock()

	if emitter != nil {
		// Sem payload de erro: detalhes internos ficam apenas no log.
		emitter.Emit(updateCheckErrorEvent, nil)
	}
}

// PromptForUpdate pergunta ao usuário se deseja atualizar.
func (c *UpdaterController) PromptForUpdate(ctx context.Context, info *updater.UpdateInfo) {
	c.promptForUpdate(ctx, info)
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
		Kind:        questionnaire.KindDecision,
		Title:       questionnaire.Keyed(updateTextKey("title"), "Atualização Disponível"),
		Description: questionnaire.KeyedWith(updateTextKey(campo), params, fallback),
		AllowCancel: true,
		Actions: []questionnaire.DecisionAction{
			{
				ID:      "update",
				Label:   questionnaire.Keyed(updateTextKey("submit"), "Atualizar"),
				Variant: "primary",
				Primary: true,
			},
			{
				ID:      "later",
				Label:   questionnaire.Keyed(updateTextKey("cancel"), "Mais Tarde"),
				Variant: "outline",
			},
		},
	}
}

// promptForUpdate pergunta ao usuário se deseja atualizar.
func (c *UpdaterController) promptForUpdate(ctx context.Context, info *updater.UpdateInfo) bool {
	if c.questionnaireMgr == nil {
		logging.Warnf(ctx, "controllers.updater-controller", "[Updater] Questionnaire manager não disponível")
		return false
	}

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := c.questionnaireMgr.RequestQuestionnaire(qCtx, updatePromptPayload(info))

	if err != nil {
		logging.Errorf(ctx, "controllers.updater-controller", "[Updater] Erro ao solicitar confirmação: %v", err)
		return false
	}

	if resp.Cancelled {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Usuário cancelou a atualização")
		return true
	}

	id, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		// Resposta sem actionId não é adiamento: é o contrato quebrado. Não
		// atualizar continua sendo o certo, mas registrado como o defeito que é.
		logging.Warnf(ctx, "controllers.updater-controller",
			"[Updater] Resposta de decisão sem %q; atualização não aplicada", questionnaire.AnswerActionID)
		return true
	}
	if id != "update" {
		logging.Infof(ctx, "controllers.updater-controller", "[Updater] Usuário adiou a atualização")
		return true
	}
	c.emitter.Emit("navigate:update", nil)
	go c.applyUpdateWithProgress(ctx)
	return true
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

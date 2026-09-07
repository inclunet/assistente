package app

import (
	"fmt"

	"assistente/controllers"
	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"
)

// bindMessagingDatabase ativa persistência SQLite para canais/contatos no boot
// (AEP-0083). Fail-closed: após database.Init esperado, DB nil aborta o boot.
// Runtime exige UseDatabase — não há fallback FS nas fachadas channels/contacts.
// JSON legado permanece apenas para import read-only e cleanup opt-in.
func bindMessagingDatabase() error {
	db := database.DB()
	if db == nil {
		return fmt.Errorf("banco de dados indisponível no boot de mensageria (AEP-0083): database.DB() == nil — sem DB não é possível chamar UseDatabase; fallback silencioso para filesystem não é permitido")
	}
	channels.UseDatabase(db)
	contacts.UseDatabase(db)
	return nil
}

// initMessaging cria e inicializa o MessagingController, que gerencia o gateway,
// conexões de canais e ferramentas de mensageria.
// A superfície Wails pública vive em wailsapi.Messaging (AEP-0088).
func (a *App) initMessaging() error {
	if err := bindMessagingDatabase(); err != nil {
		return err
	}
	a.msgCtrl = controllers.NewMessagingController(controllers.MessagingControllerConfig{
		Ctx:           a.ctx,
		ProfileMgr:    a.profileManager,
		CredMgr:       a.credMgr,
		SpeechSvc:     a.speechSvc,
		AudioRepo:     a.audioSvc,
		ToolRegistry:  a.toolRegistry,
		Emitter:       a.emitter,
		ConvSvc:       a.convSvc,
		SendMessageFn: a.sendMessageFromChannel,
	})
	a.msgCtrl.Init()
	a.msgGateway = a.msgCtrl.Gateway()
	a.responseNotifier = a.msgCtrl.ResponseNotifier()
	return nil
}

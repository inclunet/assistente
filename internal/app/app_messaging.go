package app

import (
	"assistente/controllers"
	"assistente/internal/channels"
	"assistente/internal/contacts"
)

// initMessaging cria e inicializa o MessagingController, que gerencia o gateway,
// conexões de canais e ferramentas de mensageria.
func (a *App) initMessaging() {
	a.msgCtrl = controllers.NewMessagingController(controllers.MessagingControllerConfig{
		Ctx:              a.ctx,
		ProfileMgr:       a.profileManager,
		CredMgr:          a.credMgr,
		QuestionnaireMgr: a.questionnaireMgr,
		SpeechSvc:        a.speechSvc,
		AudioRepo:        a.audioSvc,
		ToolRegistry:     a.toolRegistry,
		Emitter:          a.emitter,
		ConvSvc:          a.convSvc,
		SendMessageFn:    a.SendMessageFromChannel,
	})
	a.msgCtrl.Init()
	a.msgGateway = a.msgCtrl.Gateway()
	a.responseNotifier = a.msgCtrl.ResponseNotifier()
}

// GetMessagingStatus delega para MessagingController.
func (a *App) GetMessagingStatus() map[string]string {
	return a.msgCtrl.GetMessagingStatus()
}

// GetChannelConfig delega para MessagingController.
func (a *App) GetChannelConfig(channelName string) (*channels.ChannelConfig, error) {
	return a.msgCtrl.GetChannelConfig(channelName)
}

// SaveChannelConfig delega para MessagingController.
func (a *App) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	return a.msgCtrl.SaveChannelConfig(channelName, cfg)
}

// RestartChannel delega para MessagingController.
func (a *App) RestartChannel(channelName string) error {
	return a.msgCtrl.RestartChannel(channelName)
}

// GetAllChannelConfigs delega para MessagingController.
func (a *App) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	return a.msgCtrl.GetAllChannelConfigs()
}

// GetChannelTemplates delega para MessagingController.
func (a *App) GetChannelTemplates() []channels.ChannelTemplate {
	return a.msgCtrl.GetChannelTemplates()
}

// CreateChannelFromTemplate delega para MessagingController.
func (a *App) CreateChannelFromTemplate(templateType string, values map[string]interface{}) error {
	return a.msgCtrl.CreateChannelFromTemplate(templateType, values)
}

// GetChannelConfigAsMap delega para MessagingController.
func (a *App) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	return a.msgCtrl.GetChannelConfigAsMap(channelName)
}

// AuthorizeMessagingContactFull delega para MessagingController.
func (a *App) AuthorizeMessagingContactFull(channel, contactID, displayName, username string) error {
	return a.msgCtrl.AuthorizeMessagingContactFull(channel, contactID, displayName, username)
}

// RemoveAuthorizedContact delega para MessagingController.
func (a *App) RemoveAuthorizedContact(channel, contactID string) error {
	return a.msgCtrl.RemoveAuthorizedContact(channel, contactID)
}

// GetAuthorizedContacts delega para MessagingController.
func (a *App) GetAuthorizedContacts() (contacts.ContactsFile, error) {
	return a.msgCtrl.GetAuthorizedContacts()
}

// GetAvailableChannels delega para MessagingController.
func (a *App) GetAvailableChannels() []ChannelInfo {
	return a.msgCtrl.GetAvailableChannels()
}

// AssignConversationToChannel delega para MessagingController.
func (a *App) AssignConversationToChannel(conversationID string, channel, contactID string) error {
	return a.msgCtrl.AssignConversationToChannel(conversationID, channel, contactID)
}

// UnassignConversationFromChannel delega para MessagingController.
func (a *App) UnassignConversationFromChannel(conversationID string) error {
	return a.msgCtrl.UnassignConversationFromChannel(conversationID)
}

// GetConversationChannel delega para MessagingController.
func (a *App) GetConversationChannel(conversationID string) (string, string, error) {
	return a.msgCtrl.GetConversationChannel(conversationID)
}

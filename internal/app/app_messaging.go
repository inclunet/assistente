package app

import (
	"fmt"
	"strings"

	"assistente/controllers"
	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"
)

// initMessaging cria e inicializa o MessagingController, que gerencia o gateway,
// conexões de canais e ferramentas de mensageria.
func (a *App) initMessaging() {
	if db := database.DB(); db != nil {
		channels.UseDatabase(db)
		contacts.UseDatabase(db)
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
}

// channelOwnedBy avalia se o canal pertence ao userID atual ou está sem
// dono (legado pré-AEP-0052). Canais com dono diferente nunca são
// expostos a usuários que não sejam o dono — a única forma de "tomar"
// um canal alheio é pelo dono atual deletar+recriar.
func channelOwnedBy(cfg *channels.ChannelConfig, userID string) bool {
	if cfg == nil {
		return false
	}
	owner := strings.TrimSpace(cfg.OwnerUserID)
	return owner == "" || owner == userID
}

// errCrossUserChannel é o erro padrão quando um usuário tenta operar em
// canal que pertence a outro. Mensagem é genérica (não revela nem o nome
// do dono real, nem a existência do canal) — defesa contra enumeração.
func errCrossUserChannel(channelName string) error {
	return fmt.Errorf("canal %s não disponível", channelName)
}

// redactChannelSecrets remove tokens em texto plano do cfg antes de
// expor ao frontend. Os refs (BotTokenRef etc.) ficam — a UI usa só
// para mostrar "configurado" e o backend resolve no momento de uso.
//
// Mantém uma cópia para não mutar o cfg de quem escreve no disco.
func redactChannelSecrets(cfg *channels.ChannelConfig) *channels.ChannelConfig {
	if cfg == nil {
		return nil
	}
	c := *cfg
	c.BotToken = ""
	c.AppToken = ""
	c.APIToken = ""
	return &c
}

// GetMessagingStatus delega para MessagingController.
//
// Exige sessão autenticada (AEP-0052 / B6): mostrar status de conexão
// dos messengers só faz sentido pós-login. Como status é instance-wide
// (um adapter Telegram só pode rodar uma vez no processo), não filtra
// por dono — só impede acesso pré-login.
func (a *App) GetMessagingStatus() (map[string]string, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil, err
	}
	return a.msgCtrl.GetMessagingStatus(), nil
}

// GetChannelConfig retorna a configuração do canal, validando ownership.
//
// AEP-0052 / B6: rejeita se o canal pertence a outro usuário. Em canais
// sem dono (legado), retorna config sem tokens em texto plano para que
// a UI possa apresentar a opção de reativar sem vazar credenciais.
func (a *App) GetChannelConfig(channelName string) (*channels.ChannelConfig, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	userID, _ := database.UserIDFromContext(ctx)
	cfg, err := channels.Load(channelName)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &channels.ChannelConfig{}, nil
	}
	if !channelOwnedBy(cfg, userID) {
		return nil, errCrossUserChannel(channelName)
	}
	if strings.TrimSpace(cfg.OwnerUserID) == "" {
		return redactChannelSecrets(cfg), nil
	}
	return cfg, nil
}

// SaveChannelConfig delega para MessagingController.
//
// Carimba o config do canal com o userID autenticado atual em OwnerUserID
// (AEP-0052). Esse valor é o que o gateway usa para escopar conversas
// criadas por mensagens recebidas. O frontend não precisa enviar o campo —
// a fonte da verdade é a sessão ativa do usuário que está configurando o
// canal.
//
// M12: rejeita sobrescrita quando o canal já tem dono diferente do
// usuário atual (workflow de roubo: User A pega cfg do canal B com seus
// tokens e PUT para virar dono). Se o canal não existe ou está sem dono
// (legado), o save adota — caminho legítimo de migração single-user →
// multi-user.
func (a *App) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	if cfg == nil {
		cfg = &channels.ChannelConfig{}
	}
	if existing, _ := channels.Load(channelName); existing != nil {
		owner := strings.TrimSpace(existing.OwnerUserID)
		if owner != "" && owner != userID {
			return errCrossUserChannel(channelName)
		}
	}
	cfg.OwnerUserID = userID
	return a.msgCtrl.SaveChannelConfig(channelName, cfg)
}

// RestartChannel delega para MessagingController, validando ownership.
//
// Reiniciar canal alheio é vetor de DoS contra outro usuário. AEP-0052/B6:
// rejeita silenciosamente quando o canal pertence a outro. Canal sem
// dono também é rejeitado: precisa ser adotado via SaveChannelConfig
// antes (que carimba OwnerUserID) — restart sem dono não tem semântica.
func (a *App) RestartChannel(channelName string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	existing, _ := channels.Load(channelName)
	if existing == nil {
		return fmt.Errorf("canal %s não configurado", channelName)
	}
	if !channelOwnedBy(existing, userID) {
		return errCrossUserChannel(channelName)
	}
	if strings.TrimSpace(existing.OwnerUserID) == "" {
		return fmt.Errorf("canal %s precisa ser reativado (configurações > salvar) antes de reiniciar", channelName)
	}
	return a.msgCtrl.RestartChannel(channelName)
}

// GetAllChannelConfigs retorna apenas canais do usuário atual + canais
// sem dono (legado, para visibilidade da migração). AEP-0052 / B6.
//
// Tokens em texto plano são removidos de canais sem dono — a UI mostra
// "canal legado, reative" mas não pode ser usada para extrair credenciais
// alheias.
func (a *App) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	userID, _ := database.UserIDFromContext(ctx)
	all, err := channels.ListForUser(userID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*channels.ChannelConfig, len(all))
	for name, cfg := range all {
		if !channelOwnedBy(cfg, userID) {
			continue
		}
		if strings.TrimSpace(cfg.OwnerUserID) == "" {
			result[name] = redactChannelSecrets(cfg)
			continue
		}
		result[name] = cfg
	}
	return result, nil
}

// GetChannelTemplates delega para MessagingController. Templates são
// estáticos (não há informação per-user), mas exigimos sessão para
// não expor o catálogo a callers pré-login.
func (a *App) GetChannelTemplates() ([]channels.ChannelTemplate, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil, err
	}
	return a.msgCtrl.GetChannelTemplates(), nil
}

// CreateChannelFromTemplate cria um canal e o atribui ao usuário atual.
//
// AEP-0052 / B6: o template não conhece o conceito de owner. Aqui
// resolvemos em duas etapas — cria o cfg via template (que escreve em
// disco) e logo em seguida atualiza com OwnerUserID. Se outro usuário
// criar um canal com o mesmo nome em paralelo, vence quem grava por
// último (single-process, baixa probabilidade real).
//
// Também rejeita se já existe canal com dono diferente do usuário atual,
// evitando que o caller use Create para sobrescrever.
func (a *App) CreateChannelFromTemplate(templateType string, values map[string]interface{}) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	if existing, _ := channels.Load(templateType); existing != nil {
		owner := strings.TrimSpace(existing.OwnerUserID)
		if owner != "" && owner != userID {
			return errCrossUserChannel(templateType)
		}
	}
	if err := a.msgCtrl.CreateChannelFromTemplate(templateType, values); err != nil {
		return err
	}
	created, err := channels.Load(templateType)
	if err != nil || created == nil {
		return err
	}
	created.OwnerUserID = userID
	return channels.Save(templateType, created)
}

// GetChannelConfigAsMap delega para MessagingController, validando ownership.
func (a *App) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	userID, _ := database.UserIDFromContext(ctx)
	existing, _ := channels.Load(channelName)
	if existing != nil && !channelOwnedBy(existing, userID) {
		return nil, errCrossUserChannel(channelName)
	}
	m, err := a.msgCtrl.GetChannelConfigAsMap(channelName)
	if err != nil {
		return nil, err
	}
	if existing != nil && strings.TrimSpace(existing.OwnerUserID) == "" {
		// Canal legado: redact tokens em texto plano também aqui.
		for _, key := range []string{"bot_token", "app_token", "api_token"} {
			if _, has := m[key]; has {
				m[key] = ""
			}
		}
	}
	return m, nil
}

// AuthorizeMessagingContactFull delega para MessagingController, validando ownership.
func (a *App) AuthorizeMessagingContactFull(channel, contactID, displayName, username string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	existing, _ := channels.Load(channel)
	if existing == nil {
		return fmt.Errorf("canal %s não configurado", channel)
	}
	if !channelOwnedBy(existing, userID) {
		return errCrossUserChannel(channel)
	}
	return a.msgCtrl.AuthorizeMessagingContactFull(channel, contactID, displayName, username)
}

// RemoveAuthorizedContact delega para MessagingController, validando ownership.
func (a *App) RemoveAuthorizedContact(channel, contactID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	existing, _ := channels.Load(channel)
	if existing == nil {
		return fmt.Errorf("canal %s não configurado", channel)
	}
	if !channelOwnedBy(existing, userID) {
		return errCrossUserChannel(channel)
	}
	return a.msgCtrl.RemoveAuthorizedContact(channel, contactID)
}

// GetAuthorizedContacts retorna apenas contatos de canais do usuário atual
// (e de canais legados sem dono). AEP-0052 / B6.
func (a *App) GetAuthorizedContacts() (contacts.ContactsFile, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	userID, _ := database.UserIDFromContext(ctx)
	all, err := a.msgCtrl.GetAuthorizedContacts()
	if err != nil {
		return nil, err
	}
	allChannels, _ := channels.ListAll()
	result := make(contacts.ContactsFile, len(all))
	for channelName, list := range all {
		if cfg, ok := allChannels[channelName]; ok && !channelOwnedBy(cfg, userID) {
			continue
		}
		result[channelName] = list
	}
	return result, nil
}

// GetAvailableChannels retorna canais habilitados do usuário atual + legados
// sem dono. AEP-0052 / B6.
func (a *App) GetAvailableChannels() ([]ChannelInfo, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	userID, _ := database.UserIDFromContext(ctx)
	allChannels, _ := channels.ListAll()
	all := a.msgCtrl.GetAvailableChannels()
	filtered := make([]ChannelInfo, 0, len(all))
	for _, info := range all {
		cfg, ok := allChannels[info.Name]
		if !ok || !channelOwnedBy(cfg, userID) {
			continue
		}
		filtered = append(filtered, info)
	}
	return filtered, nil
}

// AssignConversationToChannel delega para MessagingController, validando
// ownership do canal e da conversa (esta última via DBConversationStore
// que exige RequireUserID).
func (a *App) AssignConversationToChannel(conversationID string, channel, contactID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	userID, _ := database.UserIDFromContext(ctx)
	existing, _ := channels.Load(channel)
	if existing == nil {
		return fmt.Errorf("canal %s não configurado", channel)
	}
	if !channelOwnedBy(existing, userID) {
		return errCrossUserChannel(channel)
	}
	return a.msgCtrl.AssignConversationToChannel(ctx, conversationID, channel, contactID)
}

// UnassignConversationFromChannel delega para MessagingController, validando
// ownership da conversa via ctx escopado.
func (a *App) UnassignConversationFromChannel(conversationID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.msgCtrl.UnassignConversationFromChannel(ctx, conversationID)
}

// GetConversationChannel delega para MessagingController, validando
// ownership da conversa via ctx escopado.
func (a *App) GetConversationChannel(conversationID string) (string, string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return "", "", err
	}
	return a.msgCtrl.GetConversationChannel(ctx, conversationID)
}

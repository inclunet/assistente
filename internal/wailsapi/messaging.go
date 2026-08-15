package wailsapi

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"
)

// Messaging é o bind Wails do domínio messaging / canais / contatos (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// Ownership (OwnerUserID / channelOwnedBy) e redact de tokens legados ficam
// nesta borda, preservando a semântica AEP-0052/B6/M12.
//
// initMessaging, StartAdapters e o gateway de runtime continuam no *App.
type Messaging struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.MessagingController
}

// NewMessaging cria o bind vazio; AttachMessaging preenche session + controller no startup.
func NewMessaging() *Messaging {
	return &Messaging{}
}

// AttachMessaging associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachMessaging(api *Messaging, session Session, ctrl *controllers.MessagingController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
}

func (api *Messaging) deps() (Session, *controllers.MessagingController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrMessagingNotWired
	}
	return api.session, api.ctrl, nil
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
// Mantém uma cópia para não mutar o cfg em cache/DB callers.
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
func (api *Messaging) GetMessagingStatus() (map[string]string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]string, error) {
		return ctrl.GetMessagingStatus(), nil
	})
}

// GetChannelConfig retorna a configuração do canal, validando ownership.
//
// AEP-0052 / B6: rejeita se o canal pertence a outro usuário. Em canais
// sem dono (legado), retorna config sem tokens em texto plano para que
// a UI possa apresentar a opção de reativar sem vazar credenciais.
func (api *Messaging) GetChannelConfig(channelName string) (*channels.ChannelConfig, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*channels.ChannelConfig, error) {
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
	})
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
func (api *Messaging) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		if cfg == nil {
			cfg = &channels.ChannelConfig{}
		}
		if existing, _ := channels.Load(channelName); existing != nil {
			owner := strings.TrimSpace(existing.OwnerUserID)
			if owner != "" && owner != userID {
				return struct{}{}, errCrossUserChannel(channelName)
			}
		}
		cfg.OwnerUserID = userID
		ctrl.SetCredentialUserID(userID)
		return struct{}{}, ctrl.SaveChannelConfig(channelName, cfg)
	})
	return err
}

// RestartChannel delega para MessagingController, validando ownership.
//
// Reiniciar canal alheio é vetor de DoS contra outro usuário. AEP-0052/B6:
// rejeita silenciosamente quando o canal pertence a outro. Canal sem
// dono também é rejeitado: precisa ser adotado via SaveChannelConfig
// antes (que carimba OwnerUserID) — restart sem dono não tem semântica.
func (api *Messaging) RestartChannel(channelName string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		existing, _ := channels.Load(channelName)
		if existing == nil {
			return struct{}{}, fmt.Errorf("canal %s não configurado", channelName)
		}
		if !channelOwnedBy(existing, userID) {
			return struct{}{}, errCrossUserChannel(channelName)
		}
		if strings.TrimSpace(existing.OwnerUserID) == "" {
			return struct{}{}, fmt.Errorf("canal %s precisa ser reativado (configurações > salvar) antes de reiniciar", channelName)
		}
		ctrl.SetCredentialUserID(userID)
		return struct{}{}, ctrl.RestartChannel(channelName)
	})
	return err
}

// GetAllChannelConfigs retorna apenas canais do usuário atual + canais
// sem dono (legado, para visibilidade da migração). AEP-0052 / B6.
//
// Tokens em texto plano são removidos de canais sem dono — a UI mostra
// "canal legado, reative" mas não pode ser usada para extrair credenciais
// alheias.
func (api *Messaging) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]*channels.ChannelConfig, error) {
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
	})
}

// GetChannelTemplates delega para MessagingController. Templates são
// estáticos (não há informação per-user), mas exigimos sessão para
// não expor o catálogo a callers pré-login.
func (api *Messaging) GetChannelTemplates() ([]channels.ChannelTemplate, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]channels.ChannelTemplate, error) {
		return ctrl.GetChannelTemplates(), nil
	})
}

// CreateChannelFromTemplate cria um canal e o atribui ao usuário atual.
//
// AEP-0052 / B6: o template não conhece o conceito de owner. Aqui
// resolvemos em duas etapas — cria o cfg via template (persiste no DB)
// e logo em seguida atualiza com OwnerUserID. Se outro usuário
// criar um canal com o mesmo nome em paralelo, vence quem grava por
// último (single-process, baixa probabilidade real).
//
// Também rejeita se já existe canal com dono diferente do usuário atual,
// evitando que o caller use Create para sobrescrever.
func (api *Messaging) CreateChannelFromTemplate(templateType string, values map[string]interface{}) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		if existing, _ := channels.Load(templateType); existing != nil {
			owner := strings.TrimSpace(existing.OwnerUserID)
			if owner != "" && owner != userID {
				return struct{}{}, errCrossUserChannel(templateType)
			}
		}
		if err := ctrl.CreateChannelFromTemplate(templateType, values); err != nil {
			return struct{}{}, err
		}
		created, err := channels.Load(templateType)
		if err != nil {
			return struct{}{}, err
		}
		if created == nil {
			return struct{}{}, fmt.Errorf("canal %s não encontrado após criação", templateType)
		}
		created.OwnerUserID = userID
		return struct{}{}, channels.Save(templateType, created)
	})
	return err
}

// GetChannelConfigAsMap delega para MessagingController, validando ownership.
func (api *Messaging) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]interface{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		existing, _ := channels.Load(channelName)
		if existing != nil && !channelOwnedBy(existing, userID) {
			return nil, errCrossUserChannel(channelName)
		}
		m, err := ctrl.GetChannelConfigAsMap(channelName)
		if err != nil {
			return nil, err
		}
		if existing != nil && strings.TrimSpace(existing.OwnerUserID) == "" {
			for _, key := range []string{"bot_token", "app_token", "api_token"} {
				if _, has := m[key]; has {
					m[key] = ""
				}
			}
		}
		return m, nil
	})
}

// AuthorizeMessagingContactFull delega para MessagingController, validando ownership.
func (api *Messaging) AuthorizeMessagingContactFull(channel, contactID, displayName, username string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		existing, _ := channels.Load(channel)
		if existing == nil {
			return struct{}{}, fmt.Errorf("canal %s não configurado", channel)
		}
		if !channelOwnedBy(existing, userID) {
			return struct{}{}, errCrossUserChannel(channel)
		}
		return struct{}{}, ctrl.AuthorizeMessagingContactFull(channel, contactID, displayName, username)
	})
	return err
}

// RemoveAuthorizedContact delega para MessagingController, validando ownership.
func (api *Messaging) RemoveAuthorizedContact(channel, contactID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		existing, _ := channels.Load(channel)
		if existing == nil {
			return struct{}{}, fmt.Errorf("canal %s não configurado", channel)
		}
		if !channelOwnedBy(existing, userID) {
			return struct{}{}, errCrossUserChannel(channel)
		}
		return struct{}{}, ctrl.RemoveAuthorizedContact(channel, contactID)
	})
	return err
}

// GetAuthorizedContacts retorna apenas contatos de canais do usuário atual
// (e de canais legados sem dono). AEP-0052 / B6.
func (api *Messaging) GetAuthorizedContacts() (contacts.ContactsFile, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (contacts.ContactsFile, error) {
		userID, _ := database.UserIDFromContext(ctx)
		all, err := ctrl.GetAuthorizedContacts()
		if err != nil {
			return nil, err
		}
		allChannels, err := channels.ListAll()
		if err != nil {
			// Sem o mapa de donos não dá para saber o que é nosso: falhar
			// fechado evita vazar contato de canal alheio (ou sem config).
			return nil, err
		}
		result := make(contacts.ContactsFile, len(all))
		for channelName, list := range all {
			cfg, ok := allChannels[channelName]
			if !ok || !channelOwnedBy(cfg, userID) {
				continue
			}
			result[channelName] = list
		}
		return result, nil
	})
}

// GetAvailableChannels retorna canais habilitados do usuário atual + legados
// sem dono. AEP-0052 / B6.
func (api *Messaging) GetAvailableChannels() ([]controllers.ChannelInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]controllers.ChannelInfo, error) {
		userID, _ := database.UserIDFromContext(ctx)
		allChannels, err := channels.ListAll()
		if err != nil {
			return nil, err
		}
		all := ctrl.GetAvailableChannels()
		filtered := make([]controllers.ChannelInfo, 0, len(all))
		for _, info := range all {
			cfg, ok := allChannels[info.Name]
			if !ok || !channelOwnedBy(cfg, userID) {
				continue
			}
			filtered = append(filtered, info)
		}
		return filtered, nil
	})
}

// AssignConversationToChannel delega para MessagingController, validando
// ownership do canal e da conversa (esta última via DBConversationStore
// que exige RequireUserID).
func (api *Messaging) AssignConversationToChannel(conversationID string, channel, contactID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		userID, _ := database.UserIDFromContext(ctx)
		existing, _ := channels.Load(channel)
		if existing == nil {
			return struct{}{}, fmt.Errorf("canal %s não configurado", channel)
		}
		if !channelOwnedBy(existing, userID) {
			return struct{}{}, errCrossUserChannel(channel)
		}
		return struct{}{}, ctrl.AssignConversationToChannel(ctx, conversationID, channel, contactID)
	})
	return err
}

// UnassignConversationFromChannel delega para MessagingController, validando
// ownership da conversa via ctx escopado.
func (api *Messaging) UnassignConversationFromChannel(conversationID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UnassignConversationFromChannel(ctx, conversationID)
	})
	return err
}

// GetConversationChannel delega para MessagingController, validando
// ownership da conversa via ctx escopado. Devolve um DTO único: o Wails
// só serializa o primeiro valor além do error, e (channel, contactID)
// separados perderiam o contactID no frontend.
func (api *Messaging) GetConversationChannel(conversationID string) (apidto.ConversationChannel, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return apidto.ConversationChannel{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ConversationChannel, error) {
		channel, contactID, err := ctrl.GetConversationChannel(ctx, conversationID)
		if err != nil {
			return apidto.ConversationChannel{}, err
		}
		return apidto.ConversationChannel{Channel: channel, ContactID: contactID}, nil
	})
}

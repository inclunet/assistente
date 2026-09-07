// Package channels gerencia a configuração dos canais de mensageria.
//
// Runtime (AEP-0083): persistência exclusiva via SQLite após UseDatabase.
// Arquivos legados channels/*.json existem apenas para import read-only
// (legacy_import.go) e cleanup opt-in (legacy_cleanup.go) — sem fallback FS.
package channels

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrChannelNotFound indica que o slug não existe no store DB.
var ErrChannelNotFound = errors.New("canal não encontrado")

// ErrDBNotEnabled indica que UseDatabase não foi chamado (runtime fail-closed).
var ErrDBNotEnabled = errors.New("channels DB não habilitado")

// channelsSubdir é o subdiretório legado dentro de .assistente/ (import/cleanup).
const channelsSubdir = "channels"

var mu sync.Mutex

// ChannelConfig é a configuração de um canal de mensageria.
// Continua sendo o DTO público Wails/gateway (AEP-0083); a persistência
// runtime é SQLite via UseDatabase.
type ChannelConfig struct {
	Enabled     bool   `json:"enabled"`
	BotToken    string `json:"bot_token,omitempty"`     // Telegram: token do bot
	BotTokenRef string `json:"bot_token_ref,omitempty"` // Referência no credential manager
	AppToken    string `json:"app_token,omitempty"`     // Slack: app token (socket mode)
	AppTokenRef string `json:"app_token_ref,omitempty"` // Referência no credential manager
	APIToken    string `json:"api_token,omitempty"`     // Signal: token opcional da API
	APITokenRef string `json:"api_token_ref,omitempty"` // Referência no credential manager
	Account     string `json:"account,omitempty"`       // Signal: número da conta vinculada
	APIURL      string `json:"api_url,omitempty"`       // Signal: URL da API
	Profile     string `json:"profile,omitempty"`       // Perfil de chat (vazio = ativo)
	MaxHistory  int    `json:"max_history,omitempty"`   // Mensagens no contexto (0 = padrão)
	MaxContacts int    `json:"max_contacts,omitempty"`  // Máximo de contatos (0/omitido = 1; <0 = ilimitado)

	// Type e DisplayName são persistidos na row DB (AEP-0083). Em v1, Type==Slug.
	Type        string `json:"type,omitempty"`
	DisplayName string `json:"display_name,omitempty"`

	// OwnerUserID é o userID que deve ser usado como dono das conversas
	// criadas a partir de mensagens recebidas neste canal (AEP-0052).
	// Preenchido automaticamente por App.SaveChannelConfig com o userID
	// autenticado atual. Configurações pré-AEP-0052 ficam com OwnerUserID=""
	// até que o usuário re-salve o canal — nesse meio-tempo, conversas
	// criadas por mensagens recebidas nascem órfãs (user_id="") e são
	// adotadas pelo primeiro usuário em AdoptLegacyData.
	// Com DB, mapeia para database.Channel.UserID.
	OwnerUserID string `json:"owner_user_id,omitempty"`

	// Conversations mapeia contactID → conversationID (persistido entre reinícios).
	// Permite reaproveitar conversas existentes ao reiniciar o app.
	// Com DB, vive em channel_contact_conversations.
	Conversations map[string]string `json:"conversations,omitempty"`

	// ReplyChatIDs mapeia contactID → chatID de destino para outbound
	// (ex.: Slack: contact=userID, reply=channelID). Vazio = usar contactID.
	// Com DB, persiste dentro de Settings JSON.
	ReplyChatIDs map[string]string `json:"reply_chat_ids,omitempty"`
}

// GetMaxContacts retorna o limite efetivo de contatos para Authorize/IsAuthorized.
// Configs legadas sem o campo (zero-value) voltam a 1 — single-contact.
// Valores negativos significam ilimitado (retorna -1). Em contacts, 0 é
// normalizado para 1; só n < 0 (via este retorno -1) fica ilimitado, porque
// o limite só é aplicado quando n > 0.
func (c *ChannelConfig) GetMaxContacts() int {
	if c == nil || c.MaxContacts == 0 {
		return 1
	}
	if c.MaxContacts < 0 {
		return -1
	}
	return c.MaxContacts
}

// GetConversationID retorna o conversationID salvo para um contato, ou "" se não existir.
func (c *ChannelConfig) GetConversationID(contactID string) string {
	if c.Conversations == nil {
		return ""
	}
	return c.Conversations[contactID]
}

// SaveConversationID persiste o mapeamento contactID → conversationID no config do canal.
func SaveConversationID(channelName, contactID string, conversationID string) error {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return ErrDBNotEnabled
	}
	return saveConversationIDDB(channelName, contactID, conversationID)
}

// SaveReplyChatID persiste o chatID de outbound para um contato.
// Se replyChatID for vazio ou igual ao contactID, remove override prévio
// (contrato: vazio = usar contactID).
func SaveReplyChatID(channelName, contactID, replyChatID string) error {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return ErrDBNotEnabled
	}

	cfg, err := loadFromDB(channelName)
	if err != nil || cfg == nil {
		return fmt.Errorf("canal %s não encontrado", channelName)
	}

	if replyChatID == "" || replyChatID == contactID {
		if cfg.ReplyChatIDs == nil {
			return nil
		}
		if _, ok := cfg.ReplyChatIDs[contactID]; !ok {
			return nil
		}
		delete(cfg.ReplyChatIDs, contactID)
		if len(cfg.ReplyChatIDs) == 0 {
			cfg.ReplyChatIDs = nil
		}
		// Não re-sincronizar conversations: só Settings/ReplyChatIDs mudaram.
		cfg.Conversations = nil
		return saveToDB(channelName, cfg)
	}

	if cfg.ReplyChatIDs == nil {
		cfg.ReplyChatIDs = make(map[string]string)
	}
	if cfg.ReplyChatIDs[contactID] == replyChatID {
		return nil
	}
	cfg.ReplyChatIDs[contactID] = replyChatID
	cfg.Conversations = nil
	return saveToDB(channelName, cfg)
}

// GetReplyChatID retorna o chatID de outbound para o contato, ou contactID se não houver override.
func GetReplyChatID(channelName, contactID string) string {
	cfg, err := Load(channelName)
	if err != nil || cfg == nil || cfg.ReplyChatIDs == nil {
		return contactID
	}
	if reply, ok := cfg.ReplyChatIDs[contactID]; ok && reply != "" {
		return reply
	}
	return contactID
}

// Load carrega a configuração de um canal. Retorna nil, nil se não existir.
// Sem UseDatabase retorna ErrDBNotEnabled (fail-closed — não confundir com
// canal inexistente).
func Load(name string) (*ChannelConfig, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return nil, ErrDBNotEnabled
	}
	return loadFromDB(name)
}

// Save salva a configuração de um canal no SQLite (exige UseDatabase).
func Save(name string, cfg *ChannelConfig) error {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return ErrDBNotEnabled
	}
	return saveToDB(name, cfg)
}

// Delete remove a configuração de um canal.
func Delete(name string) error {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return ErrDBNotEnabled
	}
	return deleteFromDB(name)
}

// ListAll lista todos os canais que têm configuração.
func ListAll() (map[string]*ChannelConfig, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return nil, ErrDBNotEnabled
	}
	return listAllFromDB()
}

// LoadEnabled retorna apenas os canais habilitados.
func LoadEnabled() (map[string]*ChannelConfig, error) {
	all, err := ListAll()
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]*ChannelConfig)
	for name, cfg := range all {
		if cfg.Enabled {
			enabled[name] = cfg
		}
	}
	return enabled, nil
}

// LoadEnabledForUser retorna canais habilitados do userID (+ órfãos).
// Usado no StartAdapters pós-login para não conectar adapters de outros donos.
func LoadEnabledForUser(userID string) (map[string]*ChannelConfig, error) {
	all, err := ListForUser(userID)
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]*ChannelConfig)
	for name, cfg := range all {
		if cfg != nil && cfg.Enabled {
			enabled[name] = cfg
		}
	}
	return enabled, nil
}

// AdoptOrphans atribui userID como OwnerUserID em todos os canais que estão
// sem dono (configs pré-AEP-0052) e devolve a lista de canais migrados.
//
// Faz parte do fluxo de criação do primeiro admin (AEP-0052 / B10):
// quando o admin inicial é criado, adota canais órfãos no SQLite.
// Sem isso, mensagens recebidas em canais legados são rejeitadas pelo
// gateway (ver internal/messaging/gateway.go) e o usuário não enxerga nada na UI.
//
// IMPORTANTE: NÃO chamar em Login/RefreshAuth — apenas no fluxo
// CreateAdminUser. Em multi-user, o segundo usuário a logar não deve
// "herdar" canais que ficaram sem dono por bug ou corrupção; isso vira um
// fluxo explícito de "reativar canal" no frontend (a discutir).
//
// Idempotente: canais que já têm OwnerUserID preservam o valor existente
// (não sobrescreve dono pré-existente, mesmo se for outro usuário). Apenas
// configs sem dono são reatribuídas.
//
// Adota somente rows no SQLite — não escreve JSON legado (import pós-login
// é o caminho read-only para leftovers em disco). Exige UseDatabase.
func AdoptOrphans(userID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("userID obrigatório para AdoptOrphans")
	}

	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return nil, ErrDBNotEnabled
	}
	return adoptOrphansDB(userID)
}

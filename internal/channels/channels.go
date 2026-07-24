// Package channels gerencia a configuração dos canais de mensageria
// armazenados em .assistente/channels/<nome>.json.
//
// Estrutura:
//
//	.assistente/channels/
//	├── signal.json
//	├── telegram.json
//	└── whatsapp.json
package channels

import (
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// channelsSubdir é o subdiretório dentro de .assistente/
const channelsSubdir = "channels"

var mu sync.Mutex

// ChannelConfig é a configuração de um canal de mensageria.
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
	MaxContacts int    `json:"max_contacts,omitempty"` // Máximo de contatos (0/omitido = 1; <0 = ilimitado)

	// OwnerUserID é o userID que deve ser usado como dono das conversas
	// criadas a partir de mensagens recebidas neste canal (AEP-0052).
	// Preenchido automaticamente por App.SaveChannelConfig com o userID
	// autenticado atual. Configurações pré-AEP-0052 ficam com OwnerUserID=""
	// até que o usuário re-salve o canal — nesse meio-tempo, conversas
	// criadas por mensagens recebidas nascem órfãs (user_id="") e são
	// adotadas pelo primeiro usuário em AdoptLegacyData.
	OwnerUserID string `json:"owner_user_id,omitempty"`

	// Conversations mapeia contactID → conversationID (persistido entre reinícios).
	// Permite reaproveitar conversas existentes ao reiniciar o app.
	Conversations map[string]string `json:"conversations,omitempty"`

	// ReplyChatIDs mapeia contactID → chatID de destino para outbound
	// (ex.: Slack: contact=userID, reply=channelID). Vazio = usar contactID.
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

	cfg, err := loadUnsafe(channelName)
	if err != nil || cfg == nil {
		return fmt.Errorf("canal %s não encontrado", channelName)
	}
	if cfg.Conversations == nil {
		cfg.Conversations = make(map[string]string)
	}
	cfg.Conversations[contactID] = conversationID
	return saveUnsafe(channelName, cfg)
}

// SaveReplyChatID persiste o chatID de outbound para um contato.
// Se replyChatID for vazio ou igual ao contactID, remove override prévio
// (contrato: vazio = usar contactID).
func SaveReplyChatID(channelName, contactID, replyChatID string) error {
	mu.Lock()
	defer mu.Unlock()

	cfg, err := loadUnsafe(channelName)
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
		return saveUnsafe(channelName, cfg)
	}

	if cfg.ReplyChatIDs == nil {
		cfg.ReplyChatIDs = make(map[string]string)
	}
	if cfg.ReplyChatIDs[contactID] == replyChatID {
		return nil
	}
	cfg.ReplyChatIDs[contactID] = replyChatID
	return saveUnsafe(channelName, cfg)
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

// filename retorna "nome.json"
func filename(name string) string {
	return name + ".json"
}

// channelsDir retorna o caminho da pasta channels/ no home.
func channelsHomeDir() string {
	return filepath.Join(configdir.GetHomeDir(), channelsSubdir)
}

// Load carrega a configuração de um canal. Retorna nil se não existir.
func Load(name string) (*ChannelConfig, error) {
	mu.Lock()
	defer mu.Unlock()
	return loadUnsafe(name)
}

func loadUnsafe(name string) (*ChannelConfig, error) {
	basePaths := configdir.GetBasePaths()
	var cfg *ChannelConfig

	fname := filename(name)
	var lastParseErr error
	var lastParsePath string
	for _, base := range basePaths {
		path := filepath.Join(base, channelsSubdir, fname)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var c ChannelConfig
		if err := json.Unmarshal(data, &c); err != nil {
			// M9: registra para eventual propagação. Antes engolíamos
			// silenciosamente — JSON corrompido fazia o canal "sumir"
			// da lista. Combinado com AdoptOrphans/gateway, virava
			// disabled invisível. Agora: se nenhum dos basePaths tem
			// config válido, o erro do último parse é propagado.
			logging.Errorf(context.Background(), "channels.channels", "[Channels] Erro ao parsear %s: %v", path, err)
			lastParseErr = err
			lastParsePath = path
			continue
		}
		cfg = &c
		lastParseErr = nil
	}

	if cfg == nil && lastParseErr != nil {
		return nil, fmt.Errorf("config do canal %s em %s está corrompido: %w", name, lastParsePath, lastParseErr)
	}
	return cfg, nil
}

// Save salva a configuração de um canal em .assistente/channels/<nome>.json.
func Save(name string, cfg *ChannelConfig) error {
	mu.Lock()
	defer mu.Unlock()
	return saveUnsafe(name, cfg)
}

func saveUnsafe(name string, cfg *ChannelConfig) error {
	dir := channelsHomeDir()
	// AEP-0052 / B8: diretório 0700 e arquivos 0600. Configs de canal
	// podem conter tokens em texto plano (BotToken, AppToken, APIToken)
	// quando o credential manager está indisponível ou a migração ainda
	// não rodou. Em ambientes shared (containers, multi-user POSIX),
	// 0644 deixaria os tokens world-readable.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %w", dir, err)
	}
	// Reaperta as permissões caso o diretório já existisse com modo
	// frouxo (ex.: instalações antigas pré-fix). os.MkdirAll é no-op se
	// o diretório já existe — o Chmod garante 0700 em qualquer caso.
	if err := os.Chmod(dir, 0700); err != nil {
		logging.Warnf(context.Background(), "channels.channels", "[Channels] aviso: não foi possível ajustar permissões de %s para 0700: %v", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config do canal %s: %w", name, err)
	}

	path := filepath.Join(dir, filename(name))
	return os.WriteFile(path, data, 0600)
}

// Delete remove a configuração de um canal.
func Delete(name string) error {
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(channelsHomeDir(), filename(name))
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListAll lista todos os canais que têm configuração.
func ListAll() (map[string]*ChannelConfig, error) {
	mu.Lock()
	defer mu.Unlock()
	return listAllUnsafe()
}

// listAllUnsafe percorre todos os basePaths e devolve as configs de canal.
// Não trava o mutex — o caller é responsável (Load/ListAll/AdoptOrphans).
func listAllUnsafe() (map[string]*ChannelConfig, error) {
	result := make(map[string]*ChannelConfig)

	basePaths := configdir.GetBasePaths()
	for _, base := range basePaths {
		channelsPath := filepath.Join(base, channelsSubdir)
		entries, err := os.ReadDir(channelsPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fname := entry.Name()
			if !strings.HasSuffix(fname, ".json") {
				continue
			}
			name := strings.TrimSuffix(fname, ".json")

			path := filepath.Join(channelsPath, fname)
			data, err := os.ReadFile(path)
			if err != nil {
				logging.Errorf(context.Background(), "channels.channels", "[Channels] Erro ao ler %s: %v", path, err)
				continue
			}
			var cfg ChannelConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				// M9: era engolido silenciosamente. Agora loga com
				// detalhes — JSON corrompido fica visível em logs e
				// um eventual healthcheck pode varrer por essa
				// substring para sinalizar na UI.
				logging.Errorf(context.Background(), "channels.channels", "[Channels] Erro ao parsear %s (canal removido da listagem): %v", path, err)
				continue
			}
			result[name] = &cfg
		}
	}

	return result, nil
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

// AdoptOrphans atribui userID como OwnerUserID em todos os canais que estão
// sem dono (configs pré-AEP-0052) e devolve a lista de canais migrados.
//
// Faz parte do fluxo de criação do primeiro admin (AEP-0052 / B10):
// quando o admin inicial é criado, varremos channels/*.json e carimbamos
// OwnerUserID para canais herdados de instalações pré-multi-user. Sem isso,
// mensagens recebidas em canais legados são rejeitadas pelo gateway (ver
// internal/messaging/gateway.go) e o usuário não enxerga nada na UI.
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
// Atomicidade (B9): a função opera inteiramente sob mu.Lock —
// listAllUnsafe + re-leitura por canal + saveUnsafe — para fechar a janela
// TOCTOU em que outro caller poderia setar OwnerUserID entre o ListAll e
// o Save. Re-lemos cada cfg dentro do lock para nunca sobrescrever um
// dono que tenha sido atribuído após a varredura inicial.
func AdoptOrphans(userID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("userID obrigatório para AdoptOrphans")
	}

	mu.Lock()
	defer mu.Unlock()

	all, err := listAllUnsafe()
	if err != nil {
		return nil, err
	}

	migrated := make([]string, 0, len(all))
	for name := range all {
		// Re-lê dentro do lock — entre listAllUnsafe e este ponto não
		// há janela porque o lock está retido; mas faz a re-leitura
		// para deixar explícito o invariante e cobrir mudanças de
		// implementação no listAllUnsafe (caching futuro etc.).
		fresh, err := loadUnsafe(name)
		if err != nil {
			return migrated, fmt.Errorf("erro ao reler canal %s: %w", name, err)
		}
		if fresh == nil || strings.TrimSpace(fresh.OwnerUserID) != "" {
			continue
		}
		fresh.OwnerUserID = userID
		if err := saveUnsafe(name, fresh); err != nil {
			return migrated, fmt.Errorf("erro ao migrar canal %s: %w", name, err)
		}
		migrated = append(migrated, name)
	}
	return migrated, nil
}

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
	"encoding/json"
	"fmt"
	"log"
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
	MaxContacts int    `json:"max_contacts,omitempty"`  // Máximo de contatos autorizados (0 = 1)

	// SIP: configuração para canal de telefonia
	SIPServer                string  `json:"sip_server,omitempty"`                  // Endereço do servidor SIP (ex: "asterisk.local")
	SIPPort                  int     `json:"sip_port,omitempty"`                    // Porta do servidor SIP (0 = padrão: 5060)
	SIPTransport             string  `json:"sip_transport,omitempty"`               // Transporte: "udp" (padrão), "tcp", "tls"
	SIPUser                  string  `json:"sip_user,omitempty"`                    // Ramal/usuário SIP (ex: "100")
	SIPPassword              string  `json:"sip_password,omitempty"`                // Senha SIP (em texto, será migrada para ref)
	SIPPasswordRef           string  `json:"sip_password_ref,omitempty"`            // Referência no credential manager
	SIPDisplayName           string  `json:"sip_display_name,omitempty"`            // Nome exibido no caller ID
	SIPLocalIP               string  `json:"sip_local_ip,omitempty"`                // IP local para bind (vazio = todas as interfaces)
	SIPAudioTuningConfigured bool    `json:"sip_audio_tuning_configured,omitempty"` // Indica que os ajustes abaixo devem sobrescrever os defaults
	SIPDenoise               bool    `json:"sip_denoise,omitempty"`                 // Habilita redução de ruído no preprocessamento SIP
	SIPAGC                   bool    `json:"sip_agc,omitempty"`                     // Habilita AGC no preprocessamento SIP
	SIPNoiseSuppressDB       int     `json:"sip_noise_suppress_db,omitempty"`       // Atenuação máxima de ruído em dB (negativo)
	SIPAGCTarget             int     `json:"sip_agc_target,omitempty"`              // Alvo do AGC em amplitude PCM
	SIPAGCMaxGainDB          int     `json:"sip_agc_max_gain_db,omitempty"`         // Ganho máximo do AGC em dB
	SIPVADMode               int     `json:"sip_vad_mode,omitempty"`                // Modo do WebRTC VAD (0-3)
	SIPVADSpeechMS           int     `json:"sip_vad_speech_ms,omitempty"`           // Duração mínima de fala para onset
	SIPVADSilenceMS          int     `json:"sip_vad_silence_ms,omitempty"`          // Duração de silêncio para fechar segmento
	SIPBargeInThreshold      float64 `json:"sip_barge_in_threshold,omitempty"`      // RMS mínimo para interromper TTS

	// Conversations mapeia contactID → conversationID (persistido entre reinícios).
	// Permite reaproveitar conversas existentes ao reiniciar o app.
	Conversations map[string]uint `json:"conversations,omitempty"`
}

// GetMaxContacts retorna o limite efetivo de contatos (mínimo 1).
func (c *ChannelConfig) GetMaxContacts() int {
	if c.MaxContacts <= 0 {
		return 1
	}
	return c.MaxContacts
}

// GetConversationID retorna o conversationID salvo para um contato, ou 0 se não existir.
func (c *ChannelConfig) GetConversationID(contactID string) uint {
	if c.Conversations == nil {
		return 0
	}
	return c.Conversations[contactID]
}

// SaveConversationID persiste o mapeamento contactID → conversationID no config do canal.
func SaveConversationID(channelName, contactID string, conversationID uint) error {
	mu.Lock()
	defer mu.Unlock()

	cfg, err := loadUnsafe(channelName)
	if err != nil || cfg == nil {
		return fmt.Errorf("canal %s não encontrado", channelName)
	}
	if cfg.Conversations == nil {
		cfg.Conversations = make(map[string]uint)
	}
	cfg.Conversations[contactID] = conversationID
	return saveUnsafe(channelName, cfg)
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
	for _, base := range basePaths {
		path := filepath.Join(base, channelsSubdir, fname)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var c ChannelConfig
		if err := json.Unmarshal(data, &c); err != nil {
			log.Printf("[Channels] Erro ao parsear %s: %v", path, err)
			continue
		}
		cfg = &c // Sobrescreve com maior prioridade
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config do canal %s: %w", name, err)
	}

	path := filepath.Join(dir, filename(name))
	return os.WriteFile(path, data, 0644)
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
				continue // Pula diretórios
			}
			fname := entry.Name()
			if !strings.HasSuffix(fname, ".json") {
				continue
			}
			name := strings.TrimSuffix(fname, ".json")

			path := filepath.Join(channelsPath, fname)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var cfg ChannelConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				continue
			}
			result[name] = &cfg // Maior prioridade sobrescreve
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

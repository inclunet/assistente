package channels

import (
	"encoding/json"
	"fmt"
)

// ChannelTemplate representa um template de configuração para um tipo de canal.
type ChannelTemplate struct {
	Type        string                 `json:"type"`         // "telegram", "signal", "email", "slack", "whatsapp", "teams"
	DisplayName string                 `json:"display_name"` // Nome amigável (ex: "Telegram Bot")
	Description string                 `json:"description"`  // Descrição do canal
	Icon        string                 `json:"icon"`         // Emoji/ícone
	Fields      []ChannelTemplateField `json:"fields"`       // Campos de configuração necessários
	DocURL      string                 `json:"doc_url"`      // Link para documentação
	Supported   bool                   `json:"supported"`    // Se o canal é suportado no backend
}

// ChannelTemplateField representa um campo de configuração do canal.
type ChannelTemplateField struct {
	Key          string      `json:"key"`                     // Nome do campo (ex: "bot_token", "account")
	Label        string      `json:"label"`                   // Label para UI
	Type         string      `json:"type"`                    // "text", "password", "number", "url"
	Required     bool        `json:"required"`                // Se é obrigatório
	Placeholder  string      `json:"placeholder,omitempty"`   // Texto de exemplo
	Description  string      `json:"description,omitempty"`   // Descrição adicional
	DefaultValue interface{} `json:"default_value,omitempty"` // Valor padrão
}

// GetAvailableTemplates retorna todos os templates disponíveis de canais.
func GetAvailableTemplates() []ChannelTemplate {
	return []ChannelTemplate{
		{
			Type:        "telegram",
			DisplayName: "Telegram Bot",
			Description: "Bot do Telegram para receber e enviar mensagens",
			Icon:        "✈️",
			Supported:   true,
			DocURL:      "https://core.telegram.org/bots#how-do-i-create-a-bot",
			Fields: []ChannelTemplateField{
				{
					Key:         "bot_token",
					Label:       "Bot Token",
					Type:        "password",
					Required:    true,
					Placeholder: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
					Description: "Token do bot fornecido pelo @BotFather",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos autorizados (0 = ilimitado)",
				},
			},
		},
		{
			Type:        "signal",
			DisplayName: "Signal",
			Description: "Integração com Signal via signal-cli-rest-api",
			Icon:        "📡",
			Supported:   true,
			DocURL:      "https://github.com/bbernhard/signal-cli-rest-api",
			Fields: []ChannelTemplateField{
				{
					Key:         "api_url",
					Label:       "API URL",
					Type:        "url",
					Required:    true,
					Placeholder: "http://localhost:8080",
					Description: "URL da instância signal-cli-rest-api",
				},
				{
					Key:         "account",
					Label:       "Número da Conta",
					Type:        "text",
					Required:    true,
					Placeholder: "+5511999999999",
					Description: "Número de telefone vinculado (formato E.164)",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos autorizados (0 = ilimitado)",
				},
			},
		},
		{
			Type:        "whatsapp",
			DisplayName: "WhatsApp",
			Description: "Integração com WhatsApp (via API oficial ou whatsapp-web.js)",
			Icon:        "💬",
			Supported:   false,
			DocURL:      "https://developers.facebook.com/docs/whatsapp",
			Fields: []ChannelTemplateField{
				{
					Key:         "api_url",
					Label:       "API URL",
					Type:        "url",
					Required:    true,
					Placeholder: "http://localhost:3000",
					Description: "URL da API do WhatsApp",
				},
				{
					Key:         "api_key",
					Label:       "API Key",
					Type:        "password",
					Required:    false,
					Placeholder: "",
					Description: "Chave de autenticação (se necessário)",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos autorizados (0 = ilimitado)",
				},
			},
		},
		{
			Type:        "slack",
			DisplayName: "Slack",
			Description: "Bot do Slack para canais e mensagens diretas",
			Icon:        "💼",
			Supported:   true,
			DocURL:      "https://api.slack.com/bot-users",
			Fields: []ChannelTemplateField{
				{
					Key:         "bot_token",
					Label:       "Bot Token",
					Type:        "password",
					Required:    true,
					Placeholder: "xoxb-...",
					Description: "Token do bot Slack (começa com xoxb-)",
				},
				{
					Key:         "app_token",
					Label:       "App Token",
					Type:        "password",
					Required:    false,
					Placeholder: "xapp-...",
					Description: "Token do app Slack para Socket Mode (opcional)",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos/canais autorizados",
				},
			},
		},
		{
			Type:        "teams",
			DisplayName: "Microsoft Teams",
			Description: "Bot do Microsoft Teams",
			Icon:        "🔷",
			Supported:   false,
			DocURL:      "https://docs.microsoft.com/en-us/microsoftteams/platform/bots/what-are-bots",
			Fields: []ChannelTemplateField{
				{
					Key:         "app_id",
					Label:       "App ID",
					Type:        "text",
					Required:    true,
					Placeholder: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
					Description: "ID do aplicativo no Azure",
				},
				{
					Key:         "app_password",
					Label:       "App Password",
					Type:        "password",
					Required:    true,
					Placeholder: "",
					Description: "Senha do aplicativo no Azure",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos/canais autorizados",
				},
			},
		},
		{
			Type:        "email",
			DisplayName: "Email",
			Description: "Receber e enviar mensagens por e-mail",
			Icon:        "📧",
			Supported:   false,
			DocURL:      "",
			Fields: []ChannelTemplateField{
				{
					Key:         "smtp_host",
					Label:       "SMTP Host",
					Type:        "text",
					Required:    true,
					Placeholder: "smtp.gmail.com",
					Description: "Servidor SMTP para envio",
				},
				{
					Key:          "smtp_port",
					Label:        "SMTP Port",
					Type:         "number",
					Required:     true,
					DefaultValue: 587,
					Description:  "Porta do servidor SMTP",
				},
				{
					Key:         "smtp_username",
					Label:       "SMTP Username",
					Type:        "text",
					Required:    true,
					Placeholder: "seu-email@gmail.com",
					Description: "Usuário para autenticação SMTP",
				},
				{
					Key:         "smtp_password",
					Label:       "SMTP Password",
					Type:        "password",
					Required:    true,
					Placeholder: "",
					Description: "Senha para autenticação SMTP",
				},
				{
					Key:         "imap_host",
					Label:       "IMAP Host",
					Type:        "text",
					Required:    true,
					Placeholder: "imap.gmail.com",
					Description: "Servidor IMAP para recebimento",
				},
				{
					Key:          "imap_port",
					Label:        "IMAP Port",
					Type:         "number",
					Required:     true,
					DefaultValue: 993,
					Description:  "Porta do servidor IMAP",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 1,
					Description:  "Número máximo de contatos autorizados",
				},
			},
		},
		{
			Type:        "sip",
			DisplayName: "SIP (Telefonia)",
			Description: "Canal de telefonia via protocolo SIP — recebe e realiza chamadas de voz",
			Icon:        "📞",
			Supported:   true,
			DocURL:      "https://github.com/emiago/diago",
			Fields: []ChannelTemplateField{
				{
					Key:         "sip_server",
					Label:       "Servidor SIP",
					Type:        "text",
					Required:    true,
					Placeholder: "asterisk.local ou 192.168.1.100",
					Description: "Endereço do servidor SIP (Asterisk, FreePBX, etc.)",
				},
				{
					Key:          "sip_port",
					Label:        "Porta SIP",
					Type:         "number",
					Required:     false,
					DefaultValue: 5060,
					Description:  "Porta do servidor SIP (padrão: 5060)",
				},
				{
					Key:         "sip_user",
					Label:       "Ramal / Usuário",
					Type:        "text",
					Required:    true,
					Placeholder: "100",
					Description: "Ramal ou usuário SIP para registro",
				},
				{
					Key:         "sip_password",
					Label:       "Senha SIP",
					Type:        "password",
					Required:    true,
					Placeholder: "",
					Description: "Senha de autenticação SIP",
				},
				{
					Key:         "sip_display_name",
					Label:       "Nome de Exibição",
					Type:        "text",
					Required:    false,
					Placeholder: "Assistente IA",
					Description: "Nome exibido no caller ID",
				},
				{
					Key:          "sip_transport",
					Label:        "Transporte",
					Type:         "text",
					Required:     false,
					DefaultValue: "udp",
					Description:  "Protocolo de transporte: udp, tcp ou tls",
				},
				{
					Key:         "sip_local_ip",
					Label:        "IP Local",
					Type:         "text",
					Required:     false,
					Description:  "IP da interface de rede local para bind. Vazio = todas as interfaces.",
				},
				{
					Key:          "max_contacts",
					Label:        "Máximo de Contatos",
					Type:         "number",
					Required:     false,
					DefaultValue: 0,
					Description:  "Número máximo de chamadores autorizados (0 = ilimitado)",
				},
			},
		},
	}
}

// GetSupportedTemplates retorna apenas os templates suportados no backend.
func GetSupportedTemplates() []ChannelTemplate {
	all := GetAvailableTemplates()
	supported := make([]ChannelTemplate, 0, len(all))
	for _, t := range all {
		if t.Supported {
			supported = append(supported, t)
		}
	}
	return supported
}

// CreateFromTemplate cria um arquivo de configuração a partir de um template.
// values é um mapa campo → valor fornecido pelo usuário.
func CreateFromTemplate(templateType string, values map[string]interface{}) error {
	templates := GetAvailableTemplates()
	var template *ChannelTemplate

	for _, t := range templates {
		if t.Type == templateType {
			template = &t
			break
		}
	}

	if template == nil {
		return fmt.Errorf("template de canal '%s' não encontrado", templateType)
	}

	// Valida campos obrigatórios
	for _, field := range template.Fields {
		if field.Required {
			if val, ok := values[field.Key]; !ok || val == nil || val == "" {
				return fmt.Errorf("campo obrigatório '%s' não fornecido", field.Label)
			}
		}
	}

	// Constrói o config
	cfg := &ChannelConfig{
		Enabled:     false, // Desabilitado por padrão até ser configurado
		MaxContacts: 1,
	}

	// Mapeia valores para os campos do ChannelConfig
	for key, value := range values {
		switch key {
		case "bot_token":
			if str, ok := value.(string); ok {
				cfg.BotToken = str
			}
		case "app_token":
			if str, ok := value.(string); ok {
				cfg.AppToken = str
			}
		case "account":
			if str, ok := value.(string); ok {
				cfg.Account = str
			}
		case "api_url":
			if str, ok := value.(string); ok {
				cfg.APIURL = str
			}
		case "sip_server":
			if str, ok := value.(string); ok {
				cfg.SIPServer = str
			}
		case "sip_port":
			switch v := value.(type) {
			case float64:
				cfg.SIPPort = int(v)
			case int:
				cfg.SIPPort = v
			}
		case "sip_user":
			if str, ok := value.(string); ok {
				cfg.SIPUser = str
			}
		case "sip_password":
			if str, ok := value.(string); ok {
				cfg.SIPPassword = str
			}
		case "sip_display_name":
			if str, ok := value.(string); ok {
				cfg.SIPDisplayName = str
			}
		case "sip_transport":
			if str, ok := value.(string); ok {
				cfg.SIPTransport = str
			}
		case "sip_local_ip":
			if str, ok := value.(string); ok {
				cfg.SIPLocalIP = str
			}
		case "max_contacts":
			// Aceita float64 (JSON number) ou int
			switch v := value.(type) {
			case float64:
				cfg.MaxContacts = int(v)
			case int:
				cfg.MaxContacts = v
			case string:
				// Tenta converter string para int
				var num int
				if _, err := fmt.Sscanf(v, "%d", &num); err == nil {
					cfg.MaxContacts = num
				}
			}
		}
	}

	// Aplica valores padrão para campos não fornecidos
	for _, field := range template.Fields {
		if _, provided := values[field.Key]; !provided && field.DefaultValue != nil {
			switch field.Key {
			case "max_contacts":
				if num, ok := field.DefaultValue.(int); ok {
					cfg.MaxContacts = num
				}
			}
		}
	}

	// Salva o arquivo
	return Save(templateType, cfg)
}

// GetChannelConfigAsMap retorna a configuração de um canal como mapa para exibição na UI.
func GetChannelConfigAsMap(name string) (map[string]interface{}, error) {
	cfg, err := Load(name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("canal '%s' não encontrado", name)
	}

	// Serializa para JSON e desserializa como map
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

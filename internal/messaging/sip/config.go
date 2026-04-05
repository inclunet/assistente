package sip

import "fmt"

// SIPConfig cont├®m a configura├º├úo necess├íria para conectar a um servidor SIP.
type SIPConfig struct {
	// Server ├® o endere├ºo do servidor SIP (ex: "asterisk.local", "192.168.1.100").
	Server string

	// Port ├® a porta do servidor SIP (padr├úo: 5060 para UDP/TCP, 5061 para TLS).
	Port int

	// Transport: "udp" (padr├úo), "tcp", "tls", "ws", "wss".
	Transport string

	// User ├® o ramal/usu├írio SIP (ex: "100", "ramal-assistente").
	User string

	// Password ├® a senha de autentica├º├úo SIP.
	Password string

	// DisplayName ├® o nome exibido no caller ID (ex: "Assistente IA").
	DisplayName string

	// LocalIP ├® o endere├ºo IP local para bind do socket SIP.
	// Se vazio, usa "0.0.0.0" (todas as interfaces).
	// ├Ütil quando h├í v├írias interfaces de rede (ex: VPN, Tailscale) e
	// o SO escolhe a interface errada para tr├ífego externo.
	LocalIP string
}

// Validate verifica se a configura├º├úo m├¡nima est├í presente.
func (c *SIPConfig) Validate() error {
	if c.Server == "" {
		return fmt.Errorf("sip: server ├® obrigat├│rio")
	}
	if c.User == "" {
		return fmt.Errorf("sip: user ├® obrigat├│rio")
	}
	if c.Password == "" {
		return fmt.Errorf("sip: password ├® obrigat├│rio")
	}
	return nil
}

// GetPort retorna a porta ou o padr├úo para o transporte.
func (c *SIPConfig) GetPort() int {
	if c.Port > 0 {
		return c.Port
	}
	if c.Transport == "tls" || c.Transport == "wss" {
		return 5061
	}
	return 5060
}

// GetTransport retorna o transporte ou "udp" como padr├úo.
func (c *SIPConfig) GetTransport() string {
	if c.Transport == "" {
		return "udp"
	}
	return c.Transport
}

// GetDisplayName retorna o display name ou "Assistente" como padr├úo.
func (c *SIPConfig) GetDisplayName() string {
	if c.DisplayName == "" {
		return "Assistente"
	}
	return c.DisplayName
}

// GetLocalIP retorna o IP local de bind ou "0.0.0.0" como padr├úo.
func (c *SIPConfig) GetLocalIP() string {
	if c.LocalIP == "" {
		return "0.0.0.0"
	}
	return c.LocalIP
}

// GetNonDefaultPort retorna a porta configurada apenas quando ├® n├úo-padr├úo.
// Retorna 0 (omitir da URI SIP) quando a porta ├® o padr├úo para o transporte,
// permitindo resolu├º├úo DNS SRV conforme RFC 3263.
func (c *SIPConfig) GetNonDefaultPort() int {
	port := c.GetPort()
	transport := c.GetTransport()
	if (transport == "tls" || transport == "wss") && port == 5061 {
		return 0
	}
	if port == 5060 {
		return 0
	}
	return port
}

// ServerURI retorna o endere├ºo SIP completo (ex: "sip:100@asterisk.local:5060").
func (c *SIPConfig) ServerURI() string {
	return fmt.Sprintf("sip:%s@%s:%d", c.User, c.Server, c.GetPort())
}
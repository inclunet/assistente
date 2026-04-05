package sip

import (
	"testing"
)

func TestSIPConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  SIPConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: SIPConfig{
				Server:   "asterisk.local",
				User:     "100",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "missing server",
			config: SIPConfig{
				User:     "100",
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing user",
			config: SIPConfig{
				Server:   "asterisk.local",
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			config: SIPConfig{
				Server: "asterisk.local",
				User:   "100",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSIPConfigGetPort(t *testing.T) {
	tests := []struct {
		name     string
		config   SIPConfig
		wantPort int
	}{
		{name: "default UDP", config: SIPConfig{}, wantPort: 5060},
		{name: "custom port", config: SIPConfig{Port: 5080}, wantPort: 5080},
		{name: "TLS default", config: SIPConfig{Transport: "tls"}, wantPort: 5061},
		{name: "WSS default", config: SIPConfig{Transport: "wss"}, wantPort: 5061},
		{name: "TCP default", config: SIPConfig{Transport: "tcp"}, wantPort: 5060},
		{name: "custom port overrides TLS", config: SIPConfig{Port: 5090, Transport: "tls"}, wantPort: 5090},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetPort()
			if got != tt.wantPort {
				t.Errorf("GetPort() = %d, want %d", got, tt.wantPort)
			}
		})
	}
}

func TestSIPConfigGetTransport(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		want      string
	}{
		{name: "empty ÔåÆ udp", transport: "", want: "udp"},
		{name: "tcp", transport: "tcp", want: "tcp"},
		{name: "tls", transport: "tls", want: "tls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SIPConfig{Transport: tt.transport}
			if got := cfg.GetTransport(); got != tt.want {
				t.Errorf("GetTransport() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSIPConfigGetDisplayName(t *testing.T) {
	cfg := SIPConfig{}
	if got := cfg.GetDisplayName(); got != "Assistente" {
		t.Errorf("GetDisplayName() = %q, want %q", got, "Assistente")
	}

	cfg.DisplayName = "Meu Bot"
	if got := cfg.GetDisplayName(); got != "Meu Bot" {
		t.Errorf("GetDisplayName() = %q, want %q", got, "Meu Bot")
	}
}

func TestSIPConfigGetLocalIP(t *testing.T) {
	cfg := SIPConfig{}
	if got := cfg.GetLocalIP(); got != "0.0.0.0" {
		t.Errorf("GetLocalIP() vazio = %q, want %q", got, "0.0.0.0")
	}

	cfg.LocalIP = "192.168.1.50"
	if got := cfg.GetLocalIP(); got != "192.168.1.50" {
		t.Errorf("GetLocalIP() = %q, want %q", got, "192.168.1.50")
	}
}

func TestSIPConfigGetNonDefaultPort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		transport string
		want      int
	}{
		{name: "udp default 5060 ÔåÆ 0", port: 0, transport: "", want: 0},
		{name: "udp explicit 5060 ÔåÆ 0", port: 5060, transport: "udp", want: 0},
		{name: "udp custom 5080 ÔåÆ 5080", port: 5080, transport: "udp", want: 5080},
		{name: "tls default 5061 ÔåÆ 0", port: 0, transport: "tls", want: 0},
		{name: "tls explicit 5061 ÔåÆ 0", port: 5061, transport: "tls", want: 0},
		{name: "tls custom 5062 ÔåÆ 5062", port: 5062, transport: "tls", want: 5062},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SIPConfig{Port: tt.port, Transport: tt.transport}
			if got := cfg.GetNonDefaultPort(); got != tt.want {
				t.Errorf("GetNonDefaultPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSIPConfigServerURI(t *testing.T) {
	cfg := SIPConfig{
		Server: "192.168.1.100",
		User:   "200",
		Port:   5060,
	}
	want := "sip:200@192.168.1.100:5060"
	if got := cfg.ServerURI(); got != want {
		t.Errorf("ServerURI() = %q, want %q", got, want)
	}
}
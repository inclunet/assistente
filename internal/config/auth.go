package config

import (
	"encoding/json"

	"assistente/internal/configdir"
)

const authConfigFilename = "auth.json"

var authResolver = configdir.NewResolver("")

type AuthConfig struct {
	Mode     string             `json:"mode"`
	HTTP     HTTPAuthAPIConfig  `json:"http"`
	External ExternalAuthConfig `json:"external,omitempty"`
}

type HTTPAuthAPIConfig struct {
	Enabled     bool   `json:"enabled"`
	BindAddress string `json:"bind_address"`
	TLSEnabled  bool   `json:"tls_enabled"`
	TLSCertFile string `json:"tls_cert_file,omitempty"`
	TLSKeyFile  string `json:"tls_key_file,omitempty"`
	DevInsecure bool   `json:"dev_insecure,omitempty"`
}

type ExternalAuthConfig struct {
	Issuer            string   `json:"issuer"`
	Audience          string   `json:"audience"`
	JWKSURL           string   `json:"jwks_url"`
	AllowedAlgorithms []string `json:"allowed_algorithms"`
	RequiredScopes    []string `json:"required_scopes,omitempty"`
	RoleClaim         string   `json:"role_claim,omitempty"`
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		Mode: "local",
		HTTP: HTTPAuthAPIConfig{
			Enabled:     false,
			BindAddress: "127.0.0.1:17652",
		},
		External: ExternalAuthConfig{
			AllowedAlgorithms: []string{"EdDSA", "RS256"},
			RoleClaim:         "roles",
		},
	}
}

func LoadAuthConfig() (*AuthConfig, error) {
	data, _, err := authResolver.Read(authConfigFilename)
	if err != nil {
		return DefaultAuthConfig(), nil
	}
	cfg := DefaultAuthConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Mode == "" {
		cfg.Mode = "local"
	}
	if cfg.HTTP.BindAddress == "" {
		cfg.HTTP.BindAddress = "127.0.0.1:17652"
	}
	if len(cfg.External.AllowedAlgorithms) == 0 {
		cfg.External.AllowedAlgorithms = []string{"EdDSA", "RS256"}
	}
	if cfg.External.RoleClaim == "" {
		cfg.External.RoleClaim = "roles"
	}
	return cfg, nil
}

func SaveAuthConfig(cfg *AuthConfig) error {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return authResolver.Write(authConfigFilename, data)
}

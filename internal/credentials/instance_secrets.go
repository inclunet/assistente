package credentials

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	InstanceSecretJWTSigningKey       = "internal-auth:jwt-signing-key"
	InstanceSecretAuthRefreshToken    = "internal-auth:refresh-token"
	InstanceSecretRefreshTokenPepper  = "internal-auth:refresh-token-pepper"
	InstanceSecretTLSPrivateKey       = "internal-tls:private-key"
	InstanceSecretTLSCertificate      = "internal-tls:certificate"
)

func IsInstanceSecretPattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	return strings.HasPrefix(pattern, "internal-auth:") || strings.HasPrefix(pattern, "internal-tls:")
}

// RegisterInstanceSecret stores an instance-scoped secret encrypted by the
// global DEK. Instance secrets intentionally have no user_id.
func (m *Manager) RegisterInstanceSecret(pattern, value string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("pattern do segredo de instância é obrigatório")
	}
	if !IsManagedPattern(pattern) {
		return fmt.Errorf("pattern %q não é reservado para segredo gerenciado", pattern)
	}
	return m.RegisterStoredCredentialWithContext(context.Background(), StoredCredential{
		Pattern: pattern,
		Auth: &AuthConfig{
			Type:  "secret",
			Token: value,
		},
	})
}

func (m *Manager) GetInstanceSecret(pattern string) (string, bool, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", false, errors.New("pattern do segredo de instância é obrigatório")
	}
	if !IsManagedPattern(pattern) {
		return "", false, fmt.Errorf("pattern %q não é reservado para segredo gerenciado", pattern)
	}

	auth, err := m.getInstanceSecretAuth(pattern, true)
	if err != nil {
		return "", false, err
	}
	if auth == nil {
		auth, err = m.getInstanceSecretAuth(pattern, false)
		if err != nil || auth == nil {
			return "", false, err
		}
	}
	return auth.Token, auth.Token != "", nil
}

func (m *Manager) getInstanceSecretAuth(pattern string, requireUnscoped bool) (*AuthConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, dc := range m.credentials {
		if dc.Pattern != pattern {
			continue
		}
		if requireUnscoped && dc.UserID != "" {
			continue
		}
		auth, err := m.decryptAuth(dc.Auth)
		if err != nil {
			scope := dc.UserID
			if scope == "" {
				scope = "<instância>"
			}
			return nil, fmt.Errorf("credencial %q ilegível para escopo %q: %w", pattern, scope, err)
		}
		return auth, nil
	}
	return nil, nil
}

func (m *Manager) DeleteInstanceSecret(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("pattern do segredo de instância é obrigatório")
	}
	if !IsManagedPattern(pattern) {
		return fmt.Errorf("pattern %q não é reservado para segredo gerenciado", pattern)
	}
	return m.DeletePattern(context.Background(), pattern)
}

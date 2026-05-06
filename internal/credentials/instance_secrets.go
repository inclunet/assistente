package credentials

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	InstanceSecretJWTSigningKey  = "internal-auth:jwt-signing-key"
	InstanceSecretTLSPrivateKey  = "internal-tls:private-key"
	InstanceSecretTLSCertificate = "internal-tls:certificate"
)

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
	auth, err := m.GetByPatternWithContext(context.Background(), pattern)
	if err != nil || auth == nil {
		return "", false, err
	}
	return auth.Token, auth.Token != "", nil
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

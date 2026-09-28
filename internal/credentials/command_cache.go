package credentials

import (
	"context"
	"errors"
)

// O token materializado é cifrado só em memória. Cada entrada/usuário tem sua
// própria exclusão de execução: consumidores aguardam com seu próprio contexto.
type commandCredentialCache struct {
	gate       chan struct{}
	encrypted  *AuthConfig
	generation uint64
	cancel     context.CancelFunc
}

var errCommandCredentialChanged = errors.New("credencial de comando alterada durante a resolução")

// Chamado somente sob Manager.mu, nunca aguarda a execução externa.
func (dc *DomainCredential) invalidateCommandCache() {
	dc.command.encrypted = nil
	dc.command.generation++
	if dc.command.cancel != nil {
		dc.command.cancel()
	}
}

// ClearCommandCache descarta tokens e invalida resoluções da sessão anterior.
func (m *Manager) ClearCommandCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, dc := range m.credentials {
		if dc.Auth.Source != "command" {
			continue
		}
		dc.invalidateCommandCache()
		m.credentials[i] = &DomainCredential{ID: dc.ID, UserID: dc.UserID, Pattern: dc.Pattern, regex: dc.regex, Auth: dc.Auth}
	}
}

func (m *Manager) commandEntryCurrent(dc *DomainCredential) bool {
	for _, current := range m.credentials {
		if current == dc {
			return true
		}
	}
	return false
}

func (m *Manager) resolveCredentialSource(ctx context.Context, dc *DomainCredential, auth *AuthConfig) (*AuthConfig, error) {
	if auth.Source != "command" {
		return ResolveSource(ctx, auth)
	}
	if err := ValidateSource(auth); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if !m.commandEntryCurrent(dc) {
		m.mu.Unlock()
		return nil, errCommandCredentialChanged
	}
	if dc.command.gate == nil {
		dc.command.gate = make(chan struct{}, 1)
	}
	gate := dc.command.gate
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-gate }()
	m.mu.Lock()
	if !m.commandEntryCurrent(dc) {
		m.mu.Unlock()
		return nil, errCommandCredentialChanged
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if dc.command.encrypted != nil {
		value, err := m.decryptAuth(dc.command.encrypted)
		if value != nil {
			value.commandEntry = dc
			value.commandGeneration = dc.command.generation
		}
		m.mu.Unlock()
		return value, err
	}
	generation := dc.command.generation
	commandCtx, cancel := context.WithCancel(ctx)
	dc.command.cancel = cancel
	m.mu.Unlock()
	value, err := ResolveSource(commandCtx, auth)
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	dc.command.cancel = nil
	if !m.commandEntryCurrent(dc) || dc.command.generation != generation {
		return nil, errCommandCredentialChanged
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	encrypted, err := m.encryptAuth(value)
	if err != nil {
		return nil, err
	}
	dc.command.encrypted = encrypted
	value.commandEntry = dc
	value.commandGeneration = generation
	return value, nil
}

// Um 401 atrasado não pode apagar um token mais novo. O recibo também impede
// renovar credenciais excluídas/substituídas enquanto o request estava em voo.
func (m *Manager) rejectCommandCredential(auth *AuthConfig) bool {
	if auth == nil || auth.commandEntry == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	dc := auth.commandEntry
	if !m.commandEntryCurrent(dc) {
		return false
	}
	if dc.command.generation == auth.commandGeneration {
		dc.invalidateCommandCache()
	}
	return true
}

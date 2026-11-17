package trustscope

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	"assistente/internal/authorization"
	"assistente/internal/database"
)

func (m *Manager[E, Q]) sessionID(key sessionKey, generation uint64, entry E) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", key.User, key.Conversation, generation, m.config.Adapter.Identity(entry))))
	return fmt.Sprintf("session:%s:%x", m.config.Domain, digest)
}

// SessionRules consulta todas as conversas em memória deste usuário. A chave
// inclui a geração: uma linha antiga não pode revogar a conversa reciclada.
func (m *Manager[E, Q]) SessionRules(ctx context.Context) ([]database.AuthorizationRule, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := make([]database.AuthorizationRule, 0)
	for key, entries := range m.session {
		if key.User != user {
			continue
		}
		for _, entry := range entries {
			row, err := m.rule(entry, key.Conversation)
			if err != nil {
				return nil, err
			}
			row.ID = m.sessionID(key, m.generation[key], entry)
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

func (m *Manager[E, Q]) RevokeSession(ctx context.Context, id string) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, entries := range m.session {
		if key.User != user {
			continue
		}
		for index, entry := range entries {
			if m.sessionID(key, m.generation[key], entry) == id {
				m.session[key] = append(entries[:index], entries[index+1:]...)
				// Consultas e consentimentos que capturaram a revisão antiga
				// não podem publicar uma cópia da concessão após o sucesso.
				m.generation[key]++
				return nil
			}
		}
	}
	return authorization.ErrNotFound
}

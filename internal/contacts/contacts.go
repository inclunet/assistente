// Package contacts gerencia a lista centralizada de contatos autorizados
// para todos os canais de mensageria (Signal, Telegram, WhatsApp, etc.).
//
// O número máximo de contatos por canal é configurável no config do canal
// (campo max_contacts). Quando o limite é atingido, novos contatos são
// silenciosamente ignorados até que um seja removido.
//
// Formato do arquivo contacts.json:
//
//	{
//	  "signal": [
//	    { "id": "uuid", "display_name": "Fulano", ... }
//	  ],
//	  "telegram": [
//	    { "id": "123", "display_name": "Fulano", ... }
//	  ]
//	}
package contacts

import (
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const contactsFilename = "contacts.json"

// resolver opera na raiz de .assistente/
var resolver = configdir.NewResolver("")

// mutex para serializar leitura/escrita
var mu sync.Mutex

// AuthorizedContact representa um contato autorizado para um canal.
type AuthorizedContact struct {
	ID           string `json:"id"`                 // Identificador primário (UUID, phone, chatID)
	DisplayName  string `json:"display_name"`       // Nome de exibição
	Username     string `json:"username,omitempty"` // Identificador secundário
	AuthorizedAt string `json:"authorized_at"`      // Data da autorização (ISO 8601)
}

// ContactsFile é o mapa canal → lista de contatos autorizados.
type ContactsFile map[string][]*AuthorizedContact

// Load carrega o arquivo contacts.json. Retorna mapa vazio se não existir.
func Load() (ContactsFile, error) {
	mu.Lock()
	defer mu.Unlock()
	return loadUnsafe()
}

func loadUnsafe() (ContactsFile, error) {
	data, resolved, err := resolver.Read(contactsFilename)
	if err != nil {
		return make(ContactsFile), nil
	}

	var contacts ContactsFile
	if err := json.Unmarshal(data, &contacts); err != nil {
		logging.Errorf(context.Background(), "contacts.contacts", "[Contacts] arquivo %s corrompido: %v", contactsFilename, err)
		if resolved != nil {
			backupCorruptedContactsFile(resolved.Path)
		}
		empty := make(ContactsFile)
		if saveErr := saveUnsafe(empty); saveErr != nil {
			logging.Errorf(context.Background(), "contacts.contacts", "[Contacts] falha ao recriar %s: %v", contactsFilename, saveErr)
		}
		return empty, nil
	}
	if contacts == nil {
		contacts = make(ContactsFile)
	}
	return contacts, nil
}

func backupCorruptedContactsFile(originalPath string) {
	if originalPath == "" {
		return
	}
	ts := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.corrupt-%s.bak", base, ts))

	if err := os.Rename(originalPath, backupPath); err == nil {
		return
	}

	if data, readErr := os.ReadFile(originalPath); readErr == nil {
		_ = os.WriteFile(backupPath, data, 0644)
	}
}

func saveUnsafe(contacts ContactsFile) error {
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar contatos: %w", err)
	}
	return resolver.Write(contactsFilename, data)
}

// GetAll retorna todos os contatos autorizados (mapa canal → lista).
func GetAll() (ContactsFile, error) {
	return Load()
}

// GetForChannel retorna os contatos autorizados de um canal.
func GetForChannel(channel string) ([]*AuthorizedContact, error) {
	contacts, err := Load()
	if err != nil {
		return nil, err
	}
	return contacts[channel], nil
}

// IsAuthorized verifica se algum dos identificadores fornecidos corresponde
// a um contato autorizado do canal.
//
// Retornos:
//   - (true, true)  → canal tem contatos e um deles bate
//   - (true, false) → canal tem contatos mas nenhum bate (limite atingido ou outro contato)
//   - (false, false) → canal não tem contatos autorizados (vaga disponível)
func IsAuthorized(channel string, maxContacts int, identifiers ...string) (hasContacts bool, isAllowed bool) {
	mu.Lock()
	defer mu.Unlock()

	contacts, err := loadUnsafe()
	if err != nil {
		return false, false
	}

	channelContacts := contacts[channel]
	if len(channelContacts) == 0 {
		return false, false
	}

	// Verifica se algum identificador bate com um contato existente
	for _, contact := range channelContacts {
		for _, id := range identifiers {
			if id == "" {
				continue
			}
			if id == contact.ID || id == contact.Username {
				return true, true
			}
		}
	}

	// Tem contatos mas nenhum bateu — verifica se há vaga
	if maxContacts > 0 && len(channelContacts) >= maxContacts {
		return true, false // Limite atingido, rejeita
	}

	// Há vaga (ou limite ilimitado) — contato novo pode ser autorizado
	return false, false
}

// Authorize adiciona um contato à lista de um canal.
// Respeita o limite maxContacts. Retorna erro se o limite foi atingido.
// Se o contato já existe (mesmo ID), atualiza os dados.
func Authorize(channel string, id, displayName, username string, maxContacts int) error {
	mu.Lock()
	defer mu.Unlock()

	contacts, err := loadUnsafe()
	if err != nil {
		return err
	}

	channelContacts := contacts[channel]

	// Verifica se já existe (atualiza)
	for _, c := range channelContacts {
		if c.ID == id {
			c.DisplayName = displayName
			c.Username = username
			c.AuthorizedAt = time.Now().UTC().Format(time.RFC3339)
			return saveUnsafe(contacts)
		}
	}

	// Novo contato — verifica limite (maxContacts <= 0 = ilimitado)
	if maxContacts > 0 && len(channelContacts) >= maxContacts {
		return fmt.Errorf("limite de %d contato(s) atingido para o canal %s", maxContacts, channel)
	}

	contacts[channel] = append(channelContacts, &AuthorizedContact{
		ID:           id,
		DisplayName:  displayName,
		Username:     username,
		AuthorizedAt: time.Now().UTC().Format(time.RFC3339),
	})

	return saveUnsafe(contacts)
}

// Remove remove um contato específico de um canal.
func Remove(channel, contactID string) error {
	mu.Lock()
	defer mu.Unlock()

	contacts, err := loadUnsafe()
	if err != nil {
		return err
	}

	channelContacts := contacts[channel]
	filtered := make([]*AuthorizedContact, 0, len(channelContacts))
	for _, c := range channelContacts {
		if c.ID != contactID {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == len(channelContacts) {
		return nil // Não encontrou — nada para remover
	}

	contacts[channel] = filtered
	return saveUnsafe(contacts)
}

// RemoveAll remove todos os contatos de um canal.
func RemoveAll(channel string) error {
	mu.Lock()
	defer mu.Unlock()

	contacts, err := loadUnsafe()
	if err != nil {
		return err
	}

	delete(contacts, channel)
	return saveUnsafe(contacts)
}

// Count retorna o número de contatos autorizados de um canal.
func Count(channel string) int {
	mu.Lock()
	defer mu.Unlock()

	contacts, err := loadUnsafe()
	if err != nil {
		return 0
	}
	return len(contacts[channel])
}

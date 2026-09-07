// Package contacts gerencia a lista centralizada de contatos autorizados
// para todos os canais de mensageria (Signal, Telegram, WhatsApp, etc.).
//
// Runtime (AEP-0083): persistência exclusiva via SQLite após UseDatabase
// (tabela channel_contacts). O arquivo legado contacts.json existe apenas
// para import read-only (channels/legacy_import.go) e cleanup opt-in —
// sem fallback FS nesta fachada.
//
// O número máximo de contatos por canal é configurável no config do canal
// (campo max_contacts). Preferir ChannelConfig.GetMaxContacts() ao passar o
// limite. Na API deste pacote: 0 = default 1; valor negativo = ilimitado.
// Quando o limite é atingido, novos contatos são silenciosamente ignorados
// até que um seja removido.
package contacts

import (
	"errors"
	"sync"
)

// ErrDBNotEnabled indica que UseDatabase não foi chamado (runtime fail-closed).
var ErrDBNotEnabled = errors.New("contacts DB não habilitado")

// normalizeMaxContacts alinha a API de contacts com GetMaxContacts:
// 0 → 1 (default legado seguro); <0 permanece negativo (ilimitado nas
// checagens `maxContacts > 0`); >0 → valor informado.
func normalizeMaxContacts(maxContacts int) int {
	if maxContacts == 0 {
		return 1
	}
	return maxContacts
}

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

// Load carrega todos os contatos do DB. Retorna mapa vazio se não houver.
// Sem UseDatabase retorna ErrDBNotEnabled (fail-closed).
func Load() (ContactsFile, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return nil, ErrDBNotEnabled
	}
	return getAllDB()
}

// GetAll retorna todos os contatos autorizados (mapa canal → lista).
func GetAll() (ContactsFile, error) {
	return Load()
}

// GetForChannel retorna os contatos autorizados de um canal.
func GetForChannel(channel string) ([]*AuthorizedContact, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return nil, ErrDBNotEnabled
	}
	return getForChannelDB(channel)
}

// IsAuthorized verifica se algum dos identificadores fornecidos corresponde
// a um contato autorizado do canal.
//
// Retornos (usados pelo Gateway):
//   - (true, true)   → remetente já autorizado
//   - (true, false)  → canal no limite e remetente fora da lista (rejeitar)
//   - (false, false) → há vaga para pareamento: canal sem contatos, OU já há
//     contatos mas ainda cabe mais um (maxContacts < 0 = ilimitado). Neste
//     caso o primeiro bool NÃO significa “canal vazio” — significa “não
//     rejeitar; seguir fluxo de pareamento/autorização”.
//
// Sem UseDatabase: fail-closed (true, false) — rejeita sem abrir pareamento.
// Erros internos no caminho DB (ex.: channels sem UseDatabase) também
// rejeitam com (true, false); só (false, false) quando a lista está
// vazia de fato.
func IsAuthorized(channel string, maxContacts int, identifiers ...string) (hasContacts bool, isAllowed bool) {
	mu.Lock()
	defer mu.Unlock()

	maxContacts = normalizeMaxContacts(maxContacts)

	if !usingDB() {
		return true, false
	}
	return isAuthorizedDB(channel, maxContacts, identifiers...)
}

// Authorize adiciona um contato à lista de um canal.
// Respeita o limite maxContacts. Retorna erro se o limite foi atingido.
// Se o contato já existe (mesmo ID), atualiza os dados.
func Authorize(channel string, id, displayName, username string, maxContacts int) error {
	mu.Lock()
	defer mu.Unlock()

	maxContacts = normalizeMaxContacts(maxContacts)

	if !usingDB() {
		return ErrDBNotEnabled
	}
	return authorizeDB(channel, id, displayName, username, maxContacts)
}

// Remove remove um contato específico de um canal.
func Remove(channel, contactID string) error {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return ErrDBNotEnabled
	}
	return removeDB(channel, contactID)
}

// RemoveAll remove todos os contatos de um canal.
func RemoveAll(channel string) error {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return ErrDBNotEnabled
	}
	return removeAllDB(channel)
}

// Count retorna o número de contatos autorizados de um canal.
// Sem UseDatabase retorna 0 (fail-closed).
func Count(channel string) int {
	mu.Lock()
	defer mu.Unlock()

	if !usingDB() {
		return 0
	}
	return countDB(channel)
}

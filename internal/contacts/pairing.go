package contacts

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// PairingCode representa um código de pareamento gerado para um contato desconhecido
type PairingCode struct {
	Code      string    // 6 dígitos
	Channel   string    // signal, telegram, etc
	ContactID string    // ID do contato
	CreatedAt time.Time // Quando foi gerado
	ExpiresAt time.Time // Quando expira (5 minutos)
	Attempts  int       // Número de tentativas falhadas
}

// PairingManager gerencia códigos de pareamento
type PairingManager struct {
	mu    sync.RWMutex
	codes map[string]*PairingCode // key: channel:contactID
	// Usar um cleanup periódico para remover códigos expirados
}

var pairingManager = &PairingManager{
	codes: make(map[string]*PairingCode),
}

const (
	pairingTimeout   = 5 * time.Minute
	maxAttempts      = 3
	pairingCodeLength = 6
)

// GeneratePairingCode gera um novo código de pareamento para um contato desconhecido
func GeneratePairingCode(channel, contactID string) string {
	pairingManager.mu.Lock()
	defer pairingManager.mu.Unlock()

	code := generateRandomCode(pairingCodeLength)
	key := fmt.Sprintf("%s:%s", channel, contactID)

	pairingManager.codes[key] = &PairingCode{
		Code:      code,
		Channel:   channel,
		ContactID: contactID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(pairingTimeout),
		Attempts:  0,
	}

	return code
}

// ValidatePairingCode valida um código de pareamento
// Retorna:
//   - (true, nil) se o código é válido
//   - (false, "error message") se expirou, tentativas excedidas, ou código errado
func ValidatePairingCode(channel, contactID, code string) (bool, error) {
	pairingManager.mu.Lock()
	defer pairingManager.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channel, contactID)
	pairing, exists := pairingManager.codes[key]

	if !exists {
		return false, fmt.Errorf("nenhum código de pareamento pendente para este contato")
	}

	// Verifica expiração
	if time.Now().UTC().After(pairing.ExpiresAt) {
		delete(pairingManager.codes, key)
		return false, fmt.Errorf("código expirou (5 minutos)")
	}

	// Verifica tentativas excedidas
	if pairing.Attempts >= maxAttempts {
		delete(pairingManager.codes, key)
		return false, fmt.Errorf("muitas tentativas falhadas. Tente novamente mais tarde")
	}

	// Valida o código
	if code != pairing.Code {
		pairing.Attempts++
		return false, fmt.Errorf("código inválido (%d/%d tentativas)", pairing.Attempts, maxAttempts)
	}

	// Código válido — remove para evitar reutilização
	delete(pairingManager.codes, key)
	return true, nil
}

// CancelPairingCode remove um código de pareamento (ex: usuário recusou)
func CancelPairingCode(channel, contactID string) {
	pairingManager.mu.Lock()
	defer pairingManager.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channel, contactID)
	delete(pairingManager.codes, key)
}

// GetPairingCode retorna o código pendente (apenas para debug/UI)
func GetPairingCode(channel, contactID string) *PairingCode {
	pairingManager.mu.RLock()
	defer pairingManager.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", channel, contactID)
	pairing, exists := pairingManager.codes[key]
	if !exists {
		return nil
	}

	// Verifica expiração
	if time.Now().UTC().After(pairing.ExpiresAt) {
		return nil
	}

	// Retorna cópia para segurança
	return &PairingCode{
		Code:      pairing.Code,
		Channel:   pairing.Channel,
		ContactID: pairing.ContactID,
		CreatedAt: pairing.CreatedAt,
		ExpiresAt: pairing.ExpiresAt,
		Attempts:  pairing.Attempts,
	}
}

// CleanupExpiredCodes remove códigos expirados (deve ser chamado periodicamente)
func CleanupExpiredCodes() int {
	pairingManager.mu.Lock()
	defer pairingManager.mu.Unlock()

	now := time.Now().UTC()
	count := 0

	for key, pairing := range pairingManager.codes {
		if now.After(pairing.ExpiresAt) {
			delete(pairingManager.codes, key)
			count++
		}
	}

	return count
}

// generateRandomCode gera um código aleatório de N dígitos
func generateRandomCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(48 + rand.Intn(10)) // '0' a '9'
	}
	return string(b)
}

func init() {
	// Inicia limpeza periódica de códigos expirados
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpiredCodes()
		}
	}()
}

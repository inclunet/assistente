package credentials

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
)

// gcmMinCiphertextLen é o tamanho mínimo (em bytes, pós-base64) de um
// ciphertext produzido por `Manager.encrypt`: nonce GCM (12 bytes) +
// tag de autenticação (16 bytes). Qualquer payload menor não pode ter
// saído de `encrypt`.
const gcmMinCiphertextLen = 12 + 16

// refreshTokenReencryptStore é a interface opcional que o store
// precisa implementar para a re-cifragem one-shot de refresh tokens
// legados (issue #236). Assim como `allCredentialsLister` /
// `unreadableCredentialPurger`, ela ignora intencionalmente o
// user-scope: é um caminho de manutenção executado no boot, antes de
// qualquer login, e a DEK é única por instância (AEP-0052/0061) —
// portanto cifrar linhas de todos os usuários com `m.encKey` é
// correto. NUNCA use esses métodos para servir requests do app.
type refreshTokenReencryptStore interface {
	ListCredentialsWithRefreshTokensIgnoringScope(ctx context.Context) ([]StoredCredential, error)
	UpdateRefreshTokenEncByID(ctx context.Context, id, value string) error
}

// reencryptLegacyPlaintextRefreshTokens re-cifra, com a DEK atual,
// valores de `credential_entries.refresh_token_enc` que ainda estejam
// em texto plano. É a contraparte da migração `migrateRefreshURLToEnc`
// (internal/database): aquela migração roda no boot do banco, ANTES da
// DEK estar carregada (o pacote database não pode importar credentials),
// e por isso copiava o conteúdo legado de `refresh_url` como estava.
// Esta função fecha o gap assim que o Manager é configurado com a DEK
// do keychain — ainda no startup, antes de qualquer login.
//
// Classificação de cada valor não vazio:
//
//  1. `decrypt` funciona → já é ciphertext da DEK atual; intocado.
//  2. não é base64 válido, ou decodifica para menos que nonce+tag →
//     impossível ter saído de `encrypt`; é texto plano legado (ex.:
//     URL com token na query, que contém ':' e falha o decode) →
//     cifra e regrava.
//  3. base64 plausível mas `decrypt` falha → ambíguo. Só tratamos como
//     ciphertext órfão quando a credencial inteira não tem nenhum campo
//     sensível decriptável com a DEK atual; caso contrário, preferimos
//     re-cifrar como texto plano legado para não deixar falsos positivos
//     em claro.
//
// Entradas já marcadas como ilegíveis pelo scan de integridade
// (`UnreadableCredentialIDs`, AEP-0061) só são puladas quando o refresh
// token tem formato de ciphertext; refresh tokens claramente em texto
// plano ainda são re-cifrados.
//
// Idempotente: na segunda execução todos os valores re-cifrados caem
// no caso 1. Retorna quantas linhas foram regravadas.
func (m *Manager) reencryptLegacyPlaintextRefreshTokens(ctx context.Context) (int, error) {
	if m == nil || m.store == nil {
		return 0, nil
	}
	store, ok := m.store.(refreshTokenReencryptStore)
	if !ok {
		return 0, nil
	}

	candidates, err := store.ListCredentialsWithRefreshTokensIgnoringScope(ctx)
	if err != nil {
		return 0, fmt.Errorf("listar credenciais para re-cifragem de refresh tokens legados: %w", err)
	}

	unreadable := make(map[string]struct{})
	for _, id := range m.integrity.get().UnreadableCredentialIDs {
		unreadable[id] = struct{}{}
	}

	reencrypted := 0
	for _, entry := range candidates {
		if entry.Auth == nil {
			continue
		}
		value := entry.Auth.RefreshURL
		if _, err := m.decrypt(value); err == nil {
			// Caso 1: já cifrado com a DEK atual.
			continue
		}
		looksLikeCiphertext := couldBeGCMCiphertext(value)
		if _, isUnreadable := unreadable[entry.ID]; isUnreadable && looksLikeCiphertext {
			// Credencial já diagnosticada como órfã e refresh com
			// formato de ciphertext: mantém para o fluxo AEP-0061.
			continue
		}
		if looksLikeCiphertext && !m.isAuthDecryptable(entry.Auth) {
			// Caso 3: possível ciphertext órfão (DEK divergente).
			// Deixado para o fluxo de integridade do AEP-0061.
			log.Printf("[Credentials] refresh token da credencial %s não decifra mas tem formato de ciphertext — pulando re-cifragem (possível DEK divergente, ver AEP-0061)", entry.ID)
			continue
		}
		// Caso 2: texto plano legado. Cifra antes de regravar.
		enc, err := m.encrypt(value)
		if err != nil {
			return reencrypted, fmt.Errorf("cifrar refresh token legado da credencial %s: %w", entry.ID, err)
		}
		if err := store.UpdateRefreshTokenEncByID(ctx, entry.ID, enc); err != nil {
			return reencrypted, fmt.Errorf("regravar refresh token cifrado da credencial %s: %w", entry.ID, err)
		}
		reencrypted++
	}
	return reencrypted, nil
}

// couldBeGCMCiphertext indica se o valor TEM FORMATO compatível com a
// saída de `Manager.encrypt` (base64 std de nonce+ciphertext+tag).
// Formato compatível não prova que é ciphertext — apenas que não dá
// para descartar; valores que falham este teste certamente são texto
// plano.
func couldBeGCMCiphertext(value string) bool {
	value = strings.TrimSpace(value)
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return false
	}
	return len(data) >= gcmMinCiphertextLen
}

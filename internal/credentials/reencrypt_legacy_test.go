package credentials

import (
	"context"
	"testing"

	"assistente/internal/database"
)

// insertLegacyCredentialEntry insere uma linha diretamente em
// credential_entries simulando o estado pós-migrateRefreshURLToEnc de
// uma base legada: `refresh_token_enc` carregando o valor copiado de
// `refresh_url` (texto plano), demais campos como informado.
func insertLegacyCredentialEntry(t *testing.T, id, userID, pattern, tokenEnc, refreshTokenEnc string) {
	t.Helper()
	db := database.DB()
	if err := db.Exec(
		`INSERT INTO credential_entries (id, user_id, pattern, auth_type, token_enc, username, password_enc, headers_enc, expires_at, refresh_token_enc, client_id_enc, client_secret_enc, created_at, updated_at) VALUES (?, ?, ?, 'oauth2', ?, '', '', '', 0, ?, '', '', '2026-01-01', '2026-01-01')`,
		id, userID, pattern, tokenEnc, refreshTokenEnc,
	).Error; err != nil {
		t.Fatalf("insert legacy credential entry %s: %v", id, err)
	}
}

func readRefreshTokenEnc(t *testing.T, id string) string {
	t.Helper()
	var got string
	if err := database.DB().Raw(`SELECT refresh_token_enc FROM credential_entries WHERE id = ?`, id).Scan(&got).Error; err != nil {
		t.Fatalf("ler refresh_token_enc de %s: %v", id, err)
	}
	return got
}

// TestReencryptLegacyPlaintextRefreshTokens_EncryptsPlainValues cobre o
// cenário da issue #236: base legada onde migrateRefreshURLToEnc copiou
// refresh_url (plain) para refresh_token_enc. Após a re-cifragem
// one-shot, a coluna deve conter ciphertext que decifra para o valor
// original.
func TestReencryptLegacyPlaintextRefreshTokens_EncryptsPlainValues(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	const plainRefresh = "https://auth.example.com/refresh?token=super-secret-token"
	tokenEnc, err := mgr.encrypt("access-token-1")
	if err != nil {
		t.Fatalf("encrypt token de seed: %v", err)
	}
	insertLegacyCredentialEntry(t, "cred-plain", "user-1", "api.example.com", tokenEnc, plainRefresh)

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperado 1 re-cifrado, tenho %d", n)
	}

	stored := readRefreshTokenEnc(t, "cred-plain")
	if stored == plainRefresh {
		t.Fatal("refresh_token_enc ainda contém o valor em texto plano após a re-cifragem")
	}
	if !couldBeGCMCiphertext(stored) {
		t.Fatalf("refresh_token_enc não tem formato de ciphertext: %q", stored)
	}
	dec, err := mgr.decrypt(stored)
	if err != nil {
		t.Fatalf("ciphertext gravado não decifra com a DEK atual: %v", err)
	}
	if dec != plainRefresh {
		t.Fatalf("valor decifrado divergente: esperado %q, tenho %q", plainRefresh, dec)
	}
}

// TestReencryptLegacyPlaintextRefreshTokens_Idempotent garante que
// rodar a migração duas vezes não re-cifra de novo (sem double
// encryption) e mantém o valor decifrável para o original.
func TestReencryptLegacyPlaintextRefreshTokens_Idempotent(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	const plainRefresh = "https://auth.example.com/refresh?token=abc-123"
	insertLegacyCredentialEntry(t, "cred-idem", "user-1", "api.example.com", "", plainRefresh)

	if _, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background()); err != nil {
		t.Fatalf("primeira execução: %v", err)
	}
	firstPass := readRefreshTokenEnc(t, "cred-idem")

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("segunda execução: %v", err)
	}
	if n != 0 {
		t.Fatalf("segunda execução deveria ser noop, re-cifrou %d", n)
	}
	if got := readRefreshTokenEnc(t, "cred-idem"); got != firstPass {
		t.Fatalf("segunda execução alterou o ciphertext: %q != %q", got, firstPass)
	}

	dec, err := mgr.decrypt(firstPass)
	if err != nil {
		t.Fatalf("decifrar após duas execuções: %v", err)
	}
	if dec != plainRefresh {
		t.Fatalf("valor decifrado divergente: esperado %q, tenho %q", plainRefresh, dec)
	}
}

// TestReencryptLegacyPlaintextRefreshTokens_SkipsForeignCiphertext
// garante que valores com FORMATO de ciphertext mas cifrados com outra
// DEK (órfãos do AEP-0061) NÃO são re-cifrados — caso contrário
// virariam "decifráveis para lixo" e escapariam da detecção de
// credenciais ilegíveis.
func TestReencryptLegacyPlaintextRefreshTokens_SkipsForeignCiphertext(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	foreignMgr := NewManagerWithStoreAndPersistence([]byte("another-key-exactly-32-bytes!!!!"), nil, false)
	foreignCiphertext, err := foreignMgr.encrypt("refresh-de-outra-dek")
	if err != nil {
		t.Fatalf("encrypt com DEK estrangeira: %v", err)
	}

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)
	tokenEnc, err := mgr.encrypt("access-token-ok")
	if err != nil {
		t.Fatalf("encrypt token de seed: %v", err)
	}
	insertLegacyCredentialEntry(t, "cred-foreign", "user-1", "api.example.com", tokenEnc, foreignCiphertext)

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 0 {
		t.Fatalf("ciphertext estrangeiro não deveria ser tocado, re-cifrou %d", n)
	}
	if got := readRefreshTokenEnc(t, "cred-foreign"); got != foreignCiphertext {
		t.Fatalf("ciphertext estrangeiro foi alterado: %q != %q", got, foreignCiphertext)
	}
}

// TestReencryptLegacyPlaintextRefreshTokens_SkipsUnreadableEntries
// garante que entradas já marcadas como ilegíveis pelo scan de
// integridade (AEP-0061) não são tocadas: o fluxo de purge/recovery
// existente continua dono delas.
func TestReencryptLegacyPlaintextRefreshTokens_SkipsUnreadableEntries(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	foreignMgr := NewManagerWithStoreAndPersistence([]byte("another-key-exactly-32-bytes!!!!"), nil, false)
	foreignToken, err := foreignMgr.encrypt("token-de-outra-dek")
	if err != nil {
		t.Fatalf("encrypt com DEK estrangeira: %v", err)
	}

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	const plainRefresh = "https://auth.example.com/refresh?token=orfao"
	insertLegacyCredentialEntry(t, "cred-unreadable", "user-1", "api.example.com", foreignToken, plainRefresh)

	// Simula o estado pós-scan do boot: entrada listada como ilegível.
	status := mgr.IntegrityStatus()
	status.UnreadableCredentialIDs = []string{"cred-unreadable"}
	mgr.integrity.set(status)

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 0 {
		t.Fatalf("entrada ilegível não deveria ser tocada, re-cifrou %d", n)
	}
	if got := readRefreshTokenEnc(t, "cred-unreadable"); got != plainRefresh {
		t.Fatalf("entrada ilegível foi alterada: %q", got)
	}
}

// TestLoadInstanceSecrets_ReencryptsLegacyRefreshTokens valida o ponto
// de integração real: o boot (LoadInstanceSecrets com persistência)
// re-cifra os refresh tokens legados antes de qualquer login, e o
// valor segue utilizável pelo caminho normal de leitura.
func TestLoadInstanceSecrets_ReencryptsLegacyRefreshTokens(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	seedMgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	const plainRefresh = "https://auth.example.com/refresh?token=boot-flow"
	tokenEnc, err := seedMgr.encrypt("access-token-boot")
	if err != nil {
		t.Fatalf("encrypt token de seed: %v", err)
	}
	insertLegacyCredentialEntry(t, "cred-boot", "user-1", "api.example.com", tokenEnc, plainRefresh)

	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	stored := readRefreshTokenEnc(t, "cred-boot")
	if stored == plainRefresh {
		t.Fatal("LoadInstanceSecrets não re-cifrou o refresh token legado")
	}
	dec, err := mgr.decrypt(stored)
	if err != nil {
		t.Fatalf("ciphertext gravado não decifra: %v", err)
	}
	if dec != plainRefresh {
		t.Fatalf("valor decifrado divergente: esperado %q, tenho %q", plainRefresh, dec)
	}

	// O caminho normal de leitura pós-login continua devolvendo o valor.
	if err := mgr.LoadUserCredentials(context.Background(), "user-1"); err != nil {
		t.Fatalf("LoadUserCredentials: %v", err)
	}
	userCtx := database.WithUserID(context.Background(), "user-1")
	auth, err := mgr.GetByPatternWithContext(userCtx, "api.example.com")
	if err != nil {
		t.Fatalf("GetByPatternWithContext: %v", err)
	}
	if auth == nil || auth.RefreshURL != plainRefresh {
		t.Fatalf("refresh token não chegou decifrado ao caminho de leitura: %+v", auth)
	}
}

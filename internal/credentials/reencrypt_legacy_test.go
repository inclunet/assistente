package credentials

import (
	"context"
	"encoding/base64"
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

func TestCouldBeGCMCiphertextAcceptsRawBase64AndWhitespace(t *testing.T) {
	payload := make([]byte, gcmMinCiphertextLen)
	raw := base64.RawStdEncoding.EncodeToString(payload)

	if !couldBeGCMCiphertext(raw) {
		t.Fatalf("raw std base64 sem padding deveria ser aceito como formato plausível: %q", raw)
	}
	if !couldBeGCMCiphertext(" \t" + raw + "\n") {
		t.Fatalf("base64 plausível com whitespace ao redor deveria ser aceito")
	}
}

func TestListCredentialsWithRefreshTokensIgnoringScopeFiltersEmptyRefresh(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	insertLegacyCredentialEntry(t, "cred-empty-refresh", "user-1", "api-empty.example.com", "", "")
	insertLegacyCredentialEntry(t, "cred-with-refresh", "user-2", "api-refresh.example.com", "", "legacy-refresh")
	insertLegacyCredentialEntry(t, "cred-invalid-headers", "user-3", "api-invalid.example.com", "", "legacy-refresh-with-invalid-headers")
	if err := database.DB().Exec(`UPDATE credential_entries SET headers_enc = ? WHERE id = ?`, "{invalid-json", "cred-invalid-headers").Error; err != nil {
		t.Fatalf("marcar headers_enc inválido: %v", err)
	}

	got, err := NewDBStore().ListCredentialsWithRefreshTokensIgnoringScope(context.Background())
	if err != nil {
		t.Fatalf("ListCredentialsWithRefreshTokensIgnoringScope: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperado 2 candidatos com refresh preenchido, tenho %d: %+v", len(got), got)
	}
	byID := map[string]StoredCredential{}
	for _, cred := range got {
		byID[cred.ID] = cred
	}
	if cred := byID["cred-with-refresh"]; cred.Auth == nil || cred.Auth.RefreshURL != "legacy-refresh" {
		t.Fatalf("candidato com refresh esperado não encontrado: %+v", got)
	}
	if cred := byID["cred-invalid-headers"]; cred.Auth == nil || cred.Auth.RefreshURL != "legacy-refresh-with-invalid-headers" || len(cred.Auth.Headers) != 0 {
		t.Fatalf("candidato com headers inválidos deveria manter refresh e headers vazios: %+v", cred)
	}
}

func TestUpdateRefreshTokenEncByIDReturnsErrorWhenMissing(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	if err := NewDBStore().UpdateRefreshTokenEncByID(context.Background(), "missing-cred", "encrypted"); err == nil {
		t.Fatal("UpdateRefreshTokenEncByID deveria falhar quando nenhuma linha é atualizada")
	}
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

func TestReencryptLegacyPlaintextRefreshTokens_NormalizesCurrentCiphertextWithWhitespace(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	refreshEnc, err := mgr.encrypt("refresh-token")
	if err != nil {
		t.Fatalf("encrypt refresh de seed: %v", err)
	}
	refreshWithWhitespace := " \t" + refreshEnc + "\n"
	insertLegacyCredentialEntry(t, "cred-cipher-whitespace", "user-1", "api.example.com", "", refreshWithWhitespace)

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 0 {
		t.Fatalf("ciphertext da DEK atual com whitespace não deveria ser re-cifrado, re-cifrou %d", n)
	}
	stored := readRefreshTokenEnc(t, "cred-cipher-whitespace")
	if stored != refreshEnc {
		t.Fatalf("ciphertext com whitespace deveria ser normalizado: %q != %q", stored, refreshEnc)
	}
	dec, err := mgr.decrypt(stored)
	if err != nil {
		t.Fatalf("ciphertext normalizado não decifra: %v", err)
	}
	if dec != "refresh-token" {
		t.Fatalf("valor decifrado divergente: %q", dec)
	}
}

func TestReencryptLegacyPlaintextRefreshTokens_ClearsWhitespaceOnlyRefresh(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)

	insertLegacyCredentialEntry(t, "cred-refresh-empty", "user-1", "api.example.com", "", " \t\n")

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 0 {
		t.Fatalf("refresh whitespace-only não deveria ser re-cifrado, re-cifrou %d", n)
	}
	if got := readRefreshTokenEnc(t, "cred-refresh-empty"); got != "" {
		t.Fatalf("refresh whitespace-only deveria ser limpo, tenho %q", got)
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
	foreignToken, err := foreignMgr.encrypt("access-token-de-outra-dek")
	if err != nil {
		t.Fatalf("encrypt token com DEK estrangeira: %v", err)
	}
	foreignCiphertext, err := foreignMgr.encrypt("refresh-de-outra-dek")
	if err != nil {
		t.Fatalf("encrypt com DEK estrangeira: %v", err)
	}

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)
	insertLegacyCredentialEntry(t, "cred-foreign", "user-1", "api.example.com", foreignToken, foreignCiphertext)

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

// TestReencryptLegacyPlaintextRefreshTokens_ReencryptsUnreadablePlainRefresh
// garante que UnreadableCredentialIDs não bloqueia a re-cifragem quando
// o refresh token é claramente texto plano legado.
func TestReencryptLegacyPlaintextRefreshTokens_ReencryptsUnreadablePlainRefresh(t *testing.T) {
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
	if n != 1 {
		t.Fatalf("refresh token plain em entrada ilegível deveria ser re-cifrado, tenho %d", n)
	}
	stored := readRefreshTokenEnc(t, "cred-unreadable")
	if stored == plainRefresh {
		t.Fatal("refresh_token_enc ainda contém texto plano após re-cifragem")
	}
	dec, err := mgr.decrypt(stored)
	if err != nil {
		t.Fatalf("ciphertext gravado não decifra: %v", err)
	}
	if dec != plainRefresh {
		t.Fatalf("valor decifrado divergente: esperado %q, tenho %q", plainRefresh, dec)
	}
}

func TestReencryptLegacyPlaintextRefreshTokens_ReencryptsBase64PlainWhenCredentialDecrypts(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	key := []byte("test-key-exactly-32-bytes-long!!")
	mgr := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)
	tokenEnc, err := mgr.encrypt("access-token-ok")
	if err != nil {
		t.Fatalf("encrypt token de seed: %v", err)
	}

	const base64PlainRefresh = "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	if !couldBeGCMCiphertext(base64PlainRefresh) {
		t.Fatalf("seed precisa parecer ciphertext para cobrir falso positivo: %q", base64PlainRefresh)
	}
	insertLegacyCredentialEntry(t, "cred-base64-plain", "user-1", "api.example.com", tokenEnc, base64PlainRefresh)

	n, err := mgr.reencryptLegacyPlaintextRefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("reencryptLegacyPlaintextRefreshTokens: %v", err)
	}
	if n != 1 {
		t.Fatalf("plaintext base64 em credencial decriptável deveria ser re-cifrado, tenho %d", n)
	}
	stored := readRefreshTokenEnc(t, "cred-base64-plain")
	if stored == base64PlainRefresh {
		t.Fatal("refresh_token_enc base64-like permaneceu em texto plano")
	}
	dec, err := mgr.decrypt(stored)
	if err != nil {
		t.Fatalf("ciphertext gravado não decifra: %v", err)
	}
	if dec != base64PlainRefresh {
		t.Fatalf("valor decifrado divergente: esperado %q, tenho %q", base64PlainRefresh, dec)
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

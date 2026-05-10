package database

import (
	"strings"
	"testing"
	"time"
)

// TestDedupCredentialEntriesBeforeMigrate valida o fix B31 do review do
// AEP-0052: bases legadas com (user_id, pattern) duplicado precisam ser
// deduplicadas antes de o AutoMigrate criar o índice unique. A entry com
// maior `updated_at` vence; ties são quebrados por `id` lexicográfico desc
// (UUIDv7 → entry mais recente).
//
// Setup do teste: drop temporário do índice unique para simular o estado
// pré-AEP-0052 onde a tabela já existia mas sem unique em (user_id, pattern);
// chamar `dedupCredentialEntriesBeforeMigrate`; recriar o índice (simulando
// AutoMigrate) e verificar que aplica sem erro.
func TestDedupCredentialEntriesBeforeMigrate(t *testing.T) {
	setupMultiUserTestDB(t)

	// Simula DB pré-AEP-0052: tabela existe, índice unique ainda não foi
	// criado (vai ser criado depois do dedup).
	if err := db.Exec(`DROP INDEX IF EXISTS ux_credential_entries_user_pattern`).Error; err != nil {
		t.Fatalf("drop unique index: %v", err)
	}

	now := time.Now()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)

	users := []*User{
		{Username: "ana", PasswordHash: "h", Role: UserRoleAdmin, IsActive: true},
		{Username: "leo", PasswordHash: "h", Role: UserRoleUser, IsActive: true},
	}
	for _, u := range users {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}

	mustCreate := func(userID, pattern, authType string, updatedAt time.Time) string {
		entry := &CredentialEntry{UserID: userID, Pattern: pattern, AuthType: authType}
		if err := db.Create(entry).Error; err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if err := db.Model(&CredentialEntry{}).Where("id = ?", entry.ID).Update("updated_at", updatedAt).Error; err != nil {
			t.Fatalf("touch updated_at: %v", err)
		}
		return entry.ID
	}

	older1 := mustCreate(users[0].ID, "api.openai.com", "bearer-old", older)
	newer1 := mustCreate(users[0].ID, "api.openai.com", "bearer-new", newer)
	keptDifferentUser := mustCreate(users[1].ID, "api.openai.com", "bearer-other", now)

	dedupCredentialEntriesBeforeMigrate()

	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_credential_entries_user_pattern ON credential_entries (user_id, pattern)`).Error; err != nil {
		t.Fatalf("após dedup, recriação do índice unique deveria funcionar: %v", err)
	}

	var anaCount int64
	if err := db.Model(&CredentialEntry{}).Where("user_id = ? AND pattern = ?", users[0].ID, "api.openai.com").Count(&anaCount).Error; err != nil {
		t.Fatalf("count ana: %v", err)
	}
	if anaCount != 1 {
		t.Fatalf("esperava 1 entry para (ana, api.openai.com) após dedup, tenho %d", anaCount)
	}
	var winner CredentialEntry
	if err := db.Where("user_id = ? AND pattern = ?", users[0].ID, "api.openai.com").First(&winner).Error; err != nil {
		t.Fatalf("load winner: %v", err)
	}
	if winner.ID != newer1 {
		t.Fatalf("esperava ID=%s (mais recente), tenho %s; older=%s", newer1, winner.ID, older1)
	}

	var sameUserOther CredentialEntry
	if err := db.First(&sameUserOther, "id = ?", keptDifferentUser).Error; err != nil {
		t.Fatalf("entry do outro user sumiu (leak entre users): %v", err)
	}

	if err := db.Create(&CredentialEntry{UserID: users[0].ID, Pattern: "api.openai.com", AuthType: "bearer-dup"}).Error; err == nil {
		t.Fatal("após dedup+índice, INSERT duplicado em (user_id, pattern) deveria falhar")
	}
}

// TestEnsureUsernameCaseInsensitive_NormalizesLegacyMixedCase valida B34: o
// boot deve baixar usernames mixed-case para lowercase quando não há colisão.
func TestEnsureUsernameCaseInsensitive_NormalizesLegacyMixedCase(t *testing.T) {
	setupMultiUserTestDB(t)

	if err := db.Create(&User{Username: "Alice", PasswordHash: "h", Role: UserRoleAdmin, IsActive: true}).Error; err != nil {
		t.Fatalf("create Alice: %v", err)
	}
	if err := db.Create(&User{Username: "BoB", PasswordHash: "h", Role: UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatalf("create BoB: %v", err)
	}

	if err := ensureUsernameCaseInsensitive(); err != nil {
		t.Fatalf("ensureUsernameCaseInsensitive: %v", err)
	}

	for _, want := range []string{"alice", "bob"} {
		var u User
		if err := db.Where("username = ?", want).First(&u).Error; err != nil {
			t.Fatalf("user %q não encontrado após normalização: %v", want, err)
		}
		if u.Username != want {
			t.Fatalf("username não normalizado: esperava %q, tenho %q", want, u.Username)
		}
		if !u.IsActive {
			t.Fatalf("user %q foi desativado sem motivo", want)
		}
	}
}

// TestEnsureUsernameCaseInsensitive_HandlesCollision valida B34 em cenário
// onde já existe um par `Alice`/`alice`. O perdedor (menor id) é renomeado e
// desativado, e o vencedor permanece — preserva ambos para auditoria sem
// destruir dados.
func TestEnsureUsernameCaseInsensitive_HandlesCollision(t *testing.T) {
	setupMultiUserTestDB(t)

	loser := &User{Username: "Alice", PasswordHash: "h", Role: UserRoleUser, IsActive: true}
	if err := db.Create(loser).Error; err != nil {
		t.Fatalf("create Alice: %v", err)
	}
	winner := &User{Username: "alice", PasswordHash: "h", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(winner).Error; err != nil {
		t.Fatalf("create alice: %v", err)
	}

	expectedLoserID := loser.ID
	expectedWinnerID := winner.ID
	if expectedWinnerID < expectedLoserID {
		expectedLoserID, expectedWinnerID = expectedWinnerID, expectedLoserID
	}

	if err := ensureUsernameCaseInsensitive(); err != nil {
		t.Fatalf("ensureUsernameCaseInsensitive: %v", err)
	}

	var winnerRow User
	if err := db.First(&winnerRow, "id = ?", expectedWinnerID).Error; err != nil {
		t.Fatalf("load winner: %v", err)
	}
	if winnerRow.Username != "alice" {
		t.Fatalf("winner.username esperado 'alice', tenho %q", winnerRow.Username)
	}
	if !winnerRow.IsActive {
		t.Fatal("winner não deveria ter sido desativado")
	}

	var loserRow User
	if err := db.First(&loserRow, "id = ?", expectedLoserID).Error; err != nil {
		t.Fatalf("load loser: %v", err)
	}
	if loserRow.IsActive {
		t.Fatal("loser deveria ter sido desativado")
	}
	if !strings.HasPrefix(loserRow.Username, "alice.legacy.") {
		t.Fatalf("loser.username esperado prefixo 'alice.legacy.', tenho %q", loserRow.Username)
	}
}

// TestEnsureUsernameCaseInsensitive_BlocksFutureCaseVariants valida B34:
// após boot, INSERTs com case diferente do canônico devem ser bloqueados pelo
// índice unique em LOWER(username).
func TestEnsureUsernameCaseInsensitive_BlocksFutureCaseVariants(t *testing.T) {
	setupMultiUserTestDB(t)

	if err := db.Create(&User{Username: "ana", PasswordHash: "h", Role: UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatalf("create ana: %v", err)
	}
	if err := ensureUsernameCaseInsensitive(); err != nil {
		t.Fatalf("ensureUsernameCaseInsensitive: %v", err)
	}

	if err := db.Create(&User{Username: "Ana", PasswordHash: "h", Role: UserRoleUser, IsActive: true}).Error; err == nil {
		t.Fatal("INSERT direto de case-variant deveria falhar com unique LOWER(username)")
	}
}

// TestMigrateRefreshURLToEnc_CopiesLegacyData valida B30: migração inline da
// coluna legacy `refresh_url` para `refresh_token_enc`. Como os models atuais
// não declaram `refresh_url`, simulamos via ALTER TABLE.
func TestMigrateRefreshURLToEnc_CopiesLegacyData(t *testing.T) {
	setupMultiUserTestDB(t)

	if err := db.Exec(`ALTER TABLE credential_entries ADD COLUMN refresh_url TEXT`).Error; err != nil {
		t.Fatalf("add legacy column: %v", err)
	}

	user := &User{Username: "ana", PasswordHash: "h", Role: UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	type rawEntry struct {
		ID              string
		UserID          string
		Pattern         string
		RefreshURL      string
		RefreshTokenEnc string
	}
	mustInsert := func(pattern, refreshURL, refreshTokenEnc string) string {
		entry := &CredentialEntry{UserID: user.ID, Pattern: pattern, AuthType: "bearer"}
		if err := db.Create(entry).Error; err != nil {
			t.Fatalf("create %s: %v", pattern, err)
		}
		if err := db.Exec(`UPDATE credential_entries SET refresh_url = ?, refresh_token_enc = ? WHERE id = ?`, refreshURL, refreshTokenEnc, entry.ID).Error; err != nil {
			t.Fatalf("seed legacy data %s: %v", pattern, err)
		}
		return entry.ID
	}

	withLegacyOnly := mustInsert("svc-a", "https://idp.example/refresh?token=ABC", "")
	preferEnc := mustInsert("svc-b", "https://idp.example/refresh?token=OLD", "ENCRYPTED-NEW")
	noTokens := mustInsert("svc-c", "", "")

	if err := migrateRefreshURLToEnc(); err != nil {
		t.Fatalf("migrateRefreshURLToEnc: %v", err)
	}

	var legacyColCount int
	if err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info('credential_entries') WHERE name = 'refresh_url'`).Scan(&legacyColCount).Error; err != nil {
		t.Fatalf("inspect column: %v", err)
	}
	if legacyColCount != 0 {
		t.Fatal("coluna legacy refresh_url deveria ter sido dropada")
	}

	checkEnc := func(id, want string) {
		t.Helper()
		var got string
		if err := db.Raw(`SELECT refresh_token_enc FROM credential_entries WHERE id = ?`, id).Scan(&got).Error; err != nil {
			t.Fatalf("scan %s: %v", id, err)
		}
		if got != want {
			t.Fatalf("refresh_token_enc id=%s: esperado %q, tenho %q", id, want, got)
		}
	}
	checkEnc(withLegacyOnly, "https://idp.example/refresh?token=ABC")
	checkEnc(preferEnc, "ENCRYPTED-NEW")
	checkEnc(noTokens, "")

	if err := migrateRefreshURLToEnc(); err != nil {
		t.Fatalf("migrateRefreshURLToEnc não é idempotente: %v", err)
	}
}

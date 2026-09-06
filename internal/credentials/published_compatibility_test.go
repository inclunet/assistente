package credentials

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type publishedCredentialFixture struct {
	ID                          string `json:"id"`
	UserID                      string `json:"userId"`
	Pattern                     string `json:"pattern"`
	AccessToken                 string `json:"accessToken"`
	RefreshTokenLegacyPlaintext string `json:"refreshTokenLegacyPlaintext"`
}

func TestPublishedCredentialRepairRemainsEquivalentFrom019Through050(t *testing.T) {
	path := filepath.Join("testdata", "published", "0.1.9-0.5.0-plaintext-refresh.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler fixture: %v", err)
	}
	original := append([]byte(nil), raw...)
	var fixture publishedCredentialFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parsear fixture: %v", err)
	}

	for _, release := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			setupScopedCredentialStoreTestDB(t)
			key := []byte("test-key-exactly-32-bytes-long!!")
			manager := NewManagerWithStoreAndPersistence(key, NewDBStore(), true)
			accessTokenEnc, err := manager.encrypt(fixture.AccessToken)
			if err != nil {
				t.Fatalf("cifrar token sintético: %v", err)
			}
			insertLegacyCredentialEntry(t, fixture.ID, fixture.UserID, fixture.Pattern,
				accessTokenEnc, fixture.RefreshTokenLegacyPlaintext)

			changed, err := manager.reencryptLegacyPlaintextRefreshTokens(t.Context())
			if err != nil {
				t.Fatalf("reparar credencial publicada em %s: %v", release, err)
			}
			if changed != 1 {
				t.Fatalf("release %s: esperado um reparo, obtido %d", release, changed)
			}
			stored := readRefreshTokenEnc(t, fixture.ID)
			decoded, err := manager.decrypt(stored)
			if err != nil || decoded != fixture.RefreshTokenLegacyPlaintext {
				t.Fatalf("release %s: refresh token não foi preservado: %q (%v)", release, decoded, err)
			}
			changed, err = manager.reencryptLegacyPlaintextRefreshTokens(t.Context())
			if err != nil || changed != 0 {
				t.Fatalf("release %s: segunda execução não foi idempotente: changed=%d err=%v", release, changed, err)
			}
		})
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("relê fixture: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("reparo de credenciais alterou a fixture fonte")
	}
}

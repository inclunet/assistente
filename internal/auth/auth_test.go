package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"github.com/zalando/go-keyring"
	"gorm.io/gorm"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password stored in clear text")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatalf("verify wrong password: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestIdentityServiceCreatesAndAuthenticatesUser(t *testing.T) {
	db := setupAuthTestDB(t)
	service := NewIdentityService(db)

	user, err := service.CreateLocalUser(context.Background(), CreateUserParams{
		Username: " Admin ",
		Password: "secret-password",
		Admin:    true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Username != "admin" {
		t.Fatalf("username was not normalized: %q", user.Username)
	}
	if user.Role != database.UserRoleAdmin {
		t.Fatalf("expected admin role, got %q", user.Role)
	}

	authenticated, err := service.AuthenticateLocal(context.Background(), "ADMIN", "secret-password")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authenticated.ID != user.ID {
		t.Fatalf("expected user %s, got %s", user.ID, authenticated.ID)
	}
	if authenticated.LastLoginAt == nil {
		t.Fatal("expected last login timestamp")
	}

	if _, err := service.AuthenticateLocal(context.Background(), "admin", "wrong-password"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expected invalid credential, got %v", err)
	}
}

func TestSessionServiceRefreshRotatesAndRejectsReuse(t *testing.T) {
	db := setupAuthTestDB(t)
	identity := NewIdentityService(db)
	user, err := identity.CreateLocalUser(context.Background(), CreateUserParams{
		Username: "admin",
		Password: "secret-password",
		Admin:    true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sessions, err := NewSessionService(db, SessionConfig{
		Issuer:     "test-issuer",
		Audience:   "test-client",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}

	issued, err := sessions.IssueSession(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if !strings.HasPrefix(issued.RefreshToken, "v1."+issued.SessionID+".") {
		t.Fatalf("unexpected refresh token format: %s", issued.RefreshToken)
	}

	claims, err := sessions.VerifyAccessToken(issued.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if claims.Subject != user.ID || claims.SessionID != issued.SessionID || claims.Role != database.UserRoleAdmin {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	refreshed, err := sessions.Refresh(context.Background(), issued.RefreshToken)
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.RefreshToken == issued.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	if _, err := sessions.Refresh(context.Background(), issued.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected reused refresh token to be rejected, got %v", err)
	}

	var session database.Session
	if err := db.First(&session, "id = ?", issued.SessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.RevokedAt == nil {
		t.Fatal("expected reused refresh token to revoke session")
	}
}

func TestJWKSetExposesEd25519PublicKey(t *testing.T) {
	signer, err := NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	jwks := signer.JWKSet()
	if len(jwks.Keys) != 1 {
		t.Fatalf("expected one key, got %d", len(jwks.Keys))
	}
	key := jwks.Keys[0]
	if key.KeyType != "OKP" || key.Curve != "Ed25519" || key.Algorithm != "EdDSA" || key.X == "" {
		t.Fatalf("unexpected jwk: %+v", key)
	}
}

func TestLoadOrCreateTokenSignerPersistsInCredentialManager(t *testing.T) {
	mgr := credentials.NewManager(nil)
	first, err := LoadOrCreateTokenSigner(mgr)
	if err != nil {
		t.Fatalf("load first signer: %v", err)
	}
	second, err := LoadOrCreateTokenSigner(mgr)
	if err != nil {
		t.Fatalf("load second signer: %v", err)
	}
	firstJWKS := first.JWKSet()
	secondJWKS := second.JWKSet()
	if firstJWKS.Keys[0].KeyID != secondJWKS.Keys[0].KeyID || firstJWKS.Keys[0].X != secondJWKS.Keys[0].X {
		t.Fatalf("expected stable JWKS, got first=%+v second=%+v", firstJWKS, secondJWKS)
	}
}

func TestExternalValidatorAcceptsAllowedEdDSAToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sum := sha256.Sum256(publicKey)
	keyID := base64.RawURLEncoding.EncodeToString(sum[:8])
	now := time.Now()
	token := signExternalTestToken(t, privateKey, keyID, map[string]any{
		"iss":   "https://idp.example.com",
		"aud":   []string{"assistente"},
		"sub":   "external-user",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Minute).Unix(),
		"scope": "assistente:read",
		"roles": []string{"user"},
	})

	validator := ExternalValidator{
		Issuer:            "https://idp.example.com",
		Audience:          "assistente",
		AllowedAlgorithms: map[string]bool{"EdDSA": true},
		Now:               func() time.Time { return now },
	}
	claims, err := validator.Validate(token, JWKSet{Keys: []JWK{{
		KeyType:   "OKP",
		KeyID:     keyID,
		Algorithm: "EdDSA",
		Use:       "sig",
		Curve:     "Ed25519",
		X:         base64.RawURLEncoding.EncodeToString(publicKey),
	}}})
	if err != nil {
		t.Fatalf("validate external token: %v", err)
	}
	if claims.Subject != "external-user" || claims.Scope != "assistente:read" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestExternalValidatorRejectsDisallowedAlgorithm(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	token := signExternalTestToken(t, privateKey, "kid", map[string]any{
		"iss": "issuer",
		"aud": "audience",
		"sub": "subject",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	_, err = (ExternalValidator{
		Issuer:            "issuer",
		Audience:          "audience",
		AllowedAlgorithms: map[string]bool{"RS256": true},
	}).Validate(token, JWKSet{Keys: []JWK{{KeyID: "kid", Algorithm: "EdDSA", X: base64.RawURLEncoding.EncodeToString(publicKey)}}})
	if err == nil {
		t.Fatal("expected disallowed algorithm to fail")
	}
}

func TestVaultSetupAdoptsExistingKeyringDEK(t *testing.T) {
	store := newMemoryCredentialStore()
	existingDEK := []byte("01234567890123456789012345678901")
	var configuredDEK []byte

	vault := NewVaultService(store, func(dek []byte) {
		configuredDEK = append([]byte(nil), dek...)
	})
	vault.loadKeyring = func() ([]byte, error) {
		return existingDEK, nil
	}
	vault.saveKeyring = func([]byte) error {
		t.Fatal("setup with existing keyring DEK should not save a new DEK")
		return nil
	}

	recoveryKey, err := vault.Setup(context.Background(), "master-password")
	if err != nil {
		t.Fatalf("setup vault: %v", err)
	}
	if recoveryKey == "" {
		t.Fatal("expected recovery key")
	}
	if string(configuredDEK) != string(existingDEK) {
		t.Fatalf("configured DEK was replaced")
	}
	unwrapped, err := credentials.UnlockDEKWithSecret(store, credentials.KeyWrapKindMaster, "master-password")
	if err != nil {
		t.Fatalf("unlock adopted DEK: %v", err)
	}
	if string(unwrapped) != string(existingDEK) {
		t.Fatalf("unwrapped DEK differs from existing keyring DEK")
	}
}

func TestVaultSetupDoesNotReplaceUnreadableKeyringDEK(t *testing.T) {
	store := newMemoryCredentialStore()
	keyringErr := errors.New("keyring indisponível")

	vault := NewVaultService(store, nil)
	vault.loadKeyring = func() ([]byte, error) {
		return nil, keyringErr
	}
	vault.saveKeyring = func([]byte) error {
		t.Fatal("setup must not save a new DEK when keyring read fails")
		return nil
	}

	if _, err := vault.Setup(context.Background(), "master-password"); !errors.Is(err, keyringErr) {
		t.Fatalf("expected keyring error, got %v", err)
	}
	if has, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster); err != nil || has {
		t.Fatalf("master wrap should not be created after keyring error: has=%v err=%v", has, err)
	}
}

func TestVaultSetupDoesNotReplaceMissingKeyringDEKWhenCredentialsExist(t *testing.T) {
	store := newMemoryCredentialStore()
	store.credentials = []credentials.StoredCredential{{Pattern: "api.example.com", Auth: &credentials.AuthConfig{Type: "bearer", Token: "ciphertext"}}}

	vault := NewVaultService(store, nil)
	vault.loadKeyring = func() ([]byte, error) {
		return nil, keyring.ErrNotFound
	}
	vault.saveKeyring = func([]byte) error {
		t.Fatal("setup must not save a new DEK when credentials already exist")
		return nil
	}

	if _, err := vault.Setup(context.Background(), "master-password"); !errors.Is(err, credentials.ErrExistingCredentialsWithoutDEK) {
		t.Fatalf("expected ErrExistingCredentialsWithoutDEK, got %v", err)
	}
	if has, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster); err != nil || has {
		t.Fatalf("master wrap should not be created when credentials already exist: has=%v err=%v", has, err)
	}
}

type memoryCredentialStore struct {
	wraps       map[string]credentials.KeyWrap
	credentials []credentials.StoredCredential
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{wraps: map[string]credentials.KeyWrap{}}
}

func (s *memoryCredentialStore) SaveCredential(context.Context, credentials.StoredCredential) error {
	return nil
}

func (s *memoryCredentialStore) ListCredentials(context.Context) ([]credentials.StoredCredential, error) {
	return s.credentials, nil
}

func (s *memoryCredentialStore) DeleteCredential(context.Context, string) error {
	return nil
}

func (s *memoryCredentialStore) SaveKeyWrap(_ context.Context, wrap credentials.KeyWrap) error {
	s.wraps[wrap.Kind] = wrap
	return nil
}

func (s *memoryCredentialStore) GetKeyWrap(_ context.Context, kind string) (*credentials.KeyWrap, error) {
	wrap, ok := s.wraps[kind]
	if !ok {
		return nil, credentials.ErrKeyWrapNotFound
	}
	return &wrap, nil
}

func (s *memoryCredentialStore) HasKeyWrap(_ context.Context, kind string) (bool, error) {
	_, ok := s.wraps[kind]
	return ok, nil
}

func signExternalTestToken(t *testing.T, privateKey ed25519.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "EdDSA", "kid": keyID, "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	input := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := ed25519.Sign(privateKey, []byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// TestParseArgonParamsRejectsZeroMemoryOrTime cobre B1 do review da
// Fatia 1: hash com m=0 ou t=0 antes passava silenciosamente porque
// parseUint32 retornava (0, nil). Agora deve falhar com ErrInvalidPasswordHash.
func TestParseArgonParamsRejectsZeroMemoryOrTime(t *testing.T) {
	cases := []struct {
		name    string
		hashRaw string
	}{
		{"m=0", "$assistente-argon2id$v=1$m=0,t=3,p=2$YWJjZGVmZ2hpamtsbW5vcA$" + base64.RawStdEncoding.EncodeToString(make([]byte, 32))},
		{"t=0", "$assistente-argon2id$v=1$m=65536,t=0,p=2$YWJjZGVmZ2hpamtsbW5vcA$" + base64.RawStdEncoding.EncodeToString(make([]byte, 32))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyPassword("any", tc.hashRaw)
			if !errors.Is(err, ErrInvalidPasswordHash) {
				t.Fatalf("want ErrInvalidPasswordHash, got %v", err)
			}
		})
	}
}

// TestHashPasswordRejectsTooShort cobre M4 do review da Fatia 1.
func TestHashPasswordRejectsTooShort(t *testing.T) {
	if _, err := HashPassword("a"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
	if _, err := HashPassword("1234567"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort para 7 chars, got %v", err)
	}
	if _, err := HashPassword("12345678"); err != nil {
		t.Fatalf("8 chars deveria ser aceito, got %v", err)
	}
}

// TestAuthenticateLocalEqualizesTimingForUnknownUser cobre M2 do review
// da Fatia 1 — anti-enumeration. Não medimos tempo absoluto (frágil em
// CI), só checamos que ambos os caminhos produzem ErrInvalidCredential
// e que o caminho "user not found" exercita o dummy hash sem panic.
func TestAuthenticateLocalEqualizesTimingForUnknownUser(t *testing.T) {
	db := setupAuthTestDB(t)
	service := NewIdentityService(db)

	if _, err := service.CreateLocalUser(context.Background(), CreateUserParams{
		Username: "alice",
		Password: "alice-password",
		Admin:    true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := service.AuthenticateLocal(context.Background(), "ghost", "anything-here"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("ghost user: want ErrInvalidCredential, got %v", err)
	}
	if _, err := service.AuthenticateLocal(context.Background(), "alice", "wrong-password"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong password: want ErrInvalidCredential, got %v", err)
	}
}

// TestSessionServiceRefreshAcceptsLegacySHA256Hash cobre B2 do review
// da Fatia 1: sessões emitidas em instalações pré-pepper continuam
// válidas e migram transparente no próximo refresh para HMAC.
func TestSessionServiceRefreshAcceptsLegacySHA256Hash(t *testing.T) {
	db := setupAuthTestDB(t)
	identity := NewIdentityService(db)
	user, err := identity.CreateLocalUser(context.Background(), CreateUserParams{
		Username: "legacy-user",
		Password: "legacy-password",
		Admin:    true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Cria sessão diretamente no DB com hash legacy SHA-256 puro
	// (formato pré-pepper).
	legacySecret := "legacy-secret-32-bytes-long-x"
	legacyHashSum := sha256.Sum256([]byte(legacySecret))
	legacyHash := base64.RawURLEncoding.EncodeToString(legacyHashSum[:])
	session := &database.Session{
		UserID:           user.ID,
		RefreshTokenHash: legacyHash,
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	legacyToken := "v1." + session.ID + "." + legacySecret

	pepper := []byte("pepper-de-pelo-menos-32-bytes-de-tamanho-fixo")
	sessions, err := NewSessionService(db, SessionConfig{
		Issuer:             "test",
		Audience:           "test",
		AccessTTL:          time.Minute,
		RefreshTTL:         time.Hour,
		RefreshTokenPepper: pepper,
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}

	pair, err := sessions.Refresh(context.Background(), legacyToken)
	if err != nil {
		t.Fatalf("refresh legacy: %v", err)
	}
	if pair.RefreshToken == legacyToken {
		t.Fatal("expected new refresh token after migration")
	}

	// Após o refresh, o hash em DB deve estar no formato HMAC novo.
	var migrated database.Session
	if err := db.First(&migrated, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if !strings.HasPrefix(migrated.RefreshTokenHash, "h1:") {
		t.Fatalf("expected HMAC-prefixed hash after refresh, got %q", migrated.RefreshTokenHash)
	}
}

// TestExternalValidatorRejectsRSAKeyTooSmall cobre Mi2 do review da
// Fatia 1: chaves RSA com N < 2048 bits são rejeitadas mesmo se a
// assinatura for matematicamente válida.
func TestExternalValidatorRejectsRSAKeyTooSmall(t *testing.T) {
	smallN := make([]byte, 128) // 1024 bits
	for i := range smallN {
		smallN[i] = byte(i + 1)
	}
	jwk := JWK{
		KeyType:   "RSA",
		KeyID:     "rsa-1024",
		Algorithm: "RS256",
		N:         base64.RawURLEncoding.EncodeToString(smallN),
		E:         base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
	}
	parts := []string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"rsa-1024"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{}`)),
		base64.RawURLEncoding.EncodeToString([]byte("fake-signature")),
	}
	if err := verifyExternalSignature(parts, jwk); err == nil || !strings.Contains(err.Error(), "RSA externa fraca") {
		t.Fatalf("expected weak RSA error, got %v", err)
	}
}

// TestExternalValidatorRejectsEmptySubject cobre Mi3 do review da
// Fatia 1: token sem `sub` não identifica o portador e deve ser rejeitado.
func TestExternalValidatorRejectsEmptySubject(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now()
	token := signExternalTestToken(t, privateKey, "kid", map[string]any{
		"iss": "issuer",
		"aud": "audience",
		// "sub" deliberadamente vazio
		"iat": now.Unix(),
		"exp": now.Add(time.Minute).Unix(),
	})
	validator := ExternalValidator{
		Issuer:            "issuer",
		Audience:          "audience",
		AllowedAlgorithms: map[string]bool{"EdDSA": true},
		Now:               func() time.Time { return now },
	}
	_, err = validator.Validate(token, JWKSet{Keys: []JWK{{
		KeyType:   "OKP",
		KeyID:     "kid",
		Algorithm: "EdDSA",
		Curve:     "Ed25519",
		X:         base64.RawURLEncoding.EncodeToString(publicKey),
	}}})
	if err == nil || !strings.Contains(err.Error(), "subject externo obrigatório") {
		t.Fatalf("expected empty-subject rejection, got %v", err)
	}
}

// TestIssueSessionTruncatesClientLabel cobre Mi4 do review da Fatia 1.
func TestIssueSessionTruncatesClientLabel(t *testing.T) {
	db := setupAuthTestDB(t)
	identity := NewIdentityService(db)
	user, err := identity.CreateLocalUser(context.Background(), CreateUserParams{
		Username: "label-user",
		Password: "label-password",
		Admin:    true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessions, err := NewSessionService(db, SessionConfig{
		Issuer:     "test",
		Audience:   "test",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}

	huge := strings.Repeat("x", 1024)
	pair, err := sessions.IssueSession(context.Background(), user, huge)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	var session database.Session
	if err := db.First(&session, "id = ?", pair.SessionID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if len(session.ClientLabel) != 256 {
		t.Fatalf("expected ClientLabel truncated to 256, got %d", len(session.ClientLabel))
	}
}

// TestVaultStatusUsesRuntimeFlagWhenKeyringFails cobre M7 do review da
// Fatia 1: depois de Setup/Unlock o status reporta "unlocked" mesmo se
// o keyring estiver indisponível posteriormente, porque a DEK ainda
// está em runtime.
func TestVaultStatusUsesRuntimeFlagWhenKeyringFails(t *testing.T) {
	store := newMemoryCredentialStore()
	vault := NewVaultService(store, nil)
	// Setup precisa de NotFound (não há DEK pré-existente) para criar.
	vault.loadKeyring = func() ([]byte, error) {
		return nil, keyring.ErrNotFound
	}
	vault.saveKeyring = func([]byte) error { return nil }

	if _, err := vault.Setup(context.Background(), "master-password"); err != nil {
		t.Fatalf("setup vault: %v", err)
	}

	// Após Setup, runtime está unlocked. Simulamos keyring quebrado:
	// ainda assim Status deve retornar Unlocked=true porque a flag de
	// runtime sobrevive.
	vault.loadKeyring = func() ([]byte, error) {
		return nil, errors.New("keyring offline")
	}

	status, err := vault.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Unlocked {
		t.Fatal("expected runtime-flag to keep vault marked as unlocked")
	}
}

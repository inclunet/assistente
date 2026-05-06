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

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
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
		Password: "secret",
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

	authenticated, err := service.AuthenticateLocal(context.Background(), "ADMIN", "secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authenticated.ID != user.ID {
		t.Fatalf("expected user %s, got %s", user.ID, authenticated.ID)
	}
	if authenticated.LastLoginAt == nil {
		t.Fatal("expected last login timestamp")
	}

	if _, err := service.AuthenticateLocal(context.Background(), "admin", "wrong"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expected invalid credential, got %v", err)
	}
}

func TestSessionServiceRefreshRotatesAndRejectsReuse(t *testing.T) {
	db := setupAuthTestDB(t)
	identity := NewIdentityService(db)
	user, err := identity.CreateLocalUser(context.Background(), CreateUserParams{
		Username: "admin",
		Password: "secret",
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

package auth

import (
	"context"
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

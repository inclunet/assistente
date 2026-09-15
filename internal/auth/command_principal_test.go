package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newCommandPrincipalFixture(t *testing.T) (*SessionService, *database.User, time.Time) {
	t.Helper()

	db := setupAuthTestDB(t)
	user, err := NewIdentityService(db).CreateLocalUser(context.Background(), CreateUserParams{
		Username: "command-user",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	signer, err := NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	service, err := NewSessionService(db, SessionConfig{
		Issuer:     "command-issuer",
		Audience:   "command-audience",
		AccessTTL:  time.Hour,
		RefreshTTL: time.Hour,
		Signer:     signer,
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service.now = func() time.Time { return now }
	return service, user, now
}

func TestCommandPrincipalAuthenticatesIssuedSession(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "command-test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	principal, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken)
	if err != nil {
		t.Fatalf("authenticate local access: %v", err)
	}
	if principal != (LocalSessionPrincipal{UserID: user.ID, SessionID: issued.SessionID}) {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestCommandPrincipalLogoutRevokesStillSignedJWT(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "command-test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if _, err := service.VerifyAccessToken(issued.AccessToken); err != nil {
		t.Fatalf("JWT should initially be signed and valid: %v", err)
	}

	if err := service.Logout(context.Background(), issued.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.VerifyAccessToken(issued.AccessToken); err != nil {
		t.Fatalf("logout must not alter JWT signature validity: %v", err)
	}
	if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
		t.Fatalf("expected revoked session to be rejected, got %v", err)
	}
}

func TestCommandPrincipalRejectsInactiveDeletedAndExpiredSessions(t *testing.T) {
	t.Run("inactive account", func(t *testing.T) {
		service, user, _ := newCommandPrincipalFixture(t)
		issued, err := service.IssueSession(context.Background(), user, "command-test")
		if err != nil {
			t.Fatalf("issue session: %v", err)
		}
		if err := service.db.Model(&database.User{}).Where("id = ?", user.ID).Update("is_active", false).Error; err != nil {
			t.Fatalf("deactivate user: %v", err)
		}
		if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
			t.Fatalf("expected inactive account to be rejected, got %v", err)
		}
	})

	t.Run("deleted account", func(t *testing.T) {
		service, user, _ := newCommandPrincipalFixture(t)
		issued, err := service.IssueSession(context.Background(), user, "command-test")
		if err != nil {
			t.Fatalf("issue session: %v", err)
		}
		if err := service.db.Delete(&database.User{}, "id = ?", user.ID).Error; err != nil {
			t.Fatalf("delete user: %v", err)
		}
		if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
			t.Fatalf("expected deleted account to be rejected, got %v", err)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		service, user, now := newCommandPrincipalFixture(t)
		issued, err := service.IssueSession(context.Background(), user, "command-test")
		if err != nil {
			t.Fatalf("issue session: %v", err)
		}
		if err := service.db.Model(&database.Session{}).Where("id = ?", issued.SessionID).Update("expires_at", now.Add(-time.Second)).Error; err != nil {
			t.Fatalf("expire session: %v", err)
		}
		if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
			t.Fatalf("expected expired session to be rejected, got %v", err)
		}
	})

	t.Run("deleted session", func(t *testing.T) {
		service, user, _ := newCommandPrincipalFixture(t)
		issued, err := service.IssueSession(context.Background(), user, "command-test")
		if err != nil {
			t.Fatalf("issue session: %v", err)
		}
		if err := service.db.Delete(&database.Session{}, "id = ?", issued.SessionID).Error; err != nil {
			t.Fatalf("delete session: %v", err)
		}
		if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
			t.Fatalf("expected deleted session to be rejected, got %v", err)
		}
	})
}

func TestCommandPrincipalRejectsDivergentSubjectAndExactJWTExpiry(t *testing.T) {
	service, user, now := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "command-test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	otherUserID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new divergent subject: %v", err)
	}
	otherUser := &database.User{
		UUIDModel:    database.UUIDModel{ID: otherUserID.String()},
		Username:     "other-command-user",
		PasswordHash: "unused-test-hash",
		Role:         database.UserRoleUser,
		IsActive:     true,
	}
	if err := service.db.Create(otherUser).Error; err != nil {
		t.Fatalf("create divergent active user: %v", err)
	}
	claims := AccessClaims{
		Issuer: service.issuer, Audience: service.audience,
		Subject: otherUserID.String(), SessionID: issued.SessionID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}
	divergent, err := service.signer.SignAccessToken(claims)
	if err != nil {
		t.Fatalf("sign divergent claims: %v", err)
	}
	if _, err := service.AuthenticateLocalAccess(context.Background(), divergent); !errors.Is(err, ErrUnauthenticatedLocalSession) {
		t.Fatalf("expected subject/session mismatch to be rejected, got %v", err)
	}
	if _, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken+"tampered"); !errors.Is(err, ErrUnauthenticatedLocalSession) {
		t.Fatalf("expected adulterated token to be rejected, got %v", err)
	}

	exactExpiry := claims
	exactExpiry.Subject = user.ID
	exactExpiry.ExpiresAt = now.Unix()
	exactToken, err := service.signer.SignAccessToken(exactExpiry)
	if err != nil {
		t.Fatalf("sign exact-expiry claims: %v", err)
	}
	if _, err := service.VerifyAccessToken(exactToken); err != nil {
		t.Fatalf("signer skew should still accept exact-expiry JWT for this regression test: %v", err)
	}
	if _, err := service.AuthenticateLocalAccess(context.Background(), exactToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
		t.Fatalf("expected JWT exp at now to be rejected, got %v", err)
	}

	const queryFailureCallback = "command_principal_test_query_failure"
	if err := service.db.Callback().Query().Before("gorm:query").Register(queryFailureCallback, func(db *gorm.DB) {
		db.Error = errors.New("injected query failure")
	}); err != nil {
		t.Fatalf("register query failure callback: %v", err)
	}
	defer func() {
		if err := service.db.Callback().Query().Remove(queryFailureCallback); err != nil {
			t.Errorf("remove query failure callback: %v", err)
		}
	}()
	principal, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken)
	if !errors.Is(err, ErrUnauthenticatedLocalSession) {
		t.Fatalf("expected DB failure to be mapped to sentinel, got %v", err)
	}
	if principal != (LocalSessionPrincipal{}) {
		t.Fatalf("DB failure returned non-zero principal: %+v", principal)
	}
}

func TestCommandPrincipalPreservesCancellationAndFailsClosed(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "command-test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.AuthenticateLocalAccess(canceled, issued.AccessToken); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation to be preserved, got %v", err)
	}
	if _, err := service.AuthenticateLocalAccess(nil, issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatalf("expected nil context to fail closed, got %v", err)
	}

	for name, candidate := range map[string]*SessionService{
		"nil service":  nil,
		"nil database": {signer: service.signer, issuer: service.issuer, audience: service.audience, now: service.now},
		"nil clock":    {db: service.db, signer: service.signer, issuer: service.issuer, audience: service.audience},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := candidate.AuthenticateLocalAccess(context.Background(), issued.AccessToken); !errors.Is(err, ErrUnauthenticatedLocalSession) {
				t.Fatalf("expected fail-closed sentinel, got %v", err)
			}
		})
	}
}

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCommandPrincipalChecksExpiryAfterDatabaseRead(t *testing.T) {
	service, user, now := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "expiry-test")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	originalNow := service.now
	service.now = func() time.Time {
		calls++
		if calls == 1 {
			return now
		}
		return issued.AccessTokenExpiresAt
	}
	defer func() { service.now = originalNow }()
	principal, err := service.AuthenticateLocalAccess(context.Background(), issued.AccessToken)
	if !errors.Is(err, ErrUnauthenticatedLocalSession) || principal != (LocalSessionPrincipal{}) || calls != 2 {
		t.Fatalf("expiração no fim não aplicada: calls=%d err=%v", calls, err)
	}
}

func TestSavedCredentialChecksExpiryAfterDatabaseRead(t *testing.T) {
	for _, mode := range []string{"cli", "refresh"} {
		t.Run(mode, func(t *testing.T) {
			svc, user, now := newCommandPrincipalFixture(t)
			issued, err := svc.IssueSession(t.Context(), user, "desktop")
			if err != nil {
				t.Fatal(err)
			}
			svc.now = func() time.Time { return now }
			// O callback simula que o SELECT só devolveu a linha no fim do prazo.
			if err := svc.db.Callback().Query().After("gorm:query").Register("teste:avanca_relogio", func(tx *gorm.DB) {
				if tx.Statement.Table == "sessions" {
					now = issued.RefreshTokenExpiresAt
				}
			}); err != nil {
				t.Fatal(err)
			}
			if mode == "cli" {
				_, _, err = svc.AuthenticateSavedSession(t.Context(), issued.RefreshToken)
			} else {
				_, err = svc.Refresh(t.Context(), issued.RefreshToken)
			}
			if !errors.Is(err, ErrSessionExpired) {
				t.Fatalf("credencial após espera pelo banco: %v, esperado ErrSessionExpired", err)
			}
		})
	}
}

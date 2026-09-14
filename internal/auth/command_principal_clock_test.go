package auth

import (
	"context"
	"errors"
	"testing"
	"time"
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

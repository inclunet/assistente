package auth

import (
	"context"
	"testing"
	"time"

	"assistente/internal/database"
)

// TestPurgeExpiredSessions valida o fix Mi38 do review do AEP-0052: purga
// de sessions expiradas/revogadas além da janela de retenção, preservando
// sessions ativas e sessions cuja revogação/expiração ainda está dentro do
// período de retenção (auditoria recente).
func TestPurgeExpiredSessions(t *testing.T) {
	db := setupAuthTestDB(t)

	user := &database.User{Username: "ana", PasswordHash: "h", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	retention := 24 * time.Hour

	mustSession := func(s *database.Session) string {
		s.UserID = user.ID
		s.RefreshTokenHash = randomSeedString(t)
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("create session: %v", err)
		}
		return s.ID
	}

	expiredOld := mustSession(&database.Session{ExpiresAt: now.Add(-72 * time.Hour)})

	expiredRecent := mustSession(&database.Session{ExpiresAt: now.Add(-1 * time.Hour)})

	revokedAt := now.Add(-72 * time.Hour)
	revokedOld := mustSession(&database.Session{ExpiresAt: now.Add(-1 * time.Hour), RevokedAt: &revokedAt})

	revokedRecentAt := now.Add(-1 * time.Hour)
	revokedRecent := mustSession(&database.Session{ExpiresAt: now.Add(120 * time.Hour), RevokedAt: &revokedRecentAt})

	active := mustSession(&database.Session{ExpiresAt: now.Add(24 * time.Hour)})

	svc := &SessionService{db: db, now: func() time.Time { return now }}

	deleted, err := svc.PurgeExpiredSessions(context.Background(), retention)
	if err != nil {
		t.Fatalf("PurgeExpiredSessions: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("esperava 2 sessions purgadas (expiredOld + revokedOld), tenho %d", deleted)
	}

	for _, id := range []string{expiredRecent, revokedRecent, active} {
		var got database.Session
		if err := db.First(&got, "id = ?", id).Error; err != nil {
			t.Fatalf("session %s deveria existir, mas: %v", id, err)
		}
	}

	for _, id := range []string{expiredOld, revokedOld} {
		var got database.Session
		if err := db.First(&got, "id = ?", id).Error; err == nil {
			t.Fatalf("session %s deveria ter sido deletada (expirou/revogou fora da janela)", id)
		}
	}
}

// TestPurgeExpiredSessions_ZeroRetention valida o caminho administrativo:
// retention=0 purga tudo que já expirou ou revogou, sem janela de carência.
func TestPurgeExpiredSessions_ZeroRetention(t *testing.T) {
	db := setupAuthTestDB(t)

	user := &database.User{Username: "ana", PasswordHash: "h", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	expired := &database.Session{UserID: user.ID, RefreshTokenHash: randomSeedString(t), ExpiresAt: now.Add(-1 * time.Minute)}
	if err := db.Create(expired).Error; err != nil {
		t.Fatalf("create expired: %v", err)
	}

	revokedAt := now.Add(-1 * time.Minute)
	revoked := &database.Session{UserID: user.ID, RefreshTokenHash: randomSeedString(t), ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}
	if err := db.Create(revoked).Error; err != nil {
		t.Fatalf("create revoked: %v", err)
	}

	active := &database.Session{UserID: user.ID, RefreshTokenHash: randomSeedString(t), ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(active).Error; err != nil {
		t.Fatalf("create active: %v", err)
	}

	svc := &SessionService{db: db, now: func() time.Time { return now }}
	deleted, err := svc.PurgeExpiredSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("esperava 2 deletadas (expirada + revogada), tenho %d", deleted)
	}

	var remaining database.Session
	if err := db.First(&remaining, "id = ?", active.ID).Error; err != nil {
		t.Fatalf("active session foi deletada por engano: %v", err)
	}
}

func randomSeedString(t *testing.T) string {
	t.Helper()
	s, err := newRefreshSecret()
	if err != nil {
		t.Fatalf("newRefreshSecret: %v", err)
	}
	return s
}

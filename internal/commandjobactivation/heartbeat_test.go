package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

func TestHeartbeatPassRenewsTwoLeasesWithCursorMoreAndHostTTL(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	deliver(t, c, out, fact)
	secondID := addHeartbeatLease(t, c, out, fact, *now)

	firstCursor, first, err := c.HeartbeatPass(context.Background(), "", 1, 45*time.Second)
	if err != nil || firstCursor == "" || first.Scanned != 1 || first.Renewed != 1 || first.Rejected != 0 || !first.More {
		t.Fatalf("primeiro heartbeat=(%q,%+v,%v), esperado uma renovação e More", firstCursor, first, err)
	}
	if c.lease != 3*time.Minute {
		t.Fatalf("política compartilhada foi alterada: %s", c.lease)
	}
	var firstLease Lease
	if err := c.db.Where("activation_id = ?", firstCursor).Take(&firstLease).Error; err != nil {
		t.Fatal(err)
	}
	if !firstLease.ExpiresAt.Equal((*now).Add(45 * time.Second)) {
		t.Fatalf("TTL do host não aplicado à primeira lease: %s", firstLease.ExpiresAt)
	}

	secondCursor, second, err := c.HeartbeatPass(context.Background(), firstCursor, 1, 90*time.Second)
	if err != nil || secondCursor != secondID || second.Scanned != 1 || second.Renewed != 1 || second.Rejected != 0 || second.More {
		t.Fatalf("segundo heartbeat=(%q,%+v,%v), esperado cursor final sem More", secondCursor, second, err)
	}
	var secondLease Lease
	if err := c.db.Where("activation_id = ?", secondID).Take(&secondLease).Error; err != nil {
		t.Fatal(err)
	}
	if !secondLease.ExpiresAt.Equal((*now).Add(90 * time.Second)) {
		t.Fatalf("TTL atualizado entre passagens não foi aplicado: %s", secondLease.ExpiresAt)
	}
}

func TestHeartbeatPassRejectsNonPositiveHostTTL(t *testing.T) {
	c, _, _, _, _ := fixture(t)
	for _, ttl := range []time.Duration{0, -time.Second} {
		cursor, result, err := c.HeartbeatPass(context.Background(), "", 1, ttl)
		if !errors.Is(err, ErrUnavailable) || cursor != "" || result != (HeartbeatResult{}) {
			t.Fatalf("TTL=%s resultou em (%q,%+v,%v), esperado rejeição", ttl, cursor, result, err)
		}
	}
}

func TestHeartbeatPassCancellationReportsCommittedPrefix(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	deliver(t, c, out, fact)
	secondID := addHeartbeatLease(t, c, out, fact, *now)

	originalAuthorize := c.ports.Authorize
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.ports.Authorize = func(ctx context.Context, tx *gorm.DB, userID string, workspaceID *string) (commandactivation.Owner, error) {
		calls++
		if calls == 2 {
			cancel()
			return commandactivation.Owner{}, context.Canceled
		}
		return originalAuthorize(ctx, tx, userID, workspaceID)
	}

	cursor, result, err := c.HeartbeatPass(ctx, "", 2, 45*time.Second)
	if !errors.Is(err, context.Canceled) || result.Scanned != 2 || result.Renewed != 1 || result.Rejected != 0 || !result.More {
		t.Fatalf("cancelamento parcial=(%q,%+v,%v), esperado primeiro commit preservado", cursor, result, err)
	}
	var firstLease, secondLease Lease
	if err := c.db.Where("activation_id = ?", cursor).Take(&firstLease).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("activation_id = ?", secondID).Take(&secondLease).Error; err != nil {
		t.Fatal(err)
	}
	if !firstLease.ExpiresAt.Equal((*now).Add(45 * time.Second)) {
		t.Fatalf("prefixo confirmado não foi renovado: %s", firstLease.ExpiresAt)
	}
	if !secondLease.ExpiresAt.Equal((*now).Add(3 * time.Minute)) {
		t.Fatalf("lease após cancelamento foi alterada: %s", secondLease.ExpiresAt)
	}
}

func TestHeartbeatPassRejectsExpiredAndOldRuntimeWithoutRenewing(t *testing.T) {
	t.Run("lease expirada", func(t *testing.T) {
		c, out, fact, _, now := fixture(t)
		deliver(t, c, out, fact)
		var lease Lease
		if err := c.db.Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		*now = lease.ExpiresAt
		before := lease.ExpiresAt
		cursor, result, err := c.HeartbeatPass(context.Background(), "", 1, time.Hour)
		if err != nil || cursor != lease.ActivationID || result.Scanned != 1 || result.Renewed != 0 || result.Rejected != 1 || result.More {
			t.Fatalf("lease expirada=(%q,%+v,%v), esperado rejeição fechada", cursor, result, err)
		}
		if err := c.db.Where("id = ?", lease.ID).Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		if !lease.ExpiresAt.Equal(before) {
			t.Fatalf("lease expirada foi revivida: %s", lease.ExpiresAt)
		}
	})

	t.Run("runtime de epoch anterior", func(t *testing.T) {
		c, out, fact, _, now := fixture(t)
		deliver(t, c, out, fact)
		var lease Lease
		if err := c.db.Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		before := lease.ExpiresAt
		c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
			return RuntimeIdentity{Generation: "runtime-previous", UserID: fact.UserID, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
		}
		*now = fact.OccurredAt.Add(31 * time.Second)
		cursor, result, err := c.HeartbeatPass(context.Background(), "", 1, time.Hour)
		if err != nil || cursor != lease.ActivationID || result.Scanned != 1 || result.Renewed != 0 || result.Rejected != 1 || result.More {
			t.Fatalf("runtime antigo=(%q,%+v,%v), esperado rejeição fechada", cursor, result, err)
		}
		if err := c.db.Where("id = ?", lease.ID).Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		if !lease.ExpiresAt.Equal(before) {
			t.Fatalf("runtime de epoch anterior renovou lease: %s", lease.ExpiresAt)
		}
	})
}

func addHeartbeatLease(t *testing.T, c *Consumer, out *commandjobevents.Store, fact commandjobevents.Fact, now time.Time) string {
	t.Helper()
	second := fact
	var err error
	second.SourceEventID, err = freshID()
	if err != nil {
		t.Fatal(err)
	}
	second.RunEventID = second.SourceEventID
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, second) }); err != nil {
		t.Fatal(err)
	}
	row, err := out.Get(context.Background(), second.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	var base commandactivation.Claim
	if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&base).Error; err != nil {
		t.Fatal(err)
	}
	activationID, err := freshID()
	if err != nil {
		t.Fatal(err)
	}
	sequence := int64(1)
	claim := base
	claim.ActivationID = activationID
	claim.SourceEventID = &second.SourceEventID
	claim.SourceCorrelationID = &second.RunID
	claim.Sequence = &sequence
	claim.SourceJobDatabaseID = &row.JobDatabaseID
	claim.SourceJobSlug = &row.JobSlug
	claim.EventFingerprint = &row.EventFingerprint
	claim.SourceReplayPolicyGeneration = &row.SourceReplayPolicyGeneration
	claim.SourceReplayDeadline = &row.SourceReplayDeadline
	claim.State = commandactivation.StateActive
	claim.ActivatedAt = now
	claim.UpdatedAt = now
	claim.ExpiresAt = timePtr(now.Add(30 * time.Minute))
	claim.TerminalReason = nil
	if err := c.db.Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	leaseID, err := freshID()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.db.Create(&Lease{ID: leaseID, ActivationID: activationID, UserID: claim.UserID, RunID: second.RunID, RuntimeGeneration: "runtime-1", ExpiresAt: now.Add(3 * time.Minute), UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	return activationID
}

func timePtr(value time.Time) *time.Time { return &value }

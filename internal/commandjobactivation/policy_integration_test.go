package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandmaintenance"
	"gorm.io/gorm"
)

func TestCoordinatorPublishesPolicyToRealConsumerAndPreservesReplayFloors(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })
	c, out, fact, _, now := fixture(t)
	*now = time.Now().UTC()
	fact.OccurredAt = *now
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("fato inicial não aplicado: %+v", got)
	}

	// A segunda ocorrência fica pendente para ser consumida pelo adapter real
	// durante a passagem do Coordinator, criando uma lease nova com o snapshot.
	second := fact
	var err error
	second.SourceEventID, err = freshID()
	if err != nil {
		t.Fatal(err)
	}
	second.RunEventID = second.SourceEventID
	second.RunID = "opaque:run-2"
	if err := c.db.Exec("INSERT INTO job_runs(id, user_id, job_id) SELECT ?, user_id, job_id FROM job_runs WHERE id = ?", second.RunID, fact.RunID).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, second) }); err != nil {
		t.Fatal(err)
	}

	heartbeat, err := NewMaintenanceHeartbeatAdapter(c)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := NewMaintenanceOutboxAdapter(c, "coordinator-delivery")
	if err != nil {
		t.Fatal(err)
	}
	policy := commandmaintenance.Policy{
		InvocationRetention:   time.Hour,
		ActivationRetention:   60 * 24 * time.Hour,
		LeaseDuration:         45 * time.Second,
		InvocationsPerUser:    1,
		InvocationsSystemKeep: 1,
		ActivationsPerUser:    1,
		BatchSize:             2,
	}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat:    heartbeat,
		Outbox:       outbox,
		Decisions:    maintenanceAdapterRecovery{},
		Invocations:  maintenanceAdapterRecovery{},
		Claims:       maintenanceAdapterRecovery{},
		Jobs:         maintenanceAdapterRetention{calls: new(int)},
		Tools:        maintenanceAdapterTools{calls: new(int)},
		InvocationDB: maintenanceAdapterRetention{calls: new(int)},
		Activations:  maintenanceAdapterRetention{calls: new(int)},
		Compaction:   maintenanceAdapterCompaction{calls: new(int)},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := coordinator.Run(context.Background(), policy)
	if err != nil || first.HeartbeatProcessed != 1 || first.OutboxDrained == false {
		t.Fatalf("primeira política: report=%+v err=%v", first, err)
	}

	var claims []commandactivation.Claim
	if err := c.db.Order("activation_id").Find(&claims).Error; err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 {
		t.Fatalf("claims=%d, esperado claim inicial e claim novo", len(claims))
	}
	var oldClaim, newClaim commandactivation.Claim
	for _, claim := range claims {
		if claim.SourceCorrelationID != nil && *claim.SourceCorrelationID == fact.RunID {
			oldClaim = claim
		}
		if claim.SourceCorrelationID != nil && *claim.SourceCorrelationID == second.RunID {
			newClaim = claim
		}
	}
	if oldClaim.ActivationID == "" || newClaim.ActivationID == "" {
		t.Fatalf("claims não identificadas: old=%+v new=%+v", oldClaim, newClaim)
	}
	if oldClaim.ExpiresAt == nil || newClaim.ExpiresAt == nil {
		t.Fatalf("claims sem expiry: old=%+v new=%+v", oldClaim, newClaim)
	}
	var leases []Lease
	if err := c.db.Order("activation_id").Find(&leases).Error; err != nil {
		t.Fatal(err)
	}
	if len(leases) != 2 {
		t.Fatalf("leases=%d, esperado duas leases", len(leases))
	}
	for _, lease := range leases {
		if !lease.ExpiresAt.Equal((*now).Add(policy.LeaseDuration)) {
			t.Fatalf("lease não usou snapshot da primeira policy: %+v", lease)
		}
	}

	var oldLedger, newLedger eventLedger
	if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&oldLedger).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("source_event_id = ?", second.SourceEventID).Take(&newLedger).Error; err != nil {
		t.Fatal(err)
	}
	if oldClaim.ExpiresAt.Before(oldLedger.SourceReplayDeadline) || newClaim.ExpiresAt.Before(newLedger.SourceReplayDeadline) ||
		oldLedger.ExpiresAt.Before(oldLedger.SourceReplayDeadline) || newLedger.ExpiresAt.Before(newLedger.SourceReplayDeadline) {
		t.Fatalf("piso de replay não preservado: claims=(%s,%s) ledgers=(%s,%s) replay=(%s,%s)", oldClaim.ExpiresAt, newClaim.ExpiresAt, oldLedger.ExpiresAt, newLedger.ExpiresAt, oldLedger.SourceReplayDeadline, newLedger.SourceReplayDeadline)
	}
	if !oldClaim.ExpiresAt.Equal((*now).Add(policy.ActivationRetention)) || !newClaim.ExpiresAt.Equal((*now).Add(policy.ActivationRetention)) || !newLedger.ExpiresAt.Equal((*now).Add(policy.ActivationRetention)) {
		t.Fatalf("snapshot de retention não aplicado: oldClaim=%s newClaim=%s newLedger=%s esperado=%s", oldClaim.ExpiresAt, newClaim.ExpiresAt, newLedger.ExpiresAt, (*now).Add(policy.ActivationRetention))
	}

	if oldClaim.ExpiresAt == nil || newClaim.ExpiresAt == nil {
		t.Fatal("claims sem expiry")
	}
	oldClaimExpiry, newClaimExpiry := *oldClaim.ExpiresAt, *newClaim.ExpiresAt
	oldLedgerExpiry, newLedgerExpiry := oldLedger.ExpiresAt, newLedger.ExpiresAt
	policy.LeaseDuration = 10 * time.Second
	policy.ActivationRetention = time.Minute
	secondReport, err := coordinator.Run(context.Background(), policy)
	if err != nil || secondReport.HeartbeatProcessed != 2 {
		t.Fatalf("segunda política: report=%+v err=%v", secondReport, err)
	}
	if err := c.db.Where("activation_id = ?", oldClaim.ActivationID).Take(&oldClaim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("activation_id = ?", newClaim.ActivationID).Take(&newClaim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&oldLedger).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("source_event_id = ?", second.SourceEventID).Take(&newLedger).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Order("activation_id").Find(&leases).Error; err != nil {
		t.Fatal(err)
	}
	for _, lease := range leases {
		if !lease.ExpiresAt.Equal((*now).Add(policy.LeaseDuration)) {
			t.Fatalf("segunda policy não renovou lease para 10s: %+v", lease)
		}
	}
	if oldClaim.ExpiresAt == nil || newClaim.ExpiresAt == nil || oldClaim.ExpiresAt.Before(oldClaimExpiry) || newClaim.ExpiresAt.Before(newClaimExpiry) || oldLedger.ExpiresAt.Before(oldLedgerExpiry) || newLedger.ExpiresAt.Before(newLedgerExpiry) {
		t.Fatalf("redução encurtou expiry existente: claims=(%s,%s) ledgers=(%s,%s)", oldClaim.ExpiresAt, newClaim.ExpiresAt, oldLedger.ExpiresAt, newLedger.ExpiresAt)
	}

	if oldClaim.ExpiresAt == nil || newClaim.ExpiresAt == nil {
		t.Fatal("claims sem expiry antes de policy inválida")
	}
	claimsBeforeInvalid := []time.Time{*oldClaim.ExpiresAt, *newClaim.ExpiresAt}
	ledgersBeforeInvalid := []time.Time{oldLedger.ExpiresAt, newLedger.ExpiresAt}
	leasesBeforeInvalid := make([]time.Time, 0, len(leases))
	if err := c.db.Order("activation_id").Find(&leases).Error; err != nil {
		t.Fatal(err)
	}
	for _, lease := range leases {
		leasesBeforeInvalid = append(leasesBeforeInvalid, lease.ExpiresAt)
	}
	invalid := policy
	invalid.LeaseDuration = 0
	if _, err := coordinator.Run(context.Background(), invalid); !errors.Is(err, commandmaintenance.ErrInvalid) {
		t.Fatalf("política inválida=%v, esperado ErrInvalid", err)
	}
	if err := c.db.Where("activation_id = ?", oldClaim.ActivationID).Take(&oldClaim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("activation_id = ?", newClaim.ActivationID).Take(&newClaim).Error; err != nil {
		t.Fatal(err)
	}
	if oldClaim.ExpiresAt == nil || newClaim.ExpiresAt == nil || !oldClaim.ExpiresAt.Equal(claimsBeforeInvalid[0]) || !newClaim.ExpiresAt.Equal(claimsBeforeInvalid[1]) {
		t.Fatal("política inválida alterou claims")
	}
	if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&oldLedger).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("source_event_id = ?", second.SourceEventID).Take(&newLedger).Error; err != nil {
		t.Fatal(err)
	}
	if oldLedger.ExpiresAt != ledgersBeforeInvalid[0] || newLedger.ExpiresAt != ledgersBeforeInvalid[1] {
		t.Fatal("política inválida alterou ledgers")
	}
	if err := c.db.Order("activation_id").Find(&leases).Error; err != nil {
		t.Fatal(err)
	}
	for i, lease := range leases {
		if lease.ExpiresAt != leasesBeforeInvalid[i] {
			t.Fatal("política inválida alterou leases")
		}
	}
}

package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

// Este cenário é deliberadamente adversarial: reescreve a outbox entregue
// para processing. Consume real não tem janela durável entre claims/ledger e
// ack, pois todos os efeitos são uma transação.
func TestRecoveryMatrixAdversarialOutboxResetReplayAcrossScopes(t *testing.T) {
	c, out, fact, rule, _ := fixture(t)
	addProjectionWorkspaceRule(t, c, rule, fact.UserID, "workspace-a")
	addProjectionWorkspaceRule(t, c, rule, fact.UserID, "workspace-b")
	if got := deliver(t, c, out, fact); got.Applied != 3 {
		t.Fatalf("admissão global+dois workspaces=%+v", got)
	}

	claimsBefore := countActivationClaims(t, c)
	ledgerBefore := countActivationLedger(t, c)
	if claimsBefore != 3 || ledgerBefore != 3 {
		t.Fatalf("estado inicial claims=%d ledger=%d, want 3/3", claimsBefore, ledgerBefore)
	}
	replayed := redeliverAfterAdversarialOutboxReset(t, c, out, fact)
	if replayed.Replayed != 3 || replayed.Applied != 0 {
		t.Fatalf("reentrega adversarial=%+v, want três replays sem aplicação", replayed)
	}
	if got := countActivationClaims(t, c); got != claimsBefore {
		t.Fatalf("replay duplicou claims: antes=%d depois=%d", claimsBefore, got)
	}
	if got := countActivationLedger(t, c); got != ledgerBefore {
		t.Fatalf("replay duplicou ledger: antes=%d depois=%d", ledgerBefore, got)
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != commandjobevents.DeliveryDelivered {
		t.Fatalf("outbox após replay=%q, want delivered", row.DeliveryState)
	}
}

func TestRecoveryMatrixRealNewConsumerReclaimsExpiredLease(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	realNow := time.Now().UTC().Truncate(time.Microsecond)
	*now = realNow
	fact.OccurredAt = realNow
	if _, err := out.EnsureReplayPolicyEpoch(context.Background(), commandjobevents.ProducerType, realNow.Add(-time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	firstConsumer := newRecoveryConsumer(t, c)
	claimed, _, err := firstConsumer.outbox.ClaimBatch(context.Background(), "crashed-worker", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim antes da queda=%d err=%v", len(claimed), err)
	}
	if row, err := out.Get(context.Background(), fact.SourceEventID); err != nil || row.DeliveryState != commandjobevents.DeliveryProcessing {
		t.Fatalf("outbox antes da recuperação=%+v err=%v", row, err)
	}

	// O relógio do Store é interno ao pacote commandjobevents; expirar somente
	// a lease durável aqui evita esperar e mantém o Consumer fixture estável.
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Update("lease_expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	recoveredConsumer := newRecoveryConsumer(t, c)
	result, err := recoveredConsumer.RunPass(context.Background(), "recovery-worker", 1)
	if err != nil || result.Claimed != 1 || result.Processed != 1 || result.Applied != 1 {
		t.Fatalf("recovery real=%+v err=%v", result, err)
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil || row.DeliveryState != commandjobevents.DeliveryDelivered {
		t.Fatalf("outbox recuperada=%+v err=%v", row, err)
	}

	postRecoveryConsumer := newRecoveryConsumer(t, c)
	result, err = postRecoveryConsumer.RunPass(context.Background(), "post-recovery-worker", 1)
	if err != nil || result.Claimed != 0 || result.Processed != 0 {
		t.Fatalf("RunPass pós-recovery não foi no-op=%+v err=%v", result, err)
	}
	if got := countActivationClaims(t, c); got != 1 || countActivationLedger(t, c) != 1 {
		t.Fatalf("recovery duplicou estado: claims=%d ledger=%d", got, countActivationLedger(t, c))
	}
}

func TestRecoveryMatrixAckCallbackRollbackThenReplay(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	if claimed, _, err := out.ClaimBatch(context.Background(), "ack-worker", 1); err != nil || len(claimed) != 1 {
		t.Fatalf("claim para callback=%d err=%v", len(claimed), err)
	}
	failure := errors.New("ack callback failure")
	const callbackName = "recovery_matrix_fail_outbox_update"
	if err := c.db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == (commandjobevents.ActivationOutbox{}).TableName() {
			_ = tx.AddError(failure) // Injeta a falha no statement observado pelo teste.
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.db.Callback().Update().Remove(callbackName); err != nil {
			t.Error(err)
		}
	}()

	failedConsumer := newRecoveryConsumer(t, c)
	if _, err := failedConsumer.Consume(context.Background(), fact.SourceEventID, "ack-worker"); !errors.Is(err, failure) {
		t.Fatalf("falha de ack=%v, want %v", err, failure)
	}
	if got := countActivationClaims(t, c); got != 0 || countActivationLedger(t, c) != 0 {
		t.Fatalf("rollback parcial: claims=%d ledger=%d", got, countActivationLedger(t, c))
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil || row.DeliveryState != commandjobevents.DeliveryProcessing {
		t.Fatalf("outbox após rollback=%+v err=%v", row, err)
	}

	if err := c.db.Callback().Update().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	replayedConsumer := newRecoveryConsumer(t, c)
	result, err := replayedConsumer.Consume(context.Background(), fact.SourceEventID, "ack-worker")
	if err != nil || result.Applied != 1 {
		t.Fatalf("replay após rollback=%+v err=%v", result, err)
	}
	if got := countActivationClaims(t, c); got != 1 || countActivationLedger(t, c) != 1 {
		t.Fatalf("replay após rollback duplicou estado: claims=%d ledger=%d", got, countActivationLedger(t, c))
	}
}

func TestRecoveryMatrixSequenceAndFingerprintIsolation(t *testing.T) {
	c, out, fact, rule, _ := fixture(t)
	addProjectionWorkspaceRule(t, c, rule, fact.UserID, "workspace-a")
	addProjectionWorkspaceRule(t, c, rule, fact.UserID, "workspace-b")
	if got := deliver(t, c, out, fact); got.Applied != 3 {
		t.Fatalf("admissão inicial global+dois workspaces=%+v", got)
	}

	newFact := func(sequence int, state string) commandjobevents.Fact {
		current := fact
		current.SourceEventID, _ = freshID()
		current.RunEventID = current.SourceEventID
		current.Sequence = sequence
		current.State = state
		return current
	}
	newer := newFact(2, commandjobevents.StateStarted)
	if got := deliver(t, c, out, newer); got.Applied != 3 {
		t.Fatalf("sequência nova=%+v", got)
	}
	claims := loadJobClaims(t, c)
	for _, claim := range claims {
		if claim.Sequence == nil || *claim.Sequence != 2 || claim.State != commandactivation.StateActive {
			t.Fatalf("claim após sequência nova=%+v", claim)
		}
	}

	older := newFact(1, commandjobevents.StateQueued)
	if got := deliver(t, c, out, older); got.Ignored != 3 || got.Applied != 0 {
		t.Fatalf("sequência atrasada=%+v, want ignored por regra", got)
	}
	for _, claim := range loadJobClaims(t, c) {
		if claim.Sequence == nil || *claim.Sequence != 2 || claim.State != commandactivation.StateActive {
			t.Fatalf("sequência atrasada alterou claim=%+v", claim)
		}
	}

	conflict := newFact(2, commandjobevents.StateFailed)
	if got := deliver(t, c, out, conflict); got.Conflicts != 3 || got.Applied != 0 {
		t.Fatalf("fingerprint conflitante=%+v, want conflito por regra", got)
	}
	for _, claim := range loadJobClaims(t, c) {
		if claim.Sequence == nil || *claim.Sequence != 2 || claim.State != commandactivation.StateActive {
			t.Fatalf("conflito de fingerprint alterou claim=%+v", claim)
		}
	}
	if got := countActivationClaims(t, c); got != 3 {
		t.Fatalf("claims duplicadas após sequência/fingerprint: %d", got)
	}
}

func TestRecoveryMatrixForeignUserCannotAdoptEvent(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	if claimed, _, err := out.ClaimBatch(context.Background(), "foreign-worker", 1); err != nil || len(claimed) != 1 {
		t.Fatalf("claim estrangeiro=%d err=%v", len(claimed), err)
	}
	originalAuthorize := c.ports.Authorize
	c.ports.Authorize = func(context.Context, *gorm.DB, commandjobevents.Fact, *string) (commandactivation.Owner, error) {
		return commandactivation.Owner{Scope: commandactivation.Scope{UserID: "foreign-user"}, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
	}
	if _, err := c.Consume(context.Background(), fact.SourceEventID, "foreign-worker"); err == nil {
		t.Fatal("owner estrangeiro foi aceito")
	}
	if got := countActivationClaims(t, c); got != 0 || countActivationLedger(t, c) != 0 {
		t.Fatalf("owner estrangeiro deixou estado: claims=%d ledger=%d", got, countActivationLedger(t, c))
	}
	c.ports.Authorize = originalAuthorize
	recovered := newRecoveryConsumer(t, c)
	result, err := recovered.Consume(context.Background(), fact.SourceEventID, "foreign-worker")
	if err != nil || result.Applied != 1 {
		t.Fatalf("replay com owner legítimo=%+v err=%v", result, err)
	}
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.UserID != fact.UserID {
		t.Fatalf("claim adotada por usuário estrangeiro: %q", claim.UserID)
	}
}

func TestRecoveryMatrixRemovedRunDeadLettersWithoutRetryOrClaim(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	if err := c.db.Exec("DELETE FROM job_runs WHERE id = ?", fact.RunID).Error; err != nil {
		t.Fatal(err)
	}

	result, err := c.RunPass(context.Background(), "recovery-worker", 1)
	if err != nil || result.Claimed != 1 || result.Processed != 1 || result.DeadLettered != 1 {
		t.Fatalf("run removido=%+v err=%v, want dead-letter permanente", result, err)
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != commandjobevents.DeliveryDeadLetter || row.LastErrorCode != "source_not_found" || row.Attempts != 1 {
		t.Fatalf("outbox após source_not_found=%+v", row)
	}
	if got := countActivationClaims(t, c); got != 0 {
		t.Fatalf("run removido criou claim=%d", got)
	}

	second, err := c.RunPass(context.Background(), "recovery-worker-2", 1)
	if err != nil || second.Claimed != 0 || second.Processed != 0 || second.DeadLettered != 0 {
		t.Fatalf("dead-letter foi tentado novamente=%+v err=%v", second, err)
	}
}

func redeliverAfterAdversarialOutboxReset(t *testing.T, c *Consumer, out *commandjobevents.Store, fact commandjobevents.Fact) Result {
	t.Helper()
	leaseUntil := c.now().UTC().Add(time.Minute)
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
		"delivery_state":   commandjobevents.DeliveryProcessing,
		"lease_owner":      "recovered-worker",
		"lease_expires_at": leaseUntil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := c.Consume(context.Background(), fact.SourceEventID, "recovered-worker")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func newRecoveryConsumer(t *testing.T, source *Consumer) *Consumer {
	t.Helper()
	recovered, err := New(source.db, &commandsecurity.DispatchGate{}, source.ports, source.lease, source.retention, source.now)
	if err != nil {
		t.Fatal(err)
	}
	return recovered
}

func countActivationClaims(t *testing.T, c *Consumer) int {
	t.Helper()
	var count int64
	if err := c.db.Model(&commandactivation.Claim{}).Where("source_type = ?", "job").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return int(count)
}

func countActivationLedger(t *testing.T, c *Consumer) int {
	t.Helper()
	var count int64
	if err := c.db.Model(&eventLedger{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return int(count)
}

func loadJobClaims(t *testing.T, c *Consumer) []commandactivation.Claim {
	t.Helper()
	var claims []commandactivation.Claim
	if err := c.db.Where("source_type = ?", "job").Order("workspace_id, activation_id").Find(&claims).Error; err != nil {
		t.Fatal(err)
	}
	return claims
}

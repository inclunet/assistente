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

func TestRenewRuntimeAfterReplayDeadlineDoesNotReacceptEvent(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	if _, err := out.EnsureReplayPolicyEpoch(context.Background(), commandjobevents.ProducerType, fact.OccurredAt, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	deliver(t, c, out, fact)

	var claim ClaimSnapshot
	if err := c.db.Raw("SELECT activation_id, source_event_id, source_replay_policy_generation, source_replay_deadline FROM command_layer_activation_state LIMIT 1").Scan(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.ActivationID == "" || claim.SourceEventID == "" || claim.SourceReplayDeadline.IsZero() {
		t.Fatalf("claim incompleta: %+v", claim)
	}
	row, err := out.Get(context.Background(), claim.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}

	// O epoch de 30s vence naturalmente enquanto a lease original de 3min
	// continua viva; nenhum expires_at da claim é fabricado no teste.
	*now = fact.OccurredAt.Add(31 * time.Second)
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err != nil {
		t.Fatalf("heartbeat válido após replay deadline rejeitado: %v", err)
	}

	var lease struct {
		ExpiresAt time.Time
	}
	if err := c.db.Table("command_job_activation_leases").Where("activation_id = ?", claim.ActivationID).Take(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if !lease.ExpiresAt.Equal((*now).Add(c.lease)) {
		t.Fatalf("lease não renovada pelo runtime atual: %s", lease.ExpiresAt)
	}
	after, err := out.Get(context.Background(), claim.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if after.SourceReplayPolicyGeneration != row.SourceReplayPolicyGeneration || !after.SourceReplayDeadline.Equal(row.SourceReplayDeadline) || after.EventFingerprint != row.EventFingerprint {
		t.Fatalf("heartbeat alterou barreira da ocorrência: antes=%+v depois=%+v", row, after)
	}

	// A mesma ocorrência antiga continua fora da admissão: renovar o runtime
	// não a transforma em uma nova entrega/reexecução.
	owner := "late-replay"
	deliveryExpiry := (*now).Add(time.Minute)
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", claim.SourceEventID).Updates(map[string]any{
		"delivery_state":   commandjobevents.DeliveryProcessing,
		"lease_owner":      owner,
		"lease_expires_at": deliveryExpiry,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Consume(context.Background(), claim.SourceEventID, owner); !errors.Is(err, commandjobevents.ErrInvalidFact) {
		t.Fatalf("replay fora do horizonte foi aceito: %v", err)
	}
}

func TestRenewRuntimeRejectsRevokedGrantAfterReplayDeadline(t *testing.T) {
	c, out, fact, rule, now := fixture(t)
	deliver(t, c, out, fact)
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	row, err := out.Get(context.Background(), *claim.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	*now = row.SourceReplayDeadline.Add(time.Minute)
	if err := c.db.Table("command_job_activation_leases").Where("activation_id = ?", claim.ActivationID).Update("expires_at", (*now).Add(time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Table("command_layer_automation_grants").Where("id = ?", *rule.AutomationGrantID).Update("revoked_at", *now).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err == nil {
		t.Fatal("lease renovada com grant revogado")
	}
	var lease Lease
	if err := c.db.Where("activation_id = ?", claim.ActivationID).Take(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if !lease.ExpiresAt.Equal((*now).Add(time.Minute)) {
		t.Fatalf("falha de grant alterou lease: %s", lease.ExpiresAt)
	}
}

func TestRenewRuntimeRejectsPreviousRuntimeAfterReplayDeadline(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	deliver(t, c, out, fact)
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	row, err := out.Get(context.Background(), *claim.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	*now = row.SourceReplayDeadline.Add(time.Minute)
	if err := c.db.Table("command_job_activation_leases").Where("activation_id = ?", claim.ActivationID).Update("expires_at", (*now).Add(time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
		return RuntimeIdentity{Generation: "runtime-old", UserID: claim.UserID, AuthContextType: claim.AuthContextType, AuthContextID: claim.AuthContextID, AuthGeneration: claim.AuthGeneration, SecurityGeneration: claim.SecurityGeneration}, nil
	}
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err == nil {
		t.Fatal("lease renovada por runtime de geração antiga")
	}
}

func TestTerminalClaimRemainsClosedAfterReplayDeadline(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	deliver(t, c, out, fact)
	fact.SourceEventID, _ = freshID()
	fact.RunEventID = fact.SourceEventID
	fact.Sequence = 2
	fact.State = commandjobevents.StateCompleted
	deliver(t, c, out, fact)
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateDeactivated {
		t.Fatalf("claim não terminalizada: %s", claim.State)
	}
	*now = claim.SourceReplayDeadline.Add(time.Minute)
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err == nil {
		t.Fatal("claim terminal reaberta após horizonte")
	}
}

func TestRenewRuntimeFailsClosedWhenOriginalOutboxWasPurged(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	if _, err := out.EnsureReplayPolicyEpoch(context.Background(), commandjobevents.ProducerType, fact.OccurredAt, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	deliver(t, c, out, fact)
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	row, err := out.Get(context.Background(), *claim.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	*now = row.SourceReplayDeadline.Add(time.Second)
	if err := c.db.Where("source_event_id = ?", *claim.SourceEventID).Delete(&commandjobevents.ActivationOutbox{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err == nil {
		t.Fatal("lease renovada sem a ocorrência persistida")
	}
}

type ClaimSnapshot struct {
	ActivationID                 string
	SourceEventID                string
	SourceReplayPolicyGeneration string
	SourceReplayDeadline         time.Time
}

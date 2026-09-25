package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

func projectionOwner(f commandjobevents.Fact) commandactivation.Owner {
	return commandactivation.Owner{Scope: commandactivation.Scope{UserID: f.UserID}, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}
}

func TestProjectionReturnsCurrentAuthorizedClaimWithoutMutation(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	owner := projectionOwner(fact)
	var beforeClaim commandactivation.Claim
	if err := c.db.Where("user_id = ? AND source_correlation_id = ?", fact.UserID, fact.RunID).Take(&beforeClaim).Error; err != nil {
		t.Fatal(err)
	}
	var beforeLease Lease
	if err := c.db.Where("run_id = ?", fact.RunID).Take(&beforeLease).Error; err != nil {
		t.Fatal(err)
	}
	contextCalls := 0
	c.ports.WithContext = func(ctx context.Context, fn func(context.Context) error) error {
		contextCalls++
		return fn(ctx)
	}

	snapshot, err := c.Projection(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != c.ProjectionRevision() || len(snapshot.Claims) != 1 {
		t.Fatalf("snapshot=%+v revision=%d", snapshot, c.ProjectionRevision())
	}
	entry := snapshot.Claims[0]
	if entry.RunID != fact.RunID || entry.Runtime.Generation != "runtime-1" || entry.Claim.ActivationID != beforeClaim.ActivationID {
		t.Fatalf("claim projetada=%+v", entry)
	}
	if snapshot.ValidUntil.IsZero() || !snapshot.ValidUntil.Equal(beforeLease.ExpiresAt) {
		t.Fatalf("validade=%v lease=%v", snapshot.ValidUntil, beforeLease.ExpiresAt)
	}
	if contextCalls != 1 {
		t.Fatalf("WithContext chamado %d vezes, esperado 1", contextCalls)
	}

	var afterClaim commandactivation.Claim
	if err := c.db.Where("activation_id = ?", beforeClaim.ActivationID).Take(&afterClaim).Error; err != nil {
		t.Fatal(err)
	}
	var afterLease Lease
	if err := c.db.Where("id = ?", beforeLease.ID).Take(&afterLease).Error; err != nil {
		t.Fatal(err)
	}
	if afterClaim.UpdatedAt != beforeClaim.UpdatedAt || afterLease.ExpiresAt != beforeLease.ExpiresAt || afterLease.UpdatedAt != beforeLease.UpdatedAt {
		t.Fatal("Projection alterou claim ou lease")
	}
}

func TestProjectionIncludesGlobalAndCurrentWorkspaceOnly(t *testing.T) {
	c, out, fact, baseRule, _ := fixture(t)
	workspace := "workspace-current"
	addProjectionWorkspaceRule(t, c, baseRule, fact.UserID, workspace)
	if got := deliver(t, c, out, fact); got.Applied != 2 {
		t.Fatalf("ciclos global+workspace aplicados=%+v", got)
	}

	owner := projectionOwner(fact)
	owner.WorkspaceID = &workspace
	snapshot, err := c.Projection(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Claims) != 2 {
		t.Fatalf("claims global+workspace=%d, want 2", len(snapshot.Claims))
	}
	seenGlobal, seenWorkspace := false, false
	for _, entry := range snapshot.Claims {
		switch {
		case entry.Claim.WorkspaceID == nil:
			seenGlobal = true
		case entry.Claim.WorkspaceID != nil && *entry.Claim.WorkspaceID == workspace:
			seenWorkspace = true
		default:
			t.Fatalf("claim fora do workspace atual: %+v", entry.Claim.WorkspaceID)
		}
	}
	if !seenGlobal || !seenWorkspace {
		t.Fatalf("projeção não preservou global+workspace: global=%v workspace=%v", seenGlobal, seenWorkspace)
	}

	foreign := owner
	foreignWorkspace := "workspace-foreign"
	foreign.WorkspaceID = &foreignWorkspace
	foreignSnapshot, err := c.Projection(context.Background(), foreign)
	if err != nil {
		t.Fatal(err)
	}
	if len(foreignSnapshot.Claims) != 1 || foreignSnapshot.Claims[0].Claim.WorkspaceID != nil {
		t.Fatalf("workspace estrangeiro recebeu claim indevida: %+v", foreignSnapshot.Claims)
	}
}

func addProjectionWorkspaceRule(t *testing.T, c *Consumer, base commandactivation.Rule, user, workspace string) {
	t.Helper()
	rule := base
	rule.ID, _ = freshID()
	rule.RuleRef = rule.ID
	rule.LayerRef, _ = freshID()
	rule.WorkspaceID = &workspace
	grantID, _ := freshID()
	rule.AutomationGrantID = &grantID
	if err := c.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	keys := func(context.Context, string) ([]byte, error) { return make([]byte, 32), nil }
	owner := commandautomation.Owner{UserID: user, WorkspaceID: &workspace}
	key := commandautomation.NaturalKey{Owner: owner, LayerRef: commandautomation.RuleRef{Kind: string(rule.LayerRefKind), Ref: rule.LayerRef}, RuleRef: commandautomation.RuleRef{Kind: string(rule.RuleRefKind), Ref: rule.RuleRef}}
	automationRule := commandautomation.Rule{ID: rule.ID, Owner: owner, LayerRef: key.LayerRef, RuleRef: key.RuleRef, Mode: string(rule.Mode), Condition: rule.Condition, Lifecycle: string(rule.Lifecycle), EventName: *rule.EventName, AllowedInternalProducerTypes: []string{"jobs.runtime"}, Source: rule.Source, ReviewStatus: rule.ReviewStatus}
	ruleFingerprint, err := commandautomation.FingerprintRule(context.Background(), automationRule, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	producerFingerprint, err := commandautomation.ProducerTypesFingerprint(context.Background(), "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, err := commandautomation.FingerprintGrant(context.Background(), key, ruleFingerprint, producerFingerprint, 1, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	rule.AutomationGrantFingerprint = &grantFingerprint
	if err := c.db.Model(&commandactivation.Rule{}).Where("id = ?", rule.ID).Update("automation_grant_fingerprint", grantFingerprint).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Table("command_layer_automation_grants").Create(map[string]any{
		"id": grantID, "user_id": user, "workspace_id": workspace,
		"layer_ref_kind": rule.LayerRefKind, "layer_ref": rule.LayerRef,
		"rule_ref_kind": rule.RuleRefKind, "rule_ref": rule.RuleRef,
		"rule_fingerprint": ruleFingerprint, "event_name": *rule.EventName,
		"producer_types_fingerprint": producerFingerprint, "automation_grant_generation": 1,
		"automation_grant_fingerprint": grantFingerprint, "authorization_decision_id": *rule.AuthorizationDecisionID,
		"granted_at": time.Now().UTC(), "granted_by": user,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestProjectionOmitsMissingStaleAndForeignClaims(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Consumer, *commandjobevents.Store, commandjobevents.Fact) commandactivation.Owner
	}{
		{
			name: "fonte ausente",
			setup: func(c *Consumer, out *commandjobevents.Store, fact commandjobevents.Fact) commandactivation.Owner {
				_ = c.db.Where("source_event_id = ?", fact.SourceEventID).Delete(&commandjobevents.ActivationOutbox{})
				return projectionOwner(fact)
			},
		},
		{
			name: "runtime stale",
			setup: func(c *Consumer, _ *commandjobevents.Store, fact commandjobevents.Fact) commandactivation.Owner {
				c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
					return RuntimeIdentity{Generation: "runtime-old", UserID: fact.UserID, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
				}
				return projectionOwner(fact)
			},
		},
		{
			name: "owner foreign",
			setup: func(_ *Consumer, _ *commandjobevents.Store, fact commandjobevents.Fact) commandactivation.Owner {
				owner := projectionOwner(fact)
				owner.AuthContextID = "other-session"
				return owner
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			if got := deliver(t, c, out, fact); got.Applied != 1 {
				t.Fatalf("evento inicial não aplicado: %+v", got)
			}
			owner := test.setup(c, out, fact)
			snapshot, err := c.Projection(context.Background(), owner)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Claims) != 0 {
				t.Fatalf("claim não autorizada apareceu: %+v", snapshot.Claims)
			}
		})
	}
}

func TestProjectionPropagatesInfrastructureFailureAndRevisionIsConservative(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	failure := errors.New("runtime database unavailable")
	c.ports.Condition = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error) {
		return false, failure
	}
	owner := projectionOwner(fact)
	if _, err := c.Projection(context.Background(), owner); !errors.Is(err, failure) {
		t.Fatalf("erro de infraestrutura=%v, esperado %v", err, failure)
	}
	before := c.ProjectionRevision()
	if err := c.RenewRuntime(context.Background(), "missing-activation"); err == nil {
		t.Fatal("renew de activation ausente deveria falhar")
	}
	if c.ProjectionRevision() <= before {
		t.Fatalf("revisão não avançou conservadoramente: antes=%d depois=%d", before, c.ProjectionRevision())
	}
}

func TestProjectionFailsClosedAboveClaimCap(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	var baseClaim commandactivation.Claim
	if err := c.db.Where("source_correlation_id = ?", fact.RunID).Take(&baseClaim).Error; err != nil {
		t.Fatal(err)
	}
	var baseLease Lease
	if err := c.db.Where("run_id = ?", fact.RunID).Take(&baseLease).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < projectionMaxClaims; i++ {
			activationID, err := freshID()
			if err != nil {
				return err
			}
			claim := cloneProjectionClaim(baseClaim)
			claim.ActivationID = activationID
			if err := tx.Create(&claim).Error; err != nil {
				return err
			}
			leaseID, err := freshID()
			if err != nil {
				return err
			}
			lease := baseLease
			lease.ID = leaseID
			lease.ActivationID = activationID
			if err := tx.Create(&lease).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Projection(context.Background(), projectionOwner(fact)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cap+1 leases resultou em %v, esperado ErrUnavailable", err)
	}
}

func TestProjectionRejectsNilCanceledAndExhaustedRevision(t *testing.T) {
	c, _, fact, _, _ := fixture(t)
	owner := projectionOwner(fact)
	if _, err := c.Projection(nil, owner); !errors.Is(err, ErrUnavailable) { //nolint:staticcheck // Verifica a rejeição explícita de contexto nil.
		t.Fatalf("contexto nil resultou em %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Projection(canceled, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado resultou em %v", err)
	}

	c.revision.Store(maxProjectionRevision)
	if c.ProjectionRevision() != maxProjectionRevision {
		t.Fatal("revisão máxima não foi observada")
	}
	if err := c.advanceRevision(); !errors.Is(err, ErrUnavailable) || c.ProjectionRevision() != maxProjectionRevision {
		t.Fatalf("advanceRevision permitiu wrap: err=%v revision=%d", err, c.ProjectionRevision())
	}
	if _, err := c.Projection(context.Background(), owner); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Projection em revisão exaurida resultou em %v", err)
	}
	if err := c.RenewRuntime(context.Background(), "missing-activation"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mutação em revisão exaurida resultou em %v", err)
	}
}

func TestReconcileBatchEmptyDoesNotAdvanceRevision(t *testing.T) {
	c, _, _, _, _ := fixture(t)
	before := c.ProjectionRevision()

	cursor, done, err := c.ReconcileBatch(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("reconciliação vazia falhou: %v", err)
	}
	if cursor != "" || !done {
		t.Fatalf("reconciliação vazia=(%q,%v), esperado cursor vazio e done", cursor, done)
	}
	if got := c.ProjectionRevision(); got != before {
		t.Fatalf("revisão avançou em lote vazio: antes=%d depois=%d", before, got)
	}
}

func TestReconcileBatchNonemptyFailureAdvancesRevision(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	before := c.ProjectionRevision()
	want := errors.New("layer infrastructure failure")
	c.ports.Layer = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error) {
		return false, want
	}

	cursor, done, err := c.ReconcileBatch(context.Background(), "", 1)
	if !errors.Is(err, want) {
		t.Fatalf("erro de reconciliação=%v, esperado %v", err, want)
	}
	if cursor != "" || done {
		t.Fatalf("reconciliação=(%q,%v), esperado falha sem avanço de cursor", cursor, done)
	}
	if got := c.ProjectionRevision(); got <= before {
		t.Fatalf("revisão não avançou para lote não vazio com falha: antes=%d depois=%d", before, got)
	}
}

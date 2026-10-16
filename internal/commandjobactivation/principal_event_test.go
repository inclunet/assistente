package commandjobactivation

import (
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Consumer, *commandjobevents.Store, commandjobevents.Fact, commandactivation.Rule, *time.Time) {
	t.Helper()
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "events.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, m := range []func(context.Context, *gorm.DB) error{commandconfig.Migrate, commandactivation.Migrate, commandautomation.Migrate, Migrate} {
		if err := m(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"CREATE TABLE jobs(id TEXT, user_id TEXT, slug TEXT)", "CREATE TABLE job_runs(id TEXT, user_id TEXT, job_id TEXT)"} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	id := func() string {
		s, e := freshID()
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	user, job, ruleID, layerID, decision, grantID := id(), id(), id(), id(), id(), id()
	config, e := commandconfig.New(db)
	if e != nil {
		t.Fatal(e)
	}
	if e = config.EnsureScope(ctx, commandconfig.Scope{UserID: user}); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	out := commandjobevents.NewStore(db)
	if _, err := out.EnsureReplayPolicyEpoch(ctx, commandjobevents.ProducerType, now.Add(-time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	event, producer, fp := "command-context.job-run-state.v1", `["jobs.runtime"]`, ""
	gen := int64(1)
	r := commandactivation.Rule{ID: ruleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: `{"version":1,"all":[]}`, Lifecycle: commandactivation.LifecyclePersistent, EventName: &event, AllowedInternalProducerTypes: &producer, Enabled: true, Source: "user", ReviewStatus: "active", AuthorizationDecisionID: &decision, AutomationGrantID: &grantID, AutomationGrantGeneration: &gen, AutomationGrantFingerprint: &fp}
	keys := func(context.Context, string) ([]byte, error) { return make([]byte, 32), nil }
	o := commandautomation.Owner{UserID: user}
	key := commandautomation.NaturalKey{Owner: o, LayerRef: commandautomation.RuleRef{Kind: "user", Ref: layerID}, RuleRef: commandautomation.RuleRef{Kind: "user", Ref: ruleID}}
	rule := commandautomation.Rule{ID: ruleID, Owner: o, LayerRef: key.LayerRef, RuleRef: key.RuleRef, Mode: "event", Condition: r.Condition, Lifecycle: "persistent", EventName: event, AllowedInternalProducerTypes: []string{"jobs.runtime"}, Source: "user", ReviewStatus: "active"}
	rfp, e := commandautomation.FingerprintRule(ctx, rule, "v1", keys)
	if e != nil {
		t.Fatal(e)
	}
	pfp, e := commandautomation.ProducerTypesFingerprint(ctx, "v1", keys)
	if e != nil {
		t.Fatal(e)
	}
	fp, e = commandautomation.FingerprintGrant(ctx, key, rfp, pfp, 1, "v1", keys)
	if e != nil {
		t.Fatal(e)
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	grant := map[string]any{"id": grantID, "user_id": user, "workspace_id": nil, "layer_ref_kind": "user", "layer_ref": layerID, "rule_ref_kind": "user", "rule_ref": ruleID, "rule_fingerprint": rfp, "event_name": event, "producer_types_fingerprint": pfp, "automation_grant_generation": 1, "automation_grant_fingerprint": fp, "authorization_decision_id": decision, "granted_at": now, "granted_by": user}
	if err := db.Table("command_layer_automation_grants").Create(grant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO jobs VALUES (?, ?, 'job-slug')", job, user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO job_runs VALUES ('opaque:run', ?, ?)", user, job).Error; err != nil {
		t.Fatal(err)
	}
	f := commandjobevents.Fact{SchemaVersion: event, EventName: event, SourceEventID: id(), UserID: user, JobDatabaseID: job, JobSlug: "job-slug", RunID: "opaque:run", Sequence: 1, State: "queued", OccurredAt: now, RootOriginType: "manual", RootOriginID: "root", Provenance: map[string]any{"_source": "job", "_source_job_id": "job-slug", "_chain_id": "chain", "_chain_history": []any{}}}
	f.RunEventID = f.SourceEventID
	ports := Ports{Authorize: func(_ context.Context, _ *gorm.DB, fact commandjobevents.Fact, w *string) (commandactivation.Owner, error) {
		return commandactivation.Owner{Scope: commandactivation.Scope{UserID: fact.UserID, WorkspaceID: clone(w)}, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
	}, Layer: func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error) {
		return true, nil
	}, Condition: func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error) {
		return true, nil
	}, Runtime: func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
		return RuntimeIdentity{Generation: "runtime-1", UserID: user, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
	}, Keys: keys, KeyVersion: "v1"}
	c, err := New(db, &commandsecurity.DispatchGate{}, ports, 3*time.Minute, 30*24*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return c, out, f, r, &now
}

func deliver(t *testing.T, c *Consumer, out *commandjobevents.Store, f commandjobevents.Fact) Result {
	t.Helper()
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, f) }); err != nil {
		t.Fatal(err)
	}
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", f.SourceEventID).Updates(map[string]any{"delivery_state": "processing", "lease_owner": "consumer", "lease_expires_at": c.now().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	r, err := c.Consume(context.Background(), f.SourceEventID, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestConsumeRejectsRuntimeFromPreviousSession(t *testing.T) {
	c, out, f, _, _ := fixture(t)
	current := c.ports.Runtime
	c.ports.Runtime = func(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (RuntimeIdentity, error) {
		runtime, err := current(ctx, tx, fact)
		runtime.AuthContextID = "previous-session"
		return runtime, err
	}
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, f) }); err != nil {
		t.Fatal(err)
	}
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", f.SourceEventID).Updates(map[string]any{"delivery_state": "processing", "lease_owner": "consumer", "lease_expires_at": c.now().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Consume(context.Background(), f.SourceEventID, "consumer"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("runtime de outra sessão = %v", err)
	}
	var count int64
	if err := c.db.Model(&commandactivation.Claim{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("claim persistida: %d, %v", count, err)
	}
	row, err := out.Get(context.Background(), f.SourceEventID)
	if err != nil || row.DeliveryState != commandjobevents.DeliveryProcessing {
		t.Fatalf("ack indevido: %+v, %v", row, err)
	}
}

func TestConsumeCASReplayTerminalAndFingerprint(t *testing.T) {
	c, out, f, r, _ := fixture(t)
	if got := deliver(t, c, out, f); got.Applied != 1 {
		t.Fatalf("%+v", got)
	}
	if got := deliver(t, c, out, f); got.Replayed != 1 {
		t.Fatalf("%+v", got)
	}
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.State = "started"
	if got := deliver(t, c, out, f); got.Conflicts != 1 {
		t.Fatalf("same sequence: %+v", got)
	}
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.Sequence = 3
	f.State = "completed"
	if got := deliver(t, c, out, f); got.Applied != 1 {
		t.Fatalf("%+v", got)
	}
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.Sequence = 4
	f.State = "started"
	if got := deliver(t, c, out, f); got.Ignored != 1 {
		t.Fatalf("resurrected: %+v", got)
	}
	var claim commandactivation.Claim
	if err := c.db.Where("rule_ref = ?", r.RuleRef).Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateDeactivated || *claim.Sequence != 3 {
		t.Fatalf("%+v", claim)
	}
}

func TestConsumeTamperedGrantDisablesAndDoesNotActivate(t *testing.T) {
	c, out, f, r, _ := fixture(t)
	if err := c.db.Model(&commandactivation.Rule{}).Where("id = ?", r.ID).Update("condition", `{"version":1,"all":[{"bad":true}]}`).Error; err != nil {
		t.Fatal(err)
	}
	if got := deliver(t, c, out, f); got.Ignored != 1 {
		t.Fatalf("%+v", got)
	}
	var count int64
	c.db.Model(&commandactivation.Claim{}).Count(&count)
	if count != 0 {
		t.Fatal("stale grant activated")
	}
	var rule commandactivation.Rule
	c.db.First(&rule, "id = ?", r.ID)
	if rule.Enabled {
		t.Fatal("rule enabled")
	}
}

func TestConsumeRollsBackAllRulesAndAckOnAuthFailure(t *testing.T) {
	c, out, f, _, _ := fixture(t)
	c.ports.Authorize = func(context.Context, *gorm.DB, commandjobevents.Fact, *string) (commandactivation.Owner, error) {
		return commandactivation.Owner{}, ErrUnavailable
	}
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, f) }); err != nil {
		t.Fatal(err)
	}
	c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", f.SourceEventID).Updates(map[string]any{"delivery_state": "processing", "lease_owner": "worker", "lease_expires_at": c.now().Add(time.Minute)})
	if _, err := c.Consume(context.Background(), f.SourceEventID, "worker"); err == nil {
		t.Fatal("auth ignored")
	}
	row, err := out.Get(context.Background(), f.SourceEventID)
	if err != nil || row.DeliveryState != "processing" {
		t.Fatalf("partial ack: %+v %v", row, err)
	}
}

func TestLeaseHeartbeatAndExpiryCannotResurrect(t *testing.T) {
	c, out, f, _, now := fixture(t)
	deliver(t, c, out, f)
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err != nil {
		t.Fatal(err)
	}
	var lease Lease
	if err := c.db.Take(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if !lease.ExpiresAt.Equal(now.Add(3 * time.Minute)) {
		t.Fatal("heartbeat deadline")
	}
	*now = now.Add(4 * time.Minute)
	if err := c.RenewRuntime(context.Background(), claim.ActivationID); err == nil {
		t.Fatal("expired lease renewed")
	}
	cursor, done, err := c.ReconcileBatch(context.Background(), "", 10)
	if err != nil || !done || cursor != claim.ActivationID {
		t.Fatalf("%s %v %v", cursor, done, err)
	}
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateInactive {
		t.Fatal("expired active")
	}
	if _, _, err := c.ReconcileBatch(context.Background(), "", 10); err != nil {
		t.Fatal(err)
	}
	var count int64
	c.db.Model(&eventLedger{}).Count(&count)
	if count != 1 {
		t.Fatal("ledger removed")
	}
}

func TestTerminalClosesExistingCycleEvenWhenConditionChanged(t *testing.T) {
	c, out, f, _, _ := fixture(t)
	deliver(t, c, out, f)
	c.ports.Condition = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error) {
		return false, nil
	}
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.Sequence = 2
	f.State = "completed"
	if r := deliver(t, c, out, f); r.Applied != 1 {
		t.Fatalf("%+v", r)
	}
	var claim commandactivation.Claim
	c.db.Take(&claim)
	if claim.State != commandactivation.StateDeactivated {
		t.Fatal("terminal ignored")
	}
}

func TestSourceLostAndRuntimeChangedMakeClaimInactive(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime", true: "source"}[lost], func(t *testing.T) {
			c, out, f, _, _ := fixture(t)
			deliver(t, c, out, f)
			if lost {
				if err := c.db.Exec("DELETE FROM job_runs").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
					return RuntimeIdentity{Generation: "runtime-next"}, nil
				}
			}
			if _, _, err := c.ReconcileBatch(context.Background(), "", 10); err != nil {
				t.Fatal(err)
			}
			var claim commandactivation.Claim
			c.db.Take(&claim)
			if claim.State != commandactivation.StateInactive {
				t.Fatal("source absent active")
			}
			var count int64
			c.db.Model(&commandjobevents.ActivationOutbox{}).Count(&count)
			if count != 1 {
				t.Fatal("outbox removed")
			}
		})
	}
}

func TestTerminalAuditCompactionDoesNotReopenCycle(t *testing.T) {
	c, out, f, r, _ := fixture(t)
	deliver(t, c, out, f)
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.Sequence = 2
	f.State = "completed"
	deliver(t, c, out, f)
	if err := c.db.Where("rule_ref = ?", r.RuleRef).Delete(&commandactivation.Claim{}).Error; err != nil {
		t.Fatal(err)
	}
	f.SourceEventID, _ = freshID()
	f.RunEventID = f.SourceEventID
	f.Sequence = 3
	f.State = "started"
	if got := deliver(t, c, out, f); got.Ignored != 1 {
		t.Fatalf("reopened compacted cycle: %+v", got)
	}
	var count int64
	c.db.Model(&commandactivation.Claim{}).Count(&count)
	if count != 0 {
		t.Fatal("new claim after terminal compacted")
	}
}

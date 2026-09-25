package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCleanRunsExceedingCountPreservesActivationSourcesAndClaims(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	userCtx := database.WithUserID(context.Background(), userID.String())
	job := testRepositoryJob("retention-source-job", "Retention source")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	if err := commandactivation.Migrate(context.Background(), repo.db); err != nil {
		t.Fatalf("migrate activation schema: %v", err)
	}
	if err := commandjobactivation.Migrate(context.Background(), repo.db); err != nil {
		t.Fatalf("migrate lease schema: %v", err)
	}

	clock := time.Now().UTC().Truncate(time.Microsecond)
	store := commandjobevents.NewStore(repo.db)
	if _, err := store.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, clock.Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay epoch: %v", err)
	}
	oldRun := "retention-old-run"
	pendingRun := "retention-pending-run"
	newRun := "retention-new-run"
	for i, runID := range []string{oldRun, pendingRun, newRun} {
		if err := repo.LogRun(userCtx, &RunLog{
			RunID: runID, JobID: job.ID, Status: RunStatusCompleted,
			Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: clock.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("log run %s: %v", runID, err)
		}
	}
	factID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	fact := commandjobevents.Fact{
		SchemaVersion: commandjobevents.SchemaVersion, EventName: commandjobevents.SchemaVersion,
		SourceEventID: factID.String(), RunEventID: factID.String(), UserID: userID.String(),
		JobDatabaseID: job.DatabaseID, JobSlug: job.ID, RunID: oldRun, Sequence: 1,
		State: commandjobevents.StateCompleted, OccurredAt: clock,
		RootOriginType: "manual", RootOriginID: factID.String(),
	}
	if err := repo.db.Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, fact) }); err != nil {
		t.Fatalf("insert source fact: %v", err)
	}
	pendingID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	pendingFact := fact
	pendingFact.SourceEventID, pendingFact.RunEventID = pendingID.String(), pendingID.String()
	pendingFact.RunID = pendingRun
	pendingFact.OccurredAt = clock.Add(time.Minute)
	pendingFact.RootOriginID = pendingID.String()
	if err := repo.db.Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, pendingFact) }); err != nil {
		t.Fatalf("insert pending source fact: %v", err)
	}
	activation, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	activationID := activation.String()
	expires := clock.Add(time.Hour)
	claim := commandactivation.Claim{
		ActivationID: activationID, LayerRefKind: commandactivation.BuiltinRef, LayerRef: "global",
		RuleRefKind: commandactivation.BuiltinRef, RuleRef: "retention", UserID: userID.String(),
		AuthContextType: "test", AuthContextID: "test", AuthGeneration: "1", SecurityGeneration: "1",
		SourceType: "job", SourceEventID: &fact.SourceEventID, SourceCorrelationID: &oldRun,
		State: commandactivation.StateActive, ActivatedAt: clock, ExpiresAt: &expires, UpdatedAt: clock,
	}
	if err := repo.db.Create(&claim).Error; err != nil {
		t.Fatalf("insert claim: %v", err)
	}
	leaseID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	lease := commandjobactivation.Lease{ID: leaseID.String(), ActivationID: activationID, UserID: userID.String(), RunID: oldRun, RuntimeGeneration: "runtime:1", ExpiresAt: expires, UpdatedAt: clock}
	if err := repo.db.Create(&lease).Error; err != nil {
		t.Fatalf("insert lease: %v", err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).
		Updates(map[string]any{"delivery_state": commandjobevents.DeliveryProcessing, "lease_owner": "retention-test", "lease_expires_at": expires}).Error; err != nil {
		t.Fatalf("mark source processing: %v", err)
	}

	deleted, err := repo.CleanRunsExceedingCount(userCtx, 1)
	if err != nil {
		t.Fatalf("count cap: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("count cap removeu runs ainda referenciados por outbox pendente/em processamento: deleted=%d", deleted)
	}
	for _, runID := range []string{oldRun, pendingRun, newRun} {
		var remaining database.JobRun
		if err := repo.db.Where("id = ?", runID).First(&remaining).Error; err != nil {
			t.Fatalf("run %s was not retained: %v", runID, err)
		}
	}
	row, err := store.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatalf("outbox source was removed with run: %v", err)
	}
	if row.RunID != oldRun || row.DeliveryState != commandjobevents.DeliveryProcessing || row.LeaseOwner == nil {
		t.Fatalf("outbox source changed: %+v", row)
	}
	pendingRow, err := store.Get(context.Background(), pendingFact.SourceEventID)
	if err != nil {
		t.Fatalf("pending outbox source was removed with run: %v", err)
	}
	if pendingRow.RunID != pendingRun || pendingRow.DeliveryState != commandjobevents.DeliveryPending || pendingRow.LeaseOwner != nil {
		t.Fatalf("pending outbox source changed: %+v", pendingRow)
	}
	var claimCount, leaseCount int64
	if err := repo.db.Table("command_layer_activation_state").Where("source_event_id = ?", fact.SourceEventID).Count(&claimCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Table("command_job_activation_leases").Where("run_id = ?", oldRun).Count(&leaseCount).Error; err != nil {
		t.Fatal(err)
	}
	if claimCount != 1 || leaseCount != 1 {
		t.Fatalf("source claim/lease lost: claim=%d lease=%d", claimCount, leaseCount)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).
		Where("source_event_id = ? AND user_id = ? AND run_id = ?", fact.SourceEventID, userID.String(), oldRun).
		Update("delivery_state", commandjobevents.DeliveryDelivered).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).
		Where("source_event_id = ? AND user_id = ? AND run_id = ?", pendingFact.SourceEventID, userID.String(), pendingRun).
		Update("delivery_state", commandjobevents.DeliveryDeadLetter).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err = repo.CleanRunsExceedingCount(userCtx, 1)
	if err != nil || deleted != 2 {
		t.Fatalf("count-cap não voltou a limpar após estados terminais da outbox: deleted=%d err=%v", deleted, err)
	}
	for _, runID := range []string{oldRun, pendingRun} {
		var remaining database.JobRun
		if err := repo.db.Where("user_id = ? AND id = ?", userID.String(), runID).First(&remaining).Error; err == nil {
			t.Fatalf("run terminal %s continuou retido depois de delivered/dead_letter", runID)
		}
	}
	for _, expected := range []struct{ id, runID, state string }{
		{fact.SourceEventID, oldRun, commandjobevents.DeliveryDelivered},
		{pendingFact.SourceEventID, pendingRun, commandjobevents.DeliveryDeadLetter},
	} {
		row, err := store.Get(context.Background(), expected.id)
		if err != nil || row.UserID != userID.String() || row.RunID != expected.runID || row.DeliveryState != expected.state {
			t.Fatalf("outbox terminal removida ou owner/run trocados: row=%+v err=%v want=(%s,%s,%s)", row, err, userID, expected.runID, expected.state)
		}
	}
}

func TestCountRetentionKeepsPendingSourceUntilConsumerAppliesItOnce(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7()).String()
	userCtx := database.WithUserID(ctx, userID)
	job := testRepositoryJob("retention-consumer-source", "Retention consumer source")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	for _, migrate := range []func(context.Context, *gorm.DB) error{
		commandactivation.Migrate, commandautomation.Migrate, commandjobactivation.Migrate,
	} {
		if err := migrate(ctx, repo.db); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	repo.now = func() time.Time { return now }
	outbox := commandjobevents.NewStore(repo.db)
	if _, err := outbox.EnsureReplayPolicyEpoch(ctx, commandjobevents.ProducerType, now.Add(-time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	runID := uuid.Must(uuid.NewV7()).String()
	newestRunID := uuid.Must(uuid.NewV7()).String()
	if err := repo.LogRun(userCtx, &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusRunning, Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.LogRun(userCtx, &RunLog{RunID: newestRunID, JobID: job.ID, Status: RunStatusCompleted, Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: now}); err != nil {
		t.Fatal(err)
	}

	newID := func() string { return uuid.Must(uuid.NewV7()).String() }
	ruleID, layerID, grantID, decisionID := newID(), newID(), newID(), newID()
	eventName, producerTypes, generation := commandautomation.JobRunStateEvent, `["jobs.runtime"]`, int64(1)
	keys := func(context.Context, string) ([]byte, error) { return []byte("0123456789abcdef0123456789abcdef"), nil }
	activationRule := commandactivation.Rule{
		ID: ruleID, UserID: userID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent,
		Condition: `{"version":1,"all":[]}`, Lifecycle: commandactivation.LifecyclePersistent,
		EventName: &eventName, AllowedInternalProducerTypes: &producerTypes,
		AuthorizationDecisionID: &decisionID, AutomationGrantID: &grantID,
		AutomationGrantGeneration: &generation, Enabled: true, Source: "user", ReviewStatus: "active",
	}
	automationRule := commandautomation.Rule{
		ID: ruleID, Owner: commandautomation.Owner{UserID: userID},
		LayerRef: commandautomation.RuleRef{Kind: string(commandactivation.UserRef), Ref: layerID},
		RuleRef:  commandautomation.RuleRef{Kind: string(commandactivation.UserRef), Ref: ruleID},
		Mode:     string(commandactivation.ModeEvent), Condition: activationRule.Condition,
		Lifecycle: string(commandactivation.LifecyclePersistent), EventName: eventName,
		AllowedInternalProducerTypes: []string{commandautomation.JobsRuntime}, Enabled: true,
		Source: "user", ReviewStatus: "active",
	}
	ruleFingerprint, err := commandautomation.FingerprintRule(ctx, automationRule, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	producerFingerprint, err := commandautomation.ProducerTypesFingerprint(ctx, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	key := commandautomation.NaturalKey{Owner: commandautomation.Owner{UserID: userID}, LayerRef: automationRule.LayerRef, RuleRef: automationRule.RuleRef}
	grantFingerprint, err := commandautomation.FingerprintGrant(ctx, key, ruleFingerprint, producerFingerprint, generation, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	activationRule.AutomationGrantFingerprint = &grantFingerprint
	if err := repo.db.Create(&activationRule).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Table("command_layer_automation_grants").Create(map[string]any{
		"id": grantID, "user_id": userID, "layer_ref_kind": string(commandactivation.UserRef), "layer_ref": layerID,
		"rule_ref_kind": string(commandactivation.UserRef), "rule_ref": ruleID, "rule_fingerprint": ruleFingerprint,
		"event_name": eventName, "producer_types_fingerprint": producerFingerprint,
		"automation_grant_generation": generation, "automation_grant_fingerprint": grantFingerprint,
		"authorization_decision_id": decisionID, "granted_at": now, "granted_by": userID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	fact := commandjobevents.Fact{
		SchemaVersion: commandjobevents.SchemaVersion, EventName: commandjobevents.SchemaVersion,
		SourceEventID: newID(), RunEventID: "", UserID: userID, JobDatabaseID: job.DatabaseID,
		JobSlug: job.ID, RunID: runID, Sequence: 1, State: commandjobevents.StateQueued,
		OccurredAt: now, RootOriginType: "manual", RootOriginID: newID(),
		Provenance: map[string]any{"_source": "job", "_source_job_id": job.ID, "_chain_id": "", "_chain_history": []string{}},
	}
	fact.RunEventID = fact.SourceEventID
	if err := repo.db.Transaction(func(tx *gorm.DB) error { return outbox.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	ports := commandjobactivation.Ports{
		Authorize: func(_ context.Context, _ *gorm.DB, f commandjobevents.Fact, workspaceID *string) (commandactivation.Owner, error) {
			return commandactivation.Owner{Scope: commandactivation.Scope{UserID: f.UserID, WorkspaceID: workspaceID}, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
		},
		Layer: func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error) {
			return true, nil
		},
		Condition: func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error) {
			return true, nil
		},
		Runtime: func(_ context.Context, _ *gorm.DB, f commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
			return commandjobactivation.RuntimeIdentity{Generation: "runtime:1", UserID: f.UserID, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
		},
		Keys: keys, KeyVersion: "v1",
	}
	consumer, err := commandjobactivation.New(repo.db, &commandsecurity.DispatchGate{}, ports, 3*time.Minute, 24*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := repo.CleanRunsExceedingCount(userCtx, 1); err != nil || deleted != 0 {
		t.Fatalf("count-cap antes da entrega removeu fonte pendente: deleted=%d err=%v", deleted, err)
	}
	var kept database.JobRun
	if err := repo.db.Where("id = ?", runID).First(&kept).Error; err != nil {
		t.Fatalf("run referenciado não foi retido: %v", err)
	}
	pass, err := consumer.RunPass(ctx, "retention-worker", 10)
	if err != nil || pass.Applied != 1 || pass.DeadLettered != 0 {
		row, getErr := outbox.Get(ctx, fact.SourceEventID)
		t.Fatalf("consumer não aplicou fato após count-cap: pass=%+v err=%v outbox=%+v getErr=%v", pass, err, row, getErr)
	}
	var activeClaims int64
	if err := repo.db.Model(&commandactivation.Claim{}).Where("source_correlation_id = ? AND state = ?", runID, commandactivation.StateActive).Count(&activeClaims).Error; err != nil || activeClaims != 1 {
		t.Fatalf("efeito do fato pendente: active claims=%d err=%v, want 1", activeClaims, err)
	}
	pass, err = consumer.RunPass(ctx, "retention-replay-worker", 10)
	if err != nil || pass.Claimed != 0 {
		t.Fatalf("replay do fato entregue não foi no-op: pass=%+v err=%v", pass, err)
	}
	var claimCount, ledgerCount int64
	if err := repo.db.Model(&commandactivation.Claim{}).Where("source_correlation_id = ?", runID).Count(&claimCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Table("command_activation_idempotency_keys").Where("source_correlation_id = ?", runID).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if claimCount != 1 || ledgerCount != 1 {
		t.Fatalf("efeito duplicado após replay: claims=%d ledger=%d", claimCount, ledgerCount)
	}

	// A confirmação terminal encerra a lease. Com ambas as ocorrências fora de
	// pending/processing, o run antigo deve voltar a ser elegível ao count-cap.
	now = now.Add(time.Minute)
	if err := repo.db.Model(&database.JobRun{}).Where("user_id = ? AND id = ?", userID, runID).Update("status", RunStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	completed := fact
	completed.SourceEventID, completed.RunEventID = newID(), ""
	completed.Sequence, completed.State, completed.OccurredAt = 2, commandjobevents.StateCompleted, now
	completed.RunEventID = completed.SourceEventID
	if err := repo.db.Transaction(func(tx *gorm.DB) error { return outbox.InsertFactTx(tx, completed) }); err != nil {
		t.Fatal(err)
	}
	pass, err = consumer.RunPass(ctx, "retention-terminal-worker", 10)
	if err != nil || pass.Applied != 1 || pass.DeadLettered != 0 {
		t.Fatalf("consumer não aplicou evento terminal: pass=%+v err=%v", pass, err)
	}
	if deleted, err := repo.CleanRunsExceedingCount(userCtx, 1); err != nil || deleted != 1 {
		t.Fatalf("run não voltou à retenção após entrega terminal: deleted=%d err=%v", deleted, err)
	}
	var remaining database.JobRun
	if err := repo.db.Where("user_id = ? AND id = ?", userID, runID).First(&remaining).Error; err == nil {
		t.Fatal("run encerrado permaneceu retido após outbox terminal e lease encerrada")
	}
}

func TestRetentionProtectsLiveLeaseAndReclaimsAfterExpiry(t *testing.T) {
	for _, mode := range []string{"count-cap", "age"} {
		t.Run(mode, func(t *testing.T) {
			repo, _, _ := setupJobsRepositoryTest(t)
			userID, err := uuid.NewV7()
			if err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), userID.String())
			job := testRepositoryJob("retention-live-"+mode, "Retention live "+mode)
			if err := repo.SaveJob(ctx, job); err != nil {
				t.Fatalf("save job: %v", err)
			}
			if err := commandactivation.Migrate(context.Background(), repo.db); err != nil {
				t.Fatalf("migrate activation schema: %v", err)
			}
			if err := commandjobactivation.Migrate(context.Background(), repo.db); err != nil {
				t.Fatalf("migrate lease schema: %v", err)
			}
			clockUTC := time.Now().UTC().Truncate(time.Microsecond)
			clock := clockUTC.In(time.FixedZone("BRT", -3*60*60))
			repo.now = func() time.Time { return clock }
			liveRun, expiredRun, newestRun := "live-"+mode, "expired-"+mode, "newest-"+mode
			for _, run := range []struct {
				id     string
				status string
				at     time.Time
			}{
				{liveRun, RunStatusRunning, clock.Add(-3 * time.Hour)},
				{expiredRun, RunStatusRunning, clock.Add(-2 * time.Hour)},
				{newestRun, RunStatusCompleted, clock.Add(-time.Minute)},
			} {
				if err := repo.LogRun(ctx, &RunLog{RunID: run.id, JobID: job.ID, Status: run.status, Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: run.at}); err != nil {
					t.Fatalf("log run %s: %v", run.id, err)
				}
			}
			queuedRun := "queued-null-started-" + mode
			if mode == "age" {
				if err := repo.LogRun(ctx, &RunLog{
					RunID: queuedRun, JobID: job.ID, Status: RunStatusQueued,
					Trigger: TriggerInfo{Type: TriggerManual}, QueuedAt: clock.Add(-2 * time.Hour),
				}); err != nil {
					t.Fatalf("log queued run: %v", err)
				}
				if err := repo.db.Model(&database.JobRun{}).Where("id = ?", queuedRun).Update("started_at", nil).Error; err != nil {
					t.Fatalf("set queued started_at NULL: %v", err)
				}
				var queued database.JobRun
				if err := repo.db.Where("id = ?", queuedRun).First(&queued).Error; err != nil {
					t.Fatal(err)
				}
				if !queued.StartedAt.IsZero() || !queued.QueuedAt.Before(clock.Add(-time.Hour)) {
					t.Fatalf("queued fixture não preservou started_at NULL/queued_at antigo: %+v", queued)
				}
			}

			seedLease := func(runID string, expires time.Time) (string, string) {
				t.Helper()
				activationID, err := uuid.NewV7()
				if err != nil {
					t.Fatal(err)
				}
				eventID, err := uuid.NewV7()
				if err != nil {
					t.Fatal(err)
				}
				claim := commandactivation.Claim{
					ActivationID: activationID.String(), LayerRefKind: commandactivation.BuiltinRef, LayerRef: "global",
					RuleRefKind: commandactivation.BuiltinRef, RuleRef: "retention", UserID: userID.String(),
					AuthContextType: "test", AuthContextID: "test", AuthGeneration: "1", SecurityGeneration: "1",
					SourceType: "job", SourceEventID: stringPtr(eventID.String()), SourceCorrelationID: stringPtr(runID),
					State: commandactivation.StateActive, ActivatedAt: clock, ExpiresAt: timePtr(expires), UpdatedAt: clock,
				}
				if err := repo.db.Create(&claim).Error; err != nil {
					t.Fatalf("insert claim %s: %v", runID, err)
				}
				leaseID, err := uuid.NewV7()
				if err != nil {
					t.Fatal(err)
				}
				lease := commandjobactivation.Lease{ID: leaseID.String(), ActivationID: activationID.String(), UserID: userID.String(), RunID: runID, RuntimeGeneration: "runtime:1", ExpiresAt: expires, UpdatedAt: clock}
				if err := repo.db.Create(&lease).Error; err != nil {
					t.Fatalf("insert lease %s: %v", runID, err)
				}
				return activationID.String(), eventID.String()
			}
			liveActivation, _ := seedLease(liveRun, clock.Add(time.Hour))
			_, _ = seedLease(expiredRun, clock.Add(-time.Minute))

			if mode == "count-cap" {
				deleted, err := repo.CleanRunsExceedingCount(ctx, 1)
				if err != nil || deleted != 1 {
					t.Fatalf("first count retention deleted=%d err=%v, want expired only", deleted, err)
				}
				assertRunExists(t, repo, liveRun)
				assertRunExists(t, repo, newestRun)
				assertRunMissing(t, repo, expiredRun)
			} else {
				deleted, err := repo.CleanOldRuns(ctx, time.Hour)
				if err != nil || deleted != 2 {
					t.Fatalf("first age retention deleted=%d err=%v, want expired+queued", deleted, err)
				}
				assertRunExists(t, repo, liveRun)
				assertRunMissing(t, repo, expiredRun)
				assertRunMissing(t, repo, queuedRun)
			}

			if err := repo.db.Model(&commandjobactivation.Lease{}).Where("activation_id = ?", liveActivation).Updates(map[string]any{"expires_at": clock.Add(-time.Minute)}).Error; err != nil {
				t.Fatal(err)
			}
			if err := repo.db.Model(&commandactivation.Claim{}).Where("activation_id = ?", liveActivation).Updates(map[string]any{"expires_at": clock.Add(-time.Minute)}).Error; err != nil {
				t.Fatal(err)
			}
			var deleted int
			if mode == "count-cap" {
				deleted, err = repo.CleanRunsExceedingCount(ctx, 1)
			} else {
				deleted, err = repo.CleanOldRuns(ctx, time.Hour)
			}
			if err != nil || deleted != 1 {
				t.Fatalf("retention after lease expiry deleted=%d err=%v, want live run", deleted, err)
			}
			assertRunMissing(t, repo, liveRun)
		})
	}
}

func TestCountRetentionRanksInstantAcrossOffsetsAndKeepsRecentQueuedRun(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), userID.String())
	job := testRepositoryJob("retention-offset-ranking", "Retention offset ranking")
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	if err := commandactivation.Migrate(context.Background(), repo.db); err != nil {
		t.Fatalf("migrate activation schema: %v", err)
	}
	if err := commandjobactivation.Migrate(context.Background(), repo.db); err != nil {
		t.Fatalf("migrate lease schema: %v", err)
	}
	clockUTC := time.Date(2025, time.January, 10, 15, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return clockUTC }
	clockLocal := clockUTC.In(time.FixedZone("BRT", -3*60*60))
	if err := repo.LogRun(ctx, &RunLog{
		RunID: "offset-older", JobID: job.ID, Status: RunStatusCompleted,
		Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: clockUTC.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.LogRun(ctx, &RunLog{
		RunID: "offset-newer", JobID: job.ID, Status: RunStatusCompleted,
		Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: clockLocal.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	// 11:00-03:00 é posterior a 13:00Z, embora a ordem textual seja inversa.
	deleted, err := repo.CleanRunsExceedingCount(ctx, 1)
	if err != nil || deleted != 1 {
		t.Fatalf("offset ranking deleted=%d err=%v, want 1", deleted, err)
	}
	assertRunMissing(t, repo, "offset-older")
	assertRunExists(t, repo, "offset-newer")
	if err := repo.LogRun(ctx, &RunLog{
		RunID: "queued-recent", JobID: job.ID, Status: RunStatusQueued,
		Trigger: TriggerInfo{Type: TriggerManual}, QueuedAt: clockUTC.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&database.JobRun{}).Where("id = ?", "queued-recent").Update("started_at", nil).Error; err != nil {
		t.Fatalf("set queued started_at NULL: %v", err)
	}

	deleted, err = repo.CleanRunsExceedingCount(ctx, 1)
	if err != nil || deleted != 1 {
		t.Fatalf("count ranking deleted=%d err=%v, want 1", deleted, err)
	}
	assertRunMissing(t, repo, "offset-older")
	assertRunMissing(t, repo, "offset-newer")
	assertRunExists(t, repo, "queued-recent")
}

func stringPtr(value string) *string { return &value }

func timePtr(value time.Time) *time.Time { return &value }

func assertRunExists(t *testing.T, repo *DBRepository, runID string) {
	t.Helper()
	var row database.JobRun
	if err := repo.db.Where("id = ?", runID).First(&row).Error; err != nil {
		t.Fatalf("run %s missing: %v", runID, err)
	}
}

func assertRunMissing(t *testing.T, repo *DBRepository, runID string) {
	t.Helper()
	var row database.JobRun
	if err := repo.db.Where("id = ?", runID).First(&row).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("run %s expected missing, got %v", runID, err)
	}
}

func TestCommandProvenanceAcceptsSixteenAndRejectsSeventeenthEntry(t *testing.T) {
	validChain := make([]any, 0, 17)
	for i := 0; i < 17; i++ {
		invocationID, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		validChain = append(validChain, map[string]any{
			"command_id": fmt.Sprintf("retention.step%d", i), "invocation_id": invocationID.String(), "layer_refs": []string{"global:help"},
		})
	}
	executor := &JobExecutor{}
	job := &Job{ID: "chain-target"}
	for _, tc := range []struct {
		name  string
		count int
		want  bool
	}{
		{"sixteen", 16, true},
		{"seventeen", 17, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &TriggerContext{Provenance: map[string]any{"command_chain_history": validChain[:tc.count]}}
			_, err := executor.commandRunProvenance(job, trigger, &RunLog{RunID: "run"})
			if (err == nil) != tc.want {
				t.Fatalf("count=%d err=%v wantAccepted=%v", tc.count, err, tc.want)
			}
		})
	}
}

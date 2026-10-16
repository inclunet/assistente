package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestRetentionQueryPlans uses the real job and command-retention schemas in a
// disposable WAL database. Its dataset is deliberately large enough to expose
// whether the outer run scan repeatedly probes a broad user/delivery index.
func TestRetentionQueryPlans(t *testing.T) {
	db, repo := openRetentionPlanDB(t)
	userA := mustRetentionUUID(t)
	userB := mustRetentionUUID(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-48 * time.Hour)

	const runCount = 2400
	runs := make([]database.JobRun, 0, runCount)
	for i := 0; i < runCount; i++ {
		id := fmt.Sprintf("retention-plan-run-%04d", i)
		runs = append(runs, database.JobRun{
			UUIDModel: database.UUIDModel{ID: id, CreatedAt: old, UpdatedAt: old},
			UserID:    userA, JobID: "retention-plan-job", Status: RunStatusCompleted,
			QueuedAt: old, StartedAt: old,
		})
	}
	// The cross-user row intentionally references a run belonging to A. There is
	// no FK by design, and B's fact must not affect A's retention decision.
	const crossUserRunID = "retention-plan-run-0002"
	runs = append(runs, database.JobRun{
		UUIDModel: database.UUIDModel{ID: "retention-plan-cross-user-only", CreatedAt: old, UpdatedAt: old},
		UserID:    userA, JobID: "retention-plan-job", Status: RunStatusCompleted,
		QueuedAt: old, StartedAt: old,
	})
	const crossUserLeaseRunID = "retention-plan-cross-user-lease"
	const ownLeaseRunID = "retention-plan-own-lease"
	for _, runID := range []string{crossUserLeaseRunID, ownLeaseRunID} {
		runs = append(runs, database.JobRun{
			UUIDModel: database.UUIDModel{ID: runID, CreatedAt: old, UpdatedAt: old},
			UserID:    userA, JobID: "retention-plan-job", Status: RunStatusRunning,
			QueuedAt: old, StartedAt: old,
		})
	}
	if err := db.CreateInBatches(&runs, 250).Error; err != nil {
		t.Fatalf("insert job runs: %v", err)
	}

	const outboxPerUser = 3600
	outbox := make([]commandjobevents.ActivationOutbox, 0, outboxPerUser*2+3)
	for i := 0; i < outboxPerUser*2; i++ {
		owner := userA
		if i >= outboxPerUser {
			owner = userB
		}
		state := commandjobevents.DeliveryDelivered
		switch i % 8 {
		case 0:
			state = commandjobevents.DeliveryPending
		case 1:
			state = commandjobevents.DeliveryProcessing
		}
		runID := fmt.Sprintf("outbox-unrelated-run-%05d", i)
		if owner == userA && i < runCount {
			runID = runs[i].ID
		}
		outbox = append(outbox, retentionPlanOutbox(t, owner, runID, state, now))
	}
	// Own pending and processing facts protect their source runs; B's pending
	// fact referring to A's run must not protect it.
	outbox = append(outbox,
		retentionPlanOutbox(t, userA, runs[0].ID, commandjobevents.DeliveryPending, now),
		retentionPlanOutbox(t, userA, runs[1].ID, commandjobevents.DeliveryProcessing, now),
		retentionPlanOutbox(t, userB, crossUserRunID, commandjobevents.DeliveryPending, now),
	)
	if err := db.CreateInBatches(&outbox, 200).Error; err != nil {
		t.Fatalf("insert activation outbox: %v", err)
	}

	// A thousand leases plus active claims exercise the other correlated
	// retention predicate using the production lease and claim indexes.
	leases := make([]commandjobactivation.Lease, 0, 1000)
	for i := 0; i < 1000; i++ {
		activationID := mustRetentionUUID(t)
		expiresAt := now.Add(time.Hour)
		leases = append(leases, commandjobactivation.Lease{
			ID: mustRetentionUUID(t), ActivationID: activationID, UserID: userA,
			RunID: runs[i+100].ID, RuntimeGeneration: "retention-plan-generation",
			ExpiresAt: expiresAt, UpdatedAt: now,
		})
		if i < 200 {
			if err := insertRetentionPlanClaim(t, db, activationID, userA, runs[i+100].ID, now); err != nil {
				t.Fatalf("insert active claim %d: %v", i, err)
			}
		}
	}
	if err := db.CreateInBatches(&leases, 200).Error; err != nil {
		t.Fatalf("insert activation leases: %v", err)
	}
	for _, lease := range []struct {
		userID string
		runID  string
	}{{userB, crossUserLeaseRunID}, {userA, ownLeaseRunID}} {
		activationID := mustRetentionUUID(t)
		if err := insertRetentionPlanClaim(t, db, activationID, lease.userID, lease.runID, now); err != nil {
			t.Fatalf("insert lease-isolation claim for %s: %v", lease.runID, err)
		}
		if err := db.Create(&commandjobactivation.Lease{
			ID: mustRetentionUUID(t), ActivationID: activationID, UserID: lease.userID,
			RunID: lease.runID, RuntimeGeneration: "retention-plan-generation",
			ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("insert lease-isolation lease for %s: %v", lease.runID, err)
		}
	}
	if err := db.Exec("ANALYZE").Error; err != nil {
		t.Fatalf("analyze fixture: %v", err)
	}

	ctx := database.WithUserID(context.Background(), userA)
	for _, mode := range []string{"age", "count"} {
		oldSQL, oldArgs := retentionPlanSQL(db, repo, ctx, userA, now, mode, true, true)
		newSQL, newArgs := retentionPlanSQL(db, repo, ctx, userA, now, mode, false, true)
		oldPlan := explainRetentionPlan(t, db, oldSQL, oldArgs)
		newPlan := explainRetentionPlan(t, db, newSQL, newArgs)
		t.Logf("%s baseline correlated plan:\n%s", mode, strings.Join(oldPlan, "\n"))
		t.Logf("%s optimized plan:\n%s", mode, strings.Join(newPlan, "\n"))

		if !planContains(oldPlan, "CORRELATED") {
			t.Errorf("%s baseline no longer demonstrates a correlated subquery; plan: %v", mode, oldPlan)
		}
		if planContains(newPlan, "CORRELATED") {
			t.Errorf("%s optimized query regressed to a correlated subquery; plan: %v", mode, newPlan)
		}
		for _, table := range []string{"source", "lease"} {
			if hasTablePlanStep(newPlan, "SCAN", table) {
				t.Errorf("%s optimized query scans %s rather than using an index; plan: %v", mode, table, newPlan)
			}
		}

		oldDuration := runRetentionPlanDiagnostic(t, db, oldSQL, oldArgs)
		newDuration := runRetentionPlanDiagnostic(t, db, newSQL, newArgs)
		t.Logf("%s diagnostic execution duration (non-gating): baseline=%s optimized=%s", mode, oldDuration, newDuration)

		// Compare the complete semantic result, not only the first retention
		// batch. This keeps protection fixtures meaningful even when their IDs
		// sort beyond the production LIMIT.
		oldAllSQL, oldAllArgs := retentionPlanSQL(db, repo, ctx, userA, now, mode, true, false)
		newAllSQL, newAllArgs := retentionPlanSQL(db, repo, ctx, userA, now, mode, false, false)
		oldIDs := retentionPlanIDs(t, db, oldAllSQL, oldAllArgs)
		newIDs := retentionPlanIDs(t, db, newAllSQL, newAllArgs)
		if !equalRetentionPlanIDs(oldIDs, newIDs) {
			t.Errorf("%s old/new full selection differs: old=%d IDs, optimized=%d IDs; old-only=%v optimized-only=%v",
				mode, len(oldIDs), len(newIDs), retentionPlanDifference(oldIDs, newIDs), retentionPlanDifference(newIDs, oldIDs))
		}
	}

	// Verify the user boundary and delivery states against the complete
	// optimized age selection, independently of plan wording or batch LIMIT.
	ageSQL, ageArgs := retentionPlanSQL(db, repo, ctx, userA, now, "age", false, false)
	selected := retentionPlanIDs(t, db, ageSQL, ageArgs)
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}
	for _, protected := range []string{runs[0].ID, runs[1].ID, ownLeaseRunID} {
		if selectedSet[protected] {
			t.Errorf("run %s with its own pending/processing outbox fact or live claim was selected", protected)
		}
	}
	if !selectedSet[crossUserRunID] {
		t.Errorf("run %s was protected by another user's outbox fact", crossUserRunID)
	}
	if !selectedSet["retention-plan-cross-user-only"] {
		t.Error("unprotected old run belonging to user A was not selected")
	}
	if !selectedSet[crossUserLeaseRunID] {
		t.Errorf("run %s was protected by another user's live lease and claim", crossUserLeaseRunID)
	}

	// Each half is removed independently: the remaining half must not weaken
	// fail-closed behavior for non-terminal runs.
	assertNoRetainableNonTerminalRuns(t, db, repo, userA, now, "lease")
	if err := db.AutoMigrate(&commandjobactivation.Lease{}); err != nil {
		t.Fatalf("restore empty lease schema for second fail-closed check: %v", err)
	}
	assertNoRetainableNonTerminalRuns(t, db, repo, userA, now, "claim")
}

func assertNoRetainableNonTerminalRuns(t *testing.T, db *gorm.DB, repo *DBRepository, userID string, now time.Time, missing string) {
	t.Helper()
	switch missing {
	case "lease":
		if err := db.Migrator().DropTable(&commandjobactivation.Lease{}); err != nil {
			t.Fatalf("drop lease table for fail-closed check: %v", err)
		}
	case "claim":
		if err := db.Migrator().DropTable("command_layer_activation_state"); err != nil {
			t.Fatalf("drop claim table for fail-closed check: %v", err)
		}
	default:
		t.Fatalf("unknown missing retention schema: %q", missing)
	}
	var nonTerminalIDs []string
	failClosed := repo.retainableRunQuery(
		db.Model(&database.JobRun{}).Where("user_id = ?", userID), userID, now,
	).Where("status NOT IN ?", []string{RunStatusCompleted, RunStatusFailed, RunStatusSkipped})
	if err := failClosed.Pluck("id", &nonTerminalIDs).Error; err != nil {
		t.Fatalf("select fail-closed non-terminal runs missing %s schema: %v", missing, err)
	}
	if len(nonTerminalIDs) != 0 {
		t.Errorf("non-terminal runs became retainable without %s schema: %v", missing, nonTerminalIDs)
	}
}

// BenchmarkRetentionQueryPlansScale is opt-in: unlike the regular regression
// test it builds a large database and reports (without thresholds) how SQLite
// plans and executes the old/new selector with and without sqlite_stat1.
// Example: go test ./internal/jobs -run '^$' -bench '^BenchmarkRetentionQueryPlansScale$' -benchtime=1x -count=3
func BenchmarkRetentionQueryPlansScale(b *testing.B) {
	benchmarkRetentionQueryPlans(b, 1500)
}

// The large baseline can take tens of seconds without planner statistics.
// Select a single case with -bench 'BenchmarkRetentionQueryPlansLarge/without-sqlite-stat1/age'.
func BenchmarkRetentionQueryPlansLarge(b *testing.B) {
	benchmarkRetentionQueryPlans(b, 6000)
}

func benchmarkRetentionQueryPlans(b *testing.B, runCount int) {
	db, repo, userA, now := seedRetentionPlanScaleFixture(b, runCount)
	ctx := database.WithUserID(context.Background(), userA)

	for _, withStats := range []bool{false, true} {
		statsName := "without-sqlite-stat1"
		if withStats {
			statsName = "with-sqlite-stat1"
		}
		if withStats {
			if err := db.Exec("ANALYZE").Error; err != nil {
				b.Fatalf("ANALYZE scale fixture: %v", err)
			}
		} else if err := clearRetentionPlanStatistics(db); err != nil {
			b.Fatalf("clear scale fixture statistics: %v", err)
		}

		for _, mode := range []string{"age", "count"} {
			for _, correlated := range []bool{true, false} {
				queryName := "optimized"
				if correlated {
					queryName = "correlated-baseline"
				}
				sql, args := retentionPlanSQL(db, repo, ctx, userA, now, mode, correlated, true)
				plan := explainRetentionPlan(b, db, sql, args)
				b.Logf("runs=%d stats=%s mode=%s query=%s plan/indexes:\n%s",
					runCount, statsName, mode, queryName, strings.Join(plan, "\n"))
				b.Run(statsName+"/"+mode+"/"+queryName, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_ = runRetentionPlanDiagnostic(b, db, sql, args)
					}
				})
			}
		}
	}

	// Measure dependency/run deletion independently from selector cost. Each
	// iteration rolls back so the same 128-run batch and child rows are present;
	// one JobRunEvent and one JobEvent per run provide a stable cascade workload.
	selectedSQL, selectedArgs := retentionPlanSQL(db, repo, ctx, userA, now, "age", false, true)
	selected := retentionPlanIDs(b, db, selectedSQL, selectedArgs)
	if len(selected) < retentionBatchSize {
		b.Fatalf("scale fixture yielded only %d eligible runs for cascade batch", len(selected))
	}
	selected = selected[:retentionBatchSize]
	runEvents := make([]database.JobRunEvent, 0, len(selected))
	jobEvents := make([]database.JobEvent, 0, len(selected))
	for i, runID := range selected {
		occurredAt := now.Add(-48 * time.Hour)
		runEvents = append(runEvents, database.JobRunEvent{
			UUIDModel: database.UUIDModel{ID: fmt.Sprintf("scale-run-event-%03d", i), CreatedAt: occurredAt, UpdatedAt: occurredAt},
			UserID:    userA, JobRunID: runID, Sequence: 1, OccurredAt: occurredAt, Type: "completed",
		})
		runIDCopy := runID
		jobEvents = append(jobEvents, database.JobEvent{
			UUIDModel: database.UUIDModel{ID: fmt.Sprintf("scale-job-event-%03d", i), CreatedAt: occurredAt, UpdatedAt: occurredAt},
			UserID:    userA, JobRunID: &runIDCopy, OccurredAt: occurredAt, Type: "completed",
		})
	}
	if err := db.CreateInBatches(&runEvents, 128).Error; err != nil {
		b.Fatalf("seed scale run events: %v", err)
	}
	if err := db.CreateInBatches(&jobEvents, 128).Error; err != nil {
		b.Fatalf("seed scale job events: %v", err)
	}
	b.Run("delete-cascade/128-runs-2-events-each", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tx := db.Begin()
			if tx.Error != nil {
				b.Fatal(tx.Error)
			}
			if err := repo.deleteRunDependenciesByIDsTx(ctx, tx, selected, true); err != nil {
				_ = tx.Rollback().Error
				b.Fatal(err)
			}
			if _, err := repo.deleteRunsByIDsTx(ctx, tx, selected); err != nil {
				_ = tx.Rollback().Error
				b.Fatal(err)
			}
			if err := tx.Rollback().Error; err != nil {
				b.Fatal(err)
			}
		}
	})
}

func seedRetentionPlanScaleFixture(b *testing.B, runCount int) (*gorm.DB, *DBRepository, string, time.Time) {
	b.Helper()
	db, repo := openRetentionPlanDB(b)
	// Large fixture inserts can exceed GORM's slow-SQL threshold; keep benchmark
	// output to plans/results instead of dumping each multi-row INSERT statement.
	db.Logger = logger.Default.LogMode(logger.Silent)
	userA, userB := mustRetentionUUID(b), mustRetentionUUID(b)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	runs := make([]database.JobRun, runCount)
	runIDs := make([]string, runCount)
	for i := range runs {
		runIDs[i] = fmt.Sprintf("scale-run-%06d", i)
		runs[i] = database.JobRun{
			UUIDModel: database.UUIDModel{ID: runIDs[i], CreatedAt: old, UpdatedAt: old},
			UserID:    userA, JobID: "retention-plan-scale-job", Status: RunStatusCompleted,
			QueuedAt: old, StartedAt: old,
		}
	}
	if err := db.CreateInBatches(&runs, 500).Error; err != nil {
		b.Fatalf("seed %d scale runs: %v", runCount, err)
	}

	outboxPerUser := runCount * 3 / 2
	outbox := make([]commandjobevents.ActivationOutbox, 0, outboxPerUser*2)
	for _, owner := range []string{userA, userB} {
		for i := 0; i < outboxPerUser; i++ {
			state := commandjobevents.DeliveryDelivered
			switch i % 8 {
			case 0:
				state = commandjobevents.DeliveryPending
			case 1:
				state = commandjobevents.DeliveryProcessing
			}
			runID := fmt.Sprintf("scale-unrelated-%s-%06d", owner, i)
			if owner == userA && i < runCount {
				runID = runIDs[i]
			}
			outbox = append(outbox, retentionPlanOutbox(b, owner, runID, state, now))
		}
	}
	if err := db.CreateInBatches(&outbox, 500).Error; err != nil {
		b.Fatalf("seed %d scale outbox records: %v", len(outbox), err)
	}

	leaseCount := runCount * 5 / 12
	claimCount := leaseCount / 5 // A representative 20% with active claims.
	leases := make([]commandjobactivation.Lease, leaseCount)
	claimActivations := make([]string, claimCount)
	for i := range leases {
		activationID := mustRetentionUUID(b)
		leases[i] = commandjobactivation.Lease{
			ID: mustRetentionUUID(b), ActivationID: activationID, UserID: userA,
			RunID: runIDs[i], RuntimeGeneration: "retention-plan-scale-generation",
			ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
		}
		if i < claimCount {
			claimActivations[i] = activationID
		}
	}
	if err := db.CreateInBatches(&leases, 500).Error; err != nil {
		b.Fatalf("seed %d scale leases: %v", len(leases), err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < claimCount; i++ {
			if err := insertRetentionPlanClaim(b, tx, claimActivations[i], userA, runIDs[i], now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		b.Fatalf("seed %d scale active claims: %v", claimCount, err)
	}
	return db, repo, userA, now
}

func clearRetentionPlanStatistics(db *gorm.DB) error {
	var statTableCount int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_stat1'").Scan(&statTableCount).Error; err != nil {
		return err
	}
	if statTableCount > 0 {
		if err := db.Exec("DELETE FROM sqlite_stat1").Error; err != nil {
			return err
		}
		return db.Exec("ANALYZE sqlite_schema").Error
	}
	return nil
}

func openRetentionPlanDB(t testing.TB) (*gorm.DB, *DBRepository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retention-plan.db")
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(OFF)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open disposable WAL database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	models := []any{
		&database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{},
		&database.Tag{}, &database.TagAssignment{}, &database.JobPipeline{}, &database.Job{},
		&database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{},
		&database.JobTrigger{}, &database.JobRun{}, &database.JobEvent{}, &database.JobRunEvent{},
	}
	models = append(models, commandjobevents.Models()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate production job/outbox models: %v", err)
	}
	if err := commandactivation.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate production activation schema: %v", err)
	}
	if err := commandjobactivation.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate production lease schema: %v", err)
	}
	mode := ""
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("fixture must run in WAL mode, got %q", mode)
	}
	return db, NewDBRepository(db)
}

func retentionPlanOutbox(t testing.TB, userID, runID, deliveryState string, now time.Time) commandjobevents.ActivationOutbox {
	t.Helper()
	eventID := mustRetentionUUID(t)
	return commandjobevents.ActivationOutbox{
		SourceEventID: eventID, SchemaVersion: commandjobevents.SchemaVersion,
		EventName: commandjobevents.SchemaVersion, UserID: userID,
		JobDatabaseID: mustRetentionUUID(t), JobSlug: "retention-plan-job", RunID: runID,
		Sequence: 1, State: commandjobevents.StateCompleted, OccurredAt: now,
		RootOriginType: "manual", RootOriginID: eventID,
		SourceReplayPolicyGeneration: "1", SourceReplayDeadline: now.Add(time.Hour),
		EventFingerprint: strings.Repeat("a", 64), DeliveryState: deliveryState, CreatedAt: now,
	}
}

func insertRetentionPlanClaim(t testing.TB, db *gorm.DB, activationID, userID, runID string, now time.Time) error {
	t.Helper()
	return db.Exec(`INSERT INTO command_layer_activation_state (
		activation_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref, user_id,
		workspace_id, auth_context_type, auth_context_id, auth_generation,
		security_generation, source_type, source_instance_id, source_event_id,
		source_correlation_id, state, activated_at, updated_at
	) VALUES (?, 'builtin', 'layer', 'builtin', 'rule', ?, NULL, 'user', ?, '1', '1',
		'job', ?, ?, ?, 'active', ?, ?)`, activationID, userID,
		"retention-plan-context", mustRetentionUUID(t), mustRetentionUUID(t), runID, now, now).Error
}

func retentionPlanSQL(db *gorm.DB, repo *DBRepository, ctx context.Context, userID string, now time.Time, mode string, correlated, bounded bool) (string, []any) {
	// Keep a live migrator for retainableRunQuery's schema-presence checks;
	// only the final Pluck is dry-run so its SQL and bind arguments are captured.
	base := db
	var query *gorm.DB
	if mode == "age" {
		query = database.ScopeByUser(ctx, base.Model(&database.JobRun{}), "user_id").
			Where("julianday(COALESCE(started_at, queued_at)) < julianday(?)", now.Add(-24*time.Hour))
	} else {
		query = base.Model(&database.JobRun{}).
			Select("id, ROW_NUMBER() OVER (PARTITION BY job_id ORDER BY julianday(COALESCE(started_at, queued_at)) DESC, id DESC) AS rn").
			Where("user_id = ?", userID)
	}
	if correlated {
		query = query.Where(`NOT EXISTS (
			SELECT 1 FROM command_job_activation_outbox source
			WHERE source.run_id = job_runs.id AND source.user_id = job_runs.user_id
			  AND source.delivery_state IN (?, ?)
		)`, commandjobevents.DeliveryPending, commandjobevents.DeliveryProcessing)
		query = query.Where(`status IN ? OR (status NOT IN ? AND NOT EXISTS (
			SELECT 1 FROM command_job_activation_leases lease
			JOIN command_layer_activation_state claim
			  ON claim.activation_id = lease.activation_id
			 AND claim.user_id = lease.user_id AND claim.state = 'active'
			WHERE lease.run_id = job_runs.id AND lease.user_id = job_runs.user_id
			  AND julianday(lease.expires_at) > julianday(?)
		))`, []string{RunStatusCompleted, RunStatusFailed, RunStatusSkipped}, []string{RunStatusCompleted, RunStatusFailed, RunStatusSkipped}, now)
	} else {
		query = repo.retainableRunQuery(query, userID, now)
	}
	if mode == "age" {
		query = query.Order("COALESCE(started_at, queued_at) ASC, id ASC")
	} else {
		query = base.Table("(?) AS ranked", query).Where("rn > ?", 200).
			Order("id ASC")
	}
	if bounded {
		query = query.Limit(retentionBatchSize + 1)
	}
	var ids []string
	query = query.Session(&gorm.Session{DryRun: true}).Pluck("id", &ids)
	return query.Statement.SQL.String(), query.Statement.Vars
}

func equalRetentionPlanIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func retentionPlanDifference(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, id := range right {
		rightSet[id] = struct{}{}
	}
	var difference []string
	for _, id := range left {
		if _, ok := rightSet[id]; !ok {
			difference = append(difference, id)
			if len(difference) == 10 {
				break
			}
		}
	}
	return difference
}

func explainRetentionPlan(t testing.TB, db *gorm.DB, sql string, args []any) []string {
	t.Helper()
	var rows []struct {
		ID      int
		Parent  int
		NotUsed int
		Detail  string
	}
	if err := db.Raw("EXPLAIN QUERY PLAN "+sql, args...).Scan(&rows).Error; err != nil {
		t.Fatalf("explain retention query: %v\nSQL: %s", err, sql)
	}
	plan := make([]string, 0, len(rows))
	for _, row := range rows {
		plan = append(plan, row.Detail)
	}
	return plan
}

func runRetentionPlanDiagnostic(t testing.TB, db *gorm.DB, sql string, args []any) time.Duration {
	t.Helper()
	start := time.Now()
	rows, err := db.Raw(sql, args...).Rows()
	if err != nil {
		t.Fatalf("execute retention diagnostic query: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close retention diagnostic rows: %v", err)
		}
	}()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan retention diagnostic row: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate retention diagnostic rows: %v", err)
	}
	return time.Since(start)
}

func retentionPlanIDs(t testing.TB, db *gorm.DB, sql string, args []any) []string {
	t.Helper()
	var ids []string
	if err := db.Raw(sql, args...).Scan(&ids).Error; err != nil {
		t.Fatalf("execute retention selection: %v", err)
	}
	return ids
}

func hasTablePlanStep(plan []string, operation, table string) bool {
	for _, step := range plan {
		upper := strings.ToUpper(step)
		if strings.Contains(upper, operation) && strings.Contains(upper, strings.ToUpper(table)) {
			return true
		}
	}
	return false
}

func planContains(plan []string, fragment string) bool {
	for _, step := range plan {
		if strings.Contains(strings.ToUpper(step), strings.ToUpper(fragment)) {
			return true
		}
	}
	return false
}

func mustRetentionUUID(t testing.TB) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate fixture UUID: %v", err)
	}
	return id.String()
}

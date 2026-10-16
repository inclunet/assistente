package jobs

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type commandOriginTool struct {
	mu      sync.Mutex
	results []tools.ToolResult
	calls   int
}

func (t *commandOriginTool) Name() string { return "test_tool" }

func (t *commandOriginTool) Description() string { return "tool controlada para outbox de jobs" }

func (t *commandOriginTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *commandOriginTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	index := t.calls
	t.calls++
	if index >= len(t.results) {
		index = len(t.results) - 1
	}
	return t.results[index], nil
}

func (t *commandOriginTool) reset(results ...tools.ToolResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.results = results
	t.calls = 0
}

func TestManagerExecutorPersistsAuthenticatedCommandOriginsAndAllRunStates(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate command events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay epoch: %v", err)
	}
	userID, err := database.RequireUserID(userCtx)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := uuid7ForTest(t)
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(userCtx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	tool := &commandOriginTool{results: []tools.ToolResult{
		{Content: `{"ok":true}`}, {Content: `{"ok":true}`}, {Content: `{"ok":true}`}, {Content: `{"ok":true}`}, {Content: `{"ok":true}`},
		{Content: `{"temporary":true}`, Failure: &tools.ToolFailure{Code: "temporary", Kind: tools.ErrorKindUnavailable, Retryable: true}},
		{Content: `{"ok":true}`},
		{Content: `{"denied":true}`, Failure: &tools.ToolFailure{Code: "denied", Kind: tools.ErrorKindAuthorization, Retryable: false}},
		{Content: `{"denied":true}`, Failure: &tools.ToolFailure{Code: "denied", Kind: tools.ErrorKindAuthorization, Retryable: false}},
	}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	mgr := mustNewManager(t, ManagerConfig{
		Repository:      repo,
		ToolRegistry:    registry,
		ToolInvocations: invocations,
		ContextProvider: func() context.Context { return userCtx },
		CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			watch, release, err := epochs.WatchSecurityEpoch(ctx, epoch)
			if err != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, err
			}
			return commandjobactivation.RuntimeIdentity{
				UserID: userID, AuthContextType: "local_session", AuthContextID: sessionID,
				AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
			}, watch, release, nil
		},
	})
	t.Cleanup(mgr.Stop)

	type originCase struct {
		name, want string
		trigger    *TriggerContext
	}
	origins := []originCase{
		{name: "manual", want: "manual", trigger: &TriggerContext{Type: TriggerManual}},
		{name: "cron", want: "cron", trigger: &TriggerContext{Type: TriggerCron, Expression: "0 * * * *"}},
		{name: "interval", want: "interval", trigger: &TriggerContext{Type: TriggerInterval, Every: "1m"}},
		{name: "user-hotkey", want: "user_hotkey", trigger: &TriggerContext{Type: TriggerHotkey, Keys: "Ctrl+Alt+J"}},
		{name: "internal-event", want: "internal_event", trigger: &TriggerContext{Type: TriggerEvent, EventName: "trusted.test.event", RootOriginType: "internal_event", RootOriginID: uuid.Must(uuid.NewV7()).String()}},
	}
	jobs := make([]struct {
		job    *Job
		origin originCase
	}, 0, len(origins))
	for _, origin := range origins {
		job := testRepositoryJob("origin-"+origin.name, "Origin "+origin.name)
		job.Triggers = []Trigger{triggerForCommandOrigin(origin.trigger)}
		if err := repo.SaveJob(userCtx, job); err != nil {
			t.Fatalf("save %s: %v", origin.name, err)
		}
		jobs = append(jobs, struct {
			job    *Job
			origin originCase
		}{job: job, origin: origin})
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	proofRuns := make(map[string]string)

	for i := range jobs {
		tool.reset(tools.ToolResult{Content: `{"ok":true}`})
		run := mgr.executor.Execute(userCtx, jobs[i].job, jobs[i].origin.trigger)
		if run == nil || run.Status != RunStatusCompleted {
			t.Fatalf("execução %s: %#v", jobs[i].origin.name, run)
		}
		proof := assertCommandOutboxOriginAndStates(t, repo, run.RunID, jobs[i].origin.want, jobs[i].origin.trigger.RootOriginID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateCompleted)
		assertUniqueRuntimeProof(t, proofRuns, run.RunID, proof)
	}

	retryJob := testRepositoryJob("origin-retry", "Origin retry")
	retryJob.ErrorPolicy = ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}
	if err := repo.SaveJob(userCtx, retryJob); err != nil {
		t.Fatal(err)
	}
	tool.reset(
		tools.ToolResult{Content: `{"temporary":true}`, Failure: &tools.ToolFailure{Code: "temporary", Kind: tools.ErrorKindUnavailable, Retryable: true}},
		tools.ToolResult{Content: `{"ok":true}`},
	)
	retryRun := mgr.executor.Execute(userCtx, retryJob, &TriggerContext{Type: TriggerManual})
	if retryRun.Status != RunStatusCompleted {
		t.Fatalf("retry não completou: %#v", retryRun)
	}
	proof := assertCommandOutboxOriginAndStates(t, repo, retryRun.RunID, "manual", retryRun.RunID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateRetryScheduled, commandjobevents.StateCompleted)
	assertUniqueRuntimeProof(t, proofRuns, retryRun.RunID, proof)

	failJob := testRepositoryJob("origin-failed", "Origin failed")
	if err := repo.SaveJob(userCtx, failJob); err != nil {
		t.Fatal(err)
	}
	tool.reset(tools.ToolResult{Content: `{"denied":true}`, Failure: &tools.ToolFailure{Code: "denied", Kind: tools.ErrorKindAuthorization, Retryable: false}})
	failRun := mgr.executor.Execute(userCtx, failJob, &TriggerContext{Type: TriggerManual})
	proof = assertCommandOutboxOriginAndStates(t, repo, failRun.RunID, "manual", failRun.RunID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateFailed)
	assertUniqueRuntimeProof(t, proofRuns, failRun.RunID, proof)

	skipJob := testRepositoryJob("origin-skipped", "Origin skipped")
	skipJob.ErrorPolicy = ErrorPolicy{Strategy: ErrorSkip}
	if err := repo.SaveJob(userCtx, skipJob); err != nil {
		t.Fatal(err)
	}
	tool.reset(tools.ToolResult{Content: `{"denied":true}`, Failure: &tools.ToolFailure{Code: "denied", Kind: tools.ErrorKindAuthorization, Retryable: false}})
	skipRun := mgr.executor.Execute(userCtx, skipJob, &TriggerContext{Type: TriggerManual})
	proof = assertCommandOutboxOriginAndStates(t, repo, skipRun.RunID, "manual", skipRun.RunID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateSkipped)
	assertUniqueRuntimeProof(t, proofRuns, skipRun.RunID, proof)
}

func triggerForCommandOrigin(trigger *TriggerContext) Trigger {
	switch trigger.Type {
	case TriggerCron:
		return Trigger{Type: TriggerCron, Expression: trigger.Expression}
	case TriggerInterval:
		return Trigger{Type: TriggerInterval, Every: trigger.Every}
	case TriggerHotkey:
		return Trigger{Type: TriggerHotkey, Keys: trigger.Keys}
	case TriggerEvent:
		return Trigger{Type: TriggerEvent, Listen: trigger.EventName}
	default:
		return Trigger{Type: TriggerManual}
	}
}

func assertCommandOutboxOriginAndStates(t *testing.T, repo *DBRepository, runID, wantOrigin, explicitOriginID string, wantStates ...string) string {
	t.Helper()
	var rows []commandjobevents.ActivationOutbox
	if err := repo.db.Where("run_id = ?", runID).Order("sequence ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load outbox %s: %v", runID, err)
	}
	if len(rows) == 0 {
		t.Fatalf("run %s não publicou outbox", runID)
	}
	var timeline []database.JobRunEvent
	if err := repo.db.Where("job_run_id = ?", runID).Order("sequence ASC").Find(&timeline).Error; err != nil {
		t.Fatalf("load timeline %s: %v", runID, err)
	}
	timelineByID := make(map[string]database.JobRunEvent, len(timeline))
	for _, event := range timeline {
		timelineByID[event.ID] = event
	}
	want := make(map[string]bool, len(wantStates))
	for _, state := range wantStates {
		want[state] = true
	}
	proofGeneration := ""
	seenFingerprints := make(map[string]bool, len(rows))
	seenSequences := make(map[int]bool, len(rows))
	for _, row := range rows {
		if row.RootOriginType != wantOrigin {
			t.Fatalf("run %s origin=%q, want %q", runID, row.RootOriginType, wantOrigin)
		}
		if row.RootOriginID == "" {
			t.Fatalf("run %s sem root origin id", runID)
		}
		if row.SourceEventID == "" || len(row.EventFingerprint) != 64 || seenFingerprints[row.EventFingerprint] || seenSequences[row.Sequence] {
			t.Fatalf("run %s identidade de evento inválida: source=%q fingerprint=%q sequence=%d", runID, row.SourceEventID, row.EventFingerprint, row.Sequence)
		}
		seenFingerprints[row.EventFingerprint] = true
		seenSequences[row.Sequence] = true
		event, ok := timelineByID[row.SourceEventID]
		if !ok || event.JobRunID != runID || event.Sequence != row.Sequence || event.RootOriginType != row.RootOriginType || event.RootOriginID != row.RootOriginID {
			t.Fatalf("run %s outbox/timeline divergentes: outbox=%+v timeline=%+v", runID, row, event)
		}
		outboxProof := runtimeProofGeneration(t, row.Provenance, runID)
		timelineProof := runtimeProofGeneration(t, event.Provenance, runID)
		if outboxProof != timelineProof || proofGeneration != "" && proofGeneration != outboxProof {
			t.Fatalf("run %s prova reservada divergente: outbox=%q timeline=%q run=%q", runID, outboxProof, timelineProof, proofGeneration)
		}
		proofGeneration = outboxProof
		delete(want, row.State)
	}
	if len(want) != 0 {
		t.Fatalf("run %s estados básicos ausentes: %v; rows=%+v", runID, want, rows)
	}
	if explicitOriginID != "" && wantOrigin == "internal_event" && rows[0].RootOriginID != explicitOriginID {
		t.Fatalf("internal event origin id=%q, want %q", rows[0].RootOriginID, explicitOriginID)
	}
	if wantOrigin != "internal_event" && rows[0].RootOriginID != runID {
		t.Fatalf("origin %s id=%q, want run %q", wantOrigin, rows[0].RootOriginID, runID)
	}
	return proofGeneration
}

func runtimeProofGeneration(t *testing.T, raw, runID string) string {
	t.Helper()
	var provenance map[string]any
	if err := json.Unmarshal([]byte(raw), &provenance); err != nil {
		t.Fatalf("run %s proveniência inválida: %v", runID, err)
	}
	proof, ok := provenance[commandRuntimeIdentityProvenanceKey].(map[string]any)
	if !ok {
		t.Fatalf("run %s sem prova reservada de runtime: %s", runID, raw)
	}
	for _, key := range []string{"generation", "user_id", "auth_context_type", "auth_context_id", "auth_generation", "security_generation"} {
		value, ok := proof[key].(string)
		if !ok || value == "" {
			t.Fatalf("run %s prova %q ausente/inválida: %#v", runID, key, proof)
		}
	}
	return proof["generation"].(string)
}

func assertUniqueRuntimeProof(t *testing.T, seen map[string]string, runID, proof string) {
	t.Helper()
	if previous, exists := seen[proof]; exists {
		t.Fatalf("prova de runtime reutilizada: run=%s anterior=%s generation=%s", runID, previous, proof)
	}
	seen[proof] = runID
}

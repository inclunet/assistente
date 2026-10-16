package jobs

import (
	"context"
	"encoding/json"
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

type eventOriginPublishTool struct {
	name    string
	mgr     *Manager
	event   string
	payload map[string]any
	done    chan<- struct{}
}

func (t *eventOriginPublishTool) Name() string        { return t.name }
func (t *eventOriginPublishTool) Description() string { return "publica um evento de teste" }
func (t *eventOriginPublishTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *eventOriginPublishTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	if err := t.mgr.PublishDomainEvent(ctx, t.event, t.payload); err != nil {
		return tools.ToolResult{}, err
	}
	if t.done != nil {
		t.done <- struct{}{}
	}
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

type eventOriginRecordTool struct{ done chan<- struct{} }

func (t *eventOriginRecordTool) Name() string        { return "record_event_origin" }
func (t *eventOriginRecordTool) Description() string { return "registra a chegada do evento" }
func (t *eventOriginRecordTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *eventOriginRecordTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	if t.done != nil {
		t.done <- struct{}{}
	}
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

type eventOriginRunJobContextTool struct {
	mgr         *Manager
	target      string
	result      chan<- *RunLog
	foreignUser string
}

func (t *eventOriginRunJobContextTool) Name() string { return "run_job_context_origin" }
func (t *eventOriginRunJobContextTool) Description() string {
	return "executa um job filho no contexto atual"
}
func (t *eventOriginRunJobContextTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *eventOriginRunJobContextTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	if t.foreignUser != "" {
		ctx = context.WithValue(ctx, commandEventOriginKey{}, commandEventOrigin{
			userID: t.foreignUser, rootType: "external", rootID: "foreign-root",
			chainID: "foreign-chain", history: []string{"foreign-history"},
		})
	}
	run, err := t.mgr.RunJobContext(ctx, t.target)
	if err == nil {
		t.result <- run
	}
	return tools.ToolResult{Content: `{"ok":true}`}, err
}

func TestManagerEventBusCommandOriginSurvivesQueuedChainAndRejectsPayloadSpoof(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid.Must(uuid.NewV7()).String())
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
	sessionID := uuid.Must(uuid.NewV7()).String()
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(userCtx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	const firstEvent = "tasklist.task.updated"
	const secondEvent = "tasklist.list.refresh_requested"
	childDone := make(chan struct{}, 1)
	grandchildDone := make(chan struct{}, 1)
	registry := tools.NewRegistry()
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

	registry.MustRegister(&eventOriginPublishTool{
		name: "publish_first_origin", mgr: mgr, event: firstEvent,
		done: nil,
		payload: map[string]any{
			"task_id":          "spoof-task",
			"root_origin_type": "external",
			"root_origin_id":   "spoof-root",
			"_source":          "external",
			"_source_job_id":   "spoof-job",
			"_chain_id":        "spoof-chain",
			"_chain_history":   []string{"spoof-history"},
		},
	})
	registry.MustRegister(&eventOriginPublishTool{
		name: "publish_second_origin", mgr: mgr, event: secondEvent,
		done: childDone,
		payload: map[string]any{
			"task_id":        "spoof-intermediate",
			"_source":        "unknown",
			"_source_job_id": "spoof-intermediate-job",
			"_chain_id":      "unknown-chain",
			"_chain_history": []string{"unknown", "external"},
		},
	})
	registry.MustRegister(&eventOriginRecordTool{done: grandchildDone})
	createEventOriginToolCatalogs(t, repo, "publish_first_origin", "publish_second_origin", "record_event_origin")

	parent := testRepositoryJob("event-origin-parent", "Event origin parent")
	parent.Tool = "publish_first_origin"
	child := testRepositoryJob("event-origin-child", "Event origin child")
	child.Tool = "publish_second_origin"
	child.Triggers = []Trigger{{Type: TriggerEvent, Listen: firstEvent}}
	grandchild := testRepositoryJob("event-origin-grandchild", "Event origin grandchild")
	grandchild.Tool = "record_event_origin"
	grandchild.Triggers = []Trigger{{Type: TriggerEvent, Listen: secondEvent}}
	for _, job := range []*Job{parent, child, grandchild} {
		if err := repo.SaveJob(userCtx, job); err != nil {
			t.Fatalf("save %s: %v", job.ID, err)
		}
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	parentRun, err := mgr.RunJobContext(userCtx, parent.ID)
	if err != nil || parentRun == nil || parentRun.Status != RunStatusCompleted {
		t.Fatalf("parent run: %+v, %v", parentRun, err)
	}
	select {
	case <-childDone:
	case <-time.After(3 * time.Second):
		t.Fatal("child listener did not execute")
	}
	select {
	case <-grandchildDone:
	case <-time.After(3 * time.Second):
		t.Fatal("grandchild listener did not execute")
	}
	// O sinal da tool prova apenas que o handler entrou. Close drena os
	// handlers do EventBus, incluindo a persistência terminal de cada execução,
	// antes das consultas de evidência.
	mgr.eventBus.Close()

	for _, job := range []*Job{parent, child, grandchild} {
		runs, err := repo.GetRuns(userCtx, job.ID, 1)
		if err != nil || len(runs) != 1 {
			t.Fatalf("runs %s: len=%d err=%v", job.ID, len(runs), err)
		}
		run := runs[0]
		if run.RootOriginType != "manual" || run.RootOriginID != parentRun.RunID {
			t.Fatalf("run %s root=(%q,%q), want manual/%s", job.ID, run.RootOriginType, run.RootOriginID, parentRun.RunID)
		}
		var outbox []commandjobevents.ActivationOutbox
		if err := repo.db.Where("run_id = ?", run.RunID).Find(&outbox).Error; err != nil {
			t.Fatalf("outbox %s: %v", job.ID, err)
		}
		if len(outbox) == 0 {
			t.Fatalf("run %s não publicou outbox", job.ID)
		}
		if run.Status != RunStatusCompleted {
			t.Fatalf("run %s não terminou: %q", job.ID, run.Status)
		}
		assertRunTimelineStates(t, repo, run.RunID,
			commandjobevents.StateQueued, RunStatusRunning, commandjobevents.StateCompleted)
		for _, row := range outbox {
			if row.RootOriginType != "manual" || row.RootOriginID != parentRun.RunID {
				t.Fatalf("outbox %s root=(%q,%q), want manual/%s", job.ID, row.RootOriginType, row.RootOriginID, parentRun.RunID)
			}
		}
	}

	grandchildRun, err := repo.GetRuns(userCtx, grandchild.ID, 1)
	if err != nil || len(grandchildRun) != 1 {
		t.Fatalf("load grandchild provenance: %v", err)
	}
	var provenance map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, grandchildRun[0].Provenance)), &provenance); err != nil {
		t.Fatal(err)
	}
	history, ok := provenance["_chain_history"].([]any)
	if !ok || len(history) != 3 || history[0] != parent.ID || history[1] != child.ID || history[2] != grandchild.ID {
		t.Fatalf("private chain history was washed/overwritten: %#v", provenance["_chain_history"])
	}
}

func TestInheritCommandEventOriginRejectsDifferentUser(t *testing.T) {
	userA := uuid.Must(uuid.NewV7()).String()
	userB := uuid.Must(uuid.NewV7()).String()
	ctx := database.WithUserID(context.Background(), userA)
	ctx = context.WithValue(ctx, commandEventOriginKey{}, commandEventOrigin{
		userID: userA, rootType: "manual", rootID: "root-a", chainID: "chain-a", history: []string{"root-a"},
	})
	trigger := &TriggerContext{Type: TriggerEvent, RootOriginType: "unknown", RootOriginID: ""}
	inheritCommandEventOrigin(database.WithUserID(ctx, userB), trigger)
	if trigger.RootOriginType != "unknown" || trigger.RootOriginID != "" || trigger.ChainID != "" || trigger.ChainHistory != nil {
		t.Fatalf("origin inherited across user mismatch: %+v", trigger)
	}
}

func TestManagerRunJobContextInsideToolInheritsPrivateCommandOrigin(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid.Must(uuid.NewV7()).String())
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
	sessionID := uuid.Must(uuid.NewV7()).String()
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(userCtx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	child := testRepositoryJob("run-context-child", "Run context child")
	child.Tool = "record_event_origin"
	parent := testRepositoryJob("run-context-parent", "Run context parent")
	parent.Tool = "run_job_context_origin"
	createEventOriginToolCatalogs(t, repo, "record_event_origin", "run_job_context_origin")
	if err := repo.SaveJob(userCtx, child); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveJob(userCtx, parent); err != nil {
		t.Fatal(err)
	}
	result := make(chan *RunLog, 1)
	registry := tools.NewRegistry()
	registry.MustRegister(&eventOriginRecordTool{})
	runContextTool := &eventOriginRunJobContextTool{target: child.ID, result: result}
	registry.MustRegister(runContextTool)
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	mgr := mustNewManager(t, ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocations,
		ContextProvider: func() context.Context { return userCtx },
		CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			watch, release, err := epochs.WatchSecurityEpoch(ctx, epoch)
			if err != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, err
			}
			return commandjobactivation.RuntimeIdentity{UserID: userID, AuthContextType: "local_session", AuthContextID: sessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, watch, release, nil
		},
	})
	t.Cleanup(mgr.Stop)
	runContextTool.mgr = mgr
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	parentRun, err := mgr.RunJobContext(userCtx, parent.ID)
	if err != nil || parentRun == nil || parentRun.Status != RunStatusCompleted {
		t.Fatalf("parent run: %+v, %v", parentRun, err)
	}
	var childRun *RunLog
	select {
	case childRun = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("RunJobContext filho não terminou")
	}
	if childRun.RootOriginType != "manual" || childRun.RootOriginID != parentRun.RunID {
		t.Fatalf("RunJobContext lavou a origem: child=(%q,%q), want manual/%s", childRun.RootOriginType, childRun.RootOriginID, parentRun.RunID)
	}

	runContextTool.foreignUser = uuid.Must(uuid.NewV7()).String()
	foreignParentRun, err := mgr.RunJobContext(userCtx, parent.ID)
	if err != nil || foreignParentRun == nil || foreignParentRun.Status != RunStatusCompleted {
		t.Fatalf("foreign marker parent run: %+v, %v", foreignParentRun, err)
	}
	select {
	case childRun = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("RunJobContext foreign filho não terminou")
	}
	if childRun.RootOriginType != "unknown" || childRun.RootOriginID != "" {
		t.Fatalf("marker foreign promoveu origem: child=(%q,%q), want unknown/empty", childRun.RootOriginType, childRun.RootOriginID)
	}
}

func TestRunRootWebhookLegacyUnknownAndTrustedExternalOriginPreserved(t *testing.T) {
	for _, tc := range []struct {
		name       string
		trigger    *TriggerContext
		wantType   string
		wantID     string
		ineligible bool
	}{
		{name: "webhook-without-adapter-proof", trigger: &TriggerContext{Type: TriggerWebhook}, wantType: "unknown"},
		{name: "trusted-external-adapter", trigger: &TriggerContext{Type: TriggerWebhook, RootOriginType: "external_event", RootOriginID: "adapter-run"}, wantType: "external_event", wantID: "adapter-run", ineligible: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootType, rootID := runRoot(tc.trigger, "run-origin")
			if rootType != tc.wantType || rootID != tc.wantID {
				t.Fatalf("root=(%q,%q), want=(%q,%q)", rootType, rootID, tc.wantType, tc.wantID)
			}
			if tc.ineligible && (rootType == "manual" || rootType == "internal_event") {
				t.Fatalf("webhook became command-eligible root: %q", rootType)
			}
		})
	}
}

func mustJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func createEventOriginToolCatalogs(t *testing.T, repo *DBRepository, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := repo.db.Create(&database.ToolCatalog{
			Name: name, DisplayName: name, Description: "event-origin test tool",
			Origin: "builtin", AvailabilityStatus: "available",
		}).Error; err != nil {
			t.Fatalf("seed tool catalog %s: %v", name, err)
		}
	}
}

func assertRunTimelineStates(t *testing.T, repo *DBRepository, runID string, want ...string) {
	t.Helper()
	var events []database.JobRunEvent
	if err := repo.db.Where("job_run_id = ?", runID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load run states %s: %v", runID, err)
	}
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, state := range want {
		if !seen[state] {
			t.Fatalf("run %s missing terminal timeline state %q: events=%+v", runID, state, events)
		}
	}
}

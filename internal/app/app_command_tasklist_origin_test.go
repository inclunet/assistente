package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/tasklist"
	"github.com/google/uuid"
)

type tasklistOriginNoopEmitter struct{}

func (tasklistOriginNoopEmitter) Emit(string, any) {}

func TestTasklistServiceSinkCreatesInternalEventJobOriginEndToEnd(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)

	// Montagem idêntica à produção: o Service usa o DBStore real e recebe o
	// sink estreito somente pelo wiring do App.
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{
		Store:   tasklist.NewDBStore(),
		Emitter: tasklistOriginNoopEmitter{},
	})
	// commandJobPublicationApp prepara o schema de comandos, mas não inicializa
	// o domínio tasklist. Migre somente os modelos necessários para que o
	// Service real atravesse o mesmo DBStore usado em produção.
	if err := database.DB().AutoMigrate(
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatal(err)
	}
	toolName := "tasklist-origin-blocked-" + uuid.Must(uuid.NewV7()).String()
	tool := &liveCommandToolImpl{name: toolName, started: make(chan struct{}), release: make(chan struct{})}
	a.toolRegistry.MustRegister(tool)
	t.Cleanup(func() { tool.releaseOnce.Do(func() { close(tool.release) }) })

	job := createTasklistOriginJob(t, a, ctx, toolName)
	a.wireTaskListDomainEvents()
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	if !a.jobMgr.HasDomainListener("tasklist.list.created") {
		t.Fatal("Manager real não registrou listener tasklist.list.created")
	}

	created, err := a.taskSvc.CreateTaskList(ctx, "Lista de origem interna", "teste", nil, "origin-internal-event")
	if err != nil {
		t.Fatal(err)
	}
	if created == nil || created.ID == "" {
		t.Fatalf("tasklist real não criou lista: %#v", created)
	}
	select {
	case <-tool.started:
	case <-time.After(5 * time.Second):
		t.Fatal("evento tasklist não iniciou o job real bloqueado")
	}

	if job.DatabaseID == "" {
		t.Fatal("job real não recebeu DatabaseID ao persistir")
	}
	run := waitTasklistOriginRun(t, job.DatabaseID)
	if run.RootOriginType != "internal_event" || run.RootOriginID == "" {
		t.Fatalf("run sem raiz internal_event autenticada: %+v", run)
	}
	claim, lease := waitLiveClaimAndLease(t, run.ID)
	if claim.UserID != a.currentUserID || claim.SourceCorrelationID == nil || *claim.SourceCorrelationID != run.ID {
		t.Fatalf("claim não vinculada ao owner/run real: claim=%+v", claim)
	}
	if !lease.ExpiresAt.After(time.Now()) {
		t.Fatalf("lease do owner já expirada: %+v", lease)
	}

	tool.releaseOnce.Do(func() { close(tool.release) })
	waitTasklistOriginRunCompleted(t, job.DatabaseID, run.ID)
	assertTasklistOriginOutbox(t, run.ID, run.RootOriginID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateCompleted)
}

func createTasklistOriginJob(t *testing.T, a *App, ctx context.Context, toolName string) *jobs.Job {
	t.Helper()
	db := database.DB()
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	layerID := uuid.Must(uuid.NewV7()).String()
	ruleID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	if err := db.Create(&commandconfig.Layer{ID: layerID, UserID: a.currentUserID, Name: "tasklist-origin-layer-" + layerID, Description: "camada de teste", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	eventName := commandautomation.JobRunStateEvent
	producerTypes := `["jobs.runtime"]`
	rule := commandactivation.Rule{ID: ruleID, UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: `{"version":1,"clauses":[]}`, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producerTypes, Enabled: false, Source: "user", ReviewStatus: "active"}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	decisionStore, err := commanddecision.New(db, liveCommandDecisionPresenter{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	automation, err := commandautomation.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := a.commandEpochs.Capture(ctx, a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	change, err := automation.PrepareRule(ctx, commandautomation.Owner{UserID: a.currentUserID}, ruleID)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := commandautomationKeyProvider(a)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := automation.ConfirmGrant(ctx, change, epoch, decisionStore, a.commandStorageVersion, keys, now.Add(time.Hour), func(commandautomation.Rule) (string, error) { return "autorizar origem tasklist", nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := automation.CommitConfirmedGrant(ctx, confirmed, epoch); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: uuid.Must(uuid.NewV7()).String()}, Name: toolName, DisplayName: toolName, Description: "tool bloqueada de origem tasklist", Origin: "builtin", Schema: `{"type":"object"}`, AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{ID: uuid.Must(uuid.NewV7()).String(), Name: "tasklist-origin-job", Description: "job de origem tasklist", Tool: toolName, Inputs: map[string]any{}, Enabled: true, Triggers: []jobs.Trigger{{Type: jobs.TriggerEvent, Listen: "tasklist.list.created"}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	return job
}

func waitTasklistOriginRun(t *testing.T, jobDatabaseID string) database.JobRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var run database.JobRun
		if err := database.DB().Where("job_id = ? AND status = ?", jobDatabaseID, jobs.RunStatusRunning).Order("queued_at DESC").Take(&run).Error; err == nil {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run tasklist não ficou persistido: job=%s", jobDatabaseID)
	return database.JobRun{}
}

func waitTasklistOriginRunCompleted(t *testing.T, jobDatabaseID, runID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var run database.JobRun
		if err := database.DB().Where("job_id = ? AND id = ?", jobDatabaseID, runID).Take(&run).Error; err == nil && run.Status == jobs.RunStatusCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run tasklist não concluiu: job=%s run=%s", jobDatabaseID, runID)
}

func assertTasklistOriginOutbox(t *testing.T, runID, rootID string, states ...string) {
	t.Helper()
	var rows []commandjobevents.ActivationOutbox
	if err := database.DB().Where("run_id = ?", runID).Order("sequence ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("outbox ausente para run %s", runID)
	}
	want := make(map[string]bool, len(states))
	for _, state := range states {
		want[state] = true
	}
	var timeline []database.JobRunEvent
	if err := database.DB().Where("job_run_id = ?", runID).Order("sequence ASC").Find(&timeline).Error; err != nil {
		t.Fatal(err)
	}
	timelineByID := make(map[string]database.JobRunEvent, len(timeline))
	for _, event := range timeline {
		timelineByID[event.ID] = event
	}
	for _, row := range rows {
		if row.RootOriginType != "internal_event" || row.RootOriginID != rootID || len(row.EventFingerprint) != 64 || row.SourceEventID == "" {
			t.Fatalf("outbox sem prova internal_event/fingerprint: %+v", row)
		}
		delete(want, row.State)
		event, ok := timelineByID[row.SourceEventID]
		if !ok || event.RootOriginType != row.RootOriginType || event.RootOriginID != row.RootOriginID {
			t.Fatalf("outbox/timeline divergentes: outbox=%+v timeline=%+v", row, event)
		}
		var provenance map[string]any
		if err := json.Unmarshal([]byte(row.Provenance), &provenance); err != nil {
			t.Fatalf("proveniência do outbox inválida: %v", err)
		}
		identity, ok := provenance["command_runtime_identity"].(map[string]any)
		if !ok || identity["user_id"] != row.UserID || identity["auth_context_id"] == "" || identity["generation"] == "" {
			t.Fatalf("prova owner/runtime ausente no outbox: %#v", provenance)
		}
	}
	if len(want) != 0 {
		t.Fatalf("estados do outbox ausentes: %v", want)
	}
}

package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandidentity"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/jobs"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
)

type appCommandJobServiceProbeTool struct {
	manager *jobs.Manager
	service *commandidentity.Service
	seen    chan appCommandJobServiceProbe
	release chan struct{}
}

type appCommandJobServiceProbe struct {
	ctx       context.Context
	request   commandidentity.JobServiceRequest
	identity  commandidentity.TrustedIdentity
	resolve   error
	authorize error
}

func (t *appCommandJobServiceProbeTool) Name() string { return jobprofilegrant.ToolSubagent }
func (t *appCommandJobServiceProbeTool) Description() string {
	return "probe de identidade job_service"
}
func (t *appCommandJobServiceProbeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *appCommandJobServiceProbeTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	request, err := t.manager.CommandJobServiceRequest(ctx, commandcatalog.Event)
	probe := appCommandJobServiceProbe{ctx: ctx, request: request}
	if err == nil {
		probe.identity, probe.resolve = t.service.ResolveJobService(ctx, request)
		if probe.resolve == nil {
			probe.authorize = t.service.Authorize(ctx, commandidentity.AuthorizationRequest{
				Identity: probe.identity, Definition: appCommandJobServiceDefinition(), JobService: &probe.request,
			})
		}
	} else {
		probe.resolve = err
	}
	t.seen <- probe
	select {
	case <-t.release:
		return tools.ToolResult{Content: `{"ok":true}`}, nil
	case <-ctx.Done():
		return tools.ToolResult{Content: `{"cancelled":true}`, IsError: true}, ctx.Err()
	}
}

func TestAppRealJobServiceIdentityResolvesAndRevocationIsRejected(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	// initJobs apenas precisa da fachada do controller; não inicializamos o
	// manager físico nem registramos hotkeys neste teste de identidade.
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{ProfileMgr: a.profileManager})
	a.emitter = events.NoopEmitter{}
	ctx := database.WithUserID(context.Background(), a.currentUserID)

	probe := &appCommandJobServiceProbeTool{
		manager: a.jobMgr, seen: make(chan appCommandJobServiceProbe, 1), release: make(chan struct{}),
	}
	a.toolRegistry.MustRegister(probe)
	a.toolInvocationSvc = toolinvocations.NewService(
		toolinvocations.NewDBRepository(database.DB()),
		tools.NewExecutor(a.toolRegistry, tools.ExecutorConfig{ToolTimeout: 30 * time.Second, MaxResultSize: 1024}),
	)
	a.initJobs()
	t.Cleanup(a.jobMgr.Stop)
	probe.manager = a.jobMgr
	if a.jobGrantStore == nil || a.jobMgr == nil {
		t.Fatal("wiring App de grants/jobs ausente")
	}
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	catalogID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: catalogID}, Name: probe.Name(), DisplayName: probe.Name(),
		Description: probe.Description(), Origin: "builtin", Schema: string(probe.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := &jobs.Job{ID: "app-job-service-identity", Name: "App job service identity", Tool: probe.Name(),
		Inputs: map[string]any{"profile": "job-service-profile"}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}},
		ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	repo := jobs.NewDBRepository(database.DB())
	job, err := repo.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := jobprofilegrant.Fingerprint(probe.Name(), "job-service-profile")
	snapshot, err := a.jobGrantStore.AuthorizationSnapshot(ctx, job.DatabaseID, "job-service-profile")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.jobGrantStore.Grant(ctx, job.DatabaseID, "job-service-profile", fingerprint, "test", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	job.Enabled = true
	if err := a.jobMgr.SaveJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}

	epochs, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	identityEpochs, err := commandidentity.NewCoreEpochs(epochs)
	if err != nil {
		t.Fatal(err)
	}
	probe.service, err = commandidentity.New(commandidentity.Config{
		Epochs: identityEpochs, JobRuntime: a.jobMgr, JobGrants: a.jobGrantStore,
		AuthorizationRules: []commandidentity.AuthorizationRule{{CommandID: appCommandJobServiceDefinition().ID, Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}

	resultCh := make(chan *jobs.RunLog, 1)
	runDone := make(chan struct{})
	var releaseOnce sync.Once
	releaseProbe := func() { releaseOnce.Do(func() { close(probe.release) }) }
	go func() {
		defer close(runDone)
		run, runErr := a.jobMgr.RunJobContext(ctx, job.ID)
		if runErr != nil {
			resultCh <- &jobs.RunLog{Status: jobs.RunStatusFailed, Error: runErr.Error()}
			return
		}
		resultCh <- run
	}()
	t.Cleanup(func() {
		releaseProbe()
		select {
		case <-runDone:
		case <-time.After(10 * time.Second):
			t.Errorf("cleanup: run real não encerrou após liberar a tool")
		}
	})

	var first appCommandJobServiceProbe
	select {
	case first = <-probe.seen:
	case <-time.After(10 * time.Second):
		t.Fatal("job real não chegou ao contexto job_service")
	}
	if first.resolve != nil || first.authorize != nil {
		t.Fatalf("identidade inicial: resolve=%v authorize=%v identity=%+v request=%+v", first.resolve, first.authorize, first.identity, first.request)
	}
	if first.identity.Job == nil || first.identity.Job.DatabaseID != job.DatabaseID || first.identity.Job.OwnerUserID != a.currentUserID || first.identity.Job.RunID == "" || first.request.RunID != first.identity.Job.RunID {
		t.Fatalf("prova job_service incompleta: request=%+v identity=%+v", first.request, first.identity)
	}

	if err := a.jobGrantStore.Revoke(ctx, job.DatabaseID, "job-service-profile", "test-revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.service.ResolveJobService(first.ctx, first.request); err == nil {
		t.Fatal("ResolveJobService aceitou grant revogado")
	}
	releaseProbe()
	select {
	case run := <-resultCh:
		if run == nil || run.Status != jobs.RunStatusCompleted {
			t.Fatalf("job após revogação: %+v", run)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("job não encerrou")
	}
}

func appCommandJobServiceDefinition() commandcatalog.Definition {
	return commandcatalog.Definition{
		ID: "app.job_service.identity", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Event}, HandlerClassification: commandcatalog.HandlerInternal,
	}
}

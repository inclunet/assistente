package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"assistente/internal/tools"
)

type desktopJobEffect struct{ calls atomic.Int32 }

func (*desktopJobEffect) Name() string                { return "test.desktop_job_effect" }
func (*desktopJobEffect) Description() string         { return "efeito controlado do teste" }
func (*desktopJobEffect) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (p *desktopJobEffect) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	p.calls.Add(1)
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func TestCommandJobDesktopDecisionRunsRealJobOnce(t *testing.T) {
	testCommandJobDesktopDecision(t, false, false)
}

func TestCommandJobDesktopDecisionPreservesReactiveRoot(t *testing.T) {
	testCommandJobDesktopDecision(t, true, false)
}

func TestCommandJobDesktopDecisionRejectsRetiredReactiveSource(t *testing.T) {
	testCommandJobDesktopDecision(t, true, true)
}

func testCommandJobDesktopDecision(t *testing.T, reactive, retireSource bool) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 15*time.Second)
	defer cancel()
	effect := &desktopJobEffect{}
	a.toolRegistry.MustRegister(effect)
	if err := database.DB().Create(&database.ToolCatalog{Name: effect.Name(), DisplayName: effect.Name(), Origin: "builtin", Schema: string(effect.Parameters()), AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{ID: "desktop-command-job", Name: "Desktop command job", Tool: effect.Name(), Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.SaveJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	// A mutação real do domínio de jobs invalida a projeção publicada. O
	// executor integrado só pode ser exercitado depois da republicação normal;
	// não bypassar o guard com um HostState stale.
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatalf("republicar projeção após salvar job: %v", err)
	}
	stored, err := jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	base, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := base.Lookup("fixture.confirm")
	definition.Effect, definition.HasMutableTarget = commandcatalog.Destructive, true
	definition.HandlerClassification, definition.Risk = commandcatalog.HandlerJob, commandcatalog.RiskHigh
	definition.Context = commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "workspace", Mode: commandcatalog.ExactVersion}}}
	definition.ResultSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
		"job_id": {Type: commandcatalog.SchemaString}, "run_id": {Type: commandcatalog.SchemaString}, "status": {Type: commandcatalog.SchemaString},
	}, Required: []string{"job_id", "run_id", "status"}}
	contract := commandcatalog.HandlerContract{Effect: definition.Effect, HasMutableTarget: true, Classification: commandcatalog.HandlerJob, Route: definition.HandlerRoute}
	handler, err := a.newCommandJobHandler(ctx, definition, contract, stored.DatabaseID, commandcatalog.SensitivePaths{})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		t.Fatal(err)
	}
	var parent *liveCommandJobControl
	var parentRun string
	if reactive {
		control := startCommandMaintenanceLiveJob(t, a)
		parent, parentRun = &control, <-control.RunID
		claim, _ := waitLiveClaimAndLease(t, parentRun)
		// A outbox e a projeção são reais. Pausamos a cadência para instalar
		// o catálogo controlado sem corrida com sua reconstrução produtiva.
		if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
			t.Fatal(err)
		}
		publishReactiveJobTestBinding(t, a, registry, stored.DatabaseID, definition.ID, claim, `{}`)
	}
	events := make(chan map[string]any, 2)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			events <- data.(map[string]any)
		}
	})
	config := completeFactoryConfig(registry, a.commandHost, make(chan struct{}, 1), a.currentUserID)
	config.RegistryVersion = commandProductRegistryVersion
	config.Handlers = map[string]commandexecution.Handler{definition.ID: handler}
	config.Store, err = commandledger.New(database.DB(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	config.Envelope.Authorize = func(check context.Context, principal auth.LocalSessionPrincipal, _ commandcontract.Envelope, d commandcatalog.Definition) error {
		current, err := a.sessionSvc.RevalidateLocalSession(check, principal)
		if err != nil || current != principal || principal.UserID != a.currentUserID || d.ID != definition.ID {
			return commandexecution.ErrDenied
		}
		return nil
	}
	snapshot := config.Envelope.Snapshot
	config.Envelope.Snapshot = func(check context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		envelope, err := snapshot(check, principal, candidate)
		workspaceID := a.workspaceMgr.ActiveID()
		envelope.WorkspaceID, envelope.WorkspaceConfigGeneration = &workspaceID, envelope.GlobalConfigGeneration
		return envelope, err
	}
	if reactive {
		config.Envelope.Resolve = reactiveJobTestResolver(a)
	}
	service, err := a.newCommandDesktopExecutor(config, a.commandHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	candidate := completeCandidateFor(definition.ID)
	if reactive {
		candidate, err = commandPaletteCandidate(candidate.InvocationID, candidate.CorrelationID, definition.ID, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
	}
	result := make(chan commandledger.FullRecord, 1)
	failures := make(chan error, 1)
	go func() {
		record, err := service.ExecuteEnvelope(ctx, "", candidate)
		result <- record
		failures <- err
	}()
	select {
	case payload := <-events:
		if effect.calls.Load() != 0 {
			t.Fatal("efeito anterior à decisão")
		}
		if retireSource {
			parent.Release()
			if run, err := parent.Join(); err != nil || run == nil || run.Status != jobs.RunStatusCompleted {
				t.Fatalf("encerrar fonte antes da confirmação: %+v %v", run, err)
			}
		}
		if err := a.questionnaireMgr.Respond(payload["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatal(err)
		}
	case early := <-result:
		t.Fatalf("execução terminou antes da confirmação: %s %v", early.Status, <-failures)
	case <-ctx.Done():
		t.Fatal("diálogo não chegou", ctx.Err())
	}
	var record commandledger.FullRecord
	select {
	case record = <-result:
		executionErr := <-failures
		if retireSource {
			if executionErr != nil || record.Status != commandledger.CancelledStale {
				t.Fatalf("fonte encerrada: status=%s err=%v", record.Status, executionErr)
			}
			runs, err := a.jobMgr.GetJobRunsContext(ctx, stored.ID, 10)
			if err != nil || len(runs) != 0 || effect.calls.Load() != 0 {
				t.Fatalf("fonte encerrada iniciou job: runs=%d calls=%d err=%v", len(runs), effect.calls.Load(), err)
			}
			return
		}
		if executionErr != nil || record.Status != commandledger.Succeeded {
			t.Fatalf("execução: %s %v", record.Status, executionErr)
		}
	case <-ctx.Done():
		t.Fatal("job não terminou", ctx.Err())
	}
	if record.Envelope.AuthorizationDecisionID == nil {
		t.Fatal("receipt ausente")
	}
	replay, err := service.ExecuteEnvelope(ctx, "", candidate)
	if err != nil || replay.Status != commandledger.Succeeded || effect.calls.Load() != 1 {
		t.Fatalf("replay: %s %v chamadas=%d", replay.Status, err, effect.calls.Load())
	}
	runs, err := a.jobMgr.GetJobRunsContext(ctx, stored.ID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%d err=%v", len(runs), err)
	}
	if reactive {
		verifyReactiveJobTestRun(t, ctx, stored.ID, runs[0].RunID, record, *parent, parentRun)
	}
	select {
	case <-events:
		t.Fatal("replay abriu outra decisão")
	default:
	}
}

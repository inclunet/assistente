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
	"assistente/internal/tasklist"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type commandTasklistDomainTool struct {
	app   *App
	calls atomic.Int32
	slug  string
}

func (t *commandTasklistDomainTool) Name() string { return "test.command_tasklist_create" }

func (*commandTasklistDomainTool) Description() string {
	return "tool controlada que cria uma tasklist pelo Service real"
}

func (*commandTasklistDomainTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *commandTasklistDomainTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	if t.app == nil || t.app.taskSvc == nil {
		return tools.ToolResult{}, commandexecution.ErrInvalidConfiguration
	}
	if _, err := t.app.taskSvc.CreateTaskList(ctx, "Lista criada pelo comando", "origem command tool", nil, t.slug); err != nil {
		return tools.ToolResult{}, err
	}
	return tools.ToolResult{Content: `{}`}, nil
}

func TestCommandToolCreatesTasklistAndPreservesManualOriginIntoEventJob(t *testing.T) {
	testCommandToolTasklistOrigin(t, false)
}

func TestCommandToolCreatesTasklistAndPreservesReactiveOriginIntoEventJob(t *testing.T) {
	testCommandToolTasklistOrigin(t, true)
}

func testCommandToolTasklistOrigin(t *testing.T, reactive bool) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 20*time.Second)
	defer cancel()

	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{
		Store:   tasklist.NewDBStore(),
		Emitter: tasklistOriginNoopEmitter{},
	})
	if err := database.DB().AutoMigrate(
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatal(err)
	}

	downstreamName := "tasklist-command-downstream-" + uuid.Must(uuid.NewV7()).String()
	downstream := &liveCommandToolImpl{name: downstreamName, started: make(chan struct{}), release: make(chan struct{})}
	a.toolRegistry.MustRegister(downstream)
	t.Cleanup(func() { downstream.releaseOnce.Do(func() { close(downstream.release) }) })
	job := createTasklistOriginJob(t, a, ctx, downstreamName)
	a.wireTaskListDomainEvents()
	if !reactive {
		if err := a.jobMgr.Start(); err != nil {
			t.Fatal(err)
		}
		if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatalf("republicar projeção após montar job downstream: %v", err)
	}
	if !reactive && !a.jobMgr.HasDomainListener("tasklist.list.created") {
		t.Fatal("listener real de tasklist.list.created ausente")
	}

	tool := &commandTasklistDomainTool{app: a, slug: "command-origin-" + uuid.Must(uuid.NewV7()).String()}
	a.toolRegistry.MustRegister(tool)
	catalogID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: catalogID},
		Name:               tool.Name(),
		DisplayName:        tool.Name(),
		Description:        tool.Description(),
		Origin:             "builtin",
		Schema:             string(tool.Parameters()),
		AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}

	definition := appCommandJobHandlerDefinition()
	definition.ID = "app.command_tasklist_domain_create"
	definition.HandlerRoute = "app/test-command-tasklist-domain"
	definition.HandlerClassification = commandcatalog.HandlerTool
	definition.ArgumentsSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject}
	definition.ResultSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject}
	contract := commandcatalog.HandlerContract{
		Effect:           commandcatalog.Destructive,
		HasMutableTarget: true,
		Route:            definition.HandlerRoute,
		Classification:   commandcatalog.HandlerTool,
	}
	handler, err := a.newCommandToolHandler(ctx, definition, contract, catalogID, commandcatalog.SensitivePaths{}, func(result tools.ToolResult) (json.RawMessage, error) {
		return json.RawMessage(result.Content), nil
	})
	if err != nil {
		t.Fatalf("factory command tool: %v", err)
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		t.Fatal(err)
	}
	var parent *liveCommandJobControl
	var parentRun database.JobRun
	if reactive {
		control := startCommandMaintenanceLiveJob(t, a)
		parent = &control
		runID := <-control.RunID
		claim, _ := waitLiveClaimAndLease(t, runID)
		if err := database.DB().Where("id = ?", runID).Take(&parentRun).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
			t.Fatal(err)
		}
		publishReactiveJobTestBinding(t, a, registry, catalogID, definition.ID, claim, `{}`)
	}

	events := make(chan map[string]any, 1)
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
	done := make(chan struct {
		record commandledger.FullRecord
		output json.RawMessage
		err    error
	}, 1)
	go func() {
		record, output, executeErr := service.ExecuteEnvelopeWithResult(ctx, "", candidate)
		done <- struct {
			record commandledger.FullRecord
			output json.RawMessage
			err    error
		}{record, output, executeErr}
	}()

	var prompt map[string]any
	select {
	case prompt = <-events:
		if tool.calls.Load() != 0 {
			t.Fatal("tool executou antes da confirmação")
		}
	case early := <-done:
		t.Fatalf("comando terminou antes da confirmação: status=%s err=%v", early.record.Status, early.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	decisionID, ok := prompt["id"].(string)
	if !ok || decisionID == "" {
		t.Fatalf("questionário sem id: %#v", prompt)
	}
	if err := a.questionnaireMgr.Respond(decisionID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}

	var result struct {
		record commandledger.FullRecord
		output json.RawMessage
		err    error
	}
	select {
	case result = <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if result.err != nil || result.record.Status != commandledger.Succeeded || tool.calls.Load() != 1 || string(result.output) != `{}` {
		t.Fatalf("command tool: status=%s err=%v calls=%d output=%s", result.record.Status, result.err, tool.calls.Load(), result.output)
	}

	select {
	case <-downstream.started:
	case <-ctx.Done():
		t.Fatal("evento não alcançou o job downstream", ctx.Err())
	}
	run := waitTasklistOriginRun(t, job.DatabaseID)
	wantRootType, wantRootID := "manual", candidate.InvocationID
	if reactive {
		wantRootType, wantRootID = parentRun.RootOriginType, parentRun.RootOriginID
	}
	if run.RootOriginType != wantRootType || run.RootOriginID != wantRootID {
		t.Fatalf("raiz downstream=%q/%q, want %s/%s", run.RootOriginType, run.RootOriginID, wantRootType, wantRootID)
	}
	var provenance map[string]any
	if err := json.Unmarshal([]byte(run.Provenance), &provenance); err != nil {
		t.Fatalf("proveniência do run inválida: %v", err)
	}
	rawHistory, err := json.Marshal(provenance["command_chain_history"])
	if err != nil {
		t.Fatal(err)
	}
	history, err := commandcontract.DecodeCommandChainHistory(rawHistory)
	if err != nil || len(history) != 1 || history[0].CommandID != definition.ID || history[0].InvocationID != candidate.InvocationID {
		t.Fatalf("command_chain_history downstream=%s err=%v", rawHistory, err)
	}
	if reactive {
		raw, _ := json.Marshal(provenance["_chain_history"])
		var chain []string
		if json.Unmarshal(raw, &chain) != nil || len(chain) != 2 || chain[0] != parent.JobID || chain[1] != job.ID || provenance["_chain_id"] != parentRun.ID {
			t.Fatalf("cadeia de jobs foi reiniciada: %s id=%v", raw, provenance["_chain_id"])
		}
	}

	var invocations []database.ToolInvocation
	if err := database.DB().Where("origin_type = ? AND origin_id = ? AND user_id = ?", toolinvocations.OriginCommandInvocation, candidate.InvocationID, a.currentUserID).Find(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 1 || invocations[0].OriginID != candidate.InvocationID {
		t.Fatalf("ledger command_tool=%+v", invocations)
	}

	var lists []database.TaskList
	if err := database.DB().Where("slug = ?", tool.slug).Find(&lists).Error; err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 {
		t.Fatalf("listas após primeira execução=%d", len(lists))
	}

	downstream.releaseOnce.Do(func() { close(downstream.release) })
	waitTasklistOriginRunCompleted(t, job.DatabaseID, run.ID)

	replay, output, replayErr := service.ExecuteEnvelopeWithResult(ctx, "", candidate)
	if replayErr != nil || replay.Status != commandledger.Succeeded || len(output) != 0 || tool.calls.Load() != 1 {
		t.Fatalf("replay: status=%s err=%v output=%s calls=%d", replay.Status, replayErr, output, tool.calls.Load())
	}
	var runs int64
	if err := database.DB().Model(&database.JobRun{}).Where("job_id = ?", job.DatabaseID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("runs downstream após replay=%d", runs)
	}
	if err := database.DB().Where("slug = ?", tool.slug).Find(&lists).Error; err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 {
		t.Fatalf("listas após replay=%d", len(lists))
	}
	if !a.jobMgr.HasDomainListener("tasklist.list.created") {
		t.Fatal("listener real foi removido durante replay")
	}
	if parent != nil {
		parent.Release()
		if run, err := parent.Join(); err != nil || run == nil || run.Status != jobs.RunStatusCompleted {
			t.Fatalf("encerramento da fonte reativa: %+v %v", run, err)
		}
	}
}

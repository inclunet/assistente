package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"github.com/google/uuid"
)

// Catálogo controlado, mas binding persistido, projeção, guard, Consumer,
// DecisionDialog, sessão, handler e runtime são os usados pelo App.
func publishReactiveJobTestBinding(t *testing.T, a *App, registry *commandcatalog.Registry, target, command string, claim commandactivation.Claim, arguments string) {
	t.Helper()
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	spec, _ := json.Marshal(map[string]any{"version": 1, "selection": command})
	if _, err := registry.ValidateArguments(command, []byte(arguments)); err != nil {
		t.Fatalf("argumentos do binding reativo: %v", err)
	}
	if _, err := (commandconfig.PaletteTriggerPort{}).Normalize(ctx, spec); err != nil {
		t.Fatalf("acionador do binding reativo: %v", err)
	}
	binding := commandconfig.Binding{ID: uuid.Must(uuid.NewV7()).String(), UserID: claim.UserID, WorkspaceID: cloneCommandWorkspace(claim.WorkspaceID),
		LayerRefKind: string(claim.LayerRefKind), LayerRef: claim.LayerRef, TriggerType: string(commandcatalog.Palette), TriggerSpec: string(spec),
		Arguments: arguments, Condition: `{"version":1,"clauses":[]}`, Effect: "execute", CommandID: &command,
		Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1}`}
	if err := database.DB().Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	scope, err := a.commandMutationCurrentScope(principal)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	projection, guard, err := a.commandJobLayerProjection(ctx, principal, scope)
	if err != nil || projection == nil {
		t.Fatalf("projeção do job: %v", err)
	}
	configuration, err := commandconfig.ProjectComplete(ctx, snapshot, commandconfig.CompleteProjection{
		Registry: registry, ActiveUserLayerIDs: projection.layers, TriggerPorts: commandProductTriggerPorts(),
		ExecutionScope: func(commandcatalog.Definition, []byte, commandconfig.Scope) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatalf("projetar binding reativo (%s, escopo %+v): %v", command, scope, err)
	}
	configuration, err = configuration.WithLayerProvenance(projection.sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := (commandLifecycleLoadedConfiguration{app: a, store: store, principal: principal, workspaceID: scope.WorkspaceID,
		snapshot: snapshot, configuration: configuration, activeLayers: projection.layers, guard: guard}).publish(ctx); err != nil {
		t.Fatalf("publicar binding reativo: %v", err)
	}
}

func reactiveJobTestResolver(a *App) func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate, envelope commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
		configuration, _, versions, err := a.commandHost.ResolutionSnapshot(ctx, principal)
		if err != nil || envelope.GlobalConfigGeneration == nil || *envelope.GlobalConfigGeneration != versions.GlobalConfig || envelope.ActiveLayersGeneration == nil || *envelope.ActiveLayersGeneration != versions.ActiveLayers {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
		}
		identity, err := (commandconfig.PaletteTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
		if err != nil {
			return commandexecution.EnvelopeResolution{}, err
		}
		resolved, err := configuration.Resolve(identity, commandbindings.Facts{}, nil)
		if err != nil || resolved.Status != commandbindings.Selected {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		provenance, err := commandSelectedJobProvenance(configuration.LayerProvenance(resolved.LayerRefs))
		return commandexecution.EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: resolved.CommandID, Arguments: json.RawMessage(resolved.ArgumentsKey),
			BindingIDs: resolved.BindingIDs, LayerRefs: resolved.LayerRefs, Provenance: provenance}, err
	}
}

func verifyReactiveJobTestRun(t *testing.T, ctx context.Context, target, runID string, record commandledger.FullRecord, parent liveCommandJobControl, parentRun string) {
	t.Helper()
	repo := jobs.NewDBRepository(database.DB())
	child, err := repo.GetRun(ctx, target, runID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.GetRun(ctx, parent.JobID, parentRun)
	if err != nil {
		t.Fatal(err)
	}
	if child.RootOriginType != root.RootOriginType || child.RootOriginID != root.RootOriginID || child.RootOriginID == record.Envelope.InvocationID {
		t.Fatalf("origem foi lavada: child=%s/%s parent=%s/%s", child.RootOriginType, child.RootOriginID, root.RootOriginType, root.RootOriginID)
	}
	raw, _ := json.Marshal(child.Provenance["_chain_history"])
	var history []string
	if err := json.Unmarshal(raw, &history); err != nil || len(history) != 2 || history[0] != parent.JobID || history[1] != target || child.Provenance["_chain_id"] != parentRun {
		t.Fatalf("cadeia de jobs incorreta: %s chain=%v err=%v", raw, child.Provenance["_chain_id"], err)
	}
	raw, _ = json.Marshal(child.Provenance["command_chain_history"])
	commands, err := commandcontract.DecodeCommandChainHistory(raw)
	if err != nil || len(commands) != 1 || commands[0].InvocationID != record.Envelope.InvocationID {
		t.Fatalf("cadeia de comandos incorreta: %s err=%v", raw, err)
	}
	parent.Release()
	if result, err := parent.Join(); err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("parent: %+v %v", result, err)
	}
}

func newReactiveJobTestExecutor(t *testing.T, a *App, ctx context.Context, registry *commandcatalog.Registry, definition commandcatalog.Definition, handler commandexecution.Handler) (*commandexecution.Service, chan map[string]any) {
	t.Helper()
	events := make(chan map[string]any, 1)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			events <- data.(map[string]any)
		}
	})
	config := completeFactoryConfig(registry, a.commandHost, make(chan struct{}, 1), a.currentUserID)
	config.RegistryVersion = commandProductRegistryVersion
	config.Handlers = map[string]commandexecution.Handler{definition.ID: handler}
	var err error
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
	config.Envelope.Resolve = reactiveJobTestResolver(a)
	service, err := a.newCommandDesktopExecutor(config, a.commandHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	return service, events
}

func reactiveJobTestDefinition(t *testing.T, a *App, ctx context.Context, target string) (commandcatalog.Definition, *commandcatalog.Registry, commandexecution.Handler) {
	t.Helper()
	definition := appCommandJobHandlerDefinition()
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	handler, err := a.newCommandJobHandler(ctx, definition, contract, target, definition.SensitivePaths)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		t.Fatal(err)
	}
	return definition, registry, handler
}

type reactiveJobTestExecution struct {
	record commandledger.FullRecord
	err    error
}

func waitReactiveJobTestQuestionnaire(t *testing.T, ctx context.Context, service *commandexecution.Service, candidate commandexecution.EnvelopeCandidate, events <-chan map[string]any) (map[string]any, <-chan reactiveJobTestExecution) {
	t.Helper()
	done := make(chan reactiveJobTestExecution, 1)
	go func() {
		record, err := service.ExecuteEnvelope(ctx, "", candidate)
		done <- reactiveJobTestExecution{record: record, err: err}
	}()
	select {
	case prompt := <-events:
		if prompt["id"] == nil {
			t.Fatal("questionário reativo sem id")
		}
		return prompt, done
	case early := <-done:
		t.Fatalf("execução reativa terminou antes da decisão: status=%s err=%v", early.record.Status, early.err)
	case <-ctx.Done():
		t.Fatal("questionário reativo não foi emitido", ctx.Err())
	}
	return nil, nil
}

func TestCommandJobReactiveAppRejectsSameJobDescendantBeforeTool(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 15*time.Second)
	defer cancel()

	parent := startCommandMaintenanceLiveJob(t, a)
	parentRun := <-parent.RunID
	defer func() {
		parent.Release()
		if result, err := parent.Join(); err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
			t.Errorf("fonte reativa: %+v %v", result, err)
		}
	}()
	claim, _ := waitLiveClaimAndLease(t, parentRun)
	if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := jobs.NewDBRepository(database.DB()).GetJob(ctx, parent.JobID)
	if err != nil {
		t.Fatal(err)
	}
	definition, registry, handler := reactiveJobTestDefinition(t, a, ctx, stored.DatabaseID)
	publishReactiveJobTestBinding(t, a, registry, stored.DatabaseID, definition.ID, claim, `{}`)
	service, events := newReactiveJobTestExecutor(t, a, ctx, registry, definition, handler)
	candidate, err := commandPaletteCandidate(uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), definition.ID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	prompt, promptDone := waitReactiveJobTestQuestionnaire(t, ctx, service, candidate, events)
	if err := a.questionnaireMgr.Respond(prompt["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	result := <-promptDone
	if result.err != nil || result.record.Status != commandledger.Failed {
		t.Fatalf("descendente do mesmo job não foi recusado: status=%s err=%v", result.record.Status, result.err)
	}
	runs, err := a.jobMgr.GetJobRunsContext(ctx, stored.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var loopRun *jobs.RunLog
	for i := range runs {
		if runs[i].RunID != parentRun {
			loopRun = &runs[i]
			break
		}
	}
	if loopRun == nil || loopRun.Status != jobs.RunStatusFailed || len(loopRun.RunEvents) != 0 || !strings.Contains(loopRun.Error, "event loop detected") {
		t.Fatalf("anti-loop não deixou run filho falho sem executar a tool: runs=%+v", runs)
	}
}

func TestCommandJobReactiveAppRevalidatesRevokedProfileGrant(t *testing.T) {
	a, fixtureCtx, job, targetSlug, tool := appDynamicProfileFixture(t, dynamicProfileExpression, true)
	ctx, cancel := context.WithTimeout(fixtureCtx, 15*time.Second)
	defer cancel()

	parent := startCommandMaintenanceLiveJob(t, a)
	parentRun := <-parent.RunID
	defer func() {
		parent.Release()
		if result, err := parent.Join(); err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
			t.Errorf("fonte reativa: %+v %v", result, err)
		}
	}()
	claim, _ := waitLiveClaimAndLease(t, parentRun)
	if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	definition, registry, handler := reactiveJobTestDefinition(t, a, ctx, job.DatabaseID)
	publishReactiveJobTestBinding(t, a, registry, job.DatabaseID, definition.ID, claim, `{}`)
	service, events := newReactiveJobTestExecutor(t, a, ctx, registry, definition, handler)
	candidate, err := commandPaletteCandidate(uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), definition.ID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	prompt, promptDone := waitReactiveJobTestQuestionnaire(t, ctx, service, candidate, events)
	if tool.calls.Load() != 0 {
		t.Fatal("tool do profile executou antes da confirmação")
	}
	if err := a.jobGrantStore.Revoke(ctx, job.DatabaseID, targetSlug, "reactive-test-revoke"); err != nil {
		t.Fatal(err)
	}
	if err := a.questionnaireMgr.Respond(prompt["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	result := <-promptDone
	if result.err != nil || result.record.Status != commandledger.Failed {
		t.Fatalf("grant revogado foi aceito na confirmação reativa: status=%s err=%v", result.record.Status, result.err)
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool executou após revogação revalidada: %d", got)
	}
	runs, err := a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("grant revogado criou runs do alvo: %+v", runs)
	}
}

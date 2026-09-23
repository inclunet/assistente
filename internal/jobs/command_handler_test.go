package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type commandHandlerTool struct {
	calls      *atomic.Int32
	retryFirst bool
	onCall     func()
	result     tools.ToolResult
}

func (t commandHandlerTool) Name() string { return "test_tool" }

func (t commandHandlerTool) Description() string { return "command handler test tool" }

func (t commandHandlerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *commandHandlerTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	call := t.calls.Add(1)
	if call == 1 && t.onCall != nil {
		t.onCall()
	}
	if t.retryFirst && call == 1 {
		return tools.ToolResult{Content: `{"retry":true}`, IsError: true, Failure: &tools.ToolFailure{
			Code: "retry_once", Kind: tools.ErrorKindUnavailable, Retryable: true,
		}}, nil
	}
	return t.result, nil
}

func TestCommandHandlerRunsRealJobAndPersistsChainAndToolLedger(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	handle, err := f.handler.Start(f.ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	outcome := <-handle.Done
	if outcome.Status != commandledger.Succeeded {
		t.Fatalf("outcome=%+v", outcome)
	}
	var metadata map[string]string
	if err := json.Unmarshal(outcome.Result, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 3 || metadata["job_id"] != f.job.DatabaseID || metadata["status"] != RunStatusCompleted || metadata["run_id"] == "" {
		t.Fatalf("resultado não é metadata-only: %#v", metadata)
	}

	var run database.JobRun
	if err := f.repo.db.Where("id = ? AND user_id = ?", metadata["run_id"], f.userID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	var provenance map[string]any
	if err := json.Unmarshal([]byte(run.Provenance), &provenance); err != nil {
		t.Fatal(err)
	}
	if _, ok := provenance["_chain_id"]; !ok {
		t.Fatalf("chain id ausente na proveniência: %s", run.Provenance)
	}
	var history []commandcontract.CommandChainEntry
	rawHistory, ok := provenance["command_chain_history"]
	if !ok {
		t.Fatalf("command chain ausente: %s", run.Provenance)
	}
	encodedHistory, _ := json.Marshal(rawHistory)
	if err := json.Unmarshal(encodedHistory, &history); err != nil || len(history) != 1 || history[0].CommandID != f.definition.ID || history[0].InvocationID != f.invocationID || history[0].LayerRefs == nil {
		t.Fatalf("command chain inválida: %s", encodedHistory)
	}

	var inv database.ToolInvocation
	if err := f.repo.db.Where("user_id = ? AND origin_type = ? AND origin_id = ?", f.userID, toolinvocations.OriginJobRun, metadata["run_id"]).First(&inv).Error; err != nil {
		t.Fatal(err)
	}
	if inv.Status != toolinvocations.StatusSucceeded || inv.ToolCatalogID == "" {
		t.Fatalf("ledger da tool inválido: %+v", inv)
	}
}

func TestCommandHandlerConstructorRejectsReadAndWriteContracts(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	for _, effect := range []commandcatalog.Effect{commandcatalog.Read, commandcatalog.Write} {
		definition := f.definition
		definition.Effect = effect
		contract := commandcatalog.HandlerContract{Effect: effect, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
		if _, err := f.manager.CommandHandler(CommandHandlerConfig{Definition: definition, Contract: contract, Target: CommandJobTarget{DatabaseID: f.job.DatabaseID, Slug: f.job.ID, DefinitionFingerprint: f.definitionFingerprint}, Authorize: f.authorize}); !errors.Is(err, ErrCommandJobDenied) {
			t.Fatalf("effect=%s err=%v", effect, err)
		}
	}
}

func TestCommandHandlerRedactsDelegatedToolPathsInRealLedger(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	f.job.Inputs = map[string]any{"payload_value": "private-input", "visible": "kept"}
	f.tool.result = tools.ToolResult{Content: `{"payload_value":"private-output","visible":"kept"}`}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	var err error
	f.job, err = f.repo.GetJob(f.ctx, f.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: f.definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	handler, err := f.manager.CommandHandler(CommandHandlerConfig{
		Definition: f.definition, Contract: contract, Authorize: f.authorize,
		Target:             CommandJobTarget{DatabaseID: f.job.DatabaseID, Slug: f.job.ID, DefinitionFingerprint: f.definitionFingerprint},
		ToolSensitivePaths: commandcatalog.SensitivePaths{Input: []string{"/payload_value"}, Output: []string{"/payload_value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := handler.Start(f.ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("delegação: %+v", outcome)
		}
	case <-time.After(5 * time.Second):
		handle.Cancel()
		t.Fatal("timeout aguardando delegação")
	}
	var row database.ToolInvocation
	if err := f.repo.db.Where("user_id = ? AND origin_type = ?", f.userID, toolinvocations.OriginJobRun).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{row.Input, row.Output} {
		if strings.Contains(payload, "private-input") || strings.Contains(payload, "private-output") || !strings.Contains(payload, "kept") {
			t.Fatalf("redação delegada incorreta: %s", payload)
		}
	}
}

func TestCommandHandlerRejectsForeignSystemStaleAndAuthorizerBeforeTool(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*commandHandlerFixture, *commandexecution.Invocation) context.Context
	}{
		{name: "foreign owner", mutate: func(f *commandHandlerFixture, in *commandexecution.Invocation) context.Context {
			foreign := uuid.Must(uuid.NewV7()).String()
			in.Principal.UserID = foreign
			in.Envelope.UserID = &foreign
			in.Envelope.ActorID = foreign
			return database.WithUserID(context.Background(), foreign)
		}},
		{name: "system source", mutate: func(_ *commandHandlerFixture, in *commandexecution.Invocation) context.Context {
			in.Source = commandcatalog.System
			source := commandcontract.SourceSystem
			in.Envelope.SourceType = &source
			return context.Background()
		}},
		{name: "stale definition", mutate: func(f *commandHandlerFixture, _ *commandexecution.Invocation) context.Context {
			f.job.Inputs = map[string]any{"changed_after_bootstrap": true}
			if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
				panic(err)
			}
			return nil
		}},
		{name: "authorizer", mutate: func(f *commandHandlerFixture, _ *commandexecution.Invocation) context.Context {
			f.authorize = func(context.Context, commandexecution.Invocation, *Job) error { return ErrCommandJobDenied }
			f.rebuildHandler()
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandHandlerFixture(t, nil)
			in := f.invocation(f.definition, f.job)
			startCtx := tc.mutate(f, &in)
			if startCtx == nil {
				startCtx = f.ctx
			}
			handle, err := f.handler.Start(startCtx, in)
			if tc.name == "system source" {
				if err == nil {
					t.Fatal("origem system deveria ser recusada no ingresso")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			outcome := <-handle.Done
			if outcome.Status == commandledger.Succeeded || f.calls.Load() != 0 {
				t.Fatalf("rejeição alcançou execução: outcome=%+v calls=%d", outcome, f.calls.Load())
			}
			var runs int64
			if err := f.repo.db.Model(&database.JobRun{}).Where("user_id = ?", f.userID).Count(&runs).Error; err != nil {
				t.Fatal(err)
			}
			if runs != 0 {
				t.Fatalf("rejeição criou runs: %d", runs)
			}
		})
	}
}

func TestCommandHandlerEpochChangeBeforeQueuedCreatesNoRun(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	var authorizeCalls atomic.Int32
	f.authorize = func(ctx context.Context, _ commandexecution.Invocation, _ *Job) error {
		if authorizeCalls.Add(1) == 2 {
			return f.epochs.InvalidateSession(context.Background(), f.userID, f.sessionID)
		}
		return nil
	}
	f.rebuildHandler()
	handle, err := f.handler.Start(f.ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("epoch revogada produziu sucesso: %+v", outcome)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("tool executada após mudança de epoch: %d", got)
	}
	var runs int64
	if err := f.repo.db.Model(&database.JobRun{}).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("mudança de epoch antes do queued criou runs: %d", runs)
	}
}

func TestCommandHandlerAuthorizationRevokedBeforeRetryDoesNotCallToolTwice(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	f.tool.retryFirst = true
	f.job.ErrorPolicy = ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	f.job, _ = f.repo.GetJob(f.ctx, f.job.ID)
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	var authorizeCalls atomic.Int32
	f.authorize = func(context.Context, commandexecution.Invocation, *Job) error {
		if authorizeCalls.Add(1) >= 4 {
			return ErrCommandJobDenied
		}
		return nil
	}
	f.rebuildHandler()
	handle, err := f.handler.Start(f.ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatal("autorização revogada antes do retry produziu sucesso")
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("tool foi chamada %d vezes; esperado exatamente 1", got)
	}
}

func TestCommandHandlerEpochRevokedDuringRetryDoesNotCallToolTwice(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	f.tool.retryFirst = true
	f.job.ErrorPolicy = ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	f.job, _ = f.repo.GetJob(f.ctx, f.job.ID)
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	f.tool.onCall = func() { _ = f.epochs.InvalidateSession(context.Background(), f.userID, f.sessionID) }
	f.rebuildHandler()
	handle, err := f.handler.Start(f.ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status == commandledger.Succeeded {
			t.Fatalf("epoch revogado permitiu retry: %+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout aguardando rejeição do retry")
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("tool foi chamada %d vezes após revogação: %d", got, f.calls.Load())
	}
}

func TestCommandHandlerEventChildMasksParentDispatchAndKeepsPrivateOrigin(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	child := testRepositoryJob("command-handler-event-child", "Command handler event child")
	child.Triggers = []Trigger{{Type: TriggerEvent, Listen: "command.parent.completed"}}
	child.Inputs = map[string]any{"payload_value": "child-secret"}
	if err := f.repo.SaveJob(f.ctx, child); err != nil {
		t.Fatal(err)
	}
	var childTriggers []database.JobTrigger
	if err := f.repo.db.Where("job_id = ? AND type = ?", child.DatabaseID, string(TriggerEvent)).Find(&childTriggers).Error; err != nil {
		t.Fatal(err)
	}
	if len(childTriggers) != 1 {
		t.Fatalf("trigger de evento não persistido: %+v", childTriggers)
	}
	child, err := f.repo.GetJob(f.ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	history, _ := json.Marshal([]map[string]any{{"command_id": "jobs.parent", "invocation_id": uuid.Must(uuid.NewV7()).String(), "layer_refs": []string{}}})
	origin := commandEventOrigin{userID: f.userID, rootType: "internal_event", rootID: "parent-run", chainID: "parent-chain", history: []string{"parent-run"}, commandHistory: history, hasCommandHistory: true}
	ctx := context.WithValue(f.ctx, commandEventOriginKey{}, origin)
	trigger := &TriggerContext{Type: TriggerEvent, EventName: "command.parent.completed", EventPayload: map[string]any{"spoof": true}}
	inheritCommandEventOrigin(ctx, trigger)
	ctx = context.WithValue(ctx, commandJobDispatchKey{}, commandJobDispatch{jobID: f.job.DatabaseID, paths: commandcatalog.SensitivePaths{Input: []string{"/payload_value"}}, revalidate: func(context.Context) error { return ErrCommandJobDenied }})
	run := f.manager.executor.Execute(ctx, child, trigger)
	if run == nil || run.Status != RunStatusCompleted {
		t.Fatalf("filho não executou: %+v", run)
	}
	if run.RootOriginType != "internal_event" || run.RootOriginID != "parent-run" {
		t.Fatalf("origem privada não preservada: %q/%q", run.RootOriginType, run.RootOriginID)
	}
	var childInvocation database.ToolInvocation
	if err := f.repo.db.Where("origin_type = ? AND origin_id = ?", toolinvocations.OriginJobRun, run.RunID).First(&childInvocation).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(childInvocation.Input, "child-secret") {
		t.Fatalf("dispatch do pai contaminou a persistência do filho: %s", childInvocation.Input)
	}
}

func TestCommandHandlerRejectsPreexistingPrivateOrigin(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	ctx := context.WithValue(f.ctx, commandEventOriginKey{}, commandEventOrigin{userID: f.userID, history: []string{"ancestor-run"}})
	handle, err := f.handler.Start(ctx, f.invocation(f.definition, f.job))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("origem privada preexistente reiniciou raiz: %+v", outcome)
	}
	if f.calls.Load() != 0 {
		t.Fatal("tool executada apesar da origem privada preexistente")
	}
}

type commandHandlerFixture struct {
	t                               *testing.T
	repo                            *DBRepository
	ctx                             context.Context
	userID, sessionID, invocationID string
	epochs                          *commandsecurity.EpochService
	epoch                           commandsecurity.EpochSnapshot
	job                             *Job
	definition                      commandcatalog.Definition
	definitionFingerprint           string
	handler                         commandexecution.Handler
	manager                         *Manager
	authorize                       func(context.Context, commandexecution.Invocation, *Job) error
	calls                           atomic.Int32
	tool                            *commandHandlerTool
}

func newCommandHandlerFixture(t *testing.T, authorize func(context.Context, commandexecution.Invocation, *Job) error) *commandHandlerFixture {
	t.Helper()
	repo, _, _ := setupJobsRepositoryTest(t)
	userID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	ctx := database.WithUserID(context.Background(), userID)
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(ctx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	f := &commandHandlerFixture{t: t, repo: repo, ctx: ctx, userID: userID, sessionID: sessionID, invocationID: uuid.Must(uuid.NewV7()).String(), epochs: epochs, epoch: epoch}
	f.tool = &commandHandlerTool{calls: &f.calls, result: tools.ToolResult{Content: `{"ok":true}`, Metadata: map[string]any{"note": "runtime-only"}}}
	registry := tools.NewRegistry()
	registry.MustRegister(f.tool)
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	f.job = testRepositoryJob("command-handler-job", "Command handler job")
	if err := repo.SaveJob(ctx, f.job); err != nil {
		t.Fatal(err)
	}
	f.job, err = repo.GetJob(ctx, f.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.definition = completeCommandJobDefinition()
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	if authorize == nil {
		f.authorize = func(ctx context.Context, _ commandexecution.Invocation, job *Job) error {
			owner, err := database.RequireUserID(ctx)
			if err != nil || owner != f.userID || job.DatabaseID != f.job.DatabaseID {
				return ErrCommandJobDenied
			}
			return nil
		}
	} else {
		f.authorize = authorize
	}
	f.manager = NewManager(ManagerConfig{Repository: repo, ToolRegistry: registry, ToolInvocations: invocations, ContextProvider: func() context.Context { return ctx }, CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
		watched, release, err := epochs.WatchSecurityEpoch(ctx, epoch)
		if err != nil {
			return commandjobactivation.RuntimeIdentity{}, nil, nil, err
		}
		return commandjobactivation.RuntimeIdentity{UserID: userID, AuthContextType: string(commandcontract.AuthLocalSession), AuthContextID: sessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, watched, release, nil
	}})
	t.Cleanup(f.manager.Stop)
	f.rebuildHandler()
	return f
}

func (f *commandHandlerFixture) rebuildHandler() {
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: f.definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	target := CommandJobTarget{DatabaseID: f.job.DatabaseID, Slug: f.job.ID, DefinitionFingerprint: f.definitionFingerprint}
	if err := commandcatalog.ValidateDefinitionComplete(f.definition, contract); err != nil {
		f.t.Fatalf("definition inválida: %v", err)
	}
	var err error
	f.handler, err = f.manager.CommandHandler(CommandHandlerConfig{Definition: f.definition, Contract: contract, Target: target, Authorize: f.authorize, ToolSensitivePaths: f.definition.SensitivePaths})
	if err != nil {
		f.t.Fatalf("CommandHandler: %v", err)
	}
}

func (f *commandHandlerFixture) invocation(def commandcatalog.Definition, job *Job) commandexecution.Invocation {
	commandID := def.ID
	source := commandcontract.SourcePalette
	user := f.userID
	provenance := json.RawMessage(fmt.Sprintf(`{"_chain_id":%q,"_chain_history":[],"command_chain_history":[{"command_id":%q,"invocation_id":%q,"layer_refs":[]}]}`, f.invocationID, commandID, f.invocationID))
	decision := uuid.Must(uuid.NewV7()).String()
	return commandexecution.Invocation{ID: f.invocationID, CorrelationID: f.invocationID, CommandID: commandID, Principal: auth.LocalSessionPrincipal{UserID: f.userID, SessionID: f.sessionID}, Source: commandcatalog.Palette, Envelope: &commandcontract.Envelope{Version: 1, InvocationID: f.invocationID, CommandID: &commandID, UserID: &user, AuthContextType: commandcontract.AuthLocalSession, AuthContextID: f.sessionID, AuthGeneration: f.epoch.AuthGeneration, SecurityGeneration: f.epoch.SecurityGeneration, AuthorizationDecisionID: &decision, ActorType: commandcontract.ActorUser, ActorID: f.userID, SourceType: &source, BindingIDs: []string{}, RegistryVersion: "registry:1", Provenance: &provenance, CorrelationID: f.invocationID, ReceivedAt: time.Now().UTC()}}
}

func completeCommandJobDefinition() commandcatalog.Definition {
	locales := map[string]commandcatalog.LocalizedMetadata{"pt-BR": {Name: "Delegar job", Description: "Executa um job", Category: "Testes"}, "en": {Name: "Delegate job", Description: "Run a job", Category: "Tests"}, "es": {Name: "Delegar job", Description: "Ejecuta un job", Category: "Pruebas"}}
	resultSchema := &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"job_id": {Type: commandcatalog.SchemaString}, "run_id": {Type: commandcatalog.SchemaString}, "status": {Type: commandcatalog.SchemaString}}, Required: []string{"job_id", "run_id", "status"}}
	return commandcatalog.Definition{ID: "jobs.command_delegate", Effect: commandcatalog.Destructive, Decision: commandcatalog.Interactive, HasMutableTarget: true, AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales}, ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"payload_value": {Type: commandcatalog.SchemaString}}}, ResultSchema: resultSchema, Risk: commandcatalog.RiskHigh, SensitivePaths: commandcatalog.SensitivePaths{Input: []string{"/payload_value"}}, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted}, Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: "jobs/command", HandlerClassification: commandcatalog.HandlerJob}
}

func mustDefinitionFingerprint(t *testing.T, job *Job) string {
	t.Helper()
	value, err := DefinitionFingerprint(job)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

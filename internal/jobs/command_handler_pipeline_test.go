package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
)

type commandHandlerPipelineTool struct{ calls atomic.Int32 }

func (t *commandHandlerPipelineTool) Name() string { return "test_tool" }

func (t *commandHandlerPipelineTool) Description() string { return "pipeline fixture tool" }

func (t *commandHandlerPipelineTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *commandHandlerPipelineTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	return tools.ToolResult{Content: `{"ok":true}`, Metadata: map[string]any{"correlation": "job-pipeline"}}, nil
}

func TestCommandHandlerPipelinePersistsOneRunAndDoesNotReplay(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	db := repo.db
	if err := db.AutoMigrate(&database.Session{}); err != nil {
		t.Fatalf("automigrate session: %v", err)
	}
	if err := commandledger.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate command ledger: %v", err)
	}

	userID := uuid.Must(uuid.NewV7()).String()
	user := &database.User{UUIDModel: database.UUIDModel{ID: userID}, Username: "command-pipeline", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	ctx := database.WithUserID(context.Background(), userID)
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: []byte("command-handler-pipeline-pepper-32-bytes")})
	if err != nil {
		t.Fatalf("session service: %v", err)
	}
	pair, err := sessions.IssueSession(context.Background(), user, "command-pipeline")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	gate := &commandsecurity.DispatchGate{}
	epochs, err := commandsecurity.NewEpochService(gate)
	if err != nil {
		t.Fatalf("epoch service: %v", err)
	}
	tool := &commandHandlerPipelineTool{}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	invocationRepo := toolinvocations.NewDBRepository(db)
	invocationService := toolinvocations.NewService(invocationRepo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	job := testRepositoryJob("command-pipeline-job", "Command pipeline job")
	job.Inputs = map[string]any{"query": "pipeline"}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	authoritativeJob, err := repo.GetJobByID(ctx, job.DatabaseID)
	if err != nil || authoritativeJob == nil {
		t.Fatalf("reload job by database ID: %v", err)
	}

	manager := NewManager(ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocationService,
		ContextProvider: func() context.Context { return ctx },
		CommandRuntimeIdentity: func(captureCtx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			epoch, err := epochs.Capture(captureCtx, userID, pair.SessionID)
			if err != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, err
			}
			watched, release, err := epochs.WatchSecurityEpoch(captureCtx, epoch)
			return commandjobactivation.RuntimeIdentity{
				UserID: userID, AuthContextType: string(commandcontract.AuthLocalSession), AuthContextID: pair.SessionID,
				AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
			}, watched, release, err
		},
	})
	t.Cleanup(manager.Stop)

	commandID := "jobs.command_pipeline"
	definition := commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Destructive, Decision: commandcatalog.Interactive,
		HasMutableTarget: true,
		AllowedSources:   []commandcatalog.Source{commandcatalog.Palette},
		Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
			Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion,
		}}},
		Presentation: &commandcatalog.Presentation{Version: "test-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Pipeline", Description: "Pipeline", Category: "Testes"},
			"en":    {Name: "Pipeline", Description: "Pipeline", Category: "Tests"},
			"es":    {Name: "Pipeline", Description: "Pipeline", Category: "Pruebas"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"job_id": {Type: commandcatalog.SchemaString},
			"run_id": {Type: commandcatalog.SchemaString},
			"status": {Type: commandcatalog.SchemaString},
		}, Required: []string{"job_id", "run_id", "status"}},
		Risk: commandcatalog.RiskHigh, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "internal/jobs/command_pipeline", HandlerClassification: commandcatalog.HandlerJob,
	}
	contract := commandcatalog.HandlerContract{Effect: definition.Effect, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	fingerprint, err := DefinitionFingerprint(authoritativeJob)
	if err != nil {
		t.Fatalf("job fingerprint: %v", err)
	}
	handler, err := manager.CommandHandler(CommandHandlerConfig{Definition: definition, Contract: contract, Target: CommandJobTarget{DatabaseID: authoritativeJob.DatabaseID, Slug: authoritativeJob.ID, DefinitionFingerprint: fingerprint}, Authorize: func(context.Context, commandexecution.Invocation, *Job) error { return nil }})
	if err != nil {
		t.Fatalf("command handler: %v", err)
	}
	commandRegistry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		t.Fatalf("command registry: %v", err)
	}

	state, err := commandexecution.NewHostState(epochs, "registry-v1")
	if err != nil {
		t.Fatalf("host state: %v", err)
	}
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatalf("bindings: %v", err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatalf("os state: %v", err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatalf("vault state: %v", err)
	}
	if err := state.PublishUserConfiguration(ctx, userID, bindings); err != nil {
		t.Fatalf("publish bindings: %v", err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: userID, SessionID: pair.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return bindings, nil, nil
	}); err != nil {
		t.Fatalf("rebuild bindings: %v", err)
	}
	contextBus, err := commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{
		"workspace": commandcontext.ScopedProviderFunc(func(_ context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
			if fact != "active_tab" {
				return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
			}
			return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{Version: "active-tab-v1", CapturedAt: time.Now()})
		}),
	})
	if err != nil {
		t.Fatalf("fact bus: %v", err)
	}

	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatalf("command store: %v", err)
	}
	if err := commanddecision.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate decision store: %v", err)
	}
	decisionPresenter := &commandHandlerPipelineDecisionPresenter{}
	decisions, err := commanddecision.New(db, decisionPresenter, time.Now)
	if err != nil {
		t.Fatalf("decision store: %v", err)
	}
	service, err := commandexecution.NewComplete(commandexecution.Config{
		Envelope: &commandexecution.EnvelopeConfig{
			Context:   contextBus,
			Decisions: decisions, DecisionTTL: time.Minute,
			DecisionBody: func(commandcatalog.Definition, commandcontract.Envelope) (string, error) {
				return `{"version":1,"confirm":true}`, nil
			},
			Snapshot: func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
				versions, err := state.Snapshot(ctx, principal)
				if err != nil {
					return commandcontract.Envelope{}, err
				}
				return commandcontract.Envelope{RegistryVersion: versions.Registry, GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers, CorrelationID: candidate.CorrelationID}, nil
			},
			Resolve: func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
				return commandexecution.EnvelopeResolution{}, errors.New("resolve não esperado")
			},
			Authorize: func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error {
				return nil
			},
			AuthorizeLookup: func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error { return nil },
			Actor: func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
				return commandcontract.ActorUser, userID, nil
			},
		},
		Registry: commandRegistry, RegistryVersion: "registry-v1", Handlers: map[string]commandexecution.Handler{commandID: handler}, Source: commandcatalog.Palette,
		Sessions: sessions, Epochs: epochs, Store: store, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x61}, 32), nil }, KeyVersion: "v1",
		Now: time.Now, Retention: time.Minute, ExecutionTimeout: 5 * time.Second, FinalizationTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewComplete: %v", err)
	}
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

	invocationID := uuid.Must(uuid.NewV7()).String()
	candidate := commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: commandID, Arguments: json.RawMessage(`{}`)}
	record, err := service.ExecuteEnvelope(ctx, pair.AccessToken, candidate)
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("pipeline: status=%s err=%v", record.Status, err)
	}
	if record.InvocationID != invocationID || record.Envelope.CorrelationID != candidate.CorrelationID {
		t.Fatalf("ledger perdeu correlação: invocation=%q correlation=%q", record.InvocationID, record.Envelope.CorrelationID)
	}
	if record.Envelope.AuthorizationDecisionID == nil || *record.Envelope.AuthorizationDecisionID == "" {
		t.Fatal("ledger não preservou authorization_decision_id")
	}
	var consumed int64
	if err := db.Table("command_decision_receipts").Where("decision_id = ? AND status = ?", *record.Envelope.AuthorizationDecisionID, "consumed").Count(&consumed).Error; err != nil || consumed != 1 {
		t.Fatalf("receipt não consumido exatamente uma vez: count=%d err=%v", consumed, err)
	}
	if decisionPresenter.calls.Load() != 1 {
		t.Fatalf("presenter chamado %d vezes após execução", decisionPresenter.calls.Load())
	}
	if tool.calls.Load() != 1 {
		t.Fatalf("tool calls=%d, esperado 1", tool.calls.Load())
	}

	runs, err := repo.GetRuns(ctx, job.ID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%d err=%v, esperado um run", len(runs), err)
	}
	history, ok := runs[0].Provenance["command_chain_history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("proveniência de cadeia ausente: %#v", runs[0].Provenance)
	}
	invocations, err := invocationRepo.List(ctx, toolinvocations.Filter{OriginType: toolinvocations.OriginJobRun, OriginID: runs[0].RunID, Limit: 10})
	if err != nil || len(invocations) != 1 || invocations[0].OriginType != toolinvocations.OriginJobRun {
		t.Fatalf("tool invocations=%d err=%v, esperado uma OriginJobRun", len(invocations), err)
	}
	if !strings.Contains(string(invocations[0].Output), "job-pipeline") {
		t.Fatalf("metadata de resultado ausente na invocação: %s", invocations[0].Output)
	}

	replayed, err := service.ExecuteEnvelope(ctx, pair.AccessToken, candidate)
	if err != nil || replayed.Status != commandledger.Succeeded {
		t.Fatalf("replay: status=%s err=%v", replayed.Status, err)
	}
	if tool.calls.Load() != 1 {
		t.Fatalf("replay executou a tool novamente: %d chamadas", tool.calls.Load())
	}
	if decisionPresenter.calls.Load() != 1 {
		t.Fatalf("replay abriu nova decisão: presenter=%d", decisionPresenter.calls.Load())
	}
	if replayRuns, err := repo.GetRuns(ctx, authoritativeJob.ID, 10); err != nil || len(replayRuns) != 1 {
		t.Fatalf("replay criou novo job_run: count=%d err=%v", len(replayRuns), err)
	}
}

type commandHandlerPipelineDecisionPresenter struct{ calls atomic.Int32 }

func (p *commandHandlerPipelineDecisionPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	p.calls.Add(1)
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

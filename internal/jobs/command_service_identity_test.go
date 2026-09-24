package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandidentity"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type commandServiceIdentityTool struct {
	entered     chan context.Context
	release     chan struct{}
	done        chan struct{}
	releaseOnce sync.Once
	onCall      func(context.Context) error
}

func (t *commandServiceIdentityTool) Name() string { return jobprofilegrant.ToolSubagent }

func (t *commandServiceIdentityTool) Description() string {
	return "tool de identidade de job do teste"
}

func (t *commandServiceIdentityTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"profile":{"type":"string"}}}`)
}

func (t *commandServiceIdentityTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	defer close(t.done)
	if t.onCall != nil {
		if err := t.onCall(ctx); err != nil {
			return tools.ToolResult{}, err
		}
	}
	t.entered <- ctx
	<-t.release
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func (t *commandServiceIdentityTool) unblock() {
	t.releaseOnce.Do(func() { close(t.release) })
}

func TestCommandJobServiceIdentityUsesLiveRunMarkerAndExactGrant(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userID := uuid.Must(uuid.NewV7()).String()
	ownerCtx := database.WithUserID(context.Background(), userID)
	sessionID := uuid.Must(uuid.NewV7()).String()
	gate := &commandsecurity.DispatchGate{}
	epochs, err := commandsecurity.NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(ownerCtx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	probe := &commandServiceIdentityTool{entered: make(chan context.Context, 1), release: make(chan struct{}), done: make(chan struct{})}
	registry.MustRegister(probe)
	if err := repo.db.Create(&database.ToolCatalog{
		Name: "subagent", DisplayName: "subagent", Origin: "builtin", AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	grants := jobprofilegrant.NewStore(repo.db)

	manager := NewManager(ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocations, JobProfileGrants: grants,
		ContextProvider: func() context.Context { return ownerCtx },
		CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			watched, release, watchErr := epochs.WatchSecurityEpoch(ctx, epoch)
			if watchErr != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, watchErr
			}
			return commandjobactivation.RuntimeIdentity{UserID: userID, AuthContextType: string(commandcontract.AuthLocalSession), AuthContextID: sessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, watched, release, nil
		},
	})
	t.Cleanup(manager.Stop)

	job := &Job{ID: "command-service-identity", Name: "Command service identity", Tool: jobprofilegrant.ToolSubagent, Inputs: map[string]any{"profile": "profile-a"}, Triggers: []Trigger{{Type: TriggerManual}}, ErrorPolicy: ErrorPolicy{Strategy: ErrorStop}}
	if err := manager.CreateJobContext(ownerCtx, job); err != nil {
		t.Fatal(err)
	}
	job, err = repo.GetJob(ownerCtx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := jobprofilegrant.Fingerprint(jobprofilegrant.ToolSubagent, "profile-a")
	snapshot, err := grants.AuthorizationSnapshot(ownerCtx, job.DatabaseID, "profile-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.Grant(ownerCtx, job.DatabaseID, "profile-a", fingerprint, "test", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		probe.unblock()
		select {
		case <-probe.done:
		case <-time.After(5 * time.Second):
			t.Error("tool do run não encerrou após release")
		}
	})

	epochPort, err := commandidentity.NewCoreEpochs(epochs)
	if err != nil {
		t.Fatal(err)
	}
	identityService, err := commandidentity.New(commandidentity.Config{
		Epochs: epochPort, JobRuntime: manager, JobGrants: grants,
		AuthorizationRules: []commandidentity.AuthorizationRule{{CommandID: "jobs.service_probe", Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := commandcatalog.Definition{ID: "jobs.service_probe", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, AllowedSources: []commandcatalog.Source{commandcatalog.Event}, HandlerClassification: commandcatalog.HandlerTool}
	var callbackOnce sync.Once
	var callbackErr error
	probe.onCall = func(ctx context.Context) error {
		callbackOnce.Do(func() {
			if err := repo.db.Exec("PRAGMA query_only = ON").Error; err != nil {
				callbackErr = err
				return
			}
			defer func() {
				if err := repo.db.Exec("PRAGMA query_only = OFF").Error; callbackErr == nil && err != nil {
					callbackErr = err
				}
			}()
			request, requestErr := manager.CommandJobServiceRequest(ctx, commandcatalog.Event)
			if requestErr != nil {
				callbackErr = requestErr
				return
			}
			identity, resolveErr := identityService.ResolveJobService(ctx, request)
			if resolveErr != nil {
				callbackErr = resolveErr
				return
			}
			if identity.Job == nil {
				callbackErr = errors.New("identidade de job não retornou trusted job")
				return
			}
			if err := manager.RevalidateCommandJob(ctx, request.Capability, *identity.Job); err != nil {
				callbackErr = err
				return
			}
			// A revalidação final pode ocorrer com o gate já adquirido. Não
			// pode recapturar epochs e tentar entrar novamente no mesmo gate.
			callbackErr = gate.WithMutation(ctx, func() error {
				return identityService.Authorize(ctx, commandidentity.AuthorizationRequest{Identity: identity, Definition: definition, JobService: &request})
			})
		})
		return callbackErr
	}

	resultCh := make(chan *RunLog, 1)
	runDone := make(chan struct{})
	t.Cleanup(func() {
		probe.unblock()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Error("worker não encerrou durante cleanup")
		}
	})
	go func() {
		defer close(runDone)
		run, runErr := manager.RunJobContext(ownerCtx, job.ID)
		if runErr != nil {
			resultCh <- &RunLog{Status: RunStatusFailed, Error: runErr.Error()}
			return
		}
		resultCh <- run
	}()
	var liveCtx context.Context
	select {
	case liveCtx = <-probe.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("tool real não recebeu contexto privado do run")
	}
	if callbackErr != nil {
		probe.unblock()
		t.Fatalf("identidade inicial: %v", callbackErr)
	}
	request, err := manager.CommandJobServiceRequest(liveCtx, commandcatalog.Event)
	if err != nil {
		t.Fatal(err)
	}
	newCapability := commandidentity.JobServiceCapabilityForRuntime()
	forgedRequest := request
	forgedRequest.Capability = newCapability
	if _, err := manager.ResolveCommandJob(liveCtx, newCapability, forgedRequest); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatalf("capability nova reutilizou IDs: %v", err)
	}
	forgedRun := request
	forgedRun.RunID = request.RunID + "-foreign"
	if _, err := manager.ResolveCommandJob(liveCtx, request.Capability, forgedRun); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatalf("run forjado reutilizou capability: %v", err)
	}
	foreignManager := NewManager(ManagerConfig{Repository: repo, ToolRegistry: registry, ToolInvocations: invocations, JobProfileGrants: grants, ContextProvider: func() context.Context { return ownerCtx }})
	t.Cleanup(foreignManager.Stop)
	if _, err := foreignManager.ResolveCommandJob(liveCtx, request.Capability, request); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatalf("manager estrangeiro aceitou marker: %v", err)
	}
	foreignCtx := database.WithUserID(liveCtx, uuid.Must(uuid.NewV7()).String())
	if _, err := manager.ResolveCommandJob(foreignCtx, request.Capability, request); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatalf("owner estrangeiro aceitou marker: %v", err)
	}
	originalJob, err := manager.PrepareCommandJob(liveCtx, job.DatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	originalFingerprint, err := DefinitionFingerprint(originalJob)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.db.Model(&database.Job{}).Where("id = ?", job.DatabaseID).Update("inputs", `{"profile":"profile-b"}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveCommandJob(liveCtx, request.Capability, request); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatal("definição alterada aceitou fingerprint antigo")
	}
	if err := repo.db.Model(&database.Job{}).Where("id = ?", job.DatabaseID).Update("inputs", `{"profile":"profile-a"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := grants.Revoke(ownerCtx, job.DatabaseID, "profile-a", "test-revoke"); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&database.Job{}).Where("id = ?", job.DatabaseID).Update("enabled", originalJob.Enabled).Error; err != nil {
		t.Fatal(err)
	}
	latest, err := grants.AuthorizationSnapshot(ownerCtx, job.DatabaseID, "profile-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.Grant(ownerCtx, job.DatabaseID, "profile-a", fingerprint, "test-regrant", latest.Generation); err != nil {
		t.Fatal(err)
	}
	restoredJob, err := manager.PrepareCommandJob(liveCtx, job.DatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	if restored, err := DefinitionFingerprint(restoredJob); err != nil || restored != originalFingerprint {
		t.Fatalf("regrant não isolou a geração: fingerprint=%s original=%s err=%v", restored, originalFingerprint, err)
	}
	if valid, err := grants.HasValidGeneration(ownerCtx, job.DatabaseID, "profile-a", fingerprint, latest.Generation); err != nil || !valid {
		t.Fatalf("grant novo não está válido: valid=%v err=%v", valid, err)
	}
	if _, err := manager.ResolveCommandJob(liveCtx, request.Capability, request); !errors.Is(err, commandidentity.ErrJobExecutionDenied) {
		t.Fatal("regrant aceitou geração antiga")
	}

	probe.unblock()
	var run *RunLog
	select {
	case run = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("run real não encerrou após release")
	}
	if run.Status != RunStatusCompleted {
		t.Fatalf("run real não terminou completed: %+v", run)
	}
	if _, err := manager.CommandJobServiceRequest(context.WithoutCancel(liveCtx), commandcatalog.Event); !errors.Is(err, errCommandJobServiceUnavailable) {
		t.Fatalf("run terminal continuou emitindo request: %v", err)
	}
}

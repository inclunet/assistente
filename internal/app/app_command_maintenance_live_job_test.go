package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandledger"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

const liveCommandTool = "test.command_maintenance_live_block"

type liveCommandToolImpl struct {
	name        string
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func (t *liveCommandToolImpl) Name() string {
	if t.name != "" {
		return t.name
	}
	return liveCommandTool
}

func (t *liveCommandToolImpl) Description() string {
	return "tool de teste bloqueada até o controle liberá-la"
}

func (t *liveCommandToolImpl) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *liveCommandToolImpl) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	t.startOnce.Do(func() { close(t.started) })
	select {
	case <-t.release:
		return tools.ToolResult{Content: `{"ok":true}`}, nil
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	}
}

type liveCommandJobControl struct {
	JobID   string
	RunID   <-chan string
	Started <-chan struct{}
	Release func()
	Join    func() (*jobs.RunLog, error)
}

type liveCommandDecisionPresenter struct{}

func (liveCommandDecisionPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

// startCommandMaintenanceLiveJob monta o mesmo caminho produtivo usado pelo
// App: layer, regra, grant HMAC, ledger de tool, Manager.Start e manutenção.
// O único efeito externo é o tool bloqueado, liberado pelo controle retornado.
func startCommandMaintenanceLiveJob(t *testing.T, a *App) liveCommandJobControl {
	t.Helper()
	return startCommandJobWithCondition(t, a, `{"version":1,"clauses":[]}`)
}

func startCommandJobWithCondition(t *testing.T, a *App, condition string) liveCommandJobControl {
	return startCommandJobWithConditionNamed(t, a, condition, liveCommandTool)
}

func startCommandJobWithConditionNamed(t *testing.T, a *App, condition, toolName string) liveCommandJobControl {
	t.Helper()
	if a == nil || a.currentUserID == "" || a.currentAuthUser == nil {
		t.Fatal("fixture sem usuário autenticado")
	}
	db := database.DB()
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatalf("migrar automação antes do job: %v", err)
	}

	tool := &liveCommandToolImpl{name: toolName, started: make(chan struct{}), release: make(chan struct{})}
	if a.toolRegistry == nil || a.toolInvocationSvc == nil || a.jobMgr == nil {
		t.Fatal("App sem registry, ledger de tools ou Manager real previamente montado")
	}
	a.toolRegistry.MustRegister(tool)

	layerID := uuid.Must(uuid.NewV7()).String()
	ruleID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	if err := db.Create(&commandconfig.Layer{ID: layerID, UserID: a.currentUserID, Name: "live-job-layer-" + layerID, Description: "camada de teste do job vivo", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("criar camada do job: %v", err)
	}
	eventName := commandautomation.JobRunStateEvent
	producerTypes := `["jobs.runtime"]`
	rule := commandactivation.Rule{ID: ruleID, UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: condition, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producerTypes, Enabled: false, Source: "user", ReviewStatus: "active"}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("criar regra do job: %v", err)
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
	owner := commandautomation.Owner{UserID: a.currentUserID}
	change, err := automation.PrepareRule(ctx, owner, ruleID)
	if err != nil {
		t.Fatalf("preparar regra do job: %v", err)
	}
	keys, err := commandautomationKeyProvider(a)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := automation.ConfirmGrant(ctx, change, epoch, decisionStore, a.commandStorageVersion, keys, now.Add(time.Hour), func(commandautomation.Rule) (string, error) { return "autorizar job vivo", nil })
	if err != nil {
		t.Fatalf("confirmar grant do job: %v", err)
	}
	if err := automation.CommitConfirmedGrant(ctx, confirmed, epoch); err != nil {
		t.Fatalf("persistir grant do job: %v", err)
	}

	jobID := uuid.Must(uuid.NewV7()).String()
	catalogID := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: catalogID}, Name: toolName, DisplayName: toolName, Description: "tool bloqueada do teste", Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatalf("criar tool do job: %v", err)
	}
	job := &jobs.Job{ID: jobID, Name: "live-command-maintenance-" + jobID, Description: "job vivo de teste", Tool: toolName, Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatalf("criar job: %v", err)
	}
	var jobRow database.Job
	if err := db.Where("slug = ? AND user_id = ?", job.ID, a.currentUserID).Take(&jobRow).Error; err != nil {
		t.Fatal(err)
	}
	if a.commandMaintenance.Load() == nil {
		if err := a.configureCommandMaintenance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}

	runID := make(chan string, 1)
	done := make(chan struct{})
	var result *jobs.RunLog
	var runErr error
	go func() {
		defer close(done)
		result, runErr = a.jobMgr.RunJobContext(ctx, job.ID)
	}()
	t.Cleanup(func() {
		tool.releaseOnce.Do(func() { close(tool.release) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("job vivo não encerrou durante cleanup")
		}
	})
	select {
	case <-tool.started:
	case <-time.After(5 * time.Second):
		var rows []database.JobRun
		_ = db.Where("job_id = ?", jobRow.ID).Find(&rows).Error
		select {
		case <-done:
			t.Fatalf("job não alcançou o tool bloqueado: runs=%+v err=%v result=%+v", rows, runErr, result)
		default:
			t.Fatalf("job não alcançou o tool bloqueado: runs=%+v", rows)
		}
	}
	var run database.JobRun
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := db.Where("job_id = ? AND status = ?", jobRow.ID, jobs.RunStatusRunning).Order("queued_at DESC").Take(&run).Error; err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if run.ID == "" {
		t.Fatal("run vivo não foi persistido")
	}
	runID <- run.ID
	return liveCommandJobControl{JobID: job.ID, RunID: runID, Started: tool.started, Release: func() { tool.releaseOnce.Do(func() { close(tool.release) }) }, Join: func() (*jobs.RunLog, error) {
		select {
		case <-done:
			return result, runErr
		case <-time.After(5 * time.Second):
			return nil, errors.New("join do job vivo excedeu o prazo")
		}
	}}
}

func commandautomationKeyProvider(a *App) (commandautomation.FingerprintKeyProvider, error) {
	return commandautomation.FingerprintKeyProvider(func(ctx context.Context, name string) ([]byte, error) {
		provider, err := commandledgerKeyProvider(a)
		if err != nil {
			return nil, err
		}
		return provider(ctx, name)
	}), nil
}

func commandledgerKeyProvider(a *App) (func(context.Context, string) ([]byte, error), error) {
	keys, err := commandledger.NewCredentialKeyProvider(a.credMgr)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func waitLiveClaimAndLease(t *testing.T, runID string) (commandactivation.Claim, commandjobactivation.Lease) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var claim commandactivation.Claim
		if err := database.DB().Where("source_type = ? AND source_correlation_id = ?", "job", runID).Take(&claim).Error; err == nil {
			var lease commandjobactivation.Lease
			if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&lease).Error; err == nil {
				return claim, lease
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("claim/lease do job %s não foram publicados", runID)
	return commandactivation.Claim{}, commandjobactivation.Lease{}
}

func TestCommandMaintenanceLiveJobCreatesAndRenewsLeaseUntilToolRelease(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	settings := config.DefaultMaintenanceSettings()
	settings.CommandJobActivationLeaseSeconds = 3
	if err := config.SaveMaintenance(settings); err != nil {
		t.Fatal(err)
	}
	a = restartCommandMaintenanceApp(t, a)
	control := startCommandMaintenanceLiveJob(t, a)
	runID := <-control.RunID
	claim, first := waitLiveClaimAndLease(t, runID)
	if claim.State != commandactivation.StateActive || !first.ExpiresAt.After(time.Now()) {
		t.Fatalf("claim/lease vivos inválidos: claim=%+v lease=%+v", claim, first)
	}

	var renewed commandjobactivation.Lease
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&renewed).Error; err == nil && renewed.UpdatedAt.After(first.UpdatedAt) && renewed.ExpiresAt.After(first.ExpiresAt) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !renewed.UpdatedAt.After(first.UpdatedAt) || !renewed.ExpiresAt.After(first.ExpiresAt) {
		t.Fatalf("heartbeat não renovou lease durante tool vivo: primeira=%+v atual=%+v", first, renewed)
	}

	control.Release()
	result, err := control.Join()
	if err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("job não encerrou com sucesso após release: result=%+v err=%v", result, err)
	}
	if err := waitForNoLiveLease(claim.ActivationID); err != nil {
		t.Fatal(err)
	}
}

func waitForNoLiveLease(activationID string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := database.DB().Model(&commandjobactivation.Lease{}).Where("activation_id = ?", activationID).Count(&count).Error; err != nil {
			return err
		}
		var claim commandactivation.Claim
		if err := database.DB().Where("activation_id = ?", activationID).Take(&claim).Error; err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		terminalClaim := claim.State == commandactivation.StateDeactivated && claim.TerminalReason != nil && *claim.TerminalReason == "source_terminal"
		terminalOutbox := false
		if claim.SourceCorrelationID != nil {
			var completed int64
			if err := database.DB().Model(&commandjobevents.ActivationOutbox{}).
				Where("run_id = ? AND state = ? AND delivery_state = ?", *claim.SourceCorrelationID, commandjobevents.StateCompleted, commandjobevents.DeliveryDelivered).
				Count(&completed).Error; err != nil {
				return err
			}
			terminalOutbox = completed > 0
		}
		if count == 0 && terminalClaim && terminalOutbox {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("lease permaneceu após evento terminal do job")
}

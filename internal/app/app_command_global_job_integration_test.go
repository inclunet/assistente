package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
	nativehotkey "golang.design/x/hotkey"
)

type globalJobTestHotkeyRegistrar struct {
	mu        sync.Mutex
	callbacks []hotkey.HotkeyCallback
}

func (r *globalJobTestHotkeyRegistrar) Register(_ []nativehotkey.Modifier, _ nativehotkey.Key, callback hotkey.HotkeyCallback) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
	return len(r.callbacks), nil
}

func (r *globalJobTestHotkeyRegistrar) Unregister(int) error { return nil }

func (r *globalJobTestHotkeyRegistrar) callback(t *testing.T, index int) hotkey.HotkeyCallback {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.callbacks) {
		t.Fatalf("callback nativo ausente: %d/%d", index, len(r.callbacks))
	}
	return r.callbacks[index]
}

type globalJobTestEmitter func(string, any)

func (e globalJobTestEmitter) Emit(event string, data any) { e(event, data) }

type globalJobTestTool struct {
	calls     atomic.Int32
	remaining atomic.Int64
	started   chan struct{}
	cancelled chan struct{}
}

func (*globalJobTestTool) Name() string { return "test.command_global_job" }

func (*globalJobTestTool) Description() string { return "efeito real do hotkey global" }

func (*globalJobTestTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (t *globalJobTestTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	if deadline, ok := ctx.Deadline(); ok {
		t.remaining.Store(int64(time.Until(deadline)))
	}
	if t.started != nil {
		close(t.started)
		<-ctx.Done()
		close(t.cancelled)
		return tools.ToolResult{IsError: true}, ctx.Err()
	}
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func configureGlobalJobHotkeyTestManager(t *testing.T, a *App, registrar jobs.HotkeyRegistrar, dispatch func(context.Context, jobs.CommandHotkeyOccurrence) error) context.Context {
	t.Helper()
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	a.jobMgr.Stop()
	a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig()))
	previous := a.commandProduct.Swap(nil)
	if previous != nil && previous.bridge != nil {
		if err := previous.bridge.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.mountCommandProduct(ctx); err != nil {
		t.Fatal(err)
	}
	manager := jobs.NewManager(jobs.ManagerConfig{
		Repository:             jobs.NewDBRepository(database.DB()),
		ToolRegistry:           a.toolRegistry,
		ToolInvocations:        a.toolInvocationSvc,
		ContextProvider:        a.jobsAuthenticatedContext,
		HotkeyManager:          registrar,
		DispatchCommandHotkey:  dispatch,
		CommandRuntimeIdentity: a.captureCommandJobIdentity,
	})
	a.jobMgr = manager
	t.Cleanup(manager.Stop)
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func createGlobalJobHotkeyTestJob(t *testing.T, a *App, ctx context.Context, tool *globalJobTestTool) *jobs.Job {
	t.Helper()
	a.toolRegistry.MustRegister(tool)
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: uuid.NewString()}, Name: tool.Name(), DisplayName: tool.Name(),
		Description: tool.Description(), Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{
		ID: "global-hotkey-integration-job", Name: "Global hotkey integration job", Enabled: true, Tool: tool.Name(),
		Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerHotkey, Keys: "Ctrl+Alt+G"}},
		ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop},
	}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestCommandGlobalJobHotkeyRunsThroughAdmissionDecisionAndRealTool(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	decisionManager, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisionManager
	registrar := &globalJobTestHotkeyRegistrar{}
	var admissionErr atomic.Value
	var admissionInvocationID atomic.Value
	a.emitter = globalJobTestEmitter(func(event string, data any) {
		if event != "command:global-job-admission" {
			return
		}
		payload, ok := data.(map[string]string)
		if !ok {
			admissionErr.Store(fmt.Errorf("admission payload = %T", data))
			return
		}
		admissionInvocationID.Store(payload["invocationId"])
		if !a.AdmitGlobalCommandOccurrence(payload["invocationId"], true) {
			admissionErr.Store(fmt.Errorf("admission rejected"))
		}
	})
	dispatchDone := make(chan error, 1)
	dispatch := func(dispatchCtx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
		err := a.dispatchCommandJobHotkey(dispatchCtx, occurrence)
		dispatchDone <- err
		return err
	}
	ctx := configureGlobalJobHotkeyTestManager(t, a, registrar, dispatch)
	tool := &globalJobTestTool{}
	job := createGlobalJobHotkeyTestJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		registrar.callback(t, 0)()
	}()
	var payload map[string]any
	select {
	case payload = <-decisionEvents:
	case err := <-dispatchDone:
		t.Fatalf("dispatch recusado antes do questionário: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("o diálogo de decisão não foi emitido")
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls antes da confirmação = %d, want 0", got)
	}
	finishCommandDecision(t, decisionManager, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("callback global não concluiu")
	}
	if value := admissionErr.Load(); value != nil {
		t.Fatal(value)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool calls = %d, want 1", got)
	}
	if remaining := time.Duration(tool.remaining.Load()); remaining <= 5*time.Minute || remaining > tools.DefaultToolTimeout {
		t.Fatalf("tool deadline=%v: admission must not consume the runtime tool budget", remaining)
	}

	invocationValue := admissionInvocationID.Load()
	invocationID, ok := invocationValue.(string)
	if !ok || invocationID == "" {
		t.Fatalf("invocation id de admissão = %#v", invocationValue)
	}
	var rows []struct {
		InvocationID string
		SourceType   string
		Status       string
	}
	if err := database.DB().Table("command_invocations").Select("invocation_id,source_type,status").Where("command_id = ?", commandGlobalJobID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].InvocationID != invocationID || rows[0].SourceType != "keyboard.global" || rows[0].Status != "succeeded" {
		t.Fatalf("ledger = %+v, want one keyboard.global/succeeded invocation %s", rows, invocationID)
	}
	runs, err := a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("job runs = %d, want 1: %+v", len(runs), runs)
	}
	if runs[0].RootOriginType != "user_hotkey" || runs[0].RootOriginID != invocationID {
		t.Fatalf("job run origin = %q/%q, want user_hotkey/%s", runs[0].RootOriginType, runs[0].RootOriginID, invocationID)
	}
	if runs[0].Trigger.Type != jobs.TriggerHotkey || runs[0].Trigger.Keys != "Ctrl+Alt+G" {
		t.Fatalf("job run trigger = %+v, want hotkey Ctrl+Alt+G", runs[0].Trigger)
	}
}

func TestCommandGlobalJobHotkeyDeniedAdmissionDoesNotRunTool(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	registrar := &globalJobTestHotkeyRegistrar{}
	a.emitter = globalJobTestEmitter(func(event string, data any) {
		if event != "command:global-job-admission" {
			return
		}
		payload := data.(map[string]string)
		if !a.AdmitGlobalCommandOccurrence(payload["invocationId"], false) {
			t.Errorf("admission denied by controller")
		}
	})
	ctx := configureGlobalJobHotkeyTestManager(t, a, registrar, a.dispatchCommandJobHotkey)
	tool := &globalJobTestTool{}
	createGlobalJobHotkeyTestJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	registrar.callback(t, 0)()
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d after denied admission", got)
	}
}

func TestCommandGlobalJobHotkeyNilDispatcherFailsClosed(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	registrar := &globalJobTestHotkeyRegistrar{}
	ctx := configureGlobalJobHotkeyTestManager(t, a, registrar, nil)
	tool := &globalJobTestTool{}
	createGlobalJobHotkeyTestJob(t, a, ctx, tool)
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	registrar.callback(t, 0)()
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d with nil dispatcher", got)
	}
}

var _ jobs.HotkeyRegistrar = (*globalJobTestHotkeyRegistrar)(nil)
var _ tools.Tool = (*globalJobTestTool)(nil)

func TestCommandGlobalJobRuntimeStillStopsWithProductShutdown(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	decisions, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisions
	registrar := &globalJobTestHotkeyRegistrar{}
	a.emitter = globalJobTestEmitter(func(event string, data any) {
		if event == "command:global-job-admission" {
			if !a.AdmitGlobalCommandOccurrence(data.(map[string]string)["invocationId"], true) {
				t.Error("native admission rejected")
			}
		}
	})
	ctx := configureGlobalJobHotkeyTestManager(t, a, registrar, a.dispatchCommandJobHotkey)
	tool := &globalJobTestTool{started: make(chan struct{}), cancelled: make(chan struct{})}
	createGlobalJobHotkeyTestJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	callbackDone := make(chan struct{})
	go func() { defer close(callbackDone); registrar.callback(t, 0)() }()
	select {
	case payload := <-decisionEvents:
		finishCommandDecision(t, decisions, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	case <-time.After(10 * time.Second):
		t.Fatal("decision not emitted")
	}
	select {
	case <-tool.started:
	case <-time.After(10 * time.Second):
		t.Fatal("tool not started")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.commandProduct.Load().Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-tool.cancelled:
	case <-shutdownCtx.Done():
		t.Fatal("runtime detached from product shutdown")
	}
	select {
	case <-callbackDone:
	case <-shutdownCtx.Done():
		t.Fatal("native callback did not complete")
	}
	if tool.calls.Load() != 1 {
		t.Fatal("effect was replayed")
	}
	var rows []struct{ Status string }
	if err := database.DB().Table("command_invocations").Select("status").Where("command_id = ?", commandGlobalJobID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "outcome_unknown" {
		t.Fatalf("post-handoff cancellation outcome=%+v", rows)
	}
}

package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	apphotkey "assistente/internal/hotkey"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"

	"assistente/internal/database"
	"github.com/google/uuid"
	nativehotkey "golang.design/x/hotkey"
)

// dialogGlobalCapture é somente uma porta de captura: não implementa
// prioridade, conflito ou restauração. Essas propriedades pertencem ao
// Manager e são cobertas pelos testes reais de internal/hotkey; aqui os
// callbacks são exercitados no percurso produtivo do App.
type dialogGlobalCapture struct {
	mu        sync.Mutex
	nextID    int
	lower     apphotkey.HotkeyCallback
	temporary apphotkey.HotkeyCallback
	released  apphotkey.HotkeyCallback
}

func (c *dialogGlobalCapture) Register(_ []nativehotkey.Modifier, _ nativehotkey.Key, callback apphotkey.HotkeyCallback) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextID == 0 {
		c.nextID = 1
	}
	id := c.nextID
	c.nextID++
	c.lower = callback
	return id, nil
}

func (c *dialogGlobalCapture) Unregister(int) error {
	c.mu.Lock()
	c.lower = nil
	c.mu.Unlock()
	return nil
}

func (c *dialogGlobalCapture) ReserveTemporary(_ []nativehotkey.Modifier, _ nativehotkey.Key, callback apphotkey.HotkeyCallback) (func() error, error) {
	c.mu.Lock()
	c.temporary = callback
	c.mu.Unlock()
	var once sync.Once
	return func() error {
		once.Do(func() {
			c.mu.Lock()
			c.released = c.temporary
			c.temporary = nil
			c.mu.Unlock()
		})
		return nil
	}, nil
}

func (c *dialogGlobalCapture) fireTemporary() bool {
	c.mu.Lock()
	callback := c.temporary
	c.mu.Unlock()
	if callback != nil {
		callback()
		return true
	}
	return false
}

func (c *dialogGlobalCapture) fireLower() bool {
	c.mu.Lock()
	callback := c.lower
	c.mu.Unlock()
	if callback != nil {
		callback()
		return true
	}
	return false
}

func (c *dialogGlobalCapture) fireReleasedTemporary() bool {
	c.mu.Lock()
	callback := c.released
	c.mu.Unlock()
	if callback != nil {
		callback()
		return true
	}
	return false
}

func createDialogGlobalConflictJob(t *testing.T, a *App, ctx context.Context, tool *globalJobTestTool) *jobs.Job {
	t.Helper()
	a.toolRegistry.MustRegister(tool)
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: uuid.NewString()}, Name: tool.Name(), DisplayName: tool.Name(),
		Description: tool.Description(), Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{
		ID: "dialog-global-conflict-job", Name: "Dialog global conflict job", Enabled: true, Tool: tool.Name(),
		Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerHotkey, Keys: "Ctrl+Shift+R"}},
		ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop},
	}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestAppDecisionRepeatAndGlobalJobCallbacksPreserveDecisionLifecycle(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	decisionManager, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisionManager
	capture := &dialogGlobalCapture{}

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
			admissionErr.Store(fmt.Errorf("global job admission rejected"))
		}
	})

	dispatchErrors := make(chan error, 1)
	var dispatchCalls atomic.Int32
	dispatch := func(dispatchCtx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
		dispatchCalls.Add(1)
		err := a.dispatchCommandJobHotkey(dispatchCtx, occurrence)
		if err != nil {
			select {
			case dispatchErrors <- err:
			default:
			}
		}
		return err
	}
	ctx := configureGlobalJobHotkeyTestManager(t, a, capture, dispatch)
	tool := &globalJobTestTool{}
	job := createDialogGlobalConflictJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}

	modifiers, key, err := apphotkey.ParseCombination("Ctrl+Shift+R")
	if err != nil {
		t.Fatal(err)
	}
	repeatEvents := make(chan decisionRepeatEvent, 2)
	var repeatCalls atomic.Int32
	a.decisionRepeatHotkeys = newDecisionRepeatHotkeys(
		func(callback apphotkey.HotkeyCallback) (func() error, error) {
			return capture.ReserveTemporary(modifiers, key, callback)
		},
		func(event decisionRepeatEvent) {
			repeatCalls.Add(1)
			repeatEvents <- event
		},
		func() bool { return true },
	)
	t.Cleanup(a.decisionRepeatHotkeys.shutdown)

	presenter := &commandDecisionPresenter{manager: decisionManager}
	firstDone := make(chan error, 1)
	go func() {
		_, err := presenter.Present(context.Background(), commandDecisionRequest("dialog-global-conflict", time.Minute))
		firstDone <- err
	}()
	firstPayload := receiveCommandDecisionEvent(t, decisionEvents)
	firstDialogID, ok := firstPayload["id"].(string)
	if !ok || firstDialogID == "" {
		t.Fatalf("first dialog id = %#v", firstPayload["id"])
	}

	session := a.OpenDecisionRepeatHotkeySession()
	if session == "" {
		t.Fatal("decision repeat session was not opened")
	}
	if err := a.SetDecisionRepeatHotkey(session, 1, firstDialogID); err != nil {
		t.Fatal(err)
	}

	// O Manager real seleciona a reserva topmost para esta mesma combinação;
	// este stub de captura somente entrega o callback já selecionado, sem
	// reimplementar essa propriedade (coberta em internal/hotkey).
	capture.fireTemporary()
	select {
	case event := <-repeatEvents:
		if event != (decisionRepeatEvent{SessionID: session, Revision: 1, DialogID: firstDialogID}) {
			t.Fatalf("repeat event = %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dialog reservation did not receive the conflicting hotkey")
	}
	select {
	case unexpected := <-decisionEvents:
		t.Fatalf("global hotkey escaped through an open dialog: %#v", unexpected)
	default:
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls while dialog was open = %d", got)
	}

	finishCommandDecision(t, decisionManager, firstPayload, map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}, false)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first decision did not close")
	}
	if err := a.SetDecisionRepeatHotkey(session, 2, ""); err != nil {
		t.Fatal(err)
	}

	secondDone := make(chan struct{})
	go func() {
		if !capture.fireLower() {
			dispatchErrors <- fmt.Errorf("capture lower callback missing")
		}
		close(secondDone)
	}()
	var secondPayload map[string]any
	select {
	case secondPayload = <-decisionEvents:
	case err := <-dispatchErrors:
		t.Fatalf("restored global hotkey dispatch failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("restored global hotkey did not reach the App job (dispatch calls=%d)", dispatchCalls.Load())
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls before second confirmation = %d", got)
	}

	finishCommandDecision(t, decisionManager, secondPayload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("restored global hotkey callback did not complete")
	}
	deadline := time.After(10 * time.Second)
	for tool.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("restored global hotkey did not execute the real tool")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if value := admissionErr.Load(); value != nil {
		t.Fatal(value)
	}
	repeatBeforeStaleCallback := repeatCalls.Load()
	if !capture.fireReleasedTemporary() {
		t.Fatal("released dialog callback was not retained by the capture boundary")
	}
	if got := repeatCalls.Load(); got != repeatBeforeStaleCallback {
		t.Fatalf("released dialog callback emitted repeat: before=%d after=%d", repeatBeforeStaleCallback, got)
	}

	invocationID, ok := admissionInvocationID.Load().(string)
	if !ok || invocationID == "" {
		t.Fatal("missing global invocation ID")
	}
	var rows []struct {
		InvocationID string
		SourceType   string
		Status       string
	}
	terminal := time.After(10 * time.Second)
	for {
		if err := database.DB().Table("command_invocations").Select("invocation_id,source_type,status").Where("command_id = ?", commandGlobalJobID).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) == 1 && rows[0].InvocationID == invocationID && rows[0].Status == "succeeded" {
			break
		}
		select {
		case <-terminal:
			t.Fatalf("command ledger did not reach terminal succeeded state: %+v", rows)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if rows[0].SourceType != "keyboard.global" {
		t.Fatalf("command ledger source = %q, want keyboard.global", rows[0].SourceType)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("real tool calls = %d, want 1", got)
	}
	runs, err := a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].RootOriginType != "user_hotkey" || runs[0].RootOriginID != invocationID {
		t.Fatalf("job run provenance = %+v, want one user_hotkey run for %s", runs, invocationID)
	}
}

var _ jobs.HotkeyRegistrar = (*dialogGlobalCapture)(nil)

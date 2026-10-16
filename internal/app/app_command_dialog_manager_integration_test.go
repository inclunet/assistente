package app

import (
	"context"
	"errors"
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

	nativehotkey "golang.design/x/hotkey"
)

type dialogManagerNativeFactory struct {
	mu        sync.Mutex
	captures  []*dialogManagerNative
	active    int
	maxActive int
}

func (f *dialogManagerNativeFactory) make(modifiers []nativehotkey.Modifier, key nativehotkey.Key) hotkey.NativeHotkey {
	f.mu.Lock()
	defer f.mu.Unlock()
	capture := &dialogManagerNative{
		owner:     f,
		modifiers: append([]nativehotkey.Modifier(nil), modifiers...),
		key:       key,
		down:      make(chan nativehotkey.Event, 8),
	}
	f.captures = append(f.captures, capture)
	return capture
}

func (f *dialogManagerNativeFactory) snapshot() ([]*dialogManagerNative, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*dialogManagerNative(nil), f.captures...), f.maxActive
}

type dialogManagerNative struct {
	owner      *dialogManagerNativeFactory
	modifiers  []nativehotkey.Modifier
	key        nativehotkey.Key
	down       chan nativehotkey.Event
	registered bool
	mu         sync.Mutex
}

func (n *dialogManagerNative) Register() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.registered {
		return errors.New("native capture registered twice")
	}
	n.owner.mu.Lock()
	defer n.owner.mu.Unlock()
	if n.owner.active != 0 {
		return fmt.Errorf("native captures overlapped: active=%d", n.owner.active)
	}
	n.owner.active++
	if n.owner.active > n.owner.maxActive {
		n.owner.maxActive = n.owner.active
	}
	n.registered = true
	return nil
}

func (n *dialogManagerNative) Unregister() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.registered {
		return nil
	}
	n.owner.mu.Lock()
	defer n.owner.mu.Unlock()
	n.owner.active--
	n.registered = false
	return nil
}

func (n *dialogManagerNative) Keydown() <-chan nativehotkey.Event { return n.down }

func (n *dialogManagerNative) fire() { n.down <- nativehotkey.Event{} }

func TestAppDecisionDialogAndConflictingGlobalJobShareRealHotkeyManager(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	decisions, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisions

	factory := &dialogManagerNativeFactory{}
	manager := hotkey.NewManager(hotkey.NativeFactory(factory.make))
	t.Cleanup(manager.Stop)

	var admissionErr atomic.Value
	var invocationID atomic.Value
	a.emitter = globalJobTestEmitter(func(event string, data any) {
		if event != "command:global-job-admission" {
			return
		}
		payload, ok := data.(map[string]string)
		if !ok {
			admissionErr.Store(fmt.Errorf("admission payload = %T", data))
			return
		}
		invocationID.Store(payload["invocationId"])
		if !a.AdmitGlobalCommandOccurrence(payload["invocationId"], true) {
			admissionErr.Store(errors.New("global job admission rejected"))
		}
	})

	dispatchDone := make(chan error, 1)
	var dispatchCalls atomic.Int32
	dispatch := func(ctx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
		dispatchCalls.Add(1)
		err := a.dispatchCommandJobHotkey(ctx, occurrence)
		dispatchDone <- err
		return err
	}
	ctx := configureGlobalJobHotkeyTestManager(t, a, manager, dispatch)
	tool := &globalJobTestTool{}
	job := createDialogGlobalConflictJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}

	modifiers, key, err := hotkey.ParseCombination("Ctrl+Shift+R")
	if err != nil {
		t.Fatal(err)
	}
	repeatEvents := make(chan decisionRepeatEvent, 2)
	var repeatCalls atomic.Int32
	a.decisionRepeatHotkeys = newDecisionRepeatHotkeys(
		func(callback hotkey.HotkeyCallback) (func() error, error) {
			return manager.ReserveTemporary(modifiers, key, callback)
		},
		func(event decisionRepeatEvent) {
			repeatCalls.Add(1)
			repeatEvents <- event
		},
		func() bool { return true },
	)
	t.Cleanup(a.decisionRepeatHotkeys.shutdown)

	presenter := &commandDecisionPresenter{manager: decisions}
	presenterCtx, cancelPresenter := context.WithCancel(context.Background())
	t.Cleanup(cancelPresenter)
	firstDone := make(chan error, 1)
	go func() {
		_, err := presenter.Present(presenterCtx, commandDecisionRequest("dialog-manager-conflict", time.Minute))
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

	captures, _ := factory.snapshot()
	if len(captures) != 2 {
		t.Fatalf("native captures after dialog reservation = %d, want global plus temporary", len(captures))
	}
	if captures[0].key != key || captures[1].key != key {
		t.Fatalf("native keys = %v/%v, want conflicting key %v", captures[0].key, captures[1].key, key)
	}
	// O evento atravessa o listener nativo controlado e o Manager real escolhe
	// a reserva temporária; nenhum callback é escolhido pelo teste.
	captures[1].fire()
	select {
	case event := <-repeatEvents:
		if event != (decisionRepeatEvent{SessionID: session, Revision: 1, DialogID: firstDialogID}) {
			t.Fatalf("repeat event = %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("evento nativo não chegou à reserva do diálogo")
	}
	if got := dispatchCalls.Load(); got != 0 {
		t.Fatalf("job dispatches with decision dialog open = %d, want 0", got)
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls with decision dialog open = %d, want 0", got)
	}
	select {
	case unexpected := <-decisionEvents:
		t.Fatalf("job global abriu decisão enquanto o outro diálogo estava no topo: %#v", unexpected)
	default:
	}

	finishCommandDecision(t, decisions, firstPayload, map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}, false)
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

	// Uma ocorrência enfileirada pela captura inferior encerrada não pode
	// atravessar a substituição. A captura nova, após restauração do slot,
	// precisa chegar ao executor de job.
	captures, _ = factory.snapshot()
	if len(captures) != 3 {
		t.Fatalf("native captures after release = %d, want inferior, temporary, restored", len(captures))
	}
	staleRepeatCount := repeatCalls.Load()
	captures[1].fire()
	captures[0].fire()
	select {
	case unexpected := <-decisionEvents:
		t.Fatalf("old native event opened a decision after release: %#v", unexpected)
	case <-time.After(40 * time.Millisecond):
	}
	if got := repeatCalls.Load(); got != staleRepeatCount {
		t.Fatalf("old temporary event repeated the dialog after release: before=%d after=%d", staleRepeatCount, got)
	}
	if got := dispatchCalls.Load(); got != 0 || tool.calls.Load() != 0 {
		t.Fatalf("old event effects: dispatches=%d tool calls=%d, want 0/0", got, tool.calls.Load())
	}

	captures[2].fire()
	var secondPayload map[string]any
	select {
	case secondPayload = <-decisionEvents:
	case <-time.After(5 * time.Second):
		t.Fatalf("restored global event did not reach job dispatch (calls=%d)", dispatchCalls.Load())
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool ran before the job's own confirmation: %d", got)
	}
	finishCommandDecision(t, decisions, secondPayload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Fatalf("restored global dispatch failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("dispatchCommandJobHotkey did not return after confirmation")
	}
	idValue := invocationID.Load()
	id, ok := idValue.(string)
	if !ok || id == "" {
		t.Fatalf("missing admitted invocation ID: %#v", idValue)
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
		if len(rows) == 1 && rows[0].InvocationID == id && rows[0].Status == "succeeded" {
			break
		}
		select {
		case <-terminal:
			t.Fatalf("ledger did not reach one terminal succeeded invocation: %+v", rows)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if rows[0].SourceType != "keyboard.global" {
		t.Fatalf("ledger source = %q, want keyboard.global", rows[0].SourceType)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("real tool calls = %d, want exactly 1", got)
	}
	if got := dispatchCalls.Load(); got != 1 {
		t.Fatalf("job dispatches = %d, want exactly 1", got)
	}
	if got := repeatCalls.Load(); got != staleRepeatCount {
		t.Fatalf("late repeat after job completion: before=%d after=%d", staleRepeatCount, got)
	}
	if value := admissionErr.Load(); value != nil {
		t.Fatal(value)
	}

	runs, err := a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].RootOriginType != "user_hotkey" || runs[0].RootOriginID != id {
		t.Fatalf("job runs = %+v, want one user_hotkey run for %s", runs, id)
	}
	_, maxActive := factory.snapshot()
	if maxActive != 1 {
		t.Fatalf("maximum overlapping native captures = %d, want 1", maxActive)
	}
}

var _ hotkey.NativeHotkey = (*dialogManagerNative)(nil)
var _ jobs.HotkeyRegistrar = (*hotkey.Manager)(nil)

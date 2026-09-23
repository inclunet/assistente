package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	nativehotkey "golang.design/x/hotkey"
)

type capturedHotkeyRegistrar struct {
	mu           sync.Mutex
	nextID       int
	callbacks    []hotkey.HotkeyCallback
	unregistered []int
}

func (r *capturedHotkeyRegistrar) Register(_ []nativehotkey.Modifier, _ nativehotkey.Key, callback hotkey.HotkeyCallback) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.callbacks = append(r.callbacks, callback)
	return r.nextID, nil
}

func (r *capturedHotkeyRegistrar) Unregister(id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unregistered = append(r.unregistered, id)
	return nil
}

func (r *capturedHotkeyRegistrar) callback(t *testing.T, index int) hotkey.HotkeyCallback {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.callbacks) {
		t.Fatalf("callback hotkey %d ausente; registrados=%d", index, len(r.callbacks))
	}
	return r.callbacks[index]
}

func TestManagerHotkeyCallbackPersistsAuthenticatedOriginAndStopsUnregistering(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	repo := f.repo
	userCtx := f.ctx
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate command events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay epoch: %v", err)
	}
	registrar := &capturedHotkeyRegistrar{}
	mgr := f.manager
	mgr.cfg.HotkeyManager = registrar
	job := f.job
	job.ID = "hotkey-origin"
	job.Name = "Hotkey origin"
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+J"}}
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	var err error
	f.job, err = repo.GetJob(userCtx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.definition.AllowedSources = []commandcatalog.Source{commandcatalog.KeyboardGlobal}
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	f.rebuildHandler()
	mgr.cfg.DispatchCommandHotkey = func(ctx context.Context, occurrence CommandHotkeyOccurrence) error {
		prepared, err := occurrence.Context(ctx)
		if err != nil {
			return err
		}
		in := globalCommandInvocation(t, f)
		handle, err := f.handler.Start(prepared, in)
		if err != nil {
			return err
		}
		outcome := <-handle.Done
		if outcome.Status != "succeeded" {
			return errors.New("common command handler did not succeed")
		}
		return nil
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	callback := registrar.callback(t, 0)
	t.Cleanup(mgr.Stop)
	callback()
	runs, err := repo.GetRuns(userCtx, job.ID, 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("run de hotkey = len %d err %v", len(runs), err)
	}
	run := runs[0]
	if run.RootOriginType != "user_hotkey" || run.RootOriginID != f.invocationID {
		t.Fatalf("origem hotkey = (%q,%q), invocation=%q", run.RootOriginType, run.RootOriginID, f.invocationID)
	}
	assertHotkeyCommandOutbox(t, repo, run.RunID, f.invocationID, commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateCompleted)

	mgr.Stop()
	callback()
	if runsAfterStop, err := repo.GetRuns(userCtx, job.ID, 2); err != nil || len(runsAfterStop) != 1 {
		t.Fatalf("callback antigo após Stop criou execução: runs=%d err=%v", len(runsAfterStop), err)
	}
	registrar.mu.Lock()
	defer registrar.mu.Unlock()
	if len(registrar.unregistered) != 1 || registrar.unregistered[0] != 1 {
		t.Fatalf("hotkey não desregistrado no Stop: %#v", registrar.unregistered)
	}
}

func TestManagerHotkeyCallbackWhenFalseNaoExecuta(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	registrar := &capturedHotkeyRegistrar{}
	var dispatches atomic.Int32
	mgr := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return userCtx }, HotkeyManager: registrar, DispatchCommandHotkey: func(ctx context.Context, occurrence CommandHotkeyOccurrence) error {
		prepared, err := occurrence.Context(ctx)
		if err != nil {
			return err
		}
		if prepared.Err() != nil {
			return prepared.Err()
		}
		dispatches.Add(1)
		return nil
	}})
	job := testRepositoryJob("hotkey-skipped", "Hotkey skipped")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+K", When: `{{ eq 1 2 }}`}}
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Stop)
	registrar.callback(t, 0)()
	if runs, err := repo.GetRuns(userCtx, job.ID, 1); err != nil || len(runs) != 0 {
		t.Fatalf("hotkey com when falso executou: runs=%d err=%v", len(runs), err)
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("hotkey com when falso alcançou dispatcher: %d", got)
	}

	positive := *job
	positive.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+K", When: `{{ eq 1 1 }}`}}
	if err := repo.SaveJob(userCtx, &positive); err != nil {
		t.Fatal(err)
	}
	mgr.unregisterTriggers(job)
	mgr.registry.Set(&positive)
	mgr.registerTriggers(&positive)
	registrar.callback(t, 1)()
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("controle positivo do when não alcançou dispatcher: %d", got)
	}
	mgr.Stop()
}

func assertHotkeyCommandOutbox(t *testing.T, repo *DBRepository, runID, rootOriginID string, wantStates ...string) {
	t.Helper()
	var rows []commandjobevents.ActivationOutbox
	if err := repo.db.Where("run_id = ?", runID).Order("sequence ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load hotkey outbox %s: %v", runID, err)
	}
	want := make(map[string]bool, len(wantStates))
	for _, state := range wantStates {
		want[state] = true
	}
	for _, row := range rows {
		if row.RootOriginType != "user_hotkey" || row.RootOriginID != rootOriginID {
			t.Fatalf("outbox hotkey root=(%q,%q), want=(user_hotkey,%q)", row.RootOriginType, row.RootOriginID, rootOriginID)
		}
		delete(want, row.State)
	}
	if len(want) != 0 {
		t.Fatalf("outbox hotkey sem estados=%v; rows=%+v", want, rows)
	}
}

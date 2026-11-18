package controllers

import (
	"assistente/internal/testutil/workspacefixture"
	"context"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"assistente/internal/userctx"
	"assistente/internal/workspace"
)

type workspaceControllerEmitter struct {
	mu     sync.Mutex
	events []string
	onEmit func(string)
}

func (e *workspaceControllerEmitter) Emit(event string, data any) {
	e.wsEvent(event, data)
}

func (e *workspaceControllerEmitter) wsEvent(event string, data any) {
	ws, _ := data.(*workspace.Workspace)
	entry := event
	e.mu.Lock()
	if ws != nil {
		entry = event + ":" + ws.ID
	}
	e.events = append(e.events, entry)
	onEmit := e.onEmit
	e.mu.Unlock()
	if onEmit != nil {
		onEmit(entry)
	}
}

func newWorkspaceControllerTestFixture(t *testing.T) (*workspace.Manager, *workspace.Workspace, *workspace.Workspace) {
	t.Helper()
	manager := workspacefixture.New(t, t.TempDir())
	if err := manager.Initialize(filepath.Join(t.TempDir(), "active")); err != nil {
		t.Fatal(err)
	}
	first, err := workspacefixture.Create(t, manager, "first", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspacefixture.Create(t, manager, "second", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return manager, first, second
}

func TestWorkspaceControllerSwitchCallsBootstrapBeforeEvent(t *testing.T) {
	manager, target, _ := newWorkspaceControllerTestFixture(t)
	emitter := &workspaceControllerEmitter{}
	var order []string
	emitter.onEmit = func(event string) { order = append(order, event) }
	controller := NewWorkspaceController(WorkspaceControllerConfig{
		WorkspaceMgr: manager,
		Emitter:      emitter,
		OnWorkspaceSwitched: func() {
			order = append(order, "hook:"+manager.Active().ID)
		},
	})

	if _, err := controller.SwitchWorkspace(target.ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"hook:" + target.ID, "workspace:switched:" + target.ID}; len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("ordem da troca = %#v, want %#v", order, want)
	}
}

func TestWorkspaceControllerFailedSwitchDoesNotCallBootstrap(t *testing.T) {
	manager, _, _ := newWorkspaceControllerTestFixture(t)
	emitter := &workspaceControllerEmitter{}
	called := 0
	controller := NewWorkspaceController(WorkspaceControllerConfig{
		WorkspaceMgr:        manager,
		Emitter:             emitter,
		OnWorkspaceSwitched: func() { called++ },
	})

	if _, err := controller.SwitchWorkspace("missing-workspace"); err == nil {
		t.Fatal("troca inexistente aceita")
	}
	if called != 0 || len(emitter.events) != 0 {
		t.Fatalf("falha publicou callback/evento: called=%d events=%v", called, emitter.events)
	}
}

func TestWorkspaceControllerTabActivationEmitsSnapshot(t *testing.T) {
	manager, _, _ := newWorkspaceControllerTestFixture(t)
	var event any
	var eventName string
	emitter := &captureWorkspaceControllerEmitter{capture: func(name string, data any) {
		eventName = name
		event = data
	}}
	controller := NewWorkspaceController(WorkspaceControllerConfig{
		WorkspaceMgr: manager,
		Emitter:      emitter,
	})
	active := manager.Active()
	if err := controller.SetActiveWorkspaceTab(active.Tabs.Items[0].ID); err != nil {
		t.Fatalf("SetActiveWorkspaceTab: %v", err)
	}
	got, ok := event.(*workspace.Workspace)
	if eventName != "workspace:tab_activated" || !ok || got == nil || got.SnapshotEpoch == "" || got.SnapshotSequence == "" {
		t.Fatalf("tab_activated não carregou snapshot carimbado: name=%q event=%#v", eventName, event)
	}
}

type captureWorkspaceControllerEmitter struct {
	capture func(string, any)
}

func (e *captureWorkspaceControllerEmitter) Emit(event string, data any) {
	if e.capture != nil {
		e.capture(event, data)
	}
}

func TestWorkspaceControllerSerializesConcurrentSwitchBootstrapAndEvent(t *testing.T) {
	manager, first, second := newWorkspaceControllerTestFixture(t)
	emitter := &workspaceControllerEmitter{}
	var mu sync.Mutex
	order := make([]string, 0, 4)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSafely := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseSafely()
	activeCallbacks := 0
	maxCallbacks := 0
	callbackCount := 0
	controller := NewWorkspaceController(WorkspaceControllerConfig{
		WorkspaceMgr: manager,
		Emitter:      emitter,
		OnWorkspaceSwitched: func() {
			mu.Lock()
			callbackCount++
			isFirst := callbackCount == 1
			activeCallbacks++
			if activeCallbacks > maxCallbacks {
				maxCallbacks = activeCallbacks
			}
			order = append(order, "hook:"+manager.Active().ID)
			if isFirst {
				close(started)
			}
			mu.Unlock()
			if isFirst {
				<-release
			}
			mu.Lock()
			activeCallbacks--
			mu.Unlock()
		},
	})
	emitter.onEmit = func(event string) {
		mu.Lock()
		order = append(order, event)
		mu.Unlock()
	}

	results := make(chan error, 2)
	go func() { _, err := controller.SwitchWorkspace(first.ID); results <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("primeiro bootstrap não iniciou")
	}
	go func() { _, err := controller.SwitchWorkspace(second.ID); results <- err }()
	select {
	case err := <-results:
		t.Fatalf("segunda troca concluiu durante o callback: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releaseSafely()
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("troca concorrente não concluiu")
		}
	}
	if maxCallbacks != 1 || callbackCount != 2 {
		t.Fatalf("callbacks concorrentes=%d máximo=%d", callbackCount, maxCallbacks)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 4 {
		t.Fatalf("ordem de callback/evento=%v", order)
	}
	for index := 0; index < len(order); index += 2 {
		if len(order[index]) < 5 || order[index][:5] != "hook:" || len(order[index+1]) < 19 || order[index+1][:19] != "workspace:switched:" {
			t.Fatalf("callback não precedeu seu evento: %v", order)
		}
		if order[index][5:] != order[index+1][19:] {
			t.Fatalf("par de troca desalinhado: %v", order)
		}
	}
}

func TestWorkspaceImportRegistrationFailureNotifiesOriginalSession(t *testing.T) {
	home := t.TempDir()
	db := workspacefixture.OpenIndex(t, home)
	manager := workspace.NewManager(home, func() *gorm.DB { return db })
	if err := manager.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ctx := userctx.WithSessionID(userctx.WithUserID(context.Background(), "owner"), "original")
	var notice workspace.CreationRecoveryEvent
	controller := NewWorkspaceController(WorkspaceControllerConfig{
		WorkspaceMgr: manager,
		Emitter: &captureWorkspaceControllerEmitter{capture: func(name string, data any) {
			if name == "workspace:creation-recovery" {
				notice = data.(workspace.CreationRecoveryEvent)
			}
		}},
	})
	const callback = "test:failed-import-index"
	if err := db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "desktop_workspace_states" {
			_ = tx.AddError(errors.New("index unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	_, err := controller.ImportWorkspace(ctx, "name: Imported\ntabs:\n  items: []\n", directory)
	var recovery *workspace.CreationRegistrationError
	if !errors.As(err, &recovery) {
		t.Fatalf("error=%v", err)
	}
	if notice.SessionID != "original" || notice.UserID != "owner" || notice.Path != recovery.Path {
		t.Fatalf("notice=%+v", notice)
	}
	if _, err := os.Stat(notice.Path); err != nil {
		t.Fatal(err)
	}
}

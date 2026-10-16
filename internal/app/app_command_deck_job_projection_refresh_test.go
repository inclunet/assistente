package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandadapter"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandinput"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

// A workspace tab switch changes CommandSnapshot.Version without changing
// command bindings. A live command-job claim makes the host projection guard
// observe that version. The first physical Deck navigation must refresh that
// equivalent projection before resolving the press, not discard the epoch.
func TestCommandDeckInputRefreshesEquivalentJobProjectionAfterWorkspaceTabSwitch(t *testing.T) {
	ctx := context.Background()
	a := commandJobPublicationApp(t)
	decisions := appCommandImportWailsCopyDecisions(t, a)
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Deck tab navigation", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandWorkspaceTabNextID,
			TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECKA123456","key":0}`,
			Arguments: map[string]any{}, Effect: "execute", Enabled: true},
	})
	settings := config.DefaultMaintenanceSettings()
	settings.CommandJobActivationLeaseSeconds = 3
	if err := config.SaveMaintenance(settings); err != nil {
		t.Fatal(err)
	}
	a = restartCommandMaintenanceApp(t, a)
	control := startCommandMaintenanceLiveJob(t, a)
	runID := <-control.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	if claim.State != commandactivation.StateActive {
		t.Fatalf("fixture job claim is not active: %+v", claim)
	}
	manager := a.workspaceMgr
	initial := manager.Active()
	if initial == nil || initial.Tabs.Active == "" {
		t.Fatal("workspace fixture has no active tab")
	}
	workspaceID, initialTabID := initial.ID, initial.Tabs.Active
	targetTabID := uuid.Must(uuid.NewV7()).String()
	if err := manager.AddTab(workspace.Tab{ID: targetTabID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, initialTabID); err != nil {
		t.Fatalf("select initial tab through the workspace API: %v", err)
	}

	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("command product is absent")
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		t.Fatalf("establish current job projection before navigation: %v", err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("prepare real local navigation map: %v", err)
	}
	identities, versions, err := p.deckTriggerMap(ctx)
	if err != nil || len(identities["DECKA123456"][0]) != 1 || identities["DECKA123456"][0][0].identity != "streamdeck.key:DECKA123456:key:0" {
		t.Fatalf("real configured Deck trigger missing: identities=%+v err=%v", identities, err)
	}
	events := make(chan CommandDeckLocalUIEvent, 2)
	statuses := make(chan string, 16)
	var keyboardMapChanges atomic.Int32
	previousEmitter := a.emitter
	a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-local-ui" {
			events <- payload.(CommandDeckLocalUIEvent)
			return
		}
		if name == "command:keyboard-map-changed" {
			keyboardMapChanges.Add(1)
			return
		}
		if name == "command:deck-status" {
			encoded, _ := json.Marshal(payload)
			var status struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(encoded, &status)
			statuses <- status.Status
			return
		}
		if previousEmitter != nil {
			previousEmitter.Emit(name, payload)
		}
	})
	// Keep this fixture's emitter installed through worker teardown. Restoring
	// it while the native epoch is draining would race with status publication.
	directCtx, directCancel := context.WithCancel(ctx)
	defer directCancel()
	direct := &commandDeckController{p: p, ctx: directCtx, versions: versions, identities: identities,
		pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
	direct.opened("DECKA123456")
	defer direct.reset("")
	if _, err := manager.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, targetTabID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("direct Input fixture must start with a stale guard: %v", err)
	}
	ack, err := direct.Input(ctx, commandadapter.Event{SourceInstance: "streamdeck.key:DECKA123456", Key: "key:0", Kind: commandinput.KeyDown})
	if err != nil || !ack.Accepted {
		t.Fatalf("direct Input lost first press after tab change: ack=%+v err=%v", ack, err)
	}
	select {
	case <-events:
	default:
		t.Fatal("direct Input did not publish the navigation event")
	}
	if _, err := direct.Input(ctx, commandadapter.Event{SourceInstance: "streamdeck.key:DECKA123456", Key: "key:0", Kind: commandinput.KeyUp}); err != nil {
		t.Fatal(err)
	}
	driver := &multiPhysicalDeckDriver{opened: make(chan *multiPhysicalDeckHandle, 4)}
	deckCtx, deckCancel := context.WithCancel(ctx)
	defer deckCancel()
	p.startDeck(deckCtx, driver)
	t.Cleanup(func() {
		deckCancel()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := p.Shutdown(shutdown); err != nil {
			t.Errorf("drain synthetic Deck: %v", err)
		}
	})
	var handle *multiPhysicalDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic Deck was not opened by runDeckEpoch")
	}
	press := func() {
		t.Helper()
		handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
		select {
		case event := <-events:
			if event.CommandID != commandWorkspaceTabNextID || event.Generation == "" || event.SessionID != p.principal.SessionID {
				t.Fatalf("unexpected local navigation event: %+v", event)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("synthetic Deck key was not delivered through PollOne/Input")
		}
	}
	releaseKey := func() { handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false} }
	press()
	releaseKey()
	for len(statuses) > 0 {
		<-statuses
	}
	if _, err := manager.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, initialTabID); err != nil {
		t.Fatalf("switch to the second tab through the workspace API: %v", err)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("test did not invalidate the mounted workspace projection guard: %v", err)
	}
	// Observe one complete inner runDeckEpoch ticker after the guard goes stale.
	// Without its refresh-before-deckMap, this reports unavailable, retires the
	// original handle, and reopens a new epoch on the outer ticker.
	select {
	case status := <-statuses:
		if status != "connected" {
			t.Fatalf("Deck epoch did not survive the tab-switch tick: status=%q", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDeckEpoch did not publish status after its refresh tick")
	}
	if changed := keyboardMapChanges.Load(); changed != 0 {
		t.Fatalf("equivalent job projection invalidated the live keyboard map %d times", changed)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); err != nil {
		t.Fatalf("ticker did not refresh the current host projection: %v", err)
	}
	select {
	case <-handle.closed:
		t.Fatal("ticker retired the original synthetic Deck handle")
	default:
	}
	select {
	case reopened := <-driver.opened:
		t.Fatalf("ticker reopened Deck instead of retaining its epoch: %p", reopened)
	default:
	}
	// The second physical press crosses the tick on the same synthetic handle.
	press()
	releaseKey()
	if changed := keyboardMapChanges.Load(); changed != 0 {
		t.Fatalf("equivalent refreshed projection emitted %d keyboard-map invalidations", changed)
	}
}

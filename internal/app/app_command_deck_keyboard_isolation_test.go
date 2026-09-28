package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/workspace"

	"github.com/google/uuid"
)

// O fake permite alterar a presença física enquanto Enumerate/Open/Read rodam
// nas goroutines do runtime do Deck.
type isolationDeckDriver struct {
	mu         sync.Mutex
	present    bool
	opened     chan *isolationDeckHandle
	enumerated chan bool
}

func (d *isolationDeckDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	d.mu.Lock()
	present := d.present
	d.mu.Unlock()
	select {
	case d.enumerated <- present:
	default:
	}
	if !present {
		return nil, nil
	}
	return []commanddeck.PhysicalDevice{{ID: "test-deck", Model: commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 2, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}}}, nil
}

func (d *isolationDeckDriver) Open(_ context.Context, _ commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	d.mu.Lock()
	present := d.present
	d.mu.Unlock()
	if !present {
		return nil, errors.New("device ausente")
	}
	h := &isolationDeckHandle{events: make(chan commanddeck.PhysicalKeyEvent, 8), writes: make(chan commanddeck.RenderPlan, 16), done: make(chan struct{})}
	d.opened <- h
	return h, nil
}

func (d *isolationDeckDriver) setPresent(present bool) {
	d.mu.Lock()
	d.present = present
	d.mu.Unlock()
}

type isolationDeckHandle struct {
	events chan commanddeck.PhysicalKeyEvent
	writes chan commanddeck.RenderPlan
	done   chan struct{}
	once   sync.Once
}

func (h *isolationDeckHandle) Write(ctx context.Context, plan commanddeck.RenderPlan) error {
	select {
	case <-h.done:
		return errors.New("device removido")
	case <-ctx.Done():
		return ctx.Err()
	case h.writes <- plan:
		return nil
	}
}

func (h *isolationDeckHandle) Read(ctx context.Context) (commanddeck.PhysicalKeyEvent, error) {
	select {
	case <-h.done:
		return commanddeck.PhysicalKeyEvent{}, errors.New("device removido")
	case <-ctx.Done():
		return commanddeck.PhysicalKeyEvent{}, ctx.Err()
	case event := <-h.events:
		return event, nil
	}
}

func (h *isolationDeckHandle) Close(context.Context) error {
	h.once.Do(func() { close(h.done) })
	return nil
}

func (h *isolationDeckHandle) remove() { h.once.Do(func() { close(h.done) }) }

func TestCommandDeckDisconnectKeepsLocalKeyboardMapAndBindings(t *testing.T) {
	ctx := context.Background()
	a := commandJobPublicationApp(t)
	decisions := appCommandImportWailsCopyDecisions(t, a)
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Deck device isolation", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "navigation.settings.open",
			TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`,
			Arguments: map[string]any{}, Effect: "execute", Enabled: true},
	})
	maintenance := config.DefaultMaintenanceSettings()
	maintenance.CommandJobActivationLeaseSeconds = 3
	if err := config.SaveMaintenance(maintenance); err != nil {
		t.Fatal(err)
	}
	a = restartCommandMaintenanceApp(t, a)
	job := startCommandMaintenanceLiveJob(t, a)
	t.Cleanup(func() { job.Release(); _, _ = job.Join() })
	runID := <-job.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	if claim.State != commandactivation.StateActive {
		t.Fatalf("fixture não iniciou job ativo: %+v", claim)
	}
	p := a.commandProduct.Load()
	localEvents := make(chan CommandDeckLocalUIEvent, 4)
	statuses := make(chan string, 16)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-local-ui" {
			localEvents <- value.(CommandDeckLocalUIEvent)
			return
		}
		if name == "command:deck-status" {
			encoded, _ := json.Marshal(value)
			var status struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(encoded, &status)
			select {
			case statuses <- status.Status:
			default: // Status polling must not hold up runtime shutdown on failure.
			}
		}
	})

	before, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Bindings) == 0 || len(before.ContextualBindings) == 0 {
		t.Fatal("fixture não publicou bindings locais")
	}

	driver := &isolationDeckDriver{opened: make(chan *isolationDeckHandle, 4), enumerated: make(chan bool, 16)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver) // dispositivo ausente no startup

	checkMapUnchanged := func(phase string) {
		t.Helper()
		after, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatalf("mapa %s: %v", phase, err)
		}
		if after.Generation != before.Generation || !reflect.DeepEqual(after.Bindings, before.Bindings) {
			t.Fatalf("mapa local mudou após %s: geração %q→%q; bindings antes=%+v depois=%+v", phase, before.Generation, after.Generation, before.Bindings, after.Bindings)
		}
		if !reflect.DeepEqual(after.ContextualBindings, before.ContextualBindings) {
			t.Fatalf("bindings contextuais mudaram após %s", phase)
		}
	}
	waitHandle := func(phase string) *isolationDeckHandle {
		t.Helper()
		select {
		case h := <-driver.opened:
			return h
		case <-time.After(8 * time.Second):
			t.Fatalf("Deck não conectou na fase %s", phase)
			return nil
		}
	}
	press := func(phase string, h *isolationDeckHandle) {
		t.Helper()
		h.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
		select {
		case event := <-localEvents:
			if event.CommandID != "navigation.settings.open" || event.Generation == "" {
				t.Fatalf("execução Deck inesperada em %s: %+v", phase, event)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Deck não executou o binding em %s", phase)
		}
		h.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false}
	}

	select {
	case present := <-driver.enumerated:
		if present {
			t.Fatal("driver encontrou device no startup; a ausência inicial não foi exercitada")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime não enumerou o driver ausente no startup")
	}
	checkMapUnchanged("inicialização sem dispositivo")
	driver.setPresent(true)
	first := waitHandle("conexão inicial")
	press("primeira conexão", first)
	checkMapUnchanged("conexão inicial")

	manager := a.workspaceMgr
	active := manager.Active()
	if active == nil || active.Tabs.Active == "" {
		t.Fatal("workspace do fixture não possui aba ativa")
	}
	workspaceID, initialTabID := active.ID, active.Tabs.Active
	targetTabID := uuid.Must(uuid.NewV7()).String()
	if err := manager.AddTab(workspace.Tab{ID: targetTabID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, initialTabID); err != nil {
		t.Fatalf("estabelecer aba inicial: %v", err)
	}
	if _, err := manager.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, targetTabID); err != nil {
		t.Fatalf("invalidar guard pela troca de aba: %v", err)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("troca de aba não tornou stale o guard do host: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case status := <-statuses:
			if status == "connected" {
				if _, err := p.host.Snapshot(ctx, p.principal); err == nil {
					goto refreshed
				} else if !errors.Is(err, commandexecution.ErrStale) {
					t.Fatalf("snapshot após status conectado: %v", err)
				}
			}
		case <-deadline:
			t.Fatal("runtime não publicou Deck conectado após refresh do guard stale")
		}
	}
refreshed:
	if _, err := p.host.Snapshot(ctx, p.principal); err != nil {
		t.Fatalf("ticker não atualizou o guard de workspace: %v", err)
	}
	select {
	case <-first.done:
		t.Fatal("troca de aba encerrou a conexão Deck")
	default:
	}
	press("refresh do guard stale", first)
	checkMapUnchanged("refresh do guard stale")

	driver.setPresent(false)
	first.remove()
	select {
	case <-first.done:
	case <-time.After(3 * time.Second):
		t.Fatal("remoção não encerrou o handle da primeira conexão")
	}
	disconnectDeadline := time.After(5 * time.Second)
	for {
		select {
		case status := <-statuses:
			if status == "disconnected" {
				goto disconnected
			}
		case <-disconnectDeadline:
			t.Fatal("runtime não observou a remoção do Deck")
		}
	}
disconnected:
	checkMapUnchanged("remoção do dispositivo")

	driver.setPresent(true)
	second := waitHandle("reconexão")
	press("reconexão", second)
	checkMapUnchanged("reconexão")
}

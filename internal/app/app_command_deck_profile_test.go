package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandadapter"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
	"assistente/internal/commandinput"
	"github.com/google/uuid"
)

const deckProfileTrigger = "streamdeck.key:test-deck:key:0"

func deckProfileConfiguration(t *testing.T, a *App, commandID string) (*commandProductRuntime, *commandbindings.Configuration) {
	t.Helper()
	a.workspaceMgr.SetDefaultProfile("profile-a")
	p := a.commandProduct.Load()
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID:                "deck-profile-binding",
		Trigger:           deckProfileTrigger,
		CommandID:         commandID,
		ArgumentsKey:      "{}",
		ExecutionScopeKey: "global",
		Scope:             commandbindings.Application,
		Enabled:           true,
		LayerActive:       true,
		Condition:         commandbindings.Facts{commandbindings.Profile: "profile-a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.host.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) {
		return p.principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	return p, configuration
}

func TestCommandDeckControllerInputProfileABAAcceptsWithoutRebuild(t *testing.T) {
	a := deckConfiguredFixture(t)
	p, configuration := deckProfileConfiguration(t, a, commandWorkspaceTabCloseID)
	a.emitter = commandOSBootstrapEmitter(func(string, any) {})
	versions, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	identities := deckTriggerIdentities(configuration, p.registry, versions)
	instanceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := &commandDeckController{
		p: p, ctx: context.Background(), versions: versions, identities: identities,
		pressed: map[string]bool{}, instances: map[string]commandDeckInstance{
			"test-deck": {id: uuid.Must(uuid.NewV7()).String(), ctx: instanceCtx},
		}, models: map[string]string{}, generation: p.deckInputGeneration(),
	}
	press := func() (string, error) {
		ack, err := controller.Input(context.Background(), commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown})
		return ack.InvocationID, err
	}
	release := func() {
		_, _ = controller.Input(context.Background(), commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyUp})
	}
	first, err := press()
	if err != nil || first == "" {
		t.Fatalf("press A não criou reserva auditada: invocation=%q err=%v", first, err)
	}
	release()
	before, _, _, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	a.workspaceMgr.SetDefaultProfile("profile-b")
	if _, err := press(); err == nil {
		t.Fatal("press B resolveu binding sem rebuild")
	}
	release()
	a.workspaceMgr.SetDefaultProfile("profile-a")
	last, err := press()
	if err != nil || last == "" || last == first {
		t.Fatalf("press A após B não criou nova reserva: first=%q last=%q err=%v", first, last, err)
	}
	release()
	after, _, _, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil || after != before {
		t.Fatalf("troca de perfil provocou rebuild: before=%p after=%p err=%v", before, after, err)
	}
}

func TestCommandDeckControllerInputProfileBoundLocalUIIsBlocked(t *testing.T) {
	a := deckConfiguredFixture(t)
	p, configuration := deckProfileConfiguration(t, a, "navigation.settings.open")
	a.emitter = commandOSBootstrapEmitter(func(string, any) {})
	versions, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	instanceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &commandDeckController{
		p: p, ctx: context.Background(), versions: versions, identities: deckTriggerIdentities(configuration, p.registry, versions),
		pressed: map[string]bool{}, instances: map[string]commandDeckInstance{"test-deck": {id: uuid.Must(uuid.NewV7()).String(), ctx: instanceCtx}},
		models: map[string]string{}, generation: p.deckInputGeneration(),
	}
	if ack, err := c.Input(context.Background(), commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown}); err == nil || ack.Accepted {
		t.Fatalf("localUI condicionado escapou pelo controller: ack=%+v err=%v", ack, err)
	}
}

func TestCommandDeckReservationProfileABADoesNotTakeAndStampMissingRejects(t *testing.T) {
	a := deckConfiguredFixture(t)
	p, _ := deckProfileConfiguration(t, a, commandWorkspaceTabCloseID)
	versions, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	stamp, err := p.commandOriginFacts(context.Background(), commandcatalog.StreamDeck, []commandbindings.Field{commandbindings.Profile})
	if err != nil {
		t.Fatal(err)
	}
	instanceID := uuid.Must(uuid.NewV7()).String()
	reservation, err := p.beginDeckCommand(context.Background(), "test-deck", instanceID, 0, versions, commandWorkspaceTabCloseID, p.deckInputGeneration(), stamp.version)
	if err != nil {
		t.Fatalf("reserva com stamp válida recusada: %v", err)
	}
	a.workspaceMgr.SetDefaultProfile("profile-b")
	a.workspaceMgr.SetDefaultProfile("profile-a")
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("A-B-A entre reserva e Take foi aceito")
	}
	if _, err := p.beginDeckCommand(context.Background(), "test-deck", uuid.Must(uuid.NewV7()).String(), 0, versions, commandWorkspaceTabCloseID, p.deckInputGeneration(), ""); err == nil {
		t.Fatal("reserva profile-bound aceitou stamp ausente")
	}
}

type profileDeckDiscoveryDriver struct{}

func (profileDeckDiscoveryDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 2, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}
	return []commanddeck.PhysicalDevice{{ID: "test-deck", Model: model}, {ID: "other-deck", Model: model}}, nil
}

func (profileDeckDiscoveryDriver) Open(context.Context, commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	return nil, commanddeck.ErrDriverUnavailable
}

func TestCommandDeckDiscoveryProfileFalseKeepsOnlyPotentialDevice(t *testing.T) {
	a := deckConfiguredFixture(t)
	p, _ := deckProfileConfiguration(t, a, "navigation.settings.open")
	a.workspaceMgr.SetDefaultProfile("profile-b")
	identities, _, err := p.deckTriggerMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(identities["test-deck"][0]) != 1 {
		t.Fatalf("binding potencial não preservado sob perfil falso: %+v", identities)
	}
	devices, err := (commandDeckDriverFilter{Driver: profileDeckDiscoveryDriver{}, potential: identities}).Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != "test-deck" {
		t.Fatalf("discovery abriu dispositivos indevidos: %+v", devices)
	}
}

func TestCommandDeckProfileRefreshesFrameWithoutReopeningPhysicalConnection(t *testing.T) {
	a := deckConfiguredFixture(t)
	p, _ := deckProfileConfiguration(t, a, commandWorkspaceTabCloseID)
	a.workspaceMgr.SetDefaultProfile("profile-b")
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("dispositivo potencial não foi aberto com perfil inicialmente falso")
	}
	select {
	case <-handle.writes:
	case <-time.After(3 * time.Second):
		t.Fatal("frame inicial vazio não foi publicado")
	}
	select {
	case <-driver.opened:
		t.Fatal("discovery abriu a conexão mais de uma vez antes da troca de perfil")
	default:
	}

	a.workspaceMgr.SetDefaultProfile("profile-a")
	populated := false
	deadline := time.After(5 * time.Second)
	for !populated {
		select {
		case frame := <-handle.writes:
			for _, update := range frame.Updates {
				if update.View.Title != "" {
					populated = true
				}
			}
		case <-deadline:
			t.Fatal("mudança para perfil A não atualizou o frame")
		}
	}
	select {
	case <-driver.opened:
		t.Fatal("mudança para perfil A reabriu o hardware")
	default:
	}

	a.workspaceMgr.SetDefaultProfile("profile-b")
	cleared := false
	deadline = time.After(5 * time.Second)
	for !cleared {
		select {
		case frame := <-handle.writes:
			if len(frame.Updates) == 0 {
				continue
			}
			cleared = true
			for _, update := range frame.Updates {
				if update.View.Title != "" {
					cleared = false
				}
			}
		case <-deadline:
			t.Fatal("mudança para perfil B não limpou o frame")
		}
	}
	select {
	case <-driver.opened:
		t.Fatal("mudança para perfil B reabriu o hardware")
	default:
	}
}

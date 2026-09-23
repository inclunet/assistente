package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commanddeck"
)

type discoveryDriver struct {
	devices     []commanddeck.PhysicalDevice
	err         error
	opened      int
	calls       int
	onEnumerate func()
}

func (d *discoveryDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	d.calls++
	if d.onEnumerate != nil {
		d.onEnumerate()
	}
	return d.devices, d.err
}
func (d *discoveryDriver) Open(context.Context, commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	d.opened++
	return nil, errors.New("discovery must not open devices")
}
func discoveryDevice(id string) commanddeck.PhysicalDevice {
	return commanddeck.PhysicalDevice{ID: commanddeck.DeviceID(id), Model: commanddeck.Model{ID: "test", Name: "Test Deck", Rows: 3, Columns: 5, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}}
}

func TestCommandDeckDiscoveryEnumeratesUnconfiguredDevicesWithoutOpening(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	d := &discoveryDriver{devices: []commanddeck.PhysicalDevice{discoveryDevice("ZZ"), discoveryDevice("AA"), discoveryDevice("AA"), discoveryDevice("bad:serial")}}
	p.deckDriver = d
	published := false
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-discovery" {
			published = true
		}
	})
	if _, err := a.GetCommandSettings("pt-BR"); err != nil {
		t.Fatal(err)
	}
	if published || d.calls != 0 {
		t.Fatal("settings must not expose device discovery")
	}
	got := p.discoverDeck(context.Background())
	if got.Status != "ready" || len(got.Devices) != 2 || got.Devices[0].ID != "AA" || got.Devices[0].KeyCount != 15 {
		t.Fatalf("unexpected discovery: %+v", got)
	}
	if d.opened != 0 || d.calls != 1 {
		t.Fatalf("opened=%d enumerations=%d", d.opened, d.calls)
	}
}

func TestCommandDeckDiscoveryFailureDoesNotBreakSettings(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	p.deckDriver = &discoveryDriver{err: errors.New("private native error")}
	got := p.discoverDeck(context.Background())
	if _, err := a.GetCommandSettings("pt-BR"); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || got.Devices == nil || len(got.Devices) != 0 {
		t.Fatalf("unexpected failure: %+v", got)
	}
}

func TestCommandDeckDiscoveryEmptyAndCanceled(t *testing.T) {
	p := &commandProductRuntime{deckDriver: &discoveryDriver{}}
	got := p.discoverDeck(context.Background())
	if got.Status != "ready" || got.Devices == nil || len(got.Devices) != 0 {
		t.Fatalf("unexpected empty: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := p.discoverDeck(ctx); got.Status != "unavailable" {
		t.Fatalf("canceled: %+v", got)
	}
}

func TestCommandDeckDiscoveryDoesNotPublishAfterRequestCancellation(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.ctx = ctx
	d := &discoveryDriver{devices: []commanddeck.PhysicalDevice{discoveryDevice("AA")}, onEnumerate: cancel}
	a.commandProduct.Load().deckDriver = d
	published := false
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-discovery" {
			published = true
		}
	})
	if got := a.commandProduct.Load().discoverDeck(ctx); got.Status != "unavailable" {
		t.Fatal("canceled discovery accepted")
	}
	if _, err := a.GetCommandSettings("pt-BR"); err == nil {
		t.Fatal("canceled request accepted")
	}
	if published {
		t.Fatal("discovery published after cancellation")
	}
}

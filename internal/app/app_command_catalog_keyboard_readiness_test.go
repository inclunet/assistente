package app

import (
	"context"
	"errors"
	"testing"
)

// A UI may consume a map notification immediately, before the catalog refresh
// returns. The final notification must therefore follow lifecycle readiness,
// not just configuration publication.
func TestCommandCatalogRefreshPublishesReadyKeyboardMap(t *testing.T) {
	a := commandJobPublicationApp(t)
	before, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	var maps []LocalCommandKeyboardMap
	var lastErr error
	a.emitter = commandOSBootstrapEmitter(func(name string, _ any) {
		if name != "command:keyboard-map-changed" {
			return
		}
		var view LocalCommandKeyboardMap
		view, lastErr = a.GetLocalCommandKeyboardMap()
		maps = append(maps, view)
	})
	if err := a.refreshCommandProductCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(maps) == 0 || lastErr != nil {
		t.Fatalf("last map notification was not ready: notifications=%d err=%v", len(maps), lastErr)
	}
	after := maps[len(maps)-1]
	if after.Generation == "" || after.Generation == before.Generation || len(after.Bindings) == 0 {
		t.Fatalf("notification did not install a fresh usable map: generation=%q bindings=%d", after.Generation, len(after.Bindings))
	}
}

func TestCommandCatalogRefreshDoesNotPublishReadyMapWhenBootstrapCancelled(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifications := 0
	a.emitter = commandOSBootstrapEmitter(func(name string, _ any) {
		if name == "command:keyboard-map-changed" {
			notifications++
			// Interrupt after configuration publication, before Bootstrap.
			cancel()
		}
	})
	if err := a.refreshCommandProductCatalog(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled bootstrap: %v", err)
	}
	if notifications != 1 {
		t.Fatalf("cancelled bootstrap published readiness: notifications=%d", notifications)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("cancelled replacement served a keyboard map")
	}
}

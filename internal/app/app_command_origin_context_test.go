package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
)

func TestCommandOriginFactsCompoePerfilDoProviderRealNasTresOrigens(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	p := &commandProductRuntime{app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID}
	required := []commandbindings.Field{commandbindings.Profile}

	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck} {
		got, err := p.commandOriginFacts(context.Background(), source, required)
		if err != nil {
			t.Fatalf("%s: commandOriginFacts: %v", source, err)
		}
		if got.facts[commandbindings.Profile] != "tab-profile" || got.version == "" {
			t.Fatalf("%s: contexto composto incorreto: %#v", source, got)
		}
	}
	before, err := p.commandOriginFacts(context.Background(), commandcatalog.Palette, required)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetProfile("profile-b"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetProfile("workspace-profile"); err != nil {
		t.Fatal(err)
	}
	after, err := p.commandOriginFacts(context.Background(), commandcatalog.Palette, required)
	if err != nil {
		t.Fatal(err)
	}
	if before.version == after.version {
		t.Fatal("troca A-B-A do workspace reutilizou a versão antiga")
	}
}

func TestCommandOriginFactsRecusaFatosSemProviderEManagerStale(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	p := &commandProductRuntime{app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID}
	if _, err := p.commandOriginFacts(context.Background(), commandcatalog.Palette, []commandbindings.Field{commandbindings.SurfaceID}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("surface.id sem provider: %v", err)
	}
	app.workspaceMgr = nil
	if _, err := p.commandOriginFacts(context.Background(), commandcatalog.Palette, []commandbindings.Field{commandbindings.Profile}); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("manager stale: %v", err)
	}
}

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

func TestCommandTerminalInterruptCatalogAndDeniedWithoutTarget(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	d, ok := p.registry.Lookup(commandTerminalInterruptID)
	if !ok || len(p.registry.List()) != 151 || !isWorkspaceMutationCommand(d.ID) || isLocalUICommand(d.ID) || !commandDeckLedgerCommand(d) || !d.HasMutableTarget || d.Persistence.Audit != commandcatalog.PersistenceRedacted || d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Result != commandcatalog.PersistenceNever {
		t.Fatalf("invalid classification: %+v", d)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
		if !d.AllowsSource(source) {
			t.Fatal(source)
		}
	}
	for _, source := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.CLI, commandcatalog.System, commandcatalog.Chat} {
		if d.AllowsSource(source) {
			t.Fatal("expanded origin", source)
		}
	}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		if d.Presentation.Locales[locale].Name == "" {
			t.Fatal(locale)
		}
	}
	if r, err := a.BeginUICommand(d.ID); err == nil || r.Ticket != "" {
		t.Fatal("missing manager accepted", r, err)
	}
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	if _, _, _, err := a.captureTerminalInterrupt(context.Background(), p); err == nil {
		t.Fatal("non-terminal workspace accepted")
	}
	if r, err := a.BeginUICommand(d.ID); err == nil || r.Ticket != "" {
		t.Fatal("no process accepted", r, err)
	}
}

func TestCommandTerminalInterruptPreparationCannotSelectOrRetarget(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	reservation, err := p.ui.Reserve(p.uiOwner(), commandTerminalInterruptID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &commandUIRun{reservation: reservation, cancel: cancel, done: make(chan struct{}),
		expiresAt: time.Now().Add(time.Minute), snapshot: workspace.CommandSnapshot{WorkspaceID: p.workspaceID, ActiveTabID: "tab-a"}}
	p.mu.Lock()
	p.uiRuns[reservation.Ticket] = run
	p.mu.Unlock()
	// IDs from the UI must never turn an absent runtime target into authority.
	if err := a.PrepareTerminalInterruptCommand(reservation.Ticket, p.workspaceID, "tab-a", "invented-session", "invented-command"); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("uncaptured target accepted: %v", err)
	}
	if run.terminalPrepared || ctx.Err() == nil {
		t.Fatal("failed preparation was not cancelled")
	}
	if err := a.PrepareTerminalInterruptCommand(reservation.Ticket, p.workspaceID, "tab-a", "other-session", "other-command"); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("preparation can be retried with another target: %v", err)
	}
	if err := a.PrepareTerminalInterruptCommand("unknown-ticket", p.workspaceID, "tab-a", "session", "command"); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("unknown reservation accepted: %v", err)
	}
}

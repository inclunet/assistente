package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

func commandWorkspaceInfoCount(t *testing.T, a *App) int {
	t.Helper()
	items, err := a.workspaceMgr.List()
	if err != nil {
		t.Fatal(err)
	}
	return len(items)
}

func TestCommandWorkspaceCreateCatalogContract(t *testing.T) {
	a := readyCommandProduct(t)
	registry, handlers, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Lookup(commandWorkspaceCreateID)
	if !ok {
		t.Fatal("workspace.create não foi registrado")
	}
	if definition.Effect != commandcatalog.Write || definition.Decision != commandcatalog.NoDecision ||
		!definition.HasMutableTarget || definition.HandlerClassification != commandcatalog.HandlerBackend {
		t.Fatalf("contrato workspace.create inesperado: %+v", definition)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
		if !definition.AllowsSource(source) {
			t.Fatalf("workspace.create não aceita origem %q", source)
		}
	}
	if definition.Context.None || len(definition.Context.Facts) != 1 ||
		definition.Context.Facts[0].Provider != "workspace" || definition.Context.Facts[0].Fact != "active_tab" ||
		definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
		t.Fatalf("contexto workspace.create não é active_tab/ExactVersion: %+v", definition.Context)
	}
	if definition.ArgumentsSchema == nil || definition.ArgumentsSchema.Type != commandcatalog.SchemaObject ||
		definition.ResultSchema == nil || definition.ResultSchema.Type != commandcatalog.SchemaObject {
		t.Fatalf("schemas workspace.create inesperados: args=%+v result=%+v", definition.ArgumentsSchema, definition.ResultSchema)
	}
	if definition.Persistence.Arguments != commandcatalog.PersistenceNever ||
		definition.Persistence.Result != commandcatalog.PersistenceSummary ||
		definition.Persistence.Audit != commandcatalog.PersistenceRedacted {
		t.Fatalf("política workspace.create inesperada: %+v", definition.Persistence)
	}
	if definition.HandlerRoute != "contextual/workspace/create" {
		t.Fatalf("rota workspace.create=%q", definition.HandlerRoute)
	}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		metadata, ok := definition.Presentation.Locales[locale]
		if !ok || metadata.Name == "" || metadata.Description == "" || metadata.Category == "" {
			t.Fatalf("metadata %s incompleto: %+v", locale, definition.Presentation)
		}
	}
	if handler, ok := handlers[commandWorkspaceCreateID]; !ok || handler.Start == nil || handler.Contract.Route != definition.HandlerRoute {
		t.Fatalf("handler workspace.create ausente ou divergente: %+v", handler)
	}
}

func TestCommandWorkspaceCreateIsDurableAndNotLocalUI(t *testing.T) {
	a := readyCommandProduct(t)
	definition, ok := a.commandProduct.Load().registry.Lookup(commandWorkspaceCreateID)
	if !ok {
		t.Fatal("workspace.create ausente")
	}
	if commandExecutionClassForDefinition(definition) != commandExecutionDurable {
		t.Fatalf("workspace.create não é durable: %v", commandExecutionClassForDefinition(definition))
	}
	if commandExecutionClassForID(commandWorkspaceCreateID) != commandExecutionUnknown {
		t.Fatal("workspace.create entrou indevidamente na lista local_ui")
	}
}

func TestCommandWorkspaceCreatePaletteCommitAndSourceRevalidation(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := commandWorkspaceInfoCount(t, a)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
			t.Fatalf("workspace.create palette: %v", err)
		}
		result := getUIResultEventually(t, a, reservation.Ticket)
		if result.Status != "succeeded" || commandWorkspaceInfoCount(t, a) != before+1 {
			t.Fatalf("workspace.create não persistiu uma entrada: result=%+v count=%d before=%d", result, commandWorkspaceInfoCount(t, a), before)
		}
	})
	t.Run("source stale após handoff", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := commandWorkspaceInfoCount(t, a)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		p := a.commandProduct.Load()
		p.mu.Lock()
		run := p.uiRuns[reservation.Ticket]
		if run == nil {
			p.mu.Unlock()
			t.Fatal("run workspace.create ausente")
		}
		run.sourceValid = func() bool { return false }
		p.mu.Unlock()
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil || !errors.Is(err, commandexecution.ErrStale) {
			t.Fatalf("commit com source revalidada inválida: %v", err)
		}
		if got := commandWorkspaceInfoCount(t, a); got != before {
			t.Fatalf("source stale criou workspace: got=%d before=%d", got, before)
		}
	})
}

func TestCommandWorkspaceCreateKeyboardLocalCommit(t *testing.T) {
	a := readyCommandProduct(t)
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyN", Modifiers: []string{"Control", "Shift"}}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != commandWorkspaceCreateID || binding.Handler != "contextual" {
		t.Fatalf("Ctrl+Shift+N workspace.create inesperado: %+v", binding)
	}
	before := commandWorkspaceInfoCount(t, a)
	reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
	if err != nil || reservation == nil {
		t.Fatalf("Begin teclado workspace.create: reservation=%+v err=%v", reservation, err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit teclado workspace.create: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" || commandWorkspaceInfoCount(t, a) != before+1 {
		t.Fatalf("workspace.create teclado não persistiu: result=%+v", result)
	}
}

func TestCommandWorkspaceCreateDeckGuardUsesDurablePath(t *testing.T) {
	a := readyCommandProduct(t)
	definition, ok := a.commandProduct.Load().registry.Lookup(commandWorkspaceCreateID)
	if !ok || !commandDeckUICommand(commandWorkspaceCreateID) || !commandDeckLedgerCommand(definition) {
		t.Fatalf("workspace.create não está exposto como mutação Deck durable: ok=%v definition=%+v", ok, definition)
	}
}

func TestCommandWorkspaceCreateDeckNativeCommitAuditsSource(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer, CommandID: commandWorkspaceCreateID, TriggerType: "streamdeck.key",
			TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true,
		})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	reservations := make(chan commandui.Reservation, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-ui-reservation" {
			reservations <- value.(commandui.Reservation)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("adapter nativo não abriu")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	before := commandWorkspaceInfoCount(t, a)
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var reservation commandui.Reservation
	select {
	case reservation = <-reservations:
	case <-time.After(4 * time.Second):
		t.Fatal("reserva Deck ausente")
	}
	if reservation.CommandID != commandWorkspaceCreateID {
		t.Fatalf("comando Deck=%q", reservation.CommandID)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" || commandWorkspaceInfoCount(t, a) != before+1 {
		t.Fatalf("workspace.create Deck não persistiu: %+v", result)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "streamdeck.key" {
		t.Fatalf("source=%q, esperado streamdeck.key", source.SourceType)
	}
}

func TestCommandWorkspaceCreateCommitGuardsAndKeyboardRepeat(t *testing.T) {
	t.Run("repeat recusado", func(t *testing.T) {
		a := readyCommandProduct(t)
		view, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		shortcut := LocalCommandShortcut{Version: 1, Code: "KeyN", Modifiers: []string{"Control", "Shift"}}
		if _, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, true); !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("repeat aceito: %v", err)
		}
	})

	t.Run("forjado e cancelado", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := commandWorkspaceInfoCount(t, a)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID+"-forged"); err == nil {
			t.Fatal("handoff forjado aceito")
		}
		if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("commit após cancelamento aceito")
		}
		if got := commandWorkspaceInfoCount(t, a); got != before {
			t.Fatalf("cancelamento criou workspace: %d", got)
		}

	})

	t.Run("snapshot stale", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := commandWorkspaceInfoCount(t, a)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		if err := a.workspaceMgr.AddTab(workspace.Tab{ID: "workspace-create-stale", Type: workspace.TabTypeEditor}); err != nil {
			t.Fatal(err)
		}
		if err := a.workspaceMgr.SetActiveTab("workspace-create-stale"); err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); !errors.Is(err, commandexecution.ErrStale) {
			t.Fatalf("snapshot stale retornou %v", err)
		}
		if got := commandWorkspaceInfoCount(t, a); got != before {
			t.Fatalf("snapshot stale criou workspace: %d", got)
		}

	})

	t.Run("replay", func(t *testing.T) {
		a := readyCommandProduct(t)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("replay aceito")
		}
	})

	t.Run("sessão revogada", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := commandWorkspaceInfoCount(t, a)
		reservation := beginUICommand(t, a, commandWorkspaceCreateID)
		handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceCreateID)
		principal := a.commandProduct.Load().principal
		if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", principal.SessionID).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("sessão revogada aceitou commit")
		}
		if got := commandWorkspaceInfoCount(t, a); got != before {
			t.Fatalf("sessão revogada criou workspace: %d", got)
		}
	})
}

package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/ossession"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

type commandOSBootstrapEmitter func(string, any)

func (f commandOSBootstrapEmitter) Emit(name string, data any) { f(name, data) }

func TestCommandOSObservationRecoversBootstrapBeforeFirstObservation(t *testing.T) {
	a, _ := appLifecycleProductMountFixture(t)
	// A observação valida o catálogo operacional, sem criar sessão/PTY.
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.commandHost.SetOSSessionState(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	a.bootstrapCommandLifecycleAfterUnlock(ctx)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	ready := make(chan LocalCommandKeyboardMap, 4)
	a.emitter = commandOSBootstrapEmitter(func(name string, _ any) {
		if name == "command:keyboard-map-changed" {
			if view, err := a.GetLocalCommandKeyboardMap(); err == nil && len(view.Bindings) == 62 {
				ready <- view
			}
		}
	})
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("teclado disponível sem observação inicial do SO")
	}
	observations := make(chan ossession.State)
	observed := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.observeCommandOSSession(ctx, func(ctx context.Context, observe func(ossession.State) error) error {
			for {
				select {
				case state := <-observations:
					if err := observe(state); err != nil {
						return err
					}
					observed <- struct{}{}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("monitor não encerrou")
		}
	})
	emit := func(state ossession.State) {
		t.Helper()
		select {
		case observations <- state:
		case <-time.After(3 * time.Second):
			t.Fatal("monitor não recebeu observação")
		}
		select {
		case <-observed:
		case <-time.After(3 * time.Second):
			t.Fatal("callback bloqueou observação do SO")
		}
	}
	emit(ossession.State{Known: true})
	var initial LocalCommandKeyboardMap
	select {
	case initial = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("observação inicial não notificou mapa executável após bootstrap")
	}
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil || !settings.KeyboardOperational {
		t.Fatalf("configurações: %+v err=%v", settings, err)
	}
	items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"})
	if err != nil || len(items) != 149 {
		t.Fatalf("catálogo: %+v err=%v", items, err)
	}
	for _, item := range items {
		// Recovering OS readiness must not invent bindings for manual actions.
		if isCommandLayerAction(item.ID) {
			if item.Available || item.ReadinessReason == "" {
				t.Fatalf("bootstrap liberou ação de camada sem binding: %+v", item)
			}
			continue
		}
		if item.ID == commandGlobalJobID || item.ID == commandGlobalVoiceID {
			if item.Available || item.ReadinessReason == "" || len(item.AllowedSources) != 1 || item.AllowedSources[0] != "keyboard.global" {
				t.Fatalf("global exposto como comando de paleta: %+v", item)
			}
			continue
		}
		if !item.Available {
			if item.ID == commandConversationClearID || item.ID == commandTerminalInterruptID || isChatActionCommand(item.ID) || isChatMessageCommand(item.ID) || isPageMutationCommand(item.ID) || item.ID == commandTerminalSessionCreateID || item.ID == commandTerminalSessionCloseID {
				if item.ReadinessReason == "" {
					t.Fatal("clear indisponível sem motivo")
				}
				continue
			}
			t.Fatalf("comando indisponível: %+v", item)
		}
	}
	emit(ossession.State{Known: true, Locked: true})
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("lock preservou mapa executável")
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyT", Modifiers: []string{"Control"}}
	if _, err := a.BeginLocalCommandUIKey(initial.Generation, shortcut, false); err == nil {
		t.Fatal("lock aceitou atalho")
	}
	emit(ossession.State{Known: true})
	var restored LocalCommandKeyboardMap
	select {
	case restored = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("unlock não recompôs mapa")
	}
	if restored.Generation == initial.Generation {
		t.Fatal("unlock reutilizou geração anterior")
	}
	reservation, err := a.BeginLocalCommandUIKey(restored.Generation, shortcut, false)
	if err != nil || reservation == nil || reservation.CommandID != commandWorkspaceTabChatCreateID {
		t.Fatalf("Control+T após unlock: %+v err=%v", reservation, err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
}

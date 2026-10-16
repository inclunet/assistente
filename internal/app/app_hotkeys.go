package app

import (
	"context"
	"runtime"

	"assistente/internal/hotkey"
	"assistente/internal/jobs"
	"assistente/internal/logging"
)

// ============================================================================
// Global Hotkeys
// ============================================================================

// initGlobalHotkeys inicializa o gerenciador de hotkeys.
func (a *App) initGlobalHotkeys() {
	// A CLI usa o mesmo bootstrap do App, mas não possui superfície DOM para
	// ownership nem deve reservar hotkeys do desktop ao restaurar sua sessão.
	if a == nil || !a.chatDesktopIngress || runtime.GOOS != "windows" {
		return
	}
	a.hotkeyCtrl.Init()
	barrier := hotkey.NewOwnershipBarrier(func(frame hotkey.OwnershipFrame) {
		if a.emitter != nil {
			a.emitter.Emit("command:global-ownership", frame)
		}
	})
	if err := a.hotkeyCtrl.Manager().SetOwnershipBarrier(barrier); err != nil {
		barrier.Close()
		a.hotkeyCtrl.Stop()
		logging.Errorf(context.Background(), "app.hotkeys", "Não foi possível coordenar ownership de hotkeys: %v", err)
		return
	}
	a.globalHotkeyOwnership = barrier
	a.decisionRepeatHotkeys = newDecisionRepeatHotkeys(
		func(callback hotkey.HotkeyCallback) (func() error, error) {
			modifiers, key, err := hotkey.ParseCombination("Ctrl+Shift+R")
			if err != nil {
				return nil, err
			}
			return a.hotkeyCtrl.Manager().ReserveTemporary(modifiers, key, callback)
		},
		func(event decisionRepeatEvent) {
			if a.emitter != nil {
				a.emitter.Emit("command:decision-repeat", event)
			}
		},
		func() bool {
			a.authMu.RLock()
			host := a.commandHost
			a.authMu.RUnlock()
			if host == nil {
				return false
			}
			ready, err := host.InteractiveSessionReady(context.Background())
			return err == nil && ready
		},
	)
}

// GetGlobalCommandOwnership expõe somente as reservas de teclado. Não resolve
// comandos, não autentica ocorrências e não concede autoridade para execução.
func (a *App) GetGlobalCommandOwnership() hotkey.OwnershipFrame {
	if a == nil || a.globalHotkeyOwnership == nil {
		return hotkey.OwnershipFrame{Version: 1, Platform: "unsupported", Combinations: []hotkey.OwnershipCombination{}}
	}
	return a.globalHotkeyOwnership.Snapshot()
}

// AckGlobalCommandOwnership confirma a exclusão DOM já aplicada. Só a revisão
// em publicação pode liberar o registro nativo; não existe dispatch por aqui.
func (a *App) AckGlobalCommandOwnership(instanceID string, revision uint64) bool {
	return a != nil && a.globalHotkeyOwnership != nil && a.globalHotkeyOwnership.Ack(instanceID, revision)
}

// registerActiveProfileHotkeys delega para o HotkeysController.
func (a *App) registerActiveProfileHotkeys() {
	if a == nil || a.hotkeyCtrl == nil || runtime.GOOS != "windows" || a.globalHotkeyOwnership == nil {
		return
	}
	a.hotkeyCtrl.RegisterActiveProfileHotkeys()
}

func (a *App) commandGlobalHotkeyRegistrar() jobs.HotkeyRegistrar {
	if a == nil || a.hotkeyCtrl == nil || runtime.GOOS != "windows" || a.globalHotkeyOwnership == nil {
		return nil
	}
	return a.hotkeyCtrl.Manager()
}

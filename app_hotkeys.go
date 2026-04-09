package main

import (
	"log"
	"time"

	"assistente/internal/hotkey"
	"assistente/internal/profiles"
)

// ============================================================================
// Global Hotkeys
// ============================================================================

// HotkeyInfo informações sobre um hotkey
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados
func (a *App) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// initGlobalHotkeys inicializa o gerenciador de hotkeys
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// registerActiveProfileHotkeys registra os hotkeys do perfil ativo
func (a *App) registerActiveProfileHotkeys() {
	if a.hotkeyManager == nil {
		return
	}

	activeProfile, err := a.profileManager.GetActive()
	if err != nil {
		log.Printf("[Hotkey] Erro ao obter perfil ativo: %v", err)
		return
	}

	// Remove todos os hotkeys anteriores
	a.hotkeyManager.UnregisterAllProfileHotkeys()

	if activeProfile == nil || len(activeProfile.Input.Triggers) == 0 {
		return
	}

	hotkeyCount := 0
	for _, trigger := range activeProfile.Input.Triggers {
		if !trigger.Enabled || trigger.Hotkey == "" {
			continue
		}
		hotkeyCount++

		t := trigger // Captura variável para closure

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger tipo %s...", t.Hotkey, t.Type)
		_, err := a.hotkeyManager.RegisterProfileHotkey(
			1, // Profile ID fixo (perfil global)
			t.Hotkey,
			t.Type == profiles.TriggerTypeHotkey,
			t.HotkeyBringToFront,
			func() {
				// Throttle: ignora se disparou recentemente
				now := time.Now()
				triggerKey := uint(hotkeyCount) // Usa index como key
				if lastFired, ok := a.hotkeyLastFired[triggerKey]; ok {
					elapsed := now.Sub(lastFired).Milliseconds()
					if elapsed < a.hotkeyThrottleMs {
						return
					}
				}
				a.hotkeyLastFired[triggerKey] = now

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger tipo %s", t.Type)
				a.emitter.Emit("interaction:hotkey:triggered", map[string]interface{}{
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				if t.HotkeyGlobal && t.HotkeyBringToFront {
					a.windowPort.Show()
				}
			},
		)
		if err != nil {
			log.Printf("[Hotkey] ERRO ao registrar hotkey '%s': %v", t.Hotkey, err)
		} else {
			log.Printf("[Hotkey] Hotkey '%s' registrada com sucesso", t.Hotkey)
		}
	}

	log.Printf("[Hotkey] Total: %d hotkeys registradas para perfil ativo", hotkeyCount)
}

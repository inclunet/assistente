package controllers

import (
	"log"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/hotkey"
	"assistente/internal/profiles"
)

// HotkeyInfo informações sobre um hotkey (mantido para compatibilidade com bindings futuros).
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// HotkeysControllerConfig agrupa as dependências do HotkeysController.
type HotkeysControllerConfig struct {
	ProfileMgr *profiles.Manager
	Emitter    ports.Emitter
	WindowPort ports.WindowPort
	ThrottleMs int64
}

// HotkeysController é o adapter primário (Inbound) para hotkeys globais.
// Centraliza estado (manager, lastFired, throttle) que pertencia ao App.
type HotkeysController struct {
	profileMgr *profiles.Manager
	emitter    ports.Emitter
	windowPort ports.WindowPort
	throttleMs int64

	mu        sync.Mutex
	manager   *hotkey.Manager
	lastFired map[uint]time.Time
}

// NewHotkeysController cria um HotkeysController com suas dependências.
func NewHotkeysController(cfg HotkeysControllerConfig) *HotkeysController {
	throttleMs := cfg.ThrottleMs
	if throttleMs <= 0 {
		throttleMs = 1000
	}
	return &HotkeysController{
		profileMgr: cfg.ProfileMgr,
		emitter:    cfg.Emitter,
		windowPort: cfg.WindowPort,
		throttleMs: throttleMs,
		lastFired:  make(map[uint]time.Time),
	}
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados neste sistema.
func (c *HotkeysController) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// Init inicializa o gerenciador de hotkeys.
func (c *HotkeysController) Init() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}
	c.manager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// RegisterActiveProfileHotkeys registra os hotkeys do perfil ativo.
func (c *HotkeysController) RegisterActiveProfileHotkeys() {
	if c.manager == nil {
		return
	}

	activeProfile, err := c.profileMgr.GetActive()
	if err != nil {
		log.Printf("[Hotkey] Erro ao obter perfil ativo: %v", err)
		return
	}

	c.manager.UnregisterAllProfileHotkeys()

	if activeProfile == nil || len(activeProfile.Input.Triggers) == 0 {
		return
	}

	hotkeyCount := 0
	for _, trigger := range activeProfile.Input.Triggers {
		if !trigger.Enabled || trigger.Hotkey == "" {
			continue
		}
		hotkeyCount++

		t := trigger
		triggerKey := uint(hotkeyCount)

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger tipo %s...", t.Hotkey, t.Type)
		_, err := c.manager.RegisterProfileHotkey(
			1,
			t.Hotkey,
			t.Type == profiles.TriggerTypeHotkey,
			t.HotkeyBringToFront,
			func() {
				now := time.Now()
				c.mu.Lock()
				if lastFired, ok := c.lastFired[triggerKey]; ok {
					if now.Sub(lastFired).Milliseconds() < c.throttleMs {
						c.mu.Unlock()
						return
					}
				}
				c.lastFired[triggerKey] = now
				c.mu.Unlock()

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger tipo %s", t.Type)
				c.emitter.Emit("interaction:hotkey:triggered", map[string]interface{}{
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				if t.HotkeyGlobal && t.HotkeyBringToFront {
					c.windowPort.Show()
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

// Stop para o gerenciador de hotkeys.
func (c *HotkeysController) Stop() {
	if c.manager != nil {
		c.manager.Stop()
	}
}

// Manager retorna o *hotkey.Manager interno para uso por outros componentes da infra.
func (c *HotkeysController) Manager() *hotkey.Manager {
	return c.manager
}

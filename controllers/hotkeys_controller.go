package controllers

import (
	"assistente/internal/hotkey"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"context"
	"sync"
	"time"
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
	ProfileMgr            *profiles.Manager
	DispatchCommandHotkey func(context.Context, ProfileHotkeyOccurrence) error
	ThrottleMs            int64
}

// HotkeysController é o adapter primário (Inbound) para hotkeys globais.
// O controller só admite a ocorrência nativa e faz o handoff para o executor
// injetado. Ele não conhece eventos de UI nem executa efeitos de janela.
type HotkeysController struct {
	profileMgr *profiles.Manager
	dispatch   func(context.Context, ProfileHotkeyOccurrence) error
	throttleMs int64

	mu             sync.Mutex
	registrationMu sync.Mutex
	generation     uint64
	manager        *hotkey.Manager
	registrar      profileHotkeyRegistrar
	registration   *profileHotkeyRegistration
	lastFired      map[uint]time.Time
}

type profileHotkeyRegistrar interface {
	RegisterProfileHotkey(int, string, bool, bool, hotkey.HotkeyCallback) (int, error)
	UnregisterAllProfileHotkeys()
}

type profileHotkeyRegistration struct {
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewHotkeysController cria um HotkeysController com suas dependências.
func NewHotkeysController(cfg HotkeysControllerConfig) *HotkeysController {
	throttleMs := cfg.ThrottleMs
	if throttleMs <= 0 {
		throttleMs = 1000
	}
	return &HotkeysController{
		profileMgr: cfg.ProfileMgr,
		dispatch:   cfg.DispatchCommandHotkey,
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
	c.registrationMu.Lock()
	defer c.registrationMu.Unlock()
	if !hotkey.IsSupported() {
		logging.Println(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}
	c.manager = hotkey.GetManager()
	c.registrar = c.manager
	logging.Println(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// RegisterActiveProfileHotkeys registra os hotkeys do perfil ativo.
func (c *HotkeysController) RegisterActiveProfileHotkeys() {
	c.registrationMu.Lock()
	defer c.registrationMu.Unlock()

	registration, generation := c.retireRegistration()
	if registration != nil {
		registration.cancel()
	}

	registrar := c.registrar
	if registrar != nil {
		// A geração anterior deixa de ser válida antes da retirada nativa. Isso
		// também fecha a janela em que um callback antigo poderia ser admitido.
		registrar.UnregisterAllProfileHotkeys()
	}
	if registrar == nil || c.dispatch == nil || c.profileMgr == nil {
		return
	}

	active, err := c.profileMgr.GetActiveAndSlug()
	if err != nil {
		logging.Errorf(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Erro ao obter perfil ativo: %v", err)
		return
	}
	if active == nil || active.Profile == nil || !active.Profile.Input.Enabled {
		return
	}

	bindings, err := buildProfileHotkeyBindings(active.Slug, active.Profile)
	if err != nil {
		logging.Errorf(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Erro ao projetar hotkeys do perfil ativo: %v", err)
		return
	}
	if len(bindings) == 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	current := &profileHotkeyRegistration{generation: generation, ctx: ctx, cancel: cancel}
	c.mu.Lock()
	if c.generation != generation {
		c.mu.Unlock()
		cancel()
		return
	}
	c.registration = current
	c.mu.Unlock()

	for index, binding := range bindings {
		binding := binding
		triggerKey := uint(index + 1)
		logging.Infof(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Registrando hotkey '%s' para trigger tipo %s...", binding.Hotkey, binding.TriggerType)
		_, err := registrar.RegisterProfileHotkey(
			1,
			binding.Hotkey,
			binding.TriggerType == profiles.TriggerTypeHotkey,
			binding.BringToFront,
			func() {
				c.handleNativeHotkey(current, triggerKey, binding)
			},
		)
		if err != nil {
			logging.Errorf(context.Background(), "controllers.hotkeys-controller", "[Hotkey] ERRO ao registrar hotkey '%s': %v", binding.Hotkey, err)
		} else {
			logging.Infof(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Hotkey '%s' registrada com sucesso", binding.Hotkey)
		}
	}

	logging.Infof(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Total: %d hotkeys registradas para perfil ativo", len(bindings))
}

func (c *HotkeysController) retireRegistration() (*profileHotkeyRegistration, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.registration
	c.registration = nil
	c.generation++
	c.lastFired = make(map[uint]time.Time)
	return previous, c.generation
}

func (c *HotkeysController) handleNativeHotkey(registration *profileHotkeyRegistration, triggerKey uint, binding ProfileHotkeyBinding) {
	now := time.Now()
	c.mu.Lock()
	if c.registration != registration || c.generation != registration.generation || c.dispatch == nil {
		c.mu.Unlock()
		return
	}
	if lastFired, ok := c.lastFired[triggerKey]; ok && now.Sub(lastFired).Milliseconds() < c.throttleMs {
		c.mu.Unlock()
		return
	}
	c.lastFired[triggerKey] = now
	dispatch := c.dispatch
	occurrence := ProfileHotkeyOccurrence{
		controller:   c,
		registration: registration,
		binding:      binding,
	}
	c.mu.Unlock()

	// O dispatcher pode executar Stop/reload ou bloquear aguardando o App. O
	// mutex do controller não atravessa o handoff.
	if err := dispatch(registration.ctx, occurrence); err != nil {
		logging.Errorf(context.Background(), "controllers.hotkeys-controller", "[Hotkey] Dispatch de ocorrência falhou: %v", err)
	}
}

// Stop para o gerenciador de hotkeys e aposenta a geração atual.
func (c *HotkeysController) Stop() {
	c.registrationMu.Lock()
	defer c.registrationMu.Unlock()

	registration, _ := c.retireRegistration()
	if registration != nil {
		registration.cancel()
	}
	if c.registrar != nil {
		c.registrar.UnregisterAllProfileHotkeys()
	}
	if c.manager != nil {
		c.manager.Stop()
	}
}

// Manager retorna o *hotkey.Manager interno para uso por outros componentes da infra.
func (c *HotkeysController) Manager() *hotkey.Manager {
	return c.manager
}

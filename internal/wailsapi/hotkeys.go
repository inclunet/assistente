package wailsapi

import (
	"assistente/controllers"
	"context"
	"sync"
)

// Hotkeys é o bind Wails do domínio hotkeys (AEP-0088).
//
// IsGlobalHotkeySupported é informação de plataforma (suporte OS), mas o FE
// só chama pós-login (ou não chama ainda). Preferimos WithUser fail-closed,
// alinhado aos outros binds — sem allowlist unauth.
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Hotkeys struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.HotkeysController
}

// NewHotkeys cria o bind vazio; AttachHotkeys preenche session + controller no startup.
func NewHotkeys() *Hotkeys {
	return &Hotkeys{}
}

// AttachHotkeys associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachHotkeys(h *Hotkeys, session Session, ctrl *controllers.HotkeysController) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.session = session
	h.ctrl = ctrl
}

func (h *Hotkeys) deps() (Session, *controllers.HotkeysController, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.session == nil || h.ctrl == nil {
		return nil, nil, ErrHotkeysNotWired
	}
	return h.session, h.ctrl, nil
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados neste sistema.
func (h *Hotkeys) IsGlobalHotkeySupported() (bool, error) {
	session, ctrl, err := h.deps()
	if err != nil {
		return false, err
	}
	return WithUser(session, func(ctx context.Context) (bool, error) {
		return ctrl.IsGlobalHotkeySupported(), nil
	})
}

package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// FSTrust é o bind Wails do domínio fstrust / path allowlist (AEP-0092).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type FSTrust struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.FSTrustController
}

// NewFSTrust cria o bind vazio; AttachFSTrust preenche session + controller no startup.
func NewFSTrust() *FSTrust {
	return &FSTrust{}
}

// AttachFSTrust associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachFSTrust(f *FSTrust, session Session, ctrl *controllers.FSTrustController) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.session = session
	f.ctrl = ctrl
}

func (f *FSTrust) deps() (Session, *controllers.FSTrustController, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.session == nil || f.ctrl == nil {
		return nil, nil, ErrFSTrustNotWired
	}
	return f.session, f.ctrl, nil
}

// GetPathAllowlist lista as entradas de allowlist de path.
func (f *FSTrust) GetPathAllowlist() ([]apidto.PathAllowlistView, error) {
	session, ctrl, err := f.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.PathAllowlistView, error) {
		return ctrl.GetPathAllowlist(ctx), nil
	})
}

// RemovePathAllowlistEntry remove uma entrada persistida por (scope, path, kind, operation, effect).
func (f *FSTrust) RemovePathAllowlistEntry(scope, path, kind, operation, effect string) error {
	session, ctrl, err := f.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RemovePathAllowlistEntry(ctx, scope, path, kind, operation, effect)
	})
	return err
}

// AddPathDenyEntry cria uma proibição persistente (só EffectDeny).
func (f *FSTrust) AddPathDenyEntry(path, kind, operation, scope, reason string) error {
	session, ctrl, err := f.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.AddPathDenyEntry(ctx, path, kind, operation, scope, reason)
	})
	return err
}

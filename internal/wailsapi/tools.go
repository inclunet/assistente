package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// Tools é o bind Wails do domínio tools (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Tools struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.ToolsController
}

// NewTools cria o bind vazio; AttachTools preenche session + controller no startup.
func NewTools() *Tools {
	return &Tools{}
}

// AttachTools associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachTools(t *Tools, session Session, ctrl *controllers.ToolsController) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.session = session
	t.ctrl = ctrl
}

func (t *Tools) deps() (Session, *controllers.ToolsController, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.session == nil || t.ctrl == nil {
		return nil, nil, ErrToolsNotWired
	}
	return t.session, t.ctrl, nil
}

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
func (t *Tools) GetAvailableTools() ([]apidto.ToolInfo, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.ToolInfo, error) {
		return ctrl.GetAvailableTools(), nil
	})
}

// GetRuntimeToolCatalog lista entradas do catálogo persistido de tools.
func (t *Tools) GetRuntimeToolCatalog(filter apidto.RuntimeToolCatalogFilter) ([]apidto.RuntimeToolCatalogEntry, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.RuntimeToolCatalogEntry, error) {
		return ctrl.GetRuntimeToolCatalog(ctx, filter)
	})
}

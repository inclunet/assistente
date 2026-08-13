package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/allowlist"
	"context"
	"sync"
)

// Allowlists é o bind Wails do domínio allowlists (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Allowlists struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.AllowlistController
}

// NewAllowlists cria o bind vazio; AttachAllowlists preenche session + controller no startup.
func NewAllowlists() *Allowlists {
	return &Allowlists{}
}

// AttachAllowlists associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachAllowlists(a *Allowlists, session Session, ctrl *controllers.AllowlistController) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.session = session
	a.ctrl = ctrl
}

func (a *Allowlists) deps() (Session, *controllers.AllowlistController, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil || a.ctrl == nil {
		return nil, nil, ErrAllowlistsNotWired
	}
	return a.session, a.ctrl, nil
}

// RespondQuestionnaire responde a uma solicitação de questionário.
func (a *Allowlists) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	session, ctrl, err := a.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RespondQuestionnaire(requestID, answers, cancelled)
	})
	return err
}

// GetAllowlists retorna a lista de allowlists disponíveis.
func (a *Allowlists) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	session, ctrl, err := a.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]allowlist.AllowlistInfo, error) {
		return ctrl.GetAllowlists()
	})
}

// GetAllowlist retorna uma allowlist pelo slug.
func (a *Allowlists) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	session, ctrl, err := a.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*allowlist.Allowlist, error) {
		return ctrl.GetAllowlist(slug)
	})
}

// CreateAllowlist cria uma nova allowlist.
func (a *Allowlists) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	session, ctrl, err := a.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.CreateAllowlist(al)
	})
}

// UpdateAllowlist atualiza uma allowlist existente.
func (a *Allowlists) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	session, ctrl, err := a.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateAllowlist(slug, al)
	})
	return err
}

// DeleteAllowlist exclui uma allowlist.
func (a *Allowlists) DeleteAllowlist(slug string) error {
	session, ctrl, err := a.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteAllowlist(slug)
	})
	return err
}

// GetAllowlistSearchPaths retorna os caminhos de busca de allowlists.
func (a *Allowlists) GetAllowlistSearchPaths() ([]string, error) {
	session, ctrl, err := a.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.GetAllowlistSearchPaths(), nil
	})
}

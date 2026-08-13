package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/updater"
	"context"
	"sync"
)

// Updater é o bind Wails do domínio updater (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Updater struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.UpdaterController
}

// NewUpdater cria o bind vazio; AttachUpdater preenche session + controller no startup.
func NewUpdater() *Updater {
	return &Updater{}
}

// AttachUpdater associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachUpdater(u *Updater, session Session, ctrl *controllers.UpdaterController) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.session = session
	u.ctrl = ctrl
}

func (u *Updater) deps() (Session, *controllers.UpdaterController, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.session == nil || u.ctrl == nil {
		return nil, nil, ErrUpdaterNotWired
	}
	return u.session, u.ctrl, nil
}

// GetAppVersion retorna a versão atual do aplicativo.
func (u *Updater) GetAppVersion() (string, error) {
	session, ctrl, err := u.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.GetAppVersion(), nil
	})
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis.
func (u *Updater) CheckForUpdates() (*updater.UpdateInfo, error) {
	session, ctrl, err := u.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*updater.UpdateInfo, error) {
		return ctrl.CheckForUpdates()
	})
}

// ApplyUpdate aplica a atualização (chamado pelo frontend).
func (u *Updater) ApplyUpdate() error {
	session, ctrl, err := u.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ApplyUpdate(ctx)
	})
	return err
}

// StartUpdate inicia o processo de atualização (navega para página e inicia).
func (u *Updater) StartUpdate() error {
	session, ctrl, err := u.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.StartUpdate(ctx)
	})
	return err
}

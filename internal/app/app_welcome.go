package app

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
	"context"
	"fmt"
)

// welcomeRuntime adapta *App para wailsapi.WelcomeRuntime sem expor métodos no Bind.
type welcomeRuntime struct {
	app *App
}

func (r welcomeRuntime) AppContext() context.Context {
	if r.app == nil {
		return context.Background()
	}
	return r.app.appContext()
}

func (r welcomeRuntime) IsLoggedIn() bool {
	if r.app == nil {
		return false
	}
	r.app.authMu.RLock()
	defer r.app.authMu.RUnlock()
	return r.app.currentUserID != ""
}

func (r welcomeRuntime) HasMasterKey() (bool, error) {
	store := credentials.NewDBStore()
	return store.HasKeyWrap(r.AppContext(), credentials.KeyWrapKindMaster)
}

func (r welcomeRuntime) UserCount() (int64, error) {
	if database.DB() == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var userCount int64
	if err := database.DB().Model(&database.User{}).Count(&userCount).Error; err != nil {
		return 0, err
	}
	return userCount, nil
}

func (r welcomeRuntime) ProviderCount(ctx context.Context) (int64, error) {
	if r.app == nil || r.app.providerSvc == nil {
		return 0, fmt.Errorf("provider service not initialized")
	}
	count, err := r.app.providerSvc.Count(ctx)
	return int64(count), err
}

// NeedsWelcomeWizard avalia o wizard dual-mode para a CLI (cmd/asst).
// Função de pacote (não método) para não entrar na superfície Bind do Wails;
// a UI usa wailsapi.Welcome.
func NeedsWelcomeWizard(a *App) bool {
	if a == nil {
		return true
	}
	return wailsapi.EvaluateNeedsWelcomeWizard(wailsSession{app: a}, a.welcomeCtrl, welcomeRuntime{app: a})
}

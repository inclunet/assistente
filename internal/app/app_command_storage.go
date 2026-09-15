package app

import (
	"assistente/internal/commandbootstrap"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/logging"
	"context"
	"gorm.io/gorm"
)

func (a *App) clearCommandStorageReadiness() {
	a.authMu.Lock()
	a.commandStorageVersion = ""
	a.commandStorageErr = commandbootstrap.ErrStorage
	a.authMu.Unlock()
}

// Executado no bootstrap/configuração serializada do cofre, fora do gate.
// Não constrói executor, publica mapa, abre diálogo nem registra atalhos.
func (a *App) initCommandStorage() {
	if err := a.prepareCommandStorage(a.internalBootstrapCtx(), database.DB(), a.credMgr); err != nil {
		logging.Warnf(context.Background(), "app.commands.storage", "Armazenamento de comandos indisponível; execução permanece desabilitada")
	}
}

func (a *App) prepareCommandStorage(ctx context.Context, db *gorm.DB, manager *credentials.Manager) error {
	if a == nil {
		return commandbootstrap.ErrStorage
	}
	a.clearCommandStorageReadiness()
	err := commandbootstrap.Migrate(ctx, db)
	version := ""
	if err == nil {
		version, err = commandbootstrap.PrepareKeys(ctx, db, manager)
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.credMgr != manager {
		err = commandbootstrap.ErrStorage
	}
	a.commandStorageErr = err
	if err == nil {
		a.commandStorageVersion = version
	}
	return err
}

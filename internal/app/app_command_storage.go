package app

import (
	"assistente/internal/commandbootstrap"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/logging"
	"context"
	"errors"
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
		logging.Warnf(context.Background(), "app.commands.storage", "Armazenamento de comandos indisponível; execução permanece desabilitada; diagnóstico=%s", commandStorageFailureCode(err))
	}
}

// Só códigos fechados chegam ao log: erros de drivers/providers podem conter
// caminhos, SQL ou dados privados. Não registrar err.Error() arbitrário.
func commandStorageFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, commandbootstrap.ErrFingerprintReference):
		return "stored_fingerprint_invalid"
	case errors.Is(err, commandbootstrap.ErrKeys):
		return "keys_unavailable_or_incompatible"
	case errors.Is(err, commandbootstrap.ErrStorage):
		return "schema_or_storage_unavailable"
	default:
		return "storage_initialization_failed"
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

package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/logging"
	"context"
	"sync"
)

// Database é o bind Wails do domínio database/manutenção (AEP-0088).
// Auth só via WithUser / WithAdmin — sem chamar o helper de auth do App no call site.
type Database struct {
	mu                 sync.RWMutex
	session            Session
	ctrl               *controllers.SettingsController
	afterReset         func()
	afterClearMessages func(ctx context.Context)
}

// NewDatabase cria o bind vazio; AttachDatabase preenche deps no startup.
func NewDatabase() *Database {
	return &Database{}
}

// AttachDatabase associa Session, controller e callbacks pós-operação após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachDatabase(
	d *Database,
	session Session,
	ctrl *controllers.SettingsController,
	afterReset func(),
	afterClearMessages func(ctx context.Context),
) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.session = session
	d.ctrl = ctrl
	d.afterReset = afterReset
	d.afterClearMessages = afterClearMessages
}

func (d *Database) deps() (Session, *controllers.SettingsController, func(), func(context.Context), error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.session == nil || d.ctrl == nil {
		return nil, nil, nil, nil, ErrDatabaseNotWired
	}
	return d.session, d.ctrl, d.afterReset, d.afterClearMessages, nil
}

// ResetDatabase apaga e recria o banco de dados inteiro (requer admin).
// Erros internos são logados localmente; o caller recebe ErrDatabaseResetFailed.
func (d *Database) ResetDatabase() error {
	session, ctrl, afterReset, _, err := d.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		if err := ctrl.ResetDatabase(); err != nil {
			logging.Errorf(context.Background(), "wailsapi.database", "[ResetDatabase] falha: %v", err)
			return struct{}{}, ErrDatabaseResetFailed
		}
		if afterReset != nil {
			afterReset()
		}
		return struct{}{}, nil
	})
	return err
}

// ClearMessages apaga mensagens e conversas do usuário autenticado.
func (d *Database) ClearMessages() error {
	session, ctrl, _, afterClear, err := d.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if err := ctrl.ClearMessages(ctx); err != nil {
			return struct{}{}, err
		}
		if afterClear != nil {
			afterClear(ctx)
		}
		return struct{}{}, nil
	})
	return err
}

// GetMaintenanceSettings retorna a política de retenção/compactação atual.
func (d *Database) GetMaintenanceSettings() (config.MaintenanceSettings, error) {
	session, ctrl, _, _, err := d.deps()
	if err != nil {
		return config.MaintenanceSettings{}, err
	}
	return WithUser(session, func(ctx context.Context) (config.MaintenanceSettings, error) {
		return ctrl.GetMaintenanceSettings()
	})
}

// SaveMaintenanceSettings persiste a política de retenção/compactação.
func (d *Database) SaveMaintenanceSettings(settings config.MaintenanceSettings) error {
	session, ctrl, _, _, err := d.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveMaintenanceSettings(settings)
	})
	return err
}

// GetDatabaseStats retorna o estado físico atual do banco (somente leitura).
func (d *Database) GetDatabaseStats() (database.DatabaseStats, error) {
	session, ctrl, _, _, err := d.deps()
	if err != nil {
		return database.DatabaseStats{}, err
	}
	return WithUser(session, func(ctx context.Context) (database.DatabaseStats, error) {
		return ctrl.GetDatabaseStats(ctx)
	})
}

// RunDatabaseMaintenance dispara a compactação física do banco sob demanda.
func (d *Database) RunDatabaseMaintenance(force bool) (database.CompactionResult, error) {
	session, ctrl, _, _, err := d.deps()
	if err != nil {
		return database.CompactionResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (database.CompactionResult, error) {
		return ctrl.RunDatabaseMaintenance(ctx, force)
	})
}

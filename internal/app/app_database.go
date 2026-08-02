package app

import (
	"assistente/internal/logging"
	"context"
	"errors"

	"assistente/internal/config"
	"assistente/internal/database"
)

// ErrDatabaseResetFailed é o erro genérico devolvido ao caller quando
// `ResetDatabase` falha. O detalhe real (paths de arquivo, syscalls,
// corrupção, permissão negada) só vai para o logger local — defesa
// contra leak de estrutura de filesystem em multi-user (review do
// AEP-0052, Bloco 7, M53).
var ErrDatabaseResetFailed = errors.New("database reset failed")

// ============================================================================
// Database Management API
// ============================================================================

// ResetDatabase apaga e recria o banco de dados inteiro.
//
// Fail-closed: exige role admin (review do AEP-0052, Bloco 7). É operação
// instance-wide e irreversível — em deployments multi-user qualquer
// usuário autenticado podia destruir os dados de todos. Agora só admin
// passa. Erros internos (paths, syscalls) são logados localmente; o
// caller recebe mensagens genéricas (não vaza estrutura de filesystem).
func (a *App) ResetDatabase() error {
	if _, err := a.requireAdminContext(); err != nil {
		return err
	}
	if err := a.settingsCtrl.ResetDatabase(); err != nil {
		logging.Errorf(context.Background(), "app.app-database", "[ResetDatabase] falha: %v", err)
		return ErrDatabaseResetFailed
	}
	// O banco foi recriado do zero: sessões e processos de agente que sobraram
	// falam de conversas que não existem mais (AEP-0084 D4).
	a.resetACPRuntime()
	return nil
}

func (a *App) ClearMessages() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if err := a.settingsCtrl.ClearMessages(ctx); err != nil {
		return err
	}
	// As conversas sumiram; as sessões que falam delas precisam sumir junto
	// (AEP-0084 D4).
	a.closeAllACPSessions(ctx)
	return nil
}

// ============================================================================
// Manutenção e retenção do banco (AEP-0074)
// ============================================================================

// GetMaintenanceSettings retorna a política de retenção/compactação atual.
func (a *App) GetMaintenanceSettings() (config.MaintenanceSettings, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return config.MaintenanceSettings{}, err
	}
	return a.settingsCtrl.GetMaintenanceSettings()
}

// SaveMaintenanceSettings persiste a política de retenção/compactação.
func (a *App) SaveMaintenanceSettings(settings config.MaintenanceSettings) error {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return err
	}
	return a.settingsCtrl.SaveMaintenanceSettings(settings)
}

// GetDatabaseStats retorna o estado físico atual do banco (somente leitura).
func (a *App) GetDatabaseStats() (database.DatabaseStats, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return database.DatabaseStats{}, err
	}
	return a.settingsCtrl.GetDatabaseStats(ctx)
}

// RunDatabaseMaintenance dispara a compactação física do banco sob demanda.
func (a *App) RunDatabaseMaintenance(force bool) (database.CompactionResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return database.CompactionResult{}, err
	}
	return a.settingsCtrl.RunDatabaseMaintenance(ctx, force)
}

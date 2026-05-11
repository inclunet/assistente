package app

import (
	"errors"
	"log"
)

// ErrDatabaseResetFailed é o erro genérico devolvido ao caller quando
// `ResetDatabase` falha. O detalhe real (paths de arquivo, syscalls,
// corrupção, permissão negada) só vai para `log.Printf` local — defesa
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
		log.Printf("[ResetDatabase] falha: %v", err)
		return ErrDatabaseResetFailed
	}
	return nil
}

func (a *App) ClearMessages() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.settingsCtrl.ClearMessages(ctx)
}

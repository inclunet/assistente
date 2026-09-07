package app

import (
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/logging"
	"context"
	"path/filepath"
)

// credentialSecretStore adapta credentials.Manager para a interface jobs.SecretStore.
type credentialSecretStore struct {
	app *App
}

func (s *credentialSecretStore) GetSecret(ctx context.Context, key string) (string, error) {
	if s.app.credMgr == nil {
		return "", nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", err
	}
	auth, err := s.app.credMgr.GetByPatternWithContext(ctx, key)
	if err != nil {
		return "", err
	}
	if auth == nil {
		return "", nil
	}
	if auth.Token != "" {
		return auth.Token, nil
	}
	if auth.Password != "" {
		return auth.Password, nil
	}
	return "", nil
}

func (a *App) initJobs() {
	baseDir := filepath.Join(configdir.GetHomeDir(), "jobs")

	// Re-normaliza slugs legados (jobs/pipelines/tags) para a forma canônica de
	// internal/slug antes de o Manager ler do banco. Idempotente; falha aqui é
	// logada mas não aborta o boot (degradação aceitável: apenas registros
	// legados com acentos/símbolos seguiriam não-encontráveis até a correção).
	if err := jobs.RenormalizeLegacySlugs(database.DB()); err != nil {
		logging.Warnf(context.Background(), "app.app-jobs", "[Jobs] AVISO: re-normalização de slugs legados falhou: %v", err)
	}

	a.jobMgr = jobs.NewManager(jobs.ManagerConfig{
		BaseDir:         baseDir,
		Repository:      jobs.NewDBRepository(database.DB()),
		ContextProvider: a.jobsAuthenticatedContext,
		ToolRegistry:    a.toolRegistry,
		ToolInvocations: a.toolInvocationSvc,
		HotkeyManager:   a.hotkeyCtrl.Manager(),
		MsgGateway:      a.msgGateway,
		SecretStore:     &credentialSecretStore{app: a},
		EmitEvent: func(event string, data any) {
			a.emitter.Emit(event, data)
		},
	})
}

// jobsAuthenticatedContext alimenta o ContextProvider do jobs.Manager (runtime
// interno). Métodos Wails usam WithUser no bind wailsapi.Jobs — não este helper.
func (a *App) jobsAuthenticatedContext() context.Context {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	return ctx
}

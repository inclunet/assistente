package app

import (
	"context"
	"log"
	"path/filepath"

	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/jobs"
)

// credentialSecretStore adapta credentials.Manager para a interface jobs.SecretStore.
type credentialSecretStore struct {
	app *App
}

func (s *credentialSecretStore) GetSecret(key string) (string, error) {
	if s.app.credMgr == nil {
		return "", nil
	}
	auth, err := s.app.credMgr.GetByPattern(key)
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

// --- Métodos Wails-bound para o frontend ---

func (a *App) GetJobs() []jobs.JobInfo {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	result, err := a.jobsCtrl.GetJobsContext(ctx)
	if err != nil {
		log.Printf("[Jobs] erro ao listar jobs: %v", err)
		return nil
	}
	return result
}

func (a *App) jobsAuthenticatedContext() context.Context {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	return ctx
}

func (a *App) GetJob(id string) (*jobs.Job, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobContext(ctx, id)
}
func (a *App) ToggleJob(id string, enabled bool) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.ToggleJobContext(ctx, id, enabled)
}
func (a *App) RunJob(id string) (*jobs.RunLog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.RunJobContext(ctx, id)
}
func (a *App) DryRunJob(id string) (*jobs.DryRunResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.DryRunJobContext(ctx, id)
}

func (a *App) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobRunsContext(ctx, id, limit)
}
func (a *App) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.ReplayRunContext(ctx, jobID, runID)
}
func (a *App) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobEventsContext(ctx, date)
}

func (a *App) GetJobEventsPage(date string, limit, offset int) ([]jobs.EventEntry, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobEventsPageContext(ctx, date, limit, offset)
}

func (a *App) GetJobPipelines() []jobs.PipelineInfo {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	result, err := a.jobsCtrl.GetJobPipelinesContext(ctx)
	if err != nil {
		log.Printf("[Jobs] erro ao listar pipelines: %v", err)
		return nil
	}
	return result
}
func (a *App) GetToolCatalog() ([]jobs.CatalogEntry, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetToolCatalog()
}
func (a *App) RegenerateJobCatalog() error {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return err
	}
	return a.jobsCtrl.RegenerateJobCatalog()
}
func (a *App) SaveJob(jobJSON string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.SaveJobContext(ctx, jobJSON)
}

func (a *App) TestTool(toolName, inputsJSON, eventJSON string) (*jobs.TestToolResult, error) {
	ctx, authErr := a.requireAuthenticatedContext()
	if authErr != nil {
		return nil, authErr
	}
	result, err := a.jobsCtrl.TestTool(toolName, inputsJSON, eventJSON)
	if err != nil || result == nil || a.mcpMgr == nil {
		return result, err
	}
	testErr := result.Error
	if recordErr := a.mcpMgr.RecordToolTest(ctx, toolName, result.Success, testErr); recordErr != nil {
		log.Printf("[Tools] erro ao registrar resultado de teste para %s: %v", toolName, recordErr)
	}
	return result, nil
}

func (a *App) InferEventSchema(eventName string) map[string]any {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil
	}
	return a.jobsCtrl.InferEventSchema(eventName)
}
func (a *App) ListKnownEvents() []string {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil
	}
	return a.jobsCtrl.ListKnownEvents()
}
func (a *App) DeleteJob(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.DeleteJobContext(ctx, id)
}

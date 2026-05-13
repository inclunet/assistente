package app

import (
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"log"
	"path/filepath"
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
		ContextProvider: a.internalBootstrapCtx,
		ToolRegistry:    a.toolRegistry,
		HotkeyManager:   a.hotkeyCtrl.Manager(),
		MsgGateway:      a.msgGateway,
		SecretStore:     &credentialSecretStore{app: a},
		EmitEvent: func(event string, data any) {
			a.emitter.Emit(event, data)
		},
	})
}

// --- Métodos Wails-bound para o frontend ---

func (a *App) GetJobs() []jobs.JobInfo                         { return a.jobsCtrl.GetJobs() }
func (a *App) GetJob(id string) (*jobs.Job, error)             { return a.jobsCtrl.GetJob(id) }
func (a *App) ToggleJob(id string, enabled bool) error         { return a.jobsCtrl.ToggleJob(id, enabled) }
func (a *App) RunJob(id string) (*jobs.RunLog, error)          { return a.jobsCtrl.RunJob(id) }
func (a *App) DryRunJob(id string) (*jobs.DryRunResult, error) { return a.jobsCtrl.DryRunJob(id) }

func (a *App) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	return a.jobsCtrl.GetJobRuns(id, limit)
}
func (a *App) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	return a.jobsCtrl.ReplayRun(jobID, runID)
}
func (a *App) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	return a.jobsCtrl.GetJobEvents(date)
}

func (a *App) GetJobPipelines() []jobs.PipelineInfo         { return a.jobsCtrl.GetJobPipelines() }
func (a *App) GetToolCatalog() ([]jobs.CatalogEntry, error) { return a.jobsCtrl.GetToolCatalog() }
func (a *App) RegenerateJobCatalog() error                  { return a.jobsCtrl.RegenerateJobCatalog() }
func (a *App) SaveJob(jobJSON string) error                 { return a.jobsCtrl.SaveJob(jobJSON) }

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
	return a.jobsCtrl.InferEventSchema(eventName)
}
func (a *App) ListKnownEvents() []string { return a.jobsCtrl.ListKnownEvents() }
func (a *App) DeleteJob(id string) error { return a.jobsCtrl.DeleteJob(id) }

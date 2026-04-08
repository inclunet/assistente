package main

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"assistente/internal/configdir"
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
		BaseDir:       baseDir,
		ToolRegistry:  a.toolRegistry,
		HotkeyManager: a.hotkeyManager,
		MsgGateway:    a.msgGateway,
		SecretStore:   &credentialSecretStore{app: a},
		EmitEvent: func(event string, data any) {
			a.emitter.Emit( event, data)
		},
	})

	if err := a.jobMgr.Start(); err != nil {
		log.Printf("[Jobs] Error starting manager: %v", err)
	}
}

// --- Metodos Wails-bound para o frontend ---

// GetJobs retorna a lista resumida de todos os jobs registrados.
func (a *App) GetJobs() []jobs.JobInfo {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.GetJobs()
}

// GetJob retorna detalhes completos de um job.
func (a *App) GetJob(id string) (*jobs.Job, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.GetJob(id)
}

// ToggleJob ativa ou desativa um job.
func (a *App) ToggleJob(id string, enabled bool) error {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.ToggleJob(id, enabled)
}

// RunJob executa um job manualmente e retorna o log de execucao.
func (a *App) RunJob(id string) (*jobs.RunLog, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.RunJob(id)
}

// DryRunJob executa um dry run de um job.
func (a *App) DryRunJob(id string) (*jobs.DryRunResult, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.DryRunJob(id)
}

// GetJobRuns retorna o historico de execucoes de um job.
func (a *App) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.GetJobRuns(id, limit)
}

// ReplayRun re-executa uma tool com os mesmos inputs resolvidos de um run anterior.
func (a *App) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	if a.jobMgr == nil {
		return nil, fmt.Errorf("job manager not initialized")
	}

	rl, err := a.jobMgr.GetJobRun(jobID, runID)
	if err != nil {
		return nil, fmt.Errorf("run not found: %w", err)
	}

	if rl.ToolName == "" {
		return nil, fmt.Errorf("run %s has no tool_name recorded (old format)", runID)
	}

	if rl.ResolvedInputs == nil {
		return nil, fmt.Errorf("run %s has no resolved_inputs recorded (old format)", runID)
	}

	return a.jobMgr.TestTool(rl.ToolName, rl.ResolvedInputs, nil)
}

// GetJobEvents retorna a timeline de eventos de uma data (formato "2006-01-02").
func (a *App) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.GetJobEvents(date)
}

// GetJobPipelines retorna os pipelines com seus jobs.
func (a *App) GetJobPipelines() []jobs.PipelineInfo {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.GetPipelines()
}

// GetToolCatalog retorna o catalogo de tools disponiveis para jobs.
func (a *App) GetToolCatalog() ([]jobs.CatalogEntry, error) {
	if a.jobMgr == nil {
		return nil, nil
	}
	return a.jobMgr.GetToolCatalog()
}

// RegenerateJobCatalog forca a regeneracao do catalogo de tools.
func (a *App) RegenerateJobCatalog() error {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.RegenerateCatalog()
}

// SaveJob cria ou atualiza um job a partir de dados JSON do frontend.
func (a *App) SaveJob(jobJSON string) error {
	if a.jobMgr == nil {
		return nil
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return fmt.Errorf("invalid job data: %w", err)
	}

	return a.jobMgr.SaveJob(&job)
}

// TestTool executa uma tool diretamente com inputs JSON, sem precisar de um job salvo.
func (a *App) TestTool(toolName string, inputsJSON string, eventJSON string) (*jobs.TestToolResult, error) {
	if a.jobMgr == nil {
		return nil, fmt.Errorf("job manager not initialized")
	}

	var inputs map[string]any
	if inputsJSON != "" && inputsJSON != "{}" {
		if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
			return nil, fmt.Errorf("invalid inputs: %w", err)
		}
	}
	if inputs == nil {
		inputs = make(map[string]any)
	}

	var eventData map[string]any
	if eventJSON != "" && eventJSON != "{}" {
		if err := json.Unmarshal([]byte(eventJSON), &eventData); err != nil {
			return nil, fmt.Errorf("invalid event data: %w", err)
		}
	}

	log.Printf("[Jobs] App.TestTool: eventJSON len=%d, eventData nil=%v", len(eventJSON), eventData == nil)

	return a.jobMgr.TestTool(toolName, inputs, eventData)
}

// InferEventSchema tenta inferir o schema/sample de um evento pelo nome.
func (a *App) InferEventSchema(eventName string) map[string]any {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.InferEventSchema(eventName)
}

// ListKnownEvents retorna todos os nomes de eventos conhecidos pelos jobs.
func (a *App) ListKnownEvents() []string {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.ListKnownEvents()
}

// DeleteJob remove um job pelo ID.
func (a *App) DeleteJob(id string) error {
	if a.jobMgr == nil {
		return nil
	}
	return a.jobMgr.DeleteJob(id)
}

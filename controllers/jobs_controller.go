package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/jobs"
)

// JobsControllerConfig agrupa as dependências do JobsController.
type JobsControllerConfig struct {
	JobMgr *jobs.Manager
}

// JobsController é o Inbound Adapter para automação de jobs (event-driven).
type JobsController struct {
	jobMgr *jobs.Manager
}

// NewJobsController cria um JobsController com as dependências injetadas.
func NewJobsController(cfg JobsControllerConfig) *JobsController {
	return &JobsController{jobMgr: cfg.JobMgr}
}

func (c *JobsController) GetJobs() []jobs.JobInfo {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.GetJobs()
}

func (c *JobsController) GetJobsContext(ctx context.Context) ([]jobs.JobInfo, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobsContext(ctx)
}

func (c *JobsController) GetJob(id string) (*jobs.Job, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJob(id)
}

func (c *JobsController) GetJobContext(ctx context.Context, id string) (*jobs.Job, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobContext(ctx, id)
}

func (c *JobsController) ToggleJob(id string, enabled bool) error {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.ToggleJob(id, enabled)
}

func (c *JobsController) ToggleJobContext(ctx context.Context, id string, enabled bool) error {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.ToggleJobContext(ctx, id, enabled)
}

func (c *JobsController) RunJob(id string) (*jobs.RunLog, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.RunJob(id)
}

func (c *JobsController) RunJobContext(ctx context.Context, id string) (*jobs.RunLog, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.RunJobContext(ctx, id)
}

func (c *JobsController) DryRunJob(id string) (*jobs.DryRunResult, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.DryRunJob(id)
}

func (c *JobsController) DryRunJobContext(ctx context.Context, id string) (*jobs.DryRunResult, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.DryRunJobContext(ctx, id)
}

func (c *JobsController) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobRuns(id, limit)
}

func (c *JobsController) GetJobRunsContext(ctx context.Context, id string, limit int) ([]jobs.RunLog, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobRunsContext(ctx, id, limit)
}

func (c *JobsController) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	if c.jobMgr == nil {
		return nil, fmt.Errorf("job manager not initialized")
	}

	rl, err := c.jobMgr.GetJobRun(jobID, runID)
	if err != nil {
		return nil, fmt.Errorf("run not found: %w", err)
	}
	if rl.ToolName == "" {
		return nil, fmt.Errorf("run %s has no tool_name recorded (old format)", runID)
	}
	if rl.ResolvedInputs == nil {
		return nil, fmt.Errorf("run %s has no resolved_inputs recorded (old format)", runID)
	}
	if !rl.Replayable || jobs.ContainsRedactedValue(rl.ResolvedInputs) {
		return nil, fmt.Errorf("run %s contains redacted inputs and cannot be replayed safely", runID)
	}
	return c.jobMgr.TestTool(rl.ToolName, rl.ResolvedInputs, nil)
}

func (c *JobsController) ReplayRunContext(ctx context.Context, jobID, runID string) (*jobs.TestToolResult, error) {
	if c.jobMgr == nil {
		return nil, fmt.Errorf("job manager not initialized")
	}
	rl, err := c.jobMgr.GetJobRunContext(ctx, jobID, runID)
	if err != nil {
		return nil, fmt.Errorf("run not found: %w", err)
	}
	if rl.ToolName == "" {
		return nil, fmt.Errorf("run %s has no tool_name recorded (old format)", runID)
	}
	if rl.ResolvedInputs == nil {
		return nil, fmt.Errorf("run %s has no resolved_inputs recorded (old format)", runID)
	}
	if !rl.Replayable || jobs.ContainsRedactedValue(rl.ResolvedInputs) {
		return nil, fmt.Errorf("run %s contains redacted inputs and cannot be replayed safely", runID)
	}
	return c.jobMgr.TestToolContext(ctx, rl.ToolName, rl.ResolvedInputs, nil)
}

func (c *JobsController) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobEvents(date)
}

func (c *JobsController) GetJobEventsContext(ctx context.Context, date string) ([]jobs.EventEntry, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobEventsContext(ctx, date)
}

func (c *JobsController) GetJobEventsPage(date string, limit, offset int) ([]jobs.EventEntry, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobEventsPage(date, limit, offset)
}

func (c *JobsController) GetJobEventsPageContext(ctx context.Context, date string, limit, offset int) ([]jobs.EventEntry, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetJobEventsPageContext(ctx, date, limit, offset)
}

func (c *JobsController) GetJobPipelines() []jobs.PipelineInfo {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.GetPipelines()
}

func (c *JobsController) GetJobPipelinesContext(ctx context.Context) ([]jobs.PipelineInfo, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetPipelinesContext(ctx)
}

func (c *JobsController) GetToolCatalog() ([]jobs.CatalogEntry, error) {
	if c.jobMgr == nil {
		return nil, nil
	}
	return c.jobMgr.GetToolCatalog()
}

func (c *JobsController) RegenerateJobCatalog() error {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.RegenerateCatalog()
}

func (c *JobsController) SaveJob(jobJSON string) error {
	if c.jobMgr == nil {
		return nil
	}
	var job jobs.Job
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return fmt.Errorf("invalid job data: %w", err)
	}
	return c.jobMgr.SaveJob(&job)
}

func (c *JobsController) SaveJobContext(ctx context.Context, jobJSON string) error {
	if c.jobMgr == nil {
		return nil
	}
	var job jobs.Job
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return fmt.Errorf("invalid job data: %w", err)
	}
	return c.jobMgr.SaveJobContext(ctx, &job)
}

func (c *JobsController) TestTool(toolName, inputsJSON, eventJSON string) (*jobs.TestToolResult, error) {
	if c.jobMgr == nil {
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
	return c.jobMgr.TestTool(toolName, inputs, eventData)
}

func (c *JobsController) TestToolContext(ctx context.Context, toolName, inputsJSON, eventJSON string) (*jobs.TestToolResult, error) {
	if c.jobMgr == nil {
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
	return c.jobMgr.TestToolContext(ctx, toolName, inputs, eventData)
}

func (c *JobsController) InferEventSchema(eventName string) map[string]any {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.InferEventSchema(eventName)
}

func (c *JobsController) ListKnownEvents() []string {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.ListKnownEvents()
}

func (c *JobsController) DeleteJob(id string) error {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.DeleteJob(id)
}

func (c *JobsController) DeleteJobContext(ctx context.Context, id string) error {
	if c.jobMgr == nil {
		return nil
	}
	return c.jobMgr.DeleteJobContext(ctx, id)
}

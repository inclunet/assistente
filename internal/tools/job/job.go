package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/jobs"
	"assistente/internal/tools"
)

type jobArgs struct {
	JobID          string             `json:"job_id,omitempty"`
	Delete         bool               `json:"delete,omitempty"`
	Run            bool               `json:"run,omitempty"`
	DryRun         bool               `json:"dry_run,omitempty"`
	ListRuns       bool               `json:"list_runs,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	Enabled        *bool              `json:"enabled,omitempty"`
	Name           string             `json:"name,omitempty"`
	Description    string             `json:"description,omitempty"`
	Pipeline       string             `json:"pipeline,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	Tool           string             `json:"tool,omitempty"`
	Inputs         map[string]any     `json:"inputs,omitempty"`
	Triggers       []jobs.Trigger     `json:"triggers,omitempty"`
	Output         *jobs.OutputConfig `json:"output,omitempty"`
	Events         *jobs.EventsConfig `json:"events,omitempty"`
	ErrorPolicy    *jobs.ErrorPolicy  `json:"error_policy,omitempty"`
	MaxRunsPerHour *int               `json:"max_runs_per_hour,omitempty"`
	DryRunConfig   *jobs.DryRunConfig `json:"dry_run_config,omitempty"`
	present        map[string]json.RawMessage
}

type Tool struct {
	mgr ManagerProvider
}

func NewJob(mgr Manager) *Tool {
	return NewJobWithProvider(func() Manager { return mgr })
}

func NewJobWithProvider(provider ManagerProvider) *Tool {
	return &Tool{mgr: provider}
}

func (t *Tool) Name() string { return "job" }

func (t *Tool) Description() string {
	return "Composite DB-backed job manager. No params lists jobs. job_id reads a job. With job_id plus fields updates. Without job_id plus name/tool/triggers creates. delete, run, dry_run and list_runs are mutually exclusive actions."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "job_id": {"type": "string", "description": "Stable job slug/id. Required for read, update, delete, toggle, run, dry_run and list_runs. Omit to list all jobs or create a new job."},
    "delete": {"type": "boolean", "description": "Delete the referenced job. Requires job_id."},
    "run": {"type": "boolean", "description": "Run the referenced job now. Requires job_id."},
    "dry_run": {"type": "boolean", "description": "Dry-run the referenced job. Requires job_id."},
    "list_runs": {"type": "boolean", "description": "List recent runs for the referenced job. Requires job_id."},
    "limit": {"type": "integer", "description": "Limit for list_runs. Defaults to 20."},
    "enabled": {"type": "boolean", "description": "Set enabled state when updating/creating, or toggle when sent with only job_id."},
    "name": {"type": "string", "description": "Job display name. Required when creating."},
    "description": {"type": "string"},
    "pipeline": {"type": "string", "description": "Pipeline slug/name."},
    "tags": {"type": "array", "items": {"type": "string"}},
    "tool": {"type": "string", "description": "Tool name from tool_catalog. Required when creating."},
    "inputs": {"type": "object", "additionalProperties": true},
    "triggers": {"type": "array", "items": {"type": "object", "additionalProperties": true}},
    "output": {"type": "object", "additionalProperties": true},
    "events": {"type": "object", "additionalProperties": true},
    "error_policy": {"type": "object", "additionalProperties": true},
    "max_runs_per_hour": {"type": "integer"},
    "dry_run_config": {"type": "object", "additionalProperties": true}
  },
  "additionalProperties": false
}`)
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	if strings.TrimSpace(string(args)) == "" {
		args = json.RawMessage(`{}`)
	}
	var params jobArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}
	_ = json.Unmarshal(args, &params.present)
	mgr := t.manager()
	if mgr == nil {
		return tools.ToolResult{Content: "job manager not configured", IsError: true}, nil
	}
	id := slugFromName(params.JobID)
	actionCount := boolCount(params.Delete, params.Run, params.DryRun, params.ListRuns)
	if actionCount > 1 {
		return tools.ToolResult{Content: "delete, run, dry_run and list_runs are mutually exclusive", IsError: true}, nil
	}
	if actionCount == 1 && id == "" {
		return tools.ToolResult{Content: "job_id is required for delete/run/dry_run/list_runs", IsError: true}, nil
	}

	switch {
	case params.Delete:
		return t.deleteJob(ctx, mgr, id)
	case params.Run:
		return t.runJob(ctx, mgr, id)
	case params.DryRun:
		return t.dryRunJob(ctx, mgr, id)
	case params.ListRuns:
		return t.listRuns(ctx, mgr, id, params.Limit)
	}

	hasWrite := params.hasWriteFields()
	if id == "" && params.Enabled != nil && !hasWrite {
		return tools.ToolResult{Content: "job_id is required to toggle enabled", IsError: true}, nil
	}
	if id == "" && !hasWrite {
		return t.listJobs(ctx, mgr)
	}
	if id != "" && !hasWrite {
		if params.Enabled != nil {
			return t.toggleJob(ctx, mgr, id, *params.Enabled)
		}
		return t.getJob(ctx, mgr, id)
	}
	if id == "" {
		return t.createJob(ctx, mgr, "", params)
	}
	return t.updateJob(ctx, mgr, id, params)
}

func (p jobArgs) hasWriteFields() bool {
	return p.has("name") ||
		p.has("description") ||
		p.has("pipeline") ||
		p.has("tags") ||
		p.has("tool") ||
		p.has("inputs") ||
		p.has("triggers") ||
		p.has("output") ||
		p.has("events") ||
		p.has("error_policy") ||
		p.has("max_runs_per_hour") ||
		p.has("dry_run_config")
}

func (p jobArgs) has(field string) bool {
	_, ok := p.present[field]
	return ok
}

func (t *Tool) manager() Manager {
	if t.mgr == nil {
		return nil
	}
	return t.mgr()
}

func (t *Tool) listJobs(ctx context.Context, mgr Manager) (tools.ToolResult, error) {
	jobsList, err := mgr.GetJobsContext(ctx)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing jobs: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(jobsList)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Found %d job(s):\n%s", len(jobsList), string(data)),
		Metadata: map[string]any{"count": len(jobsList)},
	}, nil
}

func (t *Tool) getJob(ctx context.Context, mgr Manager, id string) (tools.ToolResult, error) {
	job, err := mgr.GetJobContext(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	data, _ := json.Marshal(job)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id}}, nil
}

func (t *Tool) createJob(ctx context.Context, mgr Manager, explicitID string, params jobArgs) (tools.ToolResult, error) {
	if strings.TrimSpace(params.Name) == "" {
		return tools.ToolResult{Content: "name is required to create a job", IsError: true}, nil
	}
	if strings.TrimSpace(params.Tool) == "" {
		return tools.ToolResult{Content: "tool is required to create a job", IsError: true}, nil
	}
	if len(params.Triggers) == 0 {
		return tools.ToolResult{Content: "triggers is required to create a job", IsError: true}, nil
	}
	id := slugFromName(explicitID)
	if id == "" {
		id = slugFromName(params.Name)
	}
	if _, err := mgr.GetJobContext(ctx, id); err == nil {
		return tools.ToolResult{Content: fmt.Sprintf("job already exists: %s", id), IsError: true}, nil
	} else if !errors.Is(err, jobs.ErrJobNotFound) {
		return tools.ToolResult{Content: fmt.Sprintf("Error checking existing job: %v", err), IsError: true}, nil
	}
	job := &jobs.Job{
		ID:          id,
		Name:        params.Name,
		Description: params.Description,
		Enabled:     true,
		Pipeline:    params.Pipeline,
		Tags:        params.Tags,
		Tool:        params.Tool,
		Inputs:      params.Inputs,
		Triggers:    params.Triggers,
	}
	if params.Enabled != nil {
		job.Enabled = *params.Enabled
	}
	applyOptionalJobFields(job, params)
	if err := mgr.SaveJobContext(ctx, job); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating job: %v", err), IsError: true}, nil
	}
	return t.actionResult("created", job.ID)
}

func (t *Tool) updateJob(ctx context.Context, mgr Manager, id string, params jobArgs) (tools.ToolResult, error) {
	job, err := mgr.GetJobContext(ctx, id)
	if err != nil {
		if !errors.Is(err, jobs.ErrJobNotFound) {
			return tools.ToolResult{Content: fmt.Sprintf("Error reading job: %v", err), IsError: true}, nil
		}
		return t.createJob(ctx, mgr, id, params)
	}
	if strings.TrimSpace(params.Name) != "" {
		job.Name = params.Name
	}
	if params.has("description") {
		job.Description = params.Description
	}
	if params.has("pipeline") {
		job.Pipeline = params.Pipeline
	}
	if params.has("tags") {
		job.Tags = params.Tags
	}
	if strings.TrimSpace(params.Tool) != "" {
		job.Tool = params.Tool
	}
	if params.Inputs != nil {
		job.Inputs = params.Inputs
	}
	if params.has("triggers") {
		if len(params.Triggers) == 0 {
			job.Triggers = []jobs.Trigger{{Type: jobs.TriggerManual}}
		} else {
			job.Triggers = params.Triggers
		}
	}
	if params.Enabled != nil {
		job.Enabled = *params.Enabled
	}
	applyOptionalJobFields(job, params)
	if err := mgr.SaveJobContext(ctx, job); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating job: %v", err), IsError: true}, nil
	}
	return t.actionResult("updated", id)
}

func (t *Tool) deleteJob(ctx context.Context, mgr Manager, id string) (tools.ToolResult, error) {
	if err := mgr.DeleteJobContext(ctx, id); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error deleting job: %v", err), IsError: true}, nil
	}
	return t.actionResult("deleted", id)
}

func (t *Tool) toggleJob(ctx context.Context, mgr Manager, id string, enabled bool) (tools.ToolResult, error) {
	if err := mgr.ToggleJobContext(ctx, id, enabled); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error toggling job: %v", err), IsError: true}, nil
	}
	return t.actionResult("toggled", id)
}

func (t *Tool) runJob(ctx context.Context, mgr Manager, id string) (tools.ToolResult, error) {
	run, err := mgr.RunJobContext(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error running job: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(run)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id, "action": "run"}}, nil
}

func (t *Tool) dryRunJob(ctx context.Context, mgr Manager, id string) (tools.ToolResult, error) {
	result, err := mgr.DryRunJobContext(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error dry-running job: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(result)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id, "action": "dry_run"}}, nil
}

func (t *Tool) listRuns(ctx context.Context, mgr Manager, id string, limit int) (tools.ToolResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	runs, err := mgr.GetJobRunsContext(ctx, id, limit)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing runs: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(runs)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id, "count": len(runs)}}, nil
}

func (t *Tool) actionResult(action, id string) (tools.ToolResult, error) {
	payload := map[string]any{"action": action, "job_id": id}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload}, nil
}

func applyOptionalJobFields(job *jobs.Job, params jobArgs) {
	if params.Output != nil {
		job.Output = *params.Output
	}
	if params.Events != nil {
		job.Events = *params.Events
	}
	if params.ErrorPolicy != nil {
		job.ErrorPolicy = *params.ErrorPolicy
	}
	if params.MaxRunsPerHour != nil {
		job.MaxRunsPerHour = *params.MaxRunsPerHour
	}
	if params.DryRunConfig != nil {
		job.DryRun = *params.DryRunConfig
	}
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func slugFromName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

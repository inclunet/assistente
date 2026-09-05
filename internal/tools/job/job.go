package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"assistente/internal/jobs"
	"assistente/internal/tools"
)

var allowedRunStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"retrying":  {},
	"skipped":   {},
}

type jobArgs struct {
	JobID          string             `json:"job_id,omitempty"`
	RunID          string             `json:"run_id,omitempty"`
	Delete         bool               `json:"delete,omitempty"`
	Run            bool               `json:"run,omitempty"`
	DryRun         bool               `json:"dry_run,omitempty"`
	ListRuns       bool               `json:"list_runs,omitempty"`
	ListEvents     bool               `json:"list_events,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	Offset         int                `json:"offset,omitempty"`
	Status         []string           `json:"status,omitempty"`
	StartedAfter   string             `json:"started_after,omitempty"`
	StartedBefore  string             `json:"started_before,omitempty"`
	IncludeDryRun  bool               `json:"include_dry_run,omitempty"`
	Date           string             `json:"date,omitempty"`
	StartAt        string             `json:"start_at,omitempty"`
	EndAt          string             `json:"end_at,omitempty"`
	EventType      string             `json:"event_type,omitempty"`
	EventName      string             `json:"event_name,omitempty"`
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

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *Tool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "jobs", Class: "automation_management", Package: "jobs", Risk: "write"}
}

func (t *Tool) Description() string {
	return `Manage persistent event-driven jobs. One job invokes one configured tool per trigger; jobs form workflows by emitting and listening for events.
Use when: creating or changing scheduled/reactive automation, enabling it, running it now, or inspecting jobs, runs, failures, and events. No parameters lists jobs; job_id reads one.
Do not use: call the underlying tool directly for a one-off action. Use job_pipeline only to group and collectively enable/disable related jobs; a pipeline does not execute tools or define step order.
Persistence, risk, and cost: definitions, runs, and events persist in the database. Enabled cron/interval/event jobs may run repeatedly, invoke write/network tools, emit chained events, and consume external quotas. run executes now. dry_run suppresses emitted events but, without mock_output, still executes the configured tool and may have side effects. Prefer filtered list_runs/list_events before fetching one full run timeline.
Actions delete, run, dry_run, list_runs, list_events, and run_id are mutually exclusive. Creating requires name, tool, and triggers; job_id plus write fields updates or creates that stable ID.
Examples: inspect {"job_id":"daily-report"}; run history {"job_id":"daily-report","list_runs":true,"status":["failed"]}; create {"name":"Daily report","tool":"web_fetch","triggers":[{"type":"cron","expression":"0 9 * * 1-5"}]}.`
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "job_id": {"type": "string", "description": "Stable job slug/id. Required for read, update, delete, toggle, run, dry_run, list_runs and run_id. Optional for list_events (omit for global listing). When combined with required create fields and the job does not exist, creates a job with this id. Omit to list all jobs or create with a generated id."},
    "run_id": {"type": "string", "description": "When set with job_id, returns the full RunLog including run_events (operational timeline ordered by sequence) and domain_events (correlated by job_run_id). Mutually exclusive with delete/run/dry_run/list_runs/list_events."},
    "delete": {"type": "boolean", "description": "Delete the referenced job. Requires job_id."},
    "run": {"type": "boolean", "description": "Run the referenced job now. Requires job_id."},
    "dry_run": {"type": "boolean", "description": "Dry-run the referenced job. Requires job_id."},
    "list_runs": {"type": "boolean", "description": "List recent runs for the referenced job. Requires job_id. Accepts status, started_after, started_before, include_dry_run and limit filters."},
    "list_events": {"type": "boolean", "description": "List job events (domain + operational timeline) filterable by date or start_at/end_at, event_type, event_name and optional job_id. Defaults to today when no time filter is set."},
    "limit": {"type": "integer", "description": "Limit for list_runs (default 20, max 100) or list_events (default 50, max 200)."},
    "offset": {"type": "integer", "description": "Offset for list_events pagination. Defaults to 0."},
    "status": {"type": "array", "items": {"type": "string", "enum": ["completed", "failed", "retrying", "skipped"]}, "description": "Filter list_runs by status. Only valid with list_runs: true."},
    "started_after": {"type": "string", "description": "RFC3339 lower bound (inclusive) for list_runs. Only valid with list_runs: true."},
    "started_before": {"type": "string", "description": "RFC3339 upper bound (exclusive) for list_runs. Only valid with list_runs: true."},
    "include_dry_run": {"type": "boolean", "description": "When false (default), list_runs excludes dry-runs. Only valid with list_runs: true."},
    "date": {"type": "string", "description": "YYYY-MM-DD shortcut for list_events covering 24h in local time. Ignored if start_at/end_at provided. Only valid with list_events: true."},
    "start_at": {"type": "string", "description": "RFC3339 lower bound (inclusive) for list_events. Only valid with list_events: true."},
    "end_at": {"type": "string", "description": "RFC3339 upper bound (exclusive) for list_events. Only valid with list_events: true."},
    "event_type": {"type": "string", "description": "Filter list_events by event type. Only valid with list_events: true."},
    "event_name": {"type": "string", "description": "Filter list_events by event name (applies only to domain events). Only valid with list_events: true."},
    "enabled": {"type": "boolean", "description": "Set enabled state when updating/creating, or toggle when sent with only job_id."},
    "name": {"type": "string", "description": "Job display name. Required when creating."},
    "description": {"type": "string"},
    "pipeline": {"type": "string", "description": "Pipeline slug/name."},
    "tags": {"type": "array", "items": {"type": "string"}},
    "tool": {"type": "string", "description": "Tool name from tool_catalog. Required when creating."},
    "inputs": {"type": "object", "additionalProperties": true, "description": "Object map of input keys to values resolved at runtime (templates allowed). MUST be passed as a JSON object, NOT as a stringified JSON. Correct: {\"task_list_slug\": \"ops\", \"limit\": 50}. Wrong: \"{\\\"task_list_slug\\\": \\\"ops\\\"}\"."},
    "triggers": {
      "type": "array",
      "description": "List of triggers that fire this job (at least one required on create). MUST be a JSON array, not a stringified JSON.",
      "items": {
        "type": "object",
        "properties": {
          "type": {"type": "string", "enum": ["cron", "interval", "event", "hotkey", "manual", "webhook"], "description": "Trigger kind."},
          "expression": {"type": "string", "description": "Cron expression when type=cron, e.g. '0 9 * * 1-5'."},
          "every": {"type": "string", "description": "Interval duration when type=interval, e.g. '2h', '30m'."},
          "listen": {"type": "string", "description": "Event name to listen to when type=event."},
          "keys": {"type": "string", "description": "Hotkey combo when type=hotkey, e.g. 'Ctrl+Shift+J'."},
          "path": {"type": "string", "description": "Webhook path when type=webhook (v2)."},
          "when": {"type": "string", "description": "Optional Go template condition evaluated before running; truthy = run."}
        },
        "required": ["type"],
        "additionalProperties": false
      }
    },
    "output": {
      "type": "object",
      "description": "Output mapping/schema for the tool result. MUST be a JSON object, not a stringified JSON.",
      "properties": {
        "schema": {"type": "object", "additionalProperties": true, "description": "Optional free-form JSON Schema describing the expected tool output."},
        "map": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Map of output keys to Go template expressions, e.g. {\"result\": \"{{ .output.data }}\"}."}
      },
      "additionalProperties": false
    },
    "events": {
      "type": "object",
      "description": "Events emitted by the job. MUST be a JSON object, not a stringified JSON.",
      "properties": {
        "on_success": {"type": "string", "description": "Event name emitted on success."},
        "on_failure": {"type": "string", "description": "Event name emitted on failure."},
        "emit_when": {"type": "string", "description": "Optional Go template; the success event is suppressed when falsy. Evaluated per item when fan-out is used."},
        "for_each": {"type": "string", "description": "Output array path for fan-out; emits one event per item."},
        "payload_template": {"type": "string", "description": "Optional Go template that reshapes the emitted payload."},
        "payload_filter": {
          "type": "object",
          "description": "Whitelist/blacklist of payload fields.",
          "properties": {
            "include": {"type": "array", "items": {"type": "string"}, "description": "Only these payload keys are emitted."},
            "exclude": {"type": "array", "items": {"type": "string"}, "description": "These payload keys are removed."}
          },
          "additionalProperties": false
        }
      },
      "additionalProperties": false
    },
    "error_policy": {
      "type": "object",
      "description": "Error handling policy. MUST be a JSON object, not a stringified JSON.",
      "properties": {
        "strategy": {"type": "string", "enum": ["retry", "stop", "skip"], "description": "How to handle failures."},
        "max_retries": {"type": "integer", "description": "Maximum retry attempts when strategy=retry."},
        "retry_delay": {"type": "string", "description": "Delay between retries as a duration string, e.g. '30s'."},
        "backoff": {"type": "string", "enum": ["linear", "exponential", "fixed"], "description": "Backoff strategy for retries."},
        "on_exhausted": {"type": "string", "enum": ["notify", "ignore"], "description": "Action when retries are exhausted."},
        "notify_channels": {"type": "array", "items": {"type": "string"}, "description": "Channels to notify on exhaustion/failure."}
      },
      "additionalProperties": false
    },
    "max_runs_per_hour": {"type": "integer", "description": "Rate limit: maximum runs per hour."},
    "dry_run_config": {
      "type": "object",
      "description": "Configuration-level dry-run persisted on the job. MUST be a JSON object, not a stringified JSON.",
      "properties": {
        "enabled": {"type": "boolean", "description": "When true, normal executions skip the underlying tool and return mock_output (still taking the success path)."},
        "mock_output": {"type": "object", "additionalProperties": true, "description": "Free-form object returned as the mocked tool output when dry-run is enabled."}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`)
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	if strings.TrimSpace(string(args)) == "" {
		args = json.RawMessage(`{}`)
	}
	coerced, cerr := coerceArgs(args, jobTypedFields)
	if cerr != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + cerr.Error(), IsError: true}, nil
	}
	args = coerced
	var params jobArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}
	_ = json.Unmarshal(args, &params.present)
	mgr := t.manager()
	if mgr == nil {
		return tools.ToolResult{Content: "job manager not configured", IsError: true}, nil
	}
	id := normalizeRepoSlug(params.JobID)
	runID := strings.TrimSpace(params.RunID)
	runIDProvided := params.has("run_id")
	if runIDProvided && runID == "" {
		return tools.ToolResult{Content: "run_id cannot be empty", IsError: true}, nil
	}
	actionCount := boolCount(params.Delete, params.Run, params.DryRun, params.ListRuns, params.ListEvents, runIDProvided)
	if actionCount > 1 {
		return tools.ToolResult{Content: "delete, run, dry_run, list_runs, list_events and run_id are mutually exclusive", IsError: true}, nil
	}
	hasWrite := params.hasWriteFields()
	if actionCount == 1 && (hasWrite || params.Enabled != nil) {
		return tools.ToolResult{Content: "delete, run, dry_run, list_runs, list_events and run_id cannot be combined with write fields", IsError: true}, nil
	}
	if (params.Delete || params.Run || params.DryRun || params.ListRuns || runIDProvided) && id == "" {
		return tools.ToolResult{Content: "job_id is required for delete/run/dry_run/list_runs/run_id", IsError: true}, nil
	}
	if err := params.validateFilterScope(); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	switch {
	case params.Delete:
		return t.deleteJob(ctx, mgr, id)
	case params.Run:
		return t.runJob(ctx, mgr, id)
	case params.DryRun:
		return t.dryRunJob(ctx, mgr, id)
	case runIDProvided:
		return t.getRunDetail(ctx, mgr, id, runID)
	case params.ListEvents:
		return t.listEvents(ctx, mgr, id, params)
	case params.ListRuns:
		return t.listRuns(ctx, mgr, id, params)
	}

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

func (p jobArgs) validateFilterScope() error {
	runFilterFields := []string{"status", "started_after", "started_before", "include_dry_run"}
	if !p.ListRuns {
		for _, f := range runFilterFields {
			if p.has(f) {
				return fmt.Errorf("%s is only valid with list_runs: true", f)
			}
		}
	}
	eventFilterFields := []string{"date", "start_at", "end_at", "event_type", "event_name", "offset"}
	if !p.ListEvents {
		for _, f := range eventFilterFields {
			if p.has(f) {
				return fmt.Errorf("%s is only valid with list_events: true", f)
			}
		}
	}
	if !p.ListRuns && !p.ListEvents && p.has("limit") {
		return fmt.Errorf("limit is only valid with list_runs or list_events")
	}
	return nil
}

func (t *Tool) manager() Manager {
	if t.mgr == nil {
		return nil
	}
	mgr := t.mgr()
	if managerIsNil(mgr) {
		return nil
	}
	return mgr
}

func (t *Tool) listJobs(ctx context.Context, mgr Manager) (tools.ToolResult, error) {
	jobsList, err := mgr.GetJobsContext(ctx)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing jobs: %v", err), IsError: true}, nil
	}
	summaries := make([]jobSummary, 0, len(jobsList))
	for _, job := range jobsList {
		summaries = append(summaries, jobSummary{
			ID:               job.ID,
			Name:             job.Name,
			Description:      job.Description,
			Enabled:          job.Enabled,
			EffectiveEnabled: job.EffectiveEnabled,
			PipelineEnabled:  job.PipelineEnabled,
			Pipeline:         job.Pipeline,
			Tags:             job.Tags,
			Tool:             job.Tool,
			Status:           job.Status,
			Triggers:         job.Triggers,
		})
	}
	data, _ := json.Marshal(summaries)
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
	job.Inputs = jobs.RedactResolvedInputs(job.Inputs, job.Inputs)
	// Evita vazar detalhes de execuções anteriores (outputs/inputs resolvidos) em um "read" de configuração.
	job.LastRun = nil
	data, _ := json.Marshal(job)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id}}, nil
}

type jobSummary struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Enabled          bool           `json:"enabled"`
	EffectiveEnabled bool           `json:"effective_enabled"`
	PipelineEnabled  bool           `json:"pipeline_enabled"`
	Pipeline         string         `json:"pipeline,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	Tool             string         `json:"tool"`
	Status           jobs.JobStatus `json:"status"`
	Triggers         []jobs.Trigger `json:"triggers,omitempty"`
}

func (t *Tool) createJob(ctx context.Context, mgr Manager, explicitID string, params jobArgs) (tools.ToolResult, error) {
	if strings.TrimSpace(params.Name) == "" {
		return tools.ToolResult{Content: "name is required to create a job", IsError: true}, nil
	}
	if strings.TrimSpace(params.Tool) == "" {
		return tools.ToolResult{Content: "tool is required to create a job", IsError: true}, nil
	}
	if len(params.Triggers) == 0 {
		return tools.ToolResult{Content: "triggers are required to create a job", IsError: true}, nil
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
	if err := mgr.CreateJobContext(ctx, job); err != nil {
		if errors.Is(err, jobs.ErrJobAlreadyExists) {
			return tools.ToolResult{Content: fmt.Sprintf("job already exists: %s", job.ID), IsError: true}, nil
		}
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

func (t *Tool) listRuns(ctx context.Context, mgr Manager, id string, params jobArgs) (tools.ToolResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	filter := jobs.RunFilter{
		IncludeDryRun: params.IncludeDryRun,
		Limit:         limit,
	}
	for _, s := range params.Status {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := allowedRunStatuses[s]; !ok {
			return tools.ToolResult{Content: fmt.Sprintf("invalid status %q (allowed: completed, failed, retrying, skipped)", s), IsError: true}, nil
		}
		filter.Status = append(filter.Status, s)
	}
	if params.has("started_after") {
		value := strings.TrimSpace(params.StartedAfter)
		if value == "" {
			return tools.ToolResult{Content: "started_after cannot be empty", IsError: true}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("invalid started_after (expect RFC3339): %v", err), IsError: true}, nil
		}
		filter.StartedAfter = parsed
	}
	if params.has("started_before") {
		value := strings.TrimSpace(params.StartedBefore)
		if value == "" {
			return tools.ToolResult{Content: "started_before cannot be empty", IsError: true}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("invalid started_before (expect RFC3339): %v", err), IsError: true}, nil
		}
		filter.StartedBefore = parsed
	}
	runs, err := mgr.ListJobRunsContext(ctx, id, filter)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing runs: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(runs)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": id, "count": len(runs)}}, nil
}

func (t *Tool) getRunDetail(ctx context.Context, mgr Manager, jobID, runID string) (tools.ToolResult, error) {
	detail, err := mgr.GetJobRunDetailContext(ctx, jobID, runID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error reading run detail: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(detail)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"job_id": jobID, "run_id": runID, "action": "get_run"}}, nil
}

func (t *Tool) listEvents(ctx context.Context, mgr Manager, id string, params jobArgs) (tools.ToolResult, error) {
	filter := jobs.EventFilter{
		JobID:  id,
		Type:   strings.TrimSpace(params.EventType),
		Event:  strings.TrimSpace(params.EventName),
		Limit:  params.Limit,
		Offset: params.Offset,
	}
	switch {
	case params.has("start_at") || params.has("end_at"):
		if params.has("start_at") {
			value := strings.TrimSpace(params.StartAt)
			if value == "" {
				return tools.ToolResult{Content: "start_at cannot be empty", IsError: true}, nil
			}
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return tools.ToolResult{Content: fmt.Sprintf("invalid start_at (expect RFC3339): %v", err), IsError: true}, nil
			}
			filter.StartAt = parsed
		}
		if params.has("end_at") {
			value := strings.TrimSpace(params.EndAt)
			if value == "" {
				return tools.ToolResult{Content: "end_at cannot be empty", IsError: true}, nil
			}
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return tools.ToolResult{Content: fmt.Sprintf("invalid end_at (expect RFC3339): %v", err), IsError: true}, nil
			}
			filter.EndAt = parsed
		}
	default:
		date := strings.TrimSpace(params.Date)
		if date == "" {
			if params.has("date") {
				return tools.ToolResult{Content: "date cannot be empty", IsError: true}, nil
			}
			date = time.Now().In(time.Local).Format("2006-01-02")
		}
		start, err := time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("invalid date %q (expect YYYY-MM-DD): %v", date, err), IsError: true}, nil
		}
		filter.StartAt = start
		filter.EndAt = start.AddDate(0, 0, 1)
	}
	events, err := mgr.ListJobEventsContext(ctx, filter)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing events: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(events)
	meta := map[string]any{"count": len(events), "action": "list_events"}
	if id != "" {
		meta["job_id"] = id
	}
	return tools.ToolResult{Content: string(data), Metadata: meta}, nil
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

// normalizeRepoSlug replica a normalização usada pelo repository de jobs:
// lower + trim + replace espaços por '-'.
//
// Importante: isso NÃO é um "slugify" agressivo (não remove '.', '_' etc.).
func normalizeRepoSlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	if slug == "" {
		return ""
	}
	return strings.ReplaceAll(slug, " ", "-")
}

func slugFromName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	if slug == "" {
		return ""
	}

	// Evita gerar IDs inválidos (Validate() proíbe espaços e separadores de path).
	slug = strings.ReplaceAll(slug, "\\", "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, " ", "-")

	// Normaliza caracteres incomuns para '-', mantendo [a-z0-9_-].
	slug = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(slug, "-")
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

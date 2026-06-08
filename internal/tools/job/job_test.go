package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/jobs"
)

type fakeManager struct {
	jobs          map[string]*jobs.Job
	pipelines     map[string]jobs.Pipeline
	getErr        error
	lastRunLimit  int
	runsByJob     map[string][]jobs.RunLog
	runDetails    map[string]*jobs.RunDetail
	events        []jobs.EventEntry
	lastRunFilter jobs.RunFilter
	lastEventFilt jobs.EventFilter
}

func newFakeManager() *fakeManager {
	return &fakeManager{
		jobs:       make(map[string]*jobs.Job),
		pipelines:  make(map[string]jobs.Pipeline),
		runsByJob:  make(map[string][]jobs.RunLog),
		runDetails: make(map[string]*jobs.RunDetail),
	}
}

func (m *fakeManager) GetJobs() []jobs.JobInfo {
	out := make([]jobs.JobInfo, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, jobs.JobInfo{ID: job.ID, Name: job.Name, Enabled: job.Enabled, Pipeline: job.Pipeline, Tool: job.Tool, LastRun: job.LastRun})
	}
	return out
}

func (m *fakeManager) GetJobsContext(ctx context.Context) ([]jobs.JobInfo, error) {
	return m.GetJobs(), nil
}

func (m *fakeManager) GetJob(id string) (*jobs.Job, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	job, ok := m.jobs[id]
	if !ok {
		return nil, errNotFound(id)
	}
	copy := *job
	return &copy, nil
}

func (m *fakeManager) GetJobContext(ctx context.Context, id string) (*jobs.Job, error) {
	return m.GetJob(id)
}

func (m *fakeManager) CreateJobContext(ctx context.Context, job *jobs.Job) error {
	if _, ok := m.jobs[job.ID]; ok {
		return fmt.Errorf("%w: %s", jobs.ErrJobAlreadyExists, job.ID)
	}
	return m.SaveJobContext(ctx, job)
}

func (m *fakeManager) SaveJob(job *jobs.Job) error {
	copy := *job
	m.jobs[job.ID] = &copy
	return nil
}

func (m *fakeManager) SaveJobContext(ctx context.Context, job *jobs.Job) error {
	return m.SaveJob(job)
}

func (m *fakeManager) DeleteJob(id string) error {
	delete(m.jobs, id)
	return nil
}

func (m *fakeManager) DeleteJobContext(ctx context.Context, id string) error {
	return m.DeleteJob(id)
}

func (m *fakeManager) ToggleJob(id string, enabled bool) error {
	job, ok := m.jobs[id]
	if !ok {
		return errNotFound(id)
	}
	job.Enabled = enabled
	return nil
}

func (m *fakeManager) ToggleJobContext(ctx context.Context, id string, enabled bool) error {
	return m.ToggleJob(id, enabled)
}

func (m *fakeManager) RunJob(id string) (*jobs.RunLog, error) {
	return &jobs.RunLog{RunID: "run-1", JobID: id, Status: "completed"}, nil
}

func (m *fakeManager) RunJobContext(ctx context.Context, id string) (*jobs.RunLog, error) {
	return m.RunJob(id)
}

func (m *fakeManager) DryRunJob(id string) (*jobs.DryRunResult, error) {
	return &jobs.DryRunResult{Success: true, RunLog: &jobs.RunLog{RunID: "dry-1", JobID: id}}, nil
}

func (m *fakeManager) DryRunJobContext(ctx context.Context, id string) (*jobs.DryRunResult, error) {
	return m.DryRunJob(id)
}

func (m *fakeManager) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	m.lastRunLimit = limit
	return []jobs.RunLog{{RunID: "run-1", JobID: id, Status: "completed"}}, nil
}

func (m *fakeManager) GetJobRunsContext(ctx context.Context, id string, limit int) ([]jobs.RunLog, error) {
	return m.GetJobRuns(id, limit)
}

func (m *fakeManager) ListJobRunsContext(ctx context.Context, id string, filter jobs.RunFilter) ([]jobs.RunLog, error) {
	m.lastRunFilter = filter
	m.lastRunLimit = filter.Limit
	all := m.runsByJob[id]
	out := make([]jobs.RunLog, 0, len(all))
	for _, run := range all {
		if !filter.IncludeDryRun && run.IsDryRun {
			continue
		}
		if len(filter.Status) > 0 {
			match := false
			for _, s := range filter.Status {
				if run.Status == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if !filter.StartedAfter.IsZero() && run.StartedAt.Before(filter.StartedAfter) {
			continue
		}
		if !filter.StartedBefore.IsZero() && !run.StartedAt.Before(filter.StartedBefore) {
			continue
		}
		out = append(out, run)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *fakeManager) GetJobRunDetailContext(ctx context.Context, jobID, runID string) (*jobs.RunDetail, error) {
	if d, ok := m.runDetails[jobID+":"+runID]; ok {
		return d, nil
	}
	return nil, errNotFound(jobID + ":" + runID)
}

func (m *fakeManager) ListJobEventsContext(ctx context.Context, filter jobs.EventFilter) ([]jobs.EventEntry, error) {
	m.lastEventFilt = filter
	out := make([]jobs.EventEntry, 0, len(m.events))
	for _, ev := range m.events {
		if filter.JobID != "" && ev.JobID != filter.JobID {
			continue
		}
		if filter.RunID != "" && ev.RunID != filter.RunID {
			continue
		}
		if filter.Type != "" && ev.Type != filter.Type {
			continue
		}
		if filter.Event != "" && ev.Event != filter.Event {
			continue
		}
		if !filter.StartAt.IsZero() && ev.Timestamp.Before(filter.StartAt) {
			continue
		}
		if !filter.EndAt.IsZero() && !ev.Timestamp.Before(filter.EndAt) {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

func (m *fakeManager) GetPipelines() []jobs.PipelineInfo {
	out := make([]jobs.PipelineInfo, 0, len(m.pipelines))
	for _, pipeline := range m.pipelines {
		out = append(out, jobs.PipelineInfo{Name: pipeline.Slug})
	}
	return out
}

func (m *fakeManager) ListPipelines() ([]jobs.Pipeline, error) {
	out := make([]jobs.Pipeline, 0, len(m.pipelines))
	for _, pipeline := range m.pipelines {
		out = append(out, pipeline)
	}
	return out, nil
}

func (m *fakeManager) ListPipelinesContext(ctx context.Context) ([]jobs.Pipeline, error) {
	return m.ListPipelines()
}

func (m *fakeManager) CreatePipelineContext(ctx context.Context, pipeline *jobs.Pipeline) error {
	if _, ok := m.pipelines[pipeline.Slug]; ok {
		return fmt.Errorf("%w: %s", jobs.ErrPipelineAlreadyExists, pipeline.Slug)
	}
	return m.SavePipelineContext(ctx, pipeline)
}

func (m *fakeManager) SavePipeline(pipeline *jobs.Pipeline) error {
	m.pipelines[pipeline.Slug] = *pipeline
	return nil
}

func (m *fakeManager) SavePipelineContext(ctx context.Context, pipeline *jobs.Pipeline) error {
	return m.SavePipeline(pipeline)
}

func (m *fakeManager) DeletePipeline(slug string) error {
	delete(m.pipelines, slug)
	return nil
}

func (m *fakeManager) DeletePipelineContext(ctx context.Context, slug string) error {
	return m.DeletePipeline(slug)
}

func errNotFound(id string) error {
	return &notFoundError{id: id}
}

type notFoundError struct {
	id string
}

func (e *notFoundError) Error() string {
	return "not found: " + e.id
}

func (e *notFoundError) Is(target error) bool {
	return errors.Is(target, jobs.ErrJobNotFound)
}

func TestJobToolCreatesUpdatesAndLists(t *testing.T) {
	mgr := newFakeManager()
	tool := NewJob(mgr)

	createArgs := json.RawMessage(`{
		"name":"Daily Report",
		"tool":"web_fetch",
		"pipeline":"reports",
		"triggers":[{"type":"manual"}],
		"inputs":{"url":"https://example.com"}
	}`)
	result, err := tool.Execute(context.Background(), createArgs)
	if err != nil {
		t.Fatalf("Execute create error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute create returned error result: %s", result.Content)
	}
	if _, ok := mgr.jobs["daily-report"]; !ok {
		t.Fatal("expected daily-report job to be saved")
	}

	updateArgs := json.RawMessage(`{"job_id":"daily-report","enabled":false,"tags":["ops"]}`)
	result, err = tool.Execute(context.Background(), updateArgs)
	if err != nil {
		t.Fatalf("Execute update error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute update returned error result: %s", result.Content)
	}
	if mgr.jobs["daily-report"].Enabled {
		t.Fatal("expected job to be disabled")
	}
	if got := mgr.jobs["daily-report"].Tags; len(got) != 1 || got[0] != "ops" {
		t.Fatalf("unexpected tags: %#v", got)
	}
	mgr.jobs["daily-report"].LastRun = &jobs.RunLog{
		RunID:          "run-secret",
		JobID:          "daily-report",
		ResolvedInputs: map[string]any{"token": "secret"},
	}

	result, err = tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute list error = %v", err)
	}
	if !strings.Contains(result.Content, "Daily Report") {
		t.Fatalf("list output does not include job: %s", result.Content)
	}
	if strings.Contains(result.Content, "last_run") {
		t.Fatalf("list output should not include last run payload: %s", result.Content)
	}
	if strings.Contains(result.Content, "run-secret") || strings.Contains(result.Content, "secret") {
		t.Fatalf("list output leaked last run payload: %s", result.Content)
	}
}

func TestJobToolGetRedactsSensitiveInputs(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["secret-job"] = &jobs.Job{
		ID:      "secret-job",
		Name:    "Secret Job",
		Enabled: true,
		Tool:    "web_fetch",
		Inputs: map[string]any{
			"apiKey": "secret-value",
			"query":  "public",
			"nested": map[string]any{
				"token": "nested-secret",
			},
		},
		Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}},
	}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"secret-job"}`))
	if err != nil {
		t.Fatalf("Execute get error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute get returned error result: %s", result.Content)
	}
	if strings.Contains(result.Content, "secret-value") || strings.Contains(result.Content, "nested-secret") {
		t.Fatalf("get output leaked sensitive input: %s", result.Content)
	}
	if !strings.Contains(result.Content, `"apiKey":"[redacted]"`) || !strings.Contains(result.Content, `"query":"public"`) {
		t.Fatalf("get output did not redact expected fields: %s", result.Content)
	}
}

func TestJobToolHandlesTypedNilManagerProvider(t *testing.T) {
	var mgr *fakeManager
	tool := NewJobWithProvider(func() Manager { return mgr })

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute typed nil provider error = %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "job manager not configured") {
		t.Fatalf("expected configured error for typed nil manager, got %#v", result)
	}
}

func TestJobToolRejectsEnabledWithoutJobID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"enabled":false}`))
	if err != nil {
		t.Fatalf("Execute enabled-only error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected enabled-only without job_id to be an error, got %s", result.Content)
	}
	if !strings.Contains(result.Content, "job_id is required") {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

func TestJobToolRejectsActionWithWriteFields(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","run":true,"enabled":false}`))
	if err != nil {
		t.Fatalf("Execute action+write error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected action+write to be rejected, got %s", result.Content)
	}
	if !mgr.jobs["daily-report"].Enabled {
		t.Fatal("action+write should not mutate the job")
	}
}

func TestJobToolCreatesWithExplicitJobID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"job_id":"Explicit Job",
		"name":"Explicit Name",
		"tool":"web_fetch",
		"triggers":[{"type":"manual"}]
	}`))
	if err != nil {
		t.Fatalf("Execute create explicit id error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute create explicit id returned error result: %s", result.Content)
	}
	if _, ok := mgr.jobs["explicit-job"]; !ok {
		t.Fatalf("expected explicit-job to be saved, got %#v", mgr.jobs)
	}
}

func TestJobToolDoesNotCreateOnReadError(t *testing.T) {
	mgr := newFakeManager()
	mgr.getErr = errors.New("database unavailable")
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"job_id":"new-job",
		"name":"New Job",
		"tool":"web_fetch",
		"triggers":[{"type":"manual"}]
	}`))
	if err != nil {
		t.Fatalf("Execute update read error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected read error result, got %s", result.Content)
	}
	if _, ok := mgr.jobs["new-job"]; ok {
		t.Fatalf("job was created after non-not-found read error")
	}
}

func TestJobToolCapsListRunsLimit(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","list_runs":true,"limit":100000}`))
	if err != nil {
		t.Fatalf("Execute list_runs error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute list_runs returned error result: %s", result.Content)
	}
	if mgr.lastRunLimit != 100 {
		t.Fatalf("limit not capped: got %d, want 100", mgr.lastRunLimit)
	}
}

func TestJobToolGetRunReturnsDetailWithEvents(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	mgr.runDetails["daily-report:run-x"] = &jobs.RunDetail{
		RunLog: jobs.RunLog{RunID: "run-x", JobID: "daily-report", Status: "failed"},
		RunEvents: []jobs.RunEvent{
			{RunID: "run-x", Sequence: 1, Type: "triggered", Message: "started"},
			{RunID: "run-x", Sequence: 2, Type: "failed", Message: "boom"},
		},
		DomainEvents: []jobs.EventEntry{
			{JobID: "daily-report", RunID: "run-x", Type: "emitted", Event: "fail"},
		},
	}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","run_id":"run-x"}`))
	if err != nil {
		t.Fatalf("Execute get_run error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute get_run returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, `"run_events"`) || !strings.Contains(result.Content, `"domain_events"`) {
		t.Fatalf("get_run output missing event fields: %s", result.Content)
	}
	if !strings.Contains(result.Content, `"triggered"`) || !strings.Contains(result.Content, `"boom"`) {
		t.Fatalf("get_run output missing event content: %s", result.Content)
	}
}

func TestJobToolListEventsAppliesDateAndType(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	day := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	mgr.events = []jobs.EventEntry{
		{JobID: "daily-report", Timestamp: day.Add(time.Hour), Type: "emitted", Event: "done", Message: "inside"},
		{JobID: "daily-report", Timestamp: day.AddDate(0, 0, -1), Type: "emitted", Event: "done", Message: "before"},
		{JobID: "other", Timestamp: day.Add(2 * time.Hour), Type: "received", Event: "ping", Message: "other"},
	}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"list_events":true,"start_at":"2026-05-13T00:00:00Z","end_at":"2026-05-14T00:00:00Z","event_type":"emitted"}`))
	if err != nil {
		t.Fatalf("Execute list_events error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute list_events returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "inside") || strings.Contains(result.Content, "before") || strings.Contains(result.Content, "other") {
		t.Fatalf("list_events filter mismatch: %s", result.Content)
	}
	if mgr.lastEventFilt.Type != "emitted" {
		t.Fatalf("event filter type not passed: %#v", mgr.lastEventFilt)
	}
}

func TestJobToolListRunsAppliesStatusFilter(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	mgr.runsByJob["daily-report"] = []jobs.RunLog{
		{RunID: "r1", JobID: "daily-report", Status: "completed", StartedAt: time.Now()},
		{RunID: "r2", JobID: "daily-report", Status: "failed", StartedAt: time.Now()},
		{RunID: "r3", JobID: "daily-report", Status: "skipped", StartedAt: time.Now()},
	}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","list_runs":true,"status":["failed"]}`))
	if err != nil {
		t.Fatalf("Execute list_runs status error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute list_runs returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, `"r2"`) || strings.Contains(result.Content, `"r1"`) {
		t.Fatalf("status filter mismatch: %s", result.Content)
	}
	if len(mgr.lastRunFilter.Status) != 1 || mgr.lastRunFilter.Status[0] != "failed" {
		t.Fatalf("status filter not propagated: %#v", mgr.lastRunFilter)
	}
}

func TestJobToolListRunsExcludesDryRunByDefault(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["daily-report"] = &jobs.Job{ID: "daily-report", Name: "Daily Report", Enabled: true}
	mgr.runsByJob["daily-report"] = []jobs.RunLog{
		{RunID: "real", JobID: "daily-report", Status: "completed", StartedAt: time.Now()},
		{RunID: "dry", JobID: "daily-report", Status: "completed", IsDryRun: true, StartedAt: time.Now()},
	}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","list_runs":true}`))
	if err != nil {
		t.Fatalf("Execute list_runs default error = %v", err)
	}
	if strings.Contains(result.Content, `"dry"`) {
		t.Fatalf("default list_runs leaked dry-run: %s", result.Content)
	}
	if mgr.lastRunFilter.IncludeDryRun {
		t.Fatalf("default filter should not include dry runs")
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"job_id":"daily-report","list_runs":true,"include_dry_run":true}`))
	if err != nil {
		t.Fatalf("Execute list_runs include dry error = %v", err)
	}
	if !strings.Contains(result.Content, `"dry"`) || !strings.Contains(result.Content, `"real"`) {
		t.Fatalf("include_dry_run did not surface dry-run: %s", result.Content)
	}
}

func TestJobToolExclusivityValidation(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["x"] = &jobs.Job{ID: "x", Name: "X", Enabled: true}
	tool := NewJob(mgr)

	cases := []struct {
		name string
		args string
	}{
		{"list_runs+list_events", `{"job_id":"x","list_runs":true,"list_events":true}`},
		{"run_id+run", `{"job_id":"x","run_id":"r1","run":true}`},
		{"run_id+list_runs", `{"job_id":"x","run_id":"r1","list_runs":true}`},
		{"delete+list_events", `{"job_id":"x","delete":true,"list_events":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content, "mutually exclusive") {
				t.Fatalf("expected mutually-exclusive rejection, got %s", result.Content)
			}
		})
	}
}

func TestJobToolRejectsEmptyRunID(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["x"] = &jobs.Job{ID: "x", Name: "X", Enabled: true}
	tool := NewJob(mgr)

	cases := []struct {
		name string
		args string
	}{
		{"empty run_id", `{"job_id":"x","run_id":""}`},
		{"blank run_id", `{"job_id":"x","run_id":"   "}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content, "run_id cannot be empty") {
				t.Fatalf("expected empty run_id rejection, got %s", result.Content)
			}
		})
	}
}

func TestJobToolFilterRequiresList(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["x"] = &jobs.Job{ID: "x", Name: "X", Enabled: true}
	tool := NewJob(mgr)

	cases := []struct {
		name     string
		args     string
		fragment string
	}{
		{"started_after without list_runs", `{"job_id":"x","started_after":"2026-05-13T00:00:00Z"}`, "started_after is only valid with list_runs"},
		{"status without list_runs", `{"job_id":"x","status":["failed"]}`, "status is only valid with list_runs"},
		{"date without list_events", `{"job_id":"x","date":"2026-05-13"}`, "date is only valid with list_events"},
		{"event_type without list_events", `{"event_type":"emitted"}`, "event_type is only valid with list_events"},
		{"offset without list_events", `{"offset":10}`, "offset is only valid with list_events"},
		{"offset with list_runs", `{"job_id":"x","list_runs":true,"offset":10}`, "offset is only valid with list_events"},
		{"limit without list action", `{"job_id":"x","limit":10}`, "limit is only valid with list_runs or list_events"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content, c.fragment) {
				t.Fatalf("expected %q error, got %s", c.fragment, result.Content)
			}
		})
	}
}

func TestJobToolRejectsEmptyTimeFilters(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["x"] = &jobs.Job{ID: "x", Name: "X", Enabled: true}
	tool := NewJob(mgr)

	cases := []struct {
		name     string
		args     string
		fragment string
	}{
		{"blank started_after", `{"job_id":"x","list_runs":true,"started_after":"   "}`, "started_after cannot be empty"},
		{"blank started_before", `{"job_id":"x","list_runs":true,"started_before":"   "}`, "started_before cannot be empty"},
		{"blank start_at", `{"list_events":true,"start_at":"   "}`, "start_at cannot be empty"},
		{"blank end_at", `{"list_events":true,"end_at":"   "}`, "end_at cannot be empty"},
		{"blank date", `{"list_events":true,"date":"   "}`, "date cannot be empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content, c.fragment) {
				t.Fatalf("expected %q error, got %s", c.fragment, result.Content)
			}
		})
	}
}

// mustCreateJob executa um create e devolve o job persistido sob o id `want`.
func mustCreateJob(t *testing.T, args, want string) *jobs.Job {
	t.Helper()
	mgr := newFakeManager()
	tool := NewJob(mgr)
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}
	job, ok := mgr.jobs[want]
	if !ok {
		t.Fatalf("expected job %q to be created, got %#v", want, mgr.jobs)
	}
	return job
}

// TestJobToolCoercesStringifiedWriteFields prova a camada de tolerância: cada
// campo tipado enviado como string JSON é aceito e produz o valor correto no job.
func TestJobToolCoercesStringifiedWriteFields(t *testing.T) {
	cases := []struct {
		name  string
		args  string
		check func(t *testing.T, job *jobs.Job)
	}{
		{
			name: "inputs stringified",
			args: `{"job_id":"j","inputs":"{\"task_list_slug\":\"ops\",\"limit\":50}"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if got := job.Inputs["task_list_slug"]; got != "ops" {
					t.Fatalf("inputs.task_list_slug = %#v, want ops", got)
				}
				if got, ok := job.Inputs["limit"].(float64); !ok || got != 50 {
					t.Fatalf("inputs.limit = %#v, want 50", job.Inputs["limit"])
				}
			},
		},
		{
			name: "max_runs_per_hour stringified",
			args: `{"job_id":"j","max_runs_per_hour":"1000"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if job.MaxRunsPerHour != 1000 {
					t.Fatalf("max_runs_per_hour = %d, want 1000", job.MaxRunsPerHour)
				}
			},
		},
		{
			name: "triggers stringified",
			args: `{"job_id":"j","triggers":"[{\"type\":\"cron\",\"expression\":\"0 9 * * 1-5\"}]"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if len(job.Triggers) != 1 || job.Triggers[0].Type != jobs.TriggerCron || job.Triggers[0].Expression != "0 9 * * 1-5" {
					t.Fatalf("triggers = %#v", job.Triggers)
				}
			},
		},
		{
			name: "tags stringified",
			args: `{"job_id":"j","tags":"[\"ops\",\"jira\"]"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if len(job.Tags) != 2 || job.Tags[0] != "ops" || job.Tags[1] != "jira" {
					t.Fatalf("tags = %#v", job.Tags)
				}
			},
		},
		{
			name: "error_policy stringified",
			args: `{"job_id":"j","error_policy":"{\"strategy\":\"retry\",\"max_retries\":3,\"backoff\":\"exponential\"}"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if job.ErrorPolicy.Strategy != jobs.ErrorRetry || job.ErrorPolicy.MaxRetries != 3 || job.ErrorPolicy.Backoff != jobs.BackoffExponential {
					t.Fatalf("error_policy = %#v", job.ErrorPolicy)
				}
			},
		},
		{
			name: "events stringified",
			args: `{"job_id":"j","events":"{\"on_success\":\"e.done\",\"emit_when\":\"{{ true }}\"}"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if job.Events.OnSuccess != "e.done" || job.Events.EmitWhen != "{{ true }}" {
					t.Fatalf("events = %#v", job.Events)
				}
			},
		},
		{
			name: "output stringified",
			args: `{"job_id":"j","output":"{\"map\":{\"result\":\"{{ .output.data }}\"}}"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if job.Output.Map["result"] != "{{ .output.data }}" {
					t.Fatalf("output = %#v", job.Output)
				}
			},
		},
		{
			name: "dry_run_config stringified",
			args: `{"job_id":"j","dry_run_config":"{\"enabled\":true,\"mock_output\":{\"status\":\"ok\"}}"}`,
			check: func(t *testing.T, job *jobs.Job) {
				if !job.DryRun.Enabled || job.DryRun.MockOutput["status"] != "ok" {
					t.Fatalf("dry_run = %#v", job.DryRun)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mgr := newFakeManager()
			mgr.jobs["j"] = &jobs.Job{ID: "j", Name: "J", Enabled: true, Tool: "web_fetch", Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}}
			tool := NewJob(mgr)
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if result.IsError {
				t.Fatalf("Execute returned error result: %s", result.Content)
			}
			c.check(t, mgr.jobs["j"])
		})
	}
}

// TestJobToolCoercesStringifiedEnabledToggle cobre o caminho de toggle (enabled
// sozinho com job_id), onde o valor também pode chegar stringificado.
func TestJobToolCoercesStringifiedEnabledToggle(t *testing.T) {
	mgr := newFakeManager()
	mgr.jobs["j"] = &jobs.Job{ID: "j", Name: "J", Enabled: true}
	tool := NewJob(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"j","enabled":"false"}`))
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}
	if mgr.jobs["j"].Enabled {
		t.Fatal("expected enabled to be coerced to false and job disabled")
	}
}

// TestJobToolStringifiedEquivalentToTyped garante que um payload totalmente
// stringificado produz exatamente o mesmo job que o payload com tipos corretos.
func TestJobToolStringifiedEquivalentToTyped(t *testing.T) {
	typed := `{
		"name":"J","tool":"web_fetch",
		"triggers":[{"type":"cron","expression":"0 9 * * 1-5"}],
		"inputs":{"k":"v","n":3},
		"enabled":false,
		"max_runs_per_hour":1000,
		"tags":["ops"],
		"error_policy":{"strategy":"retry","max_retries":3},
		"events":{"on_success":"e.done"},
		"output":{"map":{"result":"{{ .output.data }}"}},
		"dry_run_config":{"enabled":true,"mock_output":{"status":"ok"}}
	}`
	stringified := `{
		"name":"J","tool":"web_fetch",
		"triggers":"[{\"type\":\"cron\",\"expression\":\"0 9 * * 1-5\"}]",
		"inputs":"{\"k\":\"v\",\"n\":3}",
		"enabled":"false",
		"max_runs_per_hour":"1000",
		"tags":"[\"ops\"]",
		"error_policy":"{\"strategy\":\"retry\",\"max_retries\":3}",
		"events":"{\"on_success\":\"e.done\"}",
		"output":"{\"map\":{\"result\":\"{{ .output.data }}\"}}",
		"dry_run_config":"{\"enabled\":true,\"mock_output\":{\"status\":\"ok\"}}"
	}`

	a := mustCreateJob(t, typed, "j")
	b := mustCreateJob(t, stringified, "j")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("stringified job not equivalent to typed job:\n typed=%#v\n string=%#v", a, b)
	}
}

// TestJobToolStringifiedInvalidReturnsExplanatoryError prova que strings que não
// são válidas para o tipo esperado produzem mensagem explicativa (não o erro cru).
func TestJobToolStringifiedInvalidReturnsExplanatoryError(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		fragment string
	}{
		{"enabled not bool", `{"job_id":"j","enabled":"yes"}`, "must be a boolean"},
		{"max_runs not number", `{"job_id":"j","max_runs_per_hour":"lots"}`, "must be an integer"},
		{"max_runs float string", `{"job_id":"j","max_runs_per_hour":"1.5"}`, "must be an integer"},
		{"inputs not object", `{"job_id":"j","inputs":"not json"}`, "must be a JSON object"},
		{"triggers not array", `{"job_id":"j","triggers":"nope"}`, "must be a JSON array"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mgr := newFakeManager()
			mgr.jobs["j"] = &jobs.Job{ID: "j", Name: "J", Enabled: true}
			tool := NewJob(mgr)
			result, err := tool.Execute(context.Background(), json.RawMessage(c.args))
			if err != nil {
				t.Fatalf("Execute error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected explanatory error, got %s", result.Content)
			}
			if !strings.Contains(result.Content, c.fragment) {
				t.Fatalf("expected error to contain %q, got %s", c.fragment, result.Content)
			}
			if !strings.Contains(result.Content, "Example") {
				t.Fatalf("expected explanatory error with example, got %s", result.Content)
			}
			if strings.Contains(result.Content, "cannot unmarshal") {
				t.Fatalf("expected explanatory error, not raw encoding/json error: %s", result.Content)
			}
		})
	}
}

// TestJobParametersHasNoOpaqueObjects congela o fix de raiz: nenhum campo do
// schema deve ser um `object` opaco (sem properties e sem additionalProperties
// tipado), exceto os intencionalmente livres: inputs, mock_output e output.schema.
func TestJobParametersHasNoOpaqueObjects(t *testing.T) {
	tool := NewJob(newFakeManager())
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() is not valid JSON: %v", err)
	}
	allowed := map[string]bool{"inputs": true, "mock_output": true, "schema": true}

	var offenders []string
	var walk func(name string, node map[string]any)
	walk = func(name string, node map[string]any) {
		if typ, _ := node["type"].(string); typ == "object" {
			_, hasProps := node["properties"]
			_, apIsSchema := node["additionalProperties"].(map[string]any)
			if !hasProps && !apIsSchema && !allowed[name] {
				offenders = append(offenders, name)
			}
		}
		if props, ok := node["properties"].(map[string]any); ok {
			for k, v := range props {
				if child, ok := v.(map[string]any); ok {
					walk(k, child)
				}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			walk(name+".items", items)
		}
	}
	walk("root", schema)
	if len(offenders) > 0 {
		t.Fatalf("opaque object schema fields without properties: %v", offenders)
	}
}

func TestPipelineToolCoercesStringifiedMetadata(t *testing.T) {
	mgr := newFakeManager()
	tool := NewPipeline(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Ops","metadata":"{\"owner\":\"ops\"}"}`))
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}
	if got := mgr.pipelines["ops"].Metadata["owner"]; got != "ops" {
		t.Fatalf("metadata.owner = %#v, want ops", got)
	}
}

func TestPipelineToolCreatesUpdatesAndDeletes(t *testing.T) {
	mgr := newFakeManager()
	tool := NewPipeline(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Ops Jobs","description":"Operations","enabled":false}`))
	if err != nil {
		t.Fatalf("Execute create error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute create returned error result: %s", result.Content)
	}
	if _, ok := mgr.pipelines["ops-jobs"]; !ok {
		t.Fatal("expected ops-jobs pipeline to be saved")
	}
	if mgr.pipelines["ops-jobs"].Enabled {
		t.Fatal("expected pipeline to be created disabled")
	}
	result, err = tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute list error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute list returned error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Ops Jobs") {
		t.Fatalf("list output does not include standalone pipeline: %s", result.Content)
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"slug":"Ops Jobs","description":""}`))
	if err != nil {
		t.Fatalf("Execute update error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute update returned error result: %s", result.Content)
	}
	if mgr.pipelines["ops-jobs"].Description != "" {
		t.Fatalf("expected pipeline description to be cleared, got %q", mgr.pipelines["ops-jobs"].Description)
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"slug":"Ops Jobs","delete":true}`))
	if err != nil {
		t.Fatalf("Execute delete error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute delete returned error result: %s", result.Content)
	}
	if _, ok := mgr.pipelines["ops-jobs"]; ok {
		t.Fatal("expected pipeline to be deleted")
	}
}

func TestPipelineToolRejectsDeleteWithWriteFields(t *testing.T) {
	mgr := newFakeManager()
	mgr.pipelines["ops"] = jobs.Pipeline{Slug: "ops", Name: "Ops", Enabled: true}
	tool := NewPipeline(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"slug":"ops","delete":true,"description":"new"}`))
	if err != nil {
		t.Fatalf("Execute delete+write error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected delete+write to be rejected, got %s", result.Content)
	}
	if _, ok := mgr.pipelines["ops"]; !ok {
		t.Fatal("delete+write should not delete the pipeline")
	}
}

func TestPipelineToolCreatesWithExplicitSlug(t *testing.T) {
	mgr := newFakeManager()
	tool := NewPipeline(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"slug":"ops","name":"Operations"}`))
	if err != nil {
		t.Fatalf("Execute explicit slug create error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute explicit slug create returned error result: %s", result.Content)
	}
	if pipeline, ok := mgr.pipelines["ops"]; !ok || pipeline.Name != "Operations" {
		t.Fatalf("expected explicit slug pipeline to be created, got %#v", mgr.pipelines)
	}
}

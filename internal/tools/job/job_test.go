package job

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/jobs"
)

type fakeManager struct {
	jobs      map[string]*jobs.Job
	pipelines map[string]jobs.Pipeline
}

func newFakeManager() *fakeManager {
	return &fakeManager{
		jobs:      make(map[string]*jobs.Job),
		pipelines: make(map[string]jobs.Pipeline),
	}
}

func (m *fakeManager) GetJobs() []jobs.JobInfo {
	out := make([]jobs.JobInfo, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, jobs.JobInfo{ID: job.ID, Name: job.Name, Enabled: job.Enabled, Pipeline: job.Pipeline, Tool: job.Tool})
	}
	return out
}

func (m *fakeManager) GetJob(id string) (*jobs.Job, error) {
	job, ok := m.jobs[id]
	if !ok {
		return nil, errNotFound(id)
	}
	copy := *job
	return &copy, nil
}

func (m *fakeManager) SaveJob(job *jobs.Job) error {
	copy := *job
	m.jobs[job.ID] = &copy
	return nil
}

func (m *fakeManager) DeleteJob(id string) error {
	delete(m.jobs, id)
	return nil
}

func (m *fakeManager) ToggleJob(id string, enabled bool) error {
	job, ok := m.jobs[id]
	if !ok {
		return errNotFound(id)
	}
	job.Enabled = enabled
	return nil
}

func (m *fakeManager) RunJob(id string) (*jobs.RunLog, error) {
	return &jobs.RunLog{RunID: "run-1", JobID: id, Status: "completed"}, nil
}

func (m *fakeManager) DryRunJob(id string) (*jobs.DryRunResult, error) {
	return &jobs.DryRunResult{Success: true, RunLog: &jobs.RunLog{RunID: "dry-1", JobID: id}}, nil
}

func (m *fakeManager) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	return []jobs.RunLog{{RunID: "run-1", JobID: id, Status: "completed"}}, nil
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

func (m *fakeManager) SavePipeline(pipeline *jobs.Pipeline) error {
	m.pipelines[pipeline.Slug] = *pipeline
	return nil
}

func (m *fakeManager) DeletePipeline(slug string) error {
	delete(m.pipelines, slug)
	return nil
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

	result, err = tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute list error = %v", err)
	}
	if !strings.Contains(result.Content, "Daily Report") {
		t.Fatalf("list output does not include job: %s", result.Content)
	}
}

func TestPipelineToolCreatesUpdatesAndDeletes(t *testing.T) {
	mgr := newFakeManager()
	tool := NewPipeline(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Ops Jobs","description":"Operations"}`))
	if err != nil {
		t.Fatalf("Execute create error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute create returned error result: %s", result.Content)
	}
	if _, ok := mgr.pipelines["ops-jobs"]; !ok {
		t.Fatal("expected ops-jobs pipeline to be saved")
	}

	result, err = tool.Execute(context.Background(), json.RawMessage(`{"slug":"Ops Jobs","enabled":false}`))
	if err != nil {
		t.Fatalf("Execute update error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute update returned error result: %s", result.Content)
	}
	if mgr.pipelines["ops-jobs"].Enabled {
		t.Fatal("expected pipeline to be disabled")
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

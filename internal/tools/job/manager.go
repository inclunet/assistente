package job

import (
	"context"

	"assistente/internal/jobs"
)

// Manager is the narrow surface the job tools need from the jobs runtime.
type Manager interface {
	GetJobs() []jobs.JobInfo
	GetJob(id string) (*jobs.Job, error)
	SaveJob(job *jobs.Job) error
	DeleteJob(id string) error
	ToggleJob(id string, enabled bool) error
	RunJob(id string) (*jobs.RunLog, error)
	DryRunJob(id string) (*jobs.DryRunResult, error)
	GetJobRuns(id string, limit int) ([]jobs.RunLog, error)

	GetPipelines() []jobs.PipelineInfo
	ListPipelines() ([]jobs.Pipeline, error)
	SavePipeline(pipeline *jobs.Pipeline) error
	DeletePipeline(slug string) error
}

type ManagerProvider func() Manager

type contextManager interface {
	GetJobContext(ctx context.Context, id string) (*jobs.Job, error)
	SaveJobContext(ctx context.Context, job *jobs.Job) error
	DeleteJobContext(ctx context.Context, id string) error
	ToggleJobContext(ctx context.Context, id string, enabled bool) error
	RunJobContext(ctx context.Context, id string) (*jobs.RunLog, error)
	DryRunJobContext(ctx context.Context, id string) (*jobs.DryRunResult, error)
	GetJobRunsContext(ctx context.Context, id string, limit int) ([]jobs.RunLog, error)

	ListPipelinesContext(ctx context.Context) ([]jobs.Pipeline, error)
	SavePipelineContext(ctx context.Context, pipeline *jobs.Pipeline) error
	DeletePipelineContext(ctx context.Context, slug string) error
}

func getJob(ctx context.Context, mgr Manager, id string) (*jobs.Job, error) {
	if cm, ok := mgr.(contextManager); ok {
		return cm.GetJobContext(ctx, id)
	}
	return mgr.GetJob(id)
}

func saveJob(ctx context.Context, mgr Manager, job *jobs.Job) error {
	if cm, ok := mgr.(contextManager); ok {
		return cm.SaveJobContext(ctx, job)
	}
	return mgr.SaveJob(job)
}

func deleteJob(ctx context.Context, mgr Manager, id string) error {
	if cm, ok := mgr.(contextManager); ok {
		return cm.DeleteJobContext(ctx, id)
	}
	return mgr.DeleteJob(id)
}

func toggleJob(ctx context.Context, mgr Manager, id string, enabled bool) error {
	if cm, ok := mgr.(contextManager); ok {
		return cm.ToggleJobContext(ctx, id, enabled)
	}
	return mgr.ToggleJob(id, enabled)
}

func runJob(ctx context.Context, mgr Manager, id string) (*jobs.RunLog, error) {
	if cm, ok := mgr.(contextManager); ok {
		return cm.RunJobContext(ctx, id)
	}
	return mgr.RunJob(id)
}

func dryRunJob(ctx context.Context, mgr Manager, id string) (*jobs.DryRunResult, error) {
	if cm, ok := mgr.(contextManager); ok {
		return cm.DryRunJobContext(ctx, id)
	}
	return mgr.DryRunJob(id)
}

func getJobRuns(ctx context.Context, mgr Manager, id string, limit int) ([]jobs.RunLog, error) {
	if cm, ok := mgr.(contextManager); ok {
		return cm.GetJobRunsContext(ctx, id, limit)
	}
	return mgr.GetJobRuns(id, limit)
}

func listPipelines(ctx context.Context, mgr Manager) ([]jobs.Pipeline, error) {
	if cm, ok := mgr.(contextManager); ok {
		return cm.ListPipelinesContext(ctx)
	}
	return mgr.ListPipelines()
}

func savePipeline(ctx context.Context, mgr Manager, pipeline *jobs.Pipeline) error {
	if cm, ok := mgr.(contextManager); ok {
		return cm.SavePipelineContext(ctx, pipeline)
	}
	return mgr.SavePipeline(pipeline)
}

func deletePipeline(ctx context.Context, mgr Manager, slug string) error {
	if cm, ok := mgr.(contextManager); ok {
		return cm.DeletePipelineContext(ctx, slug)
	}
	return mgr.DeletePipeline(slug)
}

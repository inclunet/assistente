package job

import (
	"context"

	"assistente/internal/jobs"
)

// Manager is the narrow surface the job tools need from the jobs runtime.
type Manager interface {
	GetJobsContext(ctx context.Context) ([]jobs.JobInfo, error)
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

type ManagerProvider func() Manager

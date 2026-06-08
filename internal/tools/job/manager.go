package job

import (
	"context"
	"reflect"

	"assistente/internal/jobs"
)

// Manager is the narrow surface the job tools need from the jobs runtime.
type Manager interface {
	GetJobsContext(ctx context.Context) ([]jobs.JobInfo, error)
	GetJobContext(ctx context.Context, id string) (*jobs.Job, error)
	CreateJobContext(ctx context.Context, job *jobs.Job) error
	SaveJobContext(ctx context.Context, job *jobs.Job) error
	DeleteJobContext(ctx context.Context, id string) error
	ToggleJobContext(ctx context.Context, id string, enabled bool) error
	RunJobContext(ctx context.Context, id string) (*jobs.RunLog, error)
	DryRunJobContext(ctx context.Context, id string) (*jobs.DryRunResult, error)
	GetJobRunsContext(ctx context.Context, id string, limit int) ([]jobs.RunLog, error)
	ListJobRunsContext(ctx context.Context, id string, filter jobs.RunFilter) ([]jobs.RunLog, error)
	GetJobRunDetailContext(ctx context.Context, jobID, runID string) (*jobs.RunDetail, error)
	ListJobEventsContext(ctx context.Context, filter jobs.EventFilter) ([]jobs.EventEntry, error)

	ListPipelinesContext(ctx context.Context) ([]jobs.Pipeline, error)
	CreatePipelineContext(ctx context.Context, pipeline *jobs.Pipeline) error
	SavePipelineContext(ctx context.Context, pipeline *jobs.Pipeline) error
	DeletePipelineContext(ctx context.Context, slug string) error
}

type ManagerProvider func() Manager

func managerIsNil(mgr Manager) bool {
	if mgr == nil {
		return true
	}
	v := reflect.ValueOf(mgr)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

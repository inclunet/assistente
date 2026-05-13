package job

import "assistente/internal/jobs"

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

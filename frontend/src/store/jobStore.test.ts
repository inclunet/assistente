import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from '@testing-library/react';
import { jobs } from '@wailsjs/go/models';
import { useJobStore } from './jobStore';

const { mockToggleJob } = vi.hoisted(() => ({
  mockToggleJob: vi.fn(),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetJobs: vi.fn(),
  GetJob: vi.fn(),
  ToggleJob: (id: string, enabled: boolean) => mockToggleJob(id, enabled),
  RunJob: vi.fn(),
  DryRunJob: vi.fn(),
  GetJobRuns: vi.fn(),
  GetJobEvents: vi.fn(),
  GetJobPipelines: vi.fn(),
  GetToolCatalog: vi.fn(),
  SaveJob: vi.fn(),
  DeleteJob: vi.fn(),
  TestToolDryRun: vi.fn(),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
}));

function jobInfo(overrides: Partial<jobs.JobInfo>): jobs.JobInfo {
  return {
    id: 'job-1',
    name: 'Job 1',
    description: '',
    enabled: true,
    effective_enabled: true,
    pipeline_enabled: true,
    status: 'idle',
    last_run: '',
    next_run: '',
    tags: [],
    trigger_count: 0,
    pipeline: '',
    ...overrides,
  } as jobs.JobInfo;
}

beforeEach(() => {
  mockToggleJob.mockReset();
  mockToggleJob.mockResolvedValue(undefined);
  useJobStore.setState({
    jobs: [],
    isLoading: false,
    error: null,
    selectedJobId: null,
    jobDetail: null,
    runLogs: [],
    events: [],
    pipelines: [],
  });
});

describe('jobStore.toggleJob', () => {
  it('deriva effective_enabled a partir do estado do job e do pipeline', async () => {
    useJobStore.setState({
      jobs: [jobInfo({ id: 'pipeline-job', pipeline_enabled: false, effective_enabled: false })],
    });

    await act(async () => {
      await useJobStore.getState().toggleJob('pipeline-job', true);
    });

    const [job] = useJobStore.getState().jobs;
    expect(job.enabled).toBe(true);
    expect(job.pipeline_enabled).toBe(false);
    expect(job.effective_enabled).toBe(false);
  });

  it('reativa effective_enabled quando job e pipeline estão ativos', async () => {
    useJobStore.setState({
      jobs: [jobInfo({ id: 'active-job', enabled: false, effective_enabled: false, pipeline_enabled: true })],
    });

    await act(async () => {
      await useJobStore.getState().toggleJob('active-job', true);
    });

    const [job] = useJobStore.getState().jobs;
    expect(job.enabled).toBe(true);
    expect(job.pipeline_enabled).toBe(true);
    expect(job.effective_enabled).toBe(true);
  });
});

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from '@testing-library/react';
import { jobs } from '@wailsjs/go/models';
import { useJobStore } from './jobStore';

const { mockGetToolCatalog, mockTestToolDryRun, mockToggleJob } = vi.hoisted(() => ({
  mockGetToolCatalog: vi.fn(),
  mockTestToolDryRun: vi.fn(),
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
  GetToolCatalog: () => mockGetToolCatalog(),
  SaveJob: vi.fn(),
  DeleteJob: vi.fn(),
  TestToolDryRun: (requestJSON: string) => mockTestToolDryRun(requestJSON),
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
  mockGetToolCatalog.mockReset();
  mockGetToolCatalog.mockResolvedValue([]);
  mockTestToolDryRun.mockReset();
  mockTestToolDryRun.mockResolvedValue({ success: true });
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

describe('jobStore.testTool', () => {
  it('envia metadata resolvida para dry-run de tool MCP bridge', async () => {
    mockGetToolCatalog.mockResolvedValue([
      {
        id: 'catalog-1',
        mcp_server_id: 'server-1',
        name: 'mcp_jira__issue__delete',
        description: 'Delete issue',
        schema: [],
        source: 'mcp',
        origin: 'mcp_bridge',
        risk: 'write',
      },
    ]);

    await act(async () => {
      await useJobStore.getState().testTool(
        '  mcp_jira__issue__delete  ',
        { issue_key: 'ABC-1' },
        { event: 'manual' },
      );
    });

    expect(mockGetToolCatalog).toHaveBeenCalledTimes(1);
    expect(mockTestToolDryRun).toHaveBeenCalledTimes(1);
    expect(JSON.parse(mockTestToolDryRun.mock.calls[0][0])).toEqual({
      tool_name: 'mcp_jira__issue__delete',
      inputs: { issue_key: 'ABC-1' },
      event_data: { event: 'manual' },
      mcp_server_id: 'server-1',
      tool_catalog_id: 'catalog-1',
      origin: 'mcp_bridge',
      risk: 'write',
    });
  });

  it('mantem request sem metadata MCP quando catalogo nao contem a tool', async () => {
    mockGetToolCatalog.mockResolvedValue([]);

    await act(async () => {
      await useJobStore.getState().testTool('mcp_jira__issue__delete', {}, undefined);
    });

    expect(mockGetToolCatalog).toHaveBeenCalledTimes(1);
    expect(mockTestToolDryRun).toHaveBeenCalledTimes(1);
    expect(JSON.parse(mockTestToolDryRun.mock.calls[0][0])).toEqual({
      tool_name: 'mcp_jira__issue__delete',
      inputs: {},
      event_data: undefined,
    });
  });

  it('usa catalogo para nomes MCP native namespaced', async () => {
    mockGetToolCatalog.mockResolvedValue([
      {
        id: 'catalog-native-1',
        mcp_server_id: 'server-1',
        name: 'mcp_native__filesystem',
        description: 'Filesystem',
        schema: [],
        source: 'mcp',
        origin: 'mcp_native',
        risk: '',
      },
    ]);

    await act(async () => {
      await useJobStore.getState().testTool('mcp_native__filesystem', {}, undefined);
    });

    expect(mockGetToolCatalog).toHaveBeenCalledTimes(1);
    expect(mockTestToolDryRun).toHaveBeenCalledTimes(1);
    expect(JSON.parse(mockTestToolDryRun.mock.calls[0][0])).toEqual({
      tool_name: 'mcp_native__filesystem',
      inputs: {},
      event_data: undefined,
      mcp_server_id: 'server-1',
      tool_catalog_id: 'catalog-native-1',
      origin: 'mcp_native',
      risk: '',
    });
  });
});

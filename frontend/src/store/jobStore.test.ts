import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from '@testing-library/react';
import { jobs } from '@wailsjs/go/models';
import { JOB_PROFILE_AUTHORIZATION_REQUIRED, useJobStore } from './jobStore';

const {
  mockAuthorizeJobProfile,
  mockGetJob,
  mockGetJobs,
  mockGetJobProfileGrantState,
  mockGetToolCatalog,
  mockTestToolDryRun,
  mockToggleJob,
} = vi.hoisted(() => ({
  mockAuthorizeJobProfile: vi.fn(),
  mockGetJob: vi.fn(),
  mockGetJobs: vi.fn(),
  mockGetJobProfileGrantState: vi.fn(),
  mockGetToolCatalog: vi.fn(),
  mockTestToolDryRun: vi.fn(),
  mockToggleJob: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Jobs', () => ({
  GetJobs: () => mockGetJobs(),
  GetJob: (id: string) => mockGetJob(id),
  GetJobProfileGrantState: (id: string) => mockGetJobProfileGrantState(id),
  AuthorizeJobProfile: (id: string, profile: string) => mockAuthorizeJobProfile(id, profile),
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
  mockGetJob.mockReset();
  mockGetJob.mockResolvedValue(undefined);
  mockGetJobs.mockReset();
  mockGetJobs.mockResolvedValue([]);
  mockGetJobProfileGrantState.mockReset();
  mockGetJobProfileGrantState.mockResolvedValue({ grants: [] });
  mockAuthorizeJobProfile.mockReset();
  mockAuthorizeJobProfile.mockResolvedValue(false);
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

  it('habilita literal com grant exato sem perguntar novamente', async () => {
    mockGetJob.mockResolvedValue({ tool: 'subagent', inputs: { profile: 'pesquisa' } });
    mockGetJobProfileGrantState.mockResolvedValue({
      grants: [{ targetProfileSlug: 'pesquisa' }],
    });
    await useJobStore.getState().toggleJob('literal', true);
    expect(mockAuthorizeJobProfile).not.toHaveBeenCalled();
    expect(mockToggleJob).toHaveBeenCalledWith('literal', true);
  });

  it('solicita autorização literal antes de habilitar', async () => {
    mockGetJob.mockResolvedValue({ tool: 'subagent', inputs: { profile: 'pesquisa' } });
    mockAuthorizeJobProfile.mockResolvedValue(true);
    await useJobStore.getState().toggleJob('literal', true);
    expect(mockAuthorizeJobProfile).toHaveBeenCalledWith('literal', 'pesquisa');
    expect(mockToggleJob).toHaveBeenCalledWith('literal', true);
  });

  it('bloqueia template dinâmico sem grant antes do backend', async () => {
    mockGetJob.mockResolvedValue({ tool: 'subagent', inputs: { profile: '{{ .event.profile }}' } });
    await expect(useJobStore.getState().toggleJob('dinamico', true)).rejects.toThrow(JOB_PROFILE_AUTHORIZATION_REQUIRED);
    expect(mockAuthorizeJobProfile).not.toHaveBeenCalled();
    expect(mockToggleJob).not.toHaveBeenCalled();
  });

  it('reconcilia lista quando backend recusa ativação', async () => {
    mockGetJob.mockResolvedValue({ tool: 'subagent', inputs: { profile: 'pesquisa' } });
    mockGetJobProfileGrantState.mockResolvedValue({
      grants: [{ targetProfileSlug: 'pesquisa' }],
    });
    mockToggleJob.mockRejectedValue(new Error('authorization_not_granted'));
    const disabled = jobInfo({ id: 'literal', enabled: false, effective_enabled: false });
    mockGetJobs.mockResolvedValue([disabled]);
    await expect(useJobStore.getState().toggleJob('literal', true)).rejects.toThrow(JOB_PROFILE_AUTHORIZATION_REQUIRED);
    expect(useJobStore.getState().jobs).toEqual([disabled]);
    expect(useJobStore.getState().error).toBeNull();
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

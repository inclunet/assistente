import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import type { jobs } from '@wailsjs/go/models';
import { JobBuilder } from './JobBuilder';

const announce = vi.hoisted(() => vi.fn());
const saveJob = vi.hoisted(() => vi.fn());
const toggleJob = vi.hoisted(() => vi.fn());
const getGrantState = vi.hoisted(() => vi.fn());
const authorizeProfile = vi.hoisted(() => vi.fn());
const revokeProfile = vi.hoisted(() => vi.fn());
const getProfiles = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (options?.profile) return `${key}:${String(options.profile)}`;
      return key;
    },
  }),
}));

vi.mock('../../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce }),
}));

vi.mock('../../../store/jobStore', () => ({
  useJobStore: () => ({
    saveJob,
    toggleJob,
    getJobProfileGrantState: getGrantState,
    authorizeJobProfile: authorizeProfile,
    revokeJobProfile: revokeProfile,
    testTool: vi.fn(),
    fetchToolCatalog: vi.fn().mockResolvedValue([]),
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Jobs', () => ({
  ListKnownEvents: vi.fn().mockResolvedValue([]),
  InferEventSchema: vi.fn().mockResolvedValue({}),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetProfiles: getProfiles,
}));

vi.mock('../../pickers/ToolPicker', () => ({
  ToolPicker: () => <div />,
}));
vi.mock('./SchemaForm', () => ({
  SchemaForm: ({ onChange }: { onChange: (key: string, value: unknown) => void }) => (
    <button type="button" onClick={() => onChange('profile', 'programacao')}>
      edit-profile
    </button>
  ),
}));
vi.mock('./TriggerEditor', () => ({ TriggerEditor: () => <div /> }));
vi.mock('./OutputExplorer', () => ({ OutputExplorer: () => <div /> }));
vi.mock('./TemplateEditor', () => ({ TemplateEditor: () => <div /> }));
vi.mock('./YAMLPreview', () => ({ YAMLPreview: () => <div /> }));

function subagentJob(profile: string): jobs.Job {
  return {
    id: 'resumo-diario',
    name: 'Resumo diário',
    description: '',
    enabled: true,
    pipeline: '',
    tags: [],
    triggers: [{ type: 'manual' }],
    tool: 'subagent',
    inputs: { profile, prompt: 'Resuma' },
    events: {},
    error_policy: { strategy: 'stop', max_retries: 0, retry_delay: '' },
    max_runs_per_hour: 0,
    dry_run: {},
    metadata: {},
    status: 'idle',
    pipeline_enabled: true,
  } as unknown as jobs.Job;
}

describe('JobBuilder grants de profiles', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getProfiles.mockResolvedValue([
      { slug: 'pesquisa', name: 'Pesquisa' },
      { slug: 'programacao', name: 'Programação' },
    ]);
    getGrantState.mockResolvedValue({ grants: [], fingerprint: 'fp', profileExpression: 'pesquisa' });
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: false,
      requestedEnabled: true,
      targetProfileSlug: '',
      dynamicProfile: false,
    });
    authorizeProfile.mockResolvedValue(true);
  });

  it('solicita uma vez para literal e mantém desativado na recusa', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: true,
      requestedEnabled: true,
      targetProfileSlug: 'pesquisa',
      dynamicProfile: false,
    });
    authorizeProfile.mockResolvedValue(false);
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={onClose} />);

    await user.click(screen.getByRole('button', { name: 'common.save' }));

    await waitFor(() => expect(authorizeProfile).toHaveBeenCalledTimes(1));
    expect(authorizeProfile).toHaveBeenCalledWith('resumo-diario', 'pesquisa');
    expect(toggleJob).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
    expect(announce).toHaveBeenCalledWith('jobs.builder.savedDisabledWithoutAuthorization', 'assertive');
  });

  it('não repete prompt quando o backend informa grant válido', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={onClose} />);

    await user.click(screen.getByRole('button', { name: 'common.save' }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(authorizeProfile).not.toHaveBeenCalled();
  });

  it('autoriza slugs individuais de template e permite revogação', async () => {
    const user = userEvent.setup();
    getGrantState
      .mockResolvedValueOnce({
        grants: [{ targetProfileSlug: 'pesquisa' }],
        fingerprint: 'fp',
        profileExpression: '{{ .event.profile }}',
      })
      .mockResolvedValue({ grants: [], fingerprint: 'fp', profileExpression: '{{ .event.profile }}' });
    render(<JobBuilder editJob={subagentJob('{{ .event.profile }}')} onClose={vi.fn()} />);

    expect(await screen.findByText('Pesquisa')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'jobs.builder.revokeProfile' }));
    await waitFor(() => expect(revokeProfile).toHaveBeenCalledWith('resumo-diario', 'pesquisa'));
    await waitFor(() => expect(document.activeElement).toBe(
      screen.getByRole('region', { name: 'jobs.builder.authorizedProfilesTitle' }),
    ));

    await user.selectOptions(
      screen.getByRole('combobox', { name: 'jobs.builder.chooseProfileToAuthorize' }),
      'programacao',
    );
    const authorizeButton = screen.getByRole('button', { name: 'jobs.builder.authorizeProfile' });
    await user.click(authorizeButton);
    await waitFor(() => expect(authorizeProfile).toHaveBeenCalledWith('resumo-diario', 'programacao'));
    expect(document.activeElement).toBe(authorizeButton);
  });

  it('mantém a seção acessível', async () => {
    const { container } = render(
      <JobBuilder editJob={subagentJob('{{ .event.profile }}')} onClose={vi.fn()} />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('bloqueia grants do estado persistido enquanto o draft diverge sem recarregar profiles', async () => {
    const user = userEvent.setup();
    getGrantState.mockResolvedValue({
      grants: [{ targetProfileSlug: 'pesquisa' }],
      fingerprint: 'fp',
      profileExpression: 'pesquisa',
    });
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={vi.fn()} />);
    await screen.findByText('Pesquisa');
    const callsBeforeEdit = getProfiles.mock.calls.length;
    await user.click(screen.getByRole('button', { name: 'edit-profile' }));
    expect(screen.getByRole('button', { name: 'jobs.builder.revokeProfile' })).toBeDisabled();
    expect(screen.getByText('jobs.builder.saveProfileConfigurationFirst')).toBeInTheDocument();
    expect(getProfiles).toHaveBeenCalledTimes(callsBeforeEdit);
  });
});

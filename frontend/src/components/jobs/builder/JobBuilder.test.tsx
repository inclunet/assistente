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
  isJobProfileAuthorizationError: (err: unknown) => (
    typeof err === 'object' && err !== null && 'code' in err
    && err.code === 'job_profile_authorization_required'
  ),
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

  it('localiza revogação concorrente entre autorização e ativação', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: true,
      requestedEnabled: true,
      targetProfileSlug: 'pesquisa',
      dynamicProfile: false,
    });
    toggleJob.mockRejectedValue({ code: 'job_profile_authorization_required' });
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'common.save' }));
    expect(await screen.findByText('jobs.builder.profileGrantNotAuthorizedError')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('mantém editor aberto quando autorizar falha operacionalmente', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: true,
      requestedEnabled: true,
      targetProfileSlug: 'pesquisa',
      dynamicProfile: false,
    });
    authorizeProfile.mockRejectedValue(new Error('store de grants indisponível'));
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'common.save' }));
    expect(await screen.findByText('jobs.builder.profileGrantUnavailableError')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(announce).not.toHaveBeenCalledWith(
      'jobs.builder.savedDisabledWithoutAuthorization',
      'assertive',
    );
  });

  it('localiza revalidação fail-closed retornada pelo SaveJob', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    saveJob.mockRejectedValue(new Error('authorization_not_granted'));
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'common.save' }));
    expect(await screen.findByText('jobs.builder.profileGrantNotAuthorizedError')).toBeInTheDocument();
    expect(screen.queryByText(/authorization_not_granted/)).not.toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('não anuncia falta de autorização quando grant foi aprovado para job desabilitado', async () => {
    const user = userEvent.setup();
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: true,
      requestedEnabled: false,
      targetProfileSlug: 'pesquisa',
      dynamicProfile: false,
    });
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(authorizeProfile).toHaveBeenCalled());
    expect(announce).not.toHaveBeenCalledWith(
      'jobs.builder.savedDisabledWithoutAuthorization',
      'assertive',
    );
  });

  it('remove grants revogados da lista após salvar nova expressão', async () => {
    const user = userEvent.setup();
    getGrantState
      .mockResolvedValueOnce({
        grants: [{ targetProfileSlug: 'pesquisa' }],
        fingerprint: 'antigo',
        profileExpression: 'pesquisa',
      })
      .mockResolvedValue({
        grants: [],
        fingerprint: 'novo',
        profileExpression: 'programacao',
      });
    saveJob.mockResolvedValue({
      jobId: 'resumo-diario',
      authorizationRequired: true,
      requestedEnabled: true,
      targetProfileSlug: 'programacao',
      dynamicProfile: false,
    });
    authorizeProfile.mockResolvedValue(false);
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={vi.fn()} />);
    expect(await screen.findByRole('button', {
      name: 'jobs.builder.revokeProfileAriaLabel:Pesquisa',
    })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'edit-profile' }));
    await user.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(screen.queryByRole('button', {
      name: 'jobs.builder.revokeProfileAriaLabel:Pesquisa',
    })).not.toBeInTheDocument());
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
    await user.click(screen.getByRole('button', { name: 'jobs.builder.revokeProfileAriaLabel:Pesquisa' }));
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

  it('localiza falhas de grant sem expor detalhes internos', async () => {
    const user = userEvent.setup();
    authorizeProfile.mockRejectedValue(new Error('authorization_not_granted: store de grants indisponível'));
    render(<JobBuilder editJob={subagentJob('pesquisa')} onClose={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: 'jobs.builder.authorizeProfile' }));
    expect(await screen.findByText('jobs.builder.profileGrantNotAuthorizedError')).toBeInTheDocument();
    expect(screen.queryByText(/store de grants indisponível/)).not.toBeInTheDocument();
  });

  it('distingue cada ação de revogação pelo nome do profile', async () => {
    getGrantState.mockResolvedValue({
      grants: [{ targetProfileSlug: 'pesquisa' }, { targetProfileSlug: 'programacao' }],
      fingerprint: 'fp',
      profileExpression: '{{ .event.profile }}',
    });
    render(<JobBuilder editJob={subagentJob('{{ .event.profile }}')} onClose={vi.fn()} />);
    expect(await screen.findByRole('button', {
      name: 'jobs.builder.revokeProfileAriaLabel:Pesquisa',
    })).toBeInTheDocument();
    expect(screen.getByRole('button', {
      name: 'jobs.builder.revokeProfileAriaLabel:Programação',
    })).toBeInTheDocument();
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
    expect(screen.getByRole('button', { name: 'jobs.builder.revokeProfileAriaLabel:Pesquisa' })).toBeDisabled();
    expect(screen.getByText('jobs.builder.saveProfileConfigurationFirst')).toBeInTheDocument();
    expect(getProfiles).toHaveBeenCalledTimes(callsBeforeEdit);
  });
});

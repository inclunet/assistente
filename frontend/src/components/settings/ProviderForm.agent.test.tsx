import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { ProviderForm } from './ProviderForm';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());
const createMock = vi.hoisted(() => vi.fn(() => Promise.resolve({ id: 'cursor-1' })));
const updateMock = vi.hoisted(() => vi.fn(() => Promise.resolve({})));
const listModelsMock = vi.hoisted(() => vi.fn(() => Promise.resolve(['gpt-4o'])));

function resolveLocaleString(key: string, vars?: Record<string, unknown>): string | undefined {
  const root = (ptBR as { translation: Record<string, unknown> }).translation;
  const value = key.split('.').reduce<unknown>((acc, part) => {
    if (!acc || typeof acc !== 'object') return undefined;
    return (acc as Record<string, unknown>)[part];
  }, root);

  if (typeof value !== 'string') return undefined;
  if (!vars) return value;
  return value.replace(/\{\{\s*(\w+)\s*\}\}/g, (_match, varName: string) => {
    const v = vars[varName];
    return v == null ? '' : String(v);
  });
}

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, options?: string | Record<string, unknown>) => {
        const vars = options && typeof options === 'object' ? (options as Record<string, unknown>) : undefined;
        return resolveLocaleString(key, vars) ?? key;
      },
    }),
  };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  DetectACPAgent: detectMock,
  CreateLLMProvider: createMock,
  UpdateLLMProvider: updateMock,
  ListModelsRaw: listModelsMock,
}));

const detected = {
  found: true,
  command: '/usr/local/bin/cursor-agent',
  args: ['acp'],
  source: '/usr/local/bin/cursor-agent',
  searched: [],
  work_dir: '/home/ana/projetos/assistente',
};

const missing = {
  found: false,
  command: '',
  args: [],
  searched: ['/home/ana/.local/bin/cursor-agent'],
  work_dir: '/home/ana/projetos/assistente',
};

/** Leva o formulário de criação até o tipo de agente, já detectado. */
async function abrirFormularioDeAgente(user: ReturnType<typeof userEvent.setup>, nome = 'Cursor local') {
  await user.type(screen.getByLabelText(/^nome/i), nome);
  await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'cursor');
  await waitFor(() => expect(detectMock).toHaveBeenCalledWith('cursor'));
}

afterEach(() => {
  vi.clearAllMocks();
});

describe('ProviderForm — provedor de agente de código', () => {
  it('troca URL, chave e modelos pelos campos do agente', async () => {
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    expect(screen.getByLabelText(/base url/i)).toBeInTheDocument();

    await abrirFormularioDeAgente(user);

    expect(screen.queryByLabelText(/base url/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/api key/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /carregar modelos/i })).not.toBeInTheDocument();
    expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/diretório de trabalho/i)).toHaveValue(detected.work_dir);
    expect(listModelsMock).not.toHaveBeenCalled();
  });

  it('salva o agente com formato acp, sem URL e sem credencial', async () => {
    detectMock.mockResolvedValue(detected);
    const onSave = vi.fn();
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={onSave} />);
    await abrirFormularioDeAgente(user);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(detected.command);
    });

    await user.click(screen.getByRole('button', { name: /criar/i }));

    await waitFor(() => expect(createMock).toHaveBeenCalled());
    const payload = createMock.mock.calls[0][0] as Record<string, unknown>;
    expect(payload).toMatchObject({
      type: 'cursor',
      api_format: 'acp',
      base_url: '',
      acp_command: detected.command,
      acp_args: detected.args,
    });
    expect(payload.api_key).toBeUndefined();
    expect(payload.default_model).toBeUndefined();
    expect(onSave).toHaveBeenCalled();
  });

  it('deixa salvar sem testar conexão, porque agente não tem endpoint para testar', async () => {
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    await abrirFormularioDeAgente(user);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /criar/i })).toBeEnabled();
    });
  });

  it('recusa salvar sem comando e explica o que falta', async () => {
    detectMock.mockResolvedValue(missing);
    const onSave = vi.fn();
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={onSave} />);
    await abrirFormularioDeAgente(user);
    await screen.findByText(/agente não encontrado nesta máquina/i);

    await user.click(screen.getByRole('button', { name: /criar/i }));

    expect(await screen.findByText(/comando do agente é obrigatório/i)).toBeInTheDocument();
    expect(createMock).not.toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('edição preserva o comando salvo e atualiza pelo mesmo contrato', async () => {
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'cursor-1',
          name: 'Cursor local',
          type: 'cursor',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/opt/cursor/agente',
          acp_args: ['acp'],
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    await waitFor(() => expect(detectMock).toHaveBeenCalled());
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('/opt/cursor/agente');
    expect(listModelsMock).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    expect(updateMock.mock.calls[0][0]).toBe('cursor-1');
    expect(updateMock.mock.calls[0][1]).toMatchObject({
      api_format: 'acp',
      acp_command: '/opt/cursor/agente',
      acp_args: ['acp'],
    });
  });
});

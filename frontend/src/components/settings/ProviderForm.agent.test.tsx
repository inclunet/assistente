import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { ProviderForm } from './ProviderForm';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());
// Assinatura declarada nos mocks para os testes poderem inspecionar o payload:
// sem os parâmetros, `mock.calls[0]` é uma tupla vazia para o TypeScript.
const createMock = vi.hoisted(() =>
  vi.fn((_payload: Record<string, unknown>) => Promise.resolve({ id: 'cursor-1' })),
);
const updateMock = vi.hoisted(() =>
  vi.fn((_id: string, _payload: Record<string, unknown>) => Promise.resolve({})),
);
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

  it('trocar o tipo de um agente salvo devolve o formulário à forma HTTP', async () => {
    // O tipo é editável na edição, e o formato é quem decide a forma do
    // formulário e o caminho de gravação: se ele não acompanhasse a troca, a
    // pessoa continuaria vendo campos de agente e gravaria um provedor HTTP
    // pelo pipeline do agente.
    detectMock.mockResolvedValue(detected);
    listModelsMock.mockResolvedValue(['llama3']);
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
    await screen.findByLabelText(/comando do agente/i);

    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'ollama');

    expect(screen.queryByLabelText(/comando do agente/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/base url/i)).toHaveValue('http://localhost:11434');
    // Agente não guardou credencial, então o campo aparece para preencher em vez
    // do botão que diria que já existe uma chave configurada.
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /carregar modelos/i }));
    await waitFor(() => expect(screen.getByRole('button', { name: /atualizar/i })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    const payload = updateMock.mock.calls[0][1] as Record<string, unknown>;
    expect(payload).toMatchObject({ type: 'ollama', base_url: 'http://localhost:11434' });
    expect(payload.api_format).not.toBe('acp');
    expect(payload.acp_command).toBeUndefined();
  });

  it('trocar um provedor HTTP salvo para agente troca a forma e o caminho de gravação', async () => {
    detectMock.mockResolvedValue(detected);
    listModelsMock.mockResolvedValue(['gpt-4o']);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'openai-1',
          name: 'OpenAI',
          type: 'openai',
          base_url: 'https://api.openai.com/v1',
          api_key: '',
          api_format: 'openai_responses',
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );
    await screen.findByLabelText(/base url/i);

    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'cursor');

    // O tipo trocado à mão não tem comando salvo, então a detecção preenche.
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(detected.command));
    expect(screen.queryByLabelText(/base url/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/api key/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    const payload = updateMock.mock.calls[0][1] as Record<string, unknown>;
    expect(payload).toMatchObject({
      type: 'cursor',
      api_format: 'acp',
      acp_command: detected.command,
      acp_args: detected.args,
    });
    expect(payload.base_url).toBeUndefined();
    expect(payload.api_key).toBeUndefined();
  });

  it('voltar ao tipo salvo devolve o comando e os argumentos que estavam salvos', async () => {
    // Trocar o tipo e desistir não pode custar a configuração: o preset do
    // agente não sabe qual comando está no banco, e a detecção acha outro.
    detectMock.mockResolvedValue(detected);
    listModelsMock.mockResolvedValue(['llama3']);
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
          acp_args: ['acp', '--forcar'],
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );
    await screen.findByLabelText(/comando do agente/i);

    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'ollama');
    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'cursor');

    // Sem clicar em detectar: o que reaparece é o que está salvo, e não o que a
    // detecção encontrou nesta máquina.
    expect(await screen.findByLabelText(/comando do agente/i)).toHaveValue('/opt/cursor/agente');
    expect(screen.getByLabelText(/argumentos/i)).toHaveValue('acp\n--forcar');

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    expect(updateMock.mock.calls[0][1]).toMatchObject({
      type: 'cursor',
      api_format: 'acp',
      acp_command: '/opt/cursor/agente',
      acp_args: ['acp', '--forcar'],
    });
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

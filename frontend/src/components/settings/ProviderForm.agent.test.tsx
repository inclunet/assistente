import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { ProviderForm } from './ProviderForm';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());
// Assinatura declarada nos mocks para os testes poderem inspecionar o payload:
// sem os parâmetros, `mock.calls[0]` é uma tupla vazia para o TypeScript.
const createMock = vi.hoisted(() =>
  vi.fn((_req: Record<string, unknown>) => Promise.resolve({ id: 'cursor-1' })),
);
const updateMock = vi.hoisted(() =>
  vi.fn((_id: string, _req: Record<string, unknown>) => Promise.resolve({})),
);
const listModelsMock = vi.hoisted(() => vi.fn((_req: Record<string, unknown>) => Promise.resolve(['gpt-4o'])));
const listCredentialsMock = vi.hoisted(() => vi.fn());

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
      i18n: { language: 'pt-BR' },
    }),
  };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock, announceRequest: vi.fn() }),
}));

const catalogMock = vi.hoisted(() => vi.fn());

vi.mock('@wailsjs/go/app/App', () => ({
  CreateLLMProvider: createMock,
  UpdateLLMProvider: updateMock,
  ListModelsRaw: listModelsMock,
  GetACPCatalog: catalogMock,
  RefreshACPCatalog: catalogMock,
  // A instalação pelo catálogo tem teste próprio; aqui basta ela não aparecer.
  ACPAgentInstallPlan: vi.fn().mockResolvedValue({}),
  InstallACPAgent: vi.fn(),
  CancelACPAgentInstall: vi.fn(),
  RemoveACPAgent: vi.fn(),
  UpdateACPAgent: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPProviders', () => ({
  DetectACPAgent: detectMock,
}));

vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
  ListCredentials: listCredentialsMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
}));

/**
 * O catálogo que o seletor de agente abre. Os dois agentes bastam: um que o app
 * sabe procurar no disco e outro que não, que é a única diferença que existe
 * entre os 38 (AEP-0086 D11).
 */
const catalogo = {
  agents: [
    {
      id: 'cursor',
      name: 'Cursor',
      distributions: ['binary'],
      runtime_found: true,
      integrity: 'digest',
      state: 'installed',
    },
    {
      id: 'claude-acp',
      name: 'Claude Code',
      distributions: ['npm'],
      runtime: 'node',
      runtime_found: true,
      integrity: 'none',
      state: 'installed',
    },
    {
      id: 'gemini-cli',
      name: 'Gemini CLI',
      distributions: ['npm'],
      runtime: 'node',
      runtime_found: true,
      integrity: 'none',
      state: 'no_detection',
    },
  ],
  age_seconds: 0,
  from_cache: false,
  stale: false,
};

const detected = {
  detectable: true,
  found: true,
  command: '/usr/local/bin/cursor-agent',
  args: ['acp'],
  source: '/usr/local/bin/cursor-agent',
  searched: [],
  work_dir: '/home/ana/projetos/assistente',
};

const missing = {
  detectable: true,
  found: false,
  command: '',
  args: [],
  searched: ['/home/ana/.local/bin/cursor-agent'],
  work_dir: '/home/ana/projetos/assistente',
};

/** Escolhe um agente do catálogo, que é como o formulário sabe qual ele é. */
async function escolherAgente(user: ReturnType<typeof userEvent.setup>, nomeDoAgente: string, agentId: string) {
  await user.click(screen.getByRole('button', { name: /agente acp/i }));
  await user.click(await screen.findByRole('option', { name: new RegExp(nomeDoAgente, 'i') }));
  await waitFor(() => expect(detectMock).toHaveBeenCalledWith(agentId));
}

/** Leva o formulário de criação até um agente escolhido e já detectado. */
async function abrirFormularioDeAgente(
  user: ReturnType<typeof userEvent.setup>,
  nome = 'Cursor local',
  nomeDoAgente = 'Cursor',
  agentId = 'cursor',
) {
  await user.type(screen.getByLabelText(/^nome/i), nome);
  await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'acp');
  await escolherAgente(user, nomeDoAgente, agentId);
}

afterEach(() => {
  vi.clearAllMocks();
});

beforeEach(() => {
  catalogMock.mockResolvedValue(catalogo);
  listCredentialsMock.mockResolvedValue([
    { pattern: 'api.openai.com', type: 'bearer', masked: 'sk-...4f2a', managed: false },
  ]);
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
    // O diretório aparece como informação lida, ligada ao seu rótulo, e não como
    // campo: não se escolhe o diretório aqui (AEP-0084 D5).
    expect(screen.getByRole('term')).toHaveTextContent(/diretório de trabalho/i);
    expect(screen.getByRole('definition')).toHaveTextContent(detected.work_dir);
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
    const payload = createMock.mock.calls[0][0];
    expect(payload).toMatchObject({
      type: 'acp',
      acp_agent_id: 'cursor',
      api_format: 'acp',
      base_url: '',
      acp_command: detected.command,
      acp_args: detected.args,
    });
    // ACPEnv não atravessa Create: token costuma parar aí. O env{} do binário
    // o backend aplica a partir do installed.json (AEP-0086).
    expect(payload).not.toHaveProperty('acp_env');
    expect(payload.api_key).toBeUndefined();
    expect(payload.default_model).toBeUndefined();
    expect(onSave).toHaveBeenCalled();
  });

  it('trata o Claude Code como agente, com o comando do adaptador e sem subcomando', async () => {
    // O segundo agente entra pelo mesmo formulário do primeiro: é isso que prova
    // que o contrato do app é com o protocolo, e não com o Cursor (AEP-0084
    // Fase 7). O comando aqui é o par node + adaptador npm, sem `acp`.
    const claudeCode = {
      detectable: true,
      found: true,
      command: '/usr/bin/node',
      args: ['/usr/lib/node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js'],
      version: '0.65.0',
      source: '/usr/lib/node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js',
      searched: [],
      work_dir: '/home/ana/projetos/assistente',
      login_command: 'claude',
    };
    detectMock.mockResolvedValue(claudeCode);
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    await abrirFormularioDeAgente(user, 'Claude Code local', 'Claude Code', 'claude-acp');
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(claudeCode.command);
    });

    expect(screen.queryByLabelText(/base url/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/api key/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /criar/i }));

    await waitFor(() => expect(createMock).toHaveBeenCalled());
    expect(createMock.mock.calls[0][0]).toMatchObject({
      type: 'acp',
      acp_agent_id: 'claude-acp',
      api_format: 'acp',
      base_url: '',
      acp_command: claudeCode.command,
      acp_args: claudeCode.args,
    });
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
          type: 'acp',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/opt/cursor/agente',
          acp_args: ['acp'],
          acp_agent_id: 'cursor',
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

    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'acp');
    await escolherAgente(user, 'Cursor', 'cursor');

    // O agente escolhido à mão não tem comando salvo, então a detecção preenche.
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(detected.command));
    expect(screen.queryByLabelText(/base url/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/api key/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    const payload = updateMock.mock.calls[0][1] as Record<string, unknown>;
    expect(payload).toMatchObject({
      type: 'acp',
      acp_agent_id: 'cursor',
      api_format: 'acp',
      acp_command: detected.command,
      acp_args: detected.args,
    });
    expect(payload.base_url).toBeUndefined();
    expect(payload.api_key).toBeUndefined();
  });

  it('voltar ao tipo salvo devolve o agente, o comando e os argumentos que estavam salvos', async () => {
    // Trocar o tipo e desistir não pode custar a configuração: nada além do
    // banco sabe qual comando está gravado, e a detecção acha outro.
    detectMock.mockResolvedValue(detected);
    listModelsMock.mockResolvedValue(['llama3']);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'cursor-1',
          name: 'Cursor local',
          type: 'acp',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/opt/cursor/agente',
          acp_args: ['acp', '--forcar'],
          acp_agent_id: 'cursor',
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );
    await screen.findByLabelText(/comando do agente/i);

    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'ollama');
    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'acp');

    // Sem escolher agente nem clicar em detectar: o que reaparece é o que está
    // salvo, e não o que a detecção encontrou nesta máquina.
    expect(await screen.findByLabelText(/comando do agente/i)).toHaveValue('/opt/cursor/agente');
    expect(screen.getByLabelText(/argumentos/i)).toHaveValue('acp\n--forcar');

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    expect(updateMock.mock.calls[0][1]).toMatchObject({
      type: 'acp',
      acp_agent_id: 'cursor',
      api_format: 'acp',
      acp_command: '/opt/cursor/agente',
      acp_args: ['acp', '--forcar'],
    });
    expect(updateMock.mock.calls[0][1]).not.toHaveProperty('acp_env');
  });

  it('trocar de agente limpa o comando do anterior e deixa a detecção do novo preencher', async () => {
    // Manter o comando faria o provedor dizer que é um agente enquanto executa
    // outro — e é o executável, não o rótulo, que sobe o processo.
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'claude-1',
          name: 'Claude local',
          type: 'acp',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/usr/bin/node',
          acp_args: ['/opt/claude-agent-acp/dist/index.js'],
          acp_agent_id: 'claude-acp',
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('/usr/bin/node'));

    await escolherAgente(user, 'Cursor', 'cursor');

    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(detected.command));

    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    expect(updateMock.mock.calls[0][1]).toMatchObject({
      type: 'acp',
      acp_agent_id: 'cursor',
      acp_command: detected.command,
      acp_args: detected.args,
    });
  });

  it('a credencial do cofre ligada na tela vai no que é salvo, como referência', async () => {
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    await abrirFormularioDeAgente(user);

    await user.type(await screen.findByLabelText(/variável de ambiente/i), 'OPENAI_API_KEY');
    await user.selectOptions(screen.getByLabelText(/entrada do cofre/i), 'api.openai.com');
    await user.click(screen.getByRole('button', { name: /ligar a credencial/i }));
    await user.click(screen.getByRole('button', { name: /^criar$/i }));

    await waitFor(() => expect(createMock).toHaveBeenCalled());
    expect(createMock.mock.calls[0][0]).toMatchObject({
      acp_credential_env: { OPENAI_API_KEY: 'api.openai.com' },
    });
  });

  it('trocar de agente desliga a credencial que era do anterior', async () => {
    // A variável que o Cursor lê não é a que o Claude Code lê, e a chave é de
    // quem se escolheu para recebê-la — manter o par entregaria o segredo a um
    // programa que ninguém apontou.
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'claude-1',
          name: 'Claude local',
          type: 'acp',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/usr/bin/node',
          acp_args: ['/opt/claude-agent-acp/dist/index.js'],
          acp_agent_id: 'claude-acp',
          acp_credential_env: { ANTHROPIC_API_KEY: 'api.anthropic.com' },
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );
    expect(
      await screen.findByText(/ANTHROPIC_API_KEY recebe a credencial de api\.anthropic\.com/),
    ).toBeInTheDocument();

    await escolherAgente(user, 'Cursor', 'cursor');
    await user.click(screen.getByRole('button', { name: /atualizar/i }));

    await waitFor(() => expect(updateMock).toHaveBeenCalled());
    // Mapa vazio, e não campo ausente: é o vazio que desliga a passagem no
    // backend, e omiti-lo pediria para não mexer.
    expect(updateMock.mock.calls[0][1]).toMatchObject({ acp_credential_env: {} });
  });

  it('o agente escolhido aparece na tela pelo nome, e não só pelo identificador', async () => {
    // Quem escolhe "Gemini CLI" numa lista precisa reencontrá-lo escrito assim:
    // o `id` do registro é o que o app grava, e não o que ele diz.
    detectMock.mockResolvedValue({ detectable: false, found: false, searched: [] });
    const user = userEvent.setup();

    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    await user.selectOptions(screen.getByLabelText(/tipo de provedor/i), 'acp');
    expect(screen.getByRole('button', { name: /agente acp/i })).toBeInTheDocument();

    await escolherAgente(user, 'Gemini CLI', 'gemini-cli');

    expect(await screen.findByText(/gemini cli/i)).toBeInTheDocument();
    // Agente que o app não sabe procurar não ganha botão de procurar.
    expect(screen.queryByRole('button', { name: /detectar e preencher comando/i })).not.toBeInTheDocument();
  });

  it('edição preserva o comando salvo e atualiza pelo mesmo contrato', async () => {
    detectMock.mockResolvedValue(detected);
    const user = userEvent.setup();

    render(
      <ProviderForm
        provider={{
          id: 'cursor-1',
          name: 'Cursor local',
          type: 'acp',
          base_url: '',
          api_key: '',
          api_format: 'acp',
          acp_command: '/opt/cursor/agente',
          acp_args: ['acp'],
          acp_agent_id: 'cursor',
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());
    expect(detectMock).not.toHaveBeenCalled();
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
